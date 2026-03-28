package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/charmbracelet/fang"
	"github.com/flowchartsman/handlebars/v3"
	"github.com/radovskyb/watcher"
	"github.com/spf13/cobra"
)

type Config struct {
	OutputPath string `toml:"output_path"`
	StaticPath string `toml:"static_path"`

	LayoutsPath   string `toml:"layouts_path"`
	TemplatesPath string `toml:"templates_path"`
	PagesPath     string `toml:"pages_path"`

	LayoutEmbedValue string `toml:"layout_embed_value"`

	PathLayoutMap map[string]string `toml:"path_layout_map"`
}

func parseTemplateDir(dir_path string, templates map[string]*handlebars.Template, prefix string) {
	files, err := os.ReadDir(dir_path)
	if err != nil {
		log.Fatalln(err)
	}

	for _, file := range files {
		if file.IsDir() {
			parseTemplateDir(path.Join(dir_path, file.Name()), templates, path.Join(prefix, file.Name()))
			continue
		}
		tmpl, err := handlebars.ParseFile(path.Join(dir_path, file.Name()))
		if err != nil {
			log.Println(err)
			continue
		}

		fileBaseName := strings.TrimSuffix(file.Name(), path.Ext(file.Name()))

		templates[path.Join(prefix, fileBaseName)] = tmpl
	}
}

func compile(conf Config) {
	layouts := make(map[string]*handlebars.Template)
	parseTemplateDir(conf.LayoutsPath, layouts, "")

	templates := make(map[string]*handlebars.Template)
	parseTemplateDir(conf.TemplatesPath, templates, "")

	pages := make(map[string]*handlebars.Template)
	parseTemplateDir(conf.PagesPath, pages, "")

	handlebars.RemoveAllPartials()
	for name, tmpl := range templates {
		handlebars.RegisterPartialTemplate(name, tmpl)
	}

	files, err := os.ReadDir(conf.OutputPath)
	if err != nil {
		log.Fatalln(err)
	}

	for _, file := range files {
		err := os.RemoveAll(path.Join(conf.OutputPath, file.Name()))
		if err != nil {
			log.Fatalln(err)
		}
	}

	for name, tmpl := range pages {
		ctx := map[string]string{
			"path": name,
		}

		var layout string
		for lp, layout_filename := range conf.PathLayoutMap {
			if strings.HasPrefix(path.Dir(name), lp) {
				layout = layout_filename
			}
		}

		if layout != "" {
			l_tmpl := layouts[layout].Clone()
			l_tmpl.RegisterPartialTemplate("embed", tmpl)

			tmpl = l_tmpl
		}

		result, err := tmpl.Exec(ctx)
		if err != nil {
			log.Fatalln(err)
		}

		outPath := path.Join(conf.OutputPath, name+".html")

		err = os.WriteFile(outPath, []byte(result), 0644)
		if err != nil {
			log.Fatalln(err)
		}
	}

	if conf.StaticPath != "" {
		fsys := os.DirFS(conf.StaticPath)
		err := os.CopyFS(conf.OutputPath, fsys)
		if err != nil {
			log.Fatalln(err)
		}
	}

	log.Println("Finished writing to output directory.")

}

func setupConfig() Config {
	var conf Config
	_, err := toml.DecodeFile(".gopher-ssg.toml", &conf)

	if err == nil && conf.PagesPath == "" {
		conf.PagesPath = "pages"

		log.Println("no pages_path provided")

		b, err := toml.Marshal(conf)
		if err != nil {
			log.Fatalln(err)
		}
		err = os.WriteFile(".gopher-ssg.toml", b, 0644)
		if err != nil {
			log.Fatalln(err)
		}
	}

	if err == nil && conf.OutputPath == "" {
		conf.OutputPath = "dist"

		log.Println("no output_path provided")

		b, err := toml.Marshal(conf)
		if err != nil {
			log.Fatalln(err)
		}
		err = os.WriteFile(".gopher-ssg.toml", b, 0644)
		if err != nil {
			log.Fatalln(err)
		}
	}

	if err != nil {
		conf = Config{
			OutputPath:       "dist",
			PagesPath:        "pages",
			TemplatesPath:    "templates",
			LayoutsPath:      "layouts",
			LayoutEmbedValue: "embed",
			PathLayoutMap: map[string]string{
				".": "default",
			},
		}

		b, err := toml.Marshal(conf)
		if err != nil {
			log.Fatalln(err)
		}
		err = os.WriteFile(".gopher-ssg.toml", b, 0644)
		if err != nil {
			log.Fatalln(err)
		}
	}

	if conf.LayoutsPath != "" {
		_ = os.Mkdir(conf.LayoutsPath, 0755)
	}
	if conf.TemplatesPath != "" {
		_ = os.Mkdir(conf.TemplatesPath, 0755)
	}
	if conf.PagesPath != "" {
		_ = os.Mkdir(conf.PagesPath, 0755)
	}
	if conf.OutputPath != "" {
		_ = os.Mkdir(conf.OutputPath, 0755)
	}
	if conf.StaticPath != "" {
		_ = os.Mkdir(conf.StaticPath, 0755)
	}

	return conf
}

func watchAndServe(conf Config) {
	compile(conf)

	w := watcher.New()

	// SetMaxEvents to 1 to allow at most 1 event's to be received
	// on the Event channel per watching cycle.
	//
	// If SetMaxEvents is not set, the default is to send all events.
	w.SetMaxEvents(1)

	go func() {
		for {
			select {
			case event := <-w.Event:
				log.Println(event) // Print the event's info.
				compile(conf)
			case err := <-w.Error:
				log.Fatalln(err)
			case <-w.Closed:
				return
			}
		}
	}()

	if conf.PagesPath != "" {
		if err := w.Add(conf.PagesPath); err != nil {
			log.Fatalln(err)
		}
	}
	if conf.LayoutsPath != "" {
		if err := w.Add(conf.LayoutsPath); err != nil {
			log.Fatalln(err)
		}
	}
	if conf.TemplatesPath != "" {
		if err := w.Add(conf.TemplatesPath); err != nil {
			log.Fatalln(err)
		}
	}
	if conf.StaticPath != "" {
		if err := w.Add(conf.StaticPath); err != nil {
			log.Fatalln(err)
		}
	}

	fmt.Println("Watching folders...")

	// Trigger 2 events after watcher started.
	go func() {
		w.Wait()
	}()

	go func() {
		viewPath := conf.OutputPath

		http.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				r.URL.Path = "/index"
			}

			requestedPath := strings.TrimLeft(filepath.Clean(r.URL.Path), "/")
			filename := fmt.Sprintf("%s/%s", viewPath, requestedPath)

			_, err := os.Stat(filename)
			if os.IsExist(err) {
				http.ServeFile(w, r, filename)
			} else {
				http.ServeFile(w, r, filename+".html")
			}

		}))

		log.Printf("Listening on :%d for path %s/\n", 8000, viewPath)
		err := http.ListenAndServe(":8000", nil)
		if err != nil {
			log.Fatalln(err)
		}
	}()

	// Start the watching process - it'll check for changes every 100ms.
	if err := w.Start(time.Millisecond * 100); err != nil {
		log.Fatalln(err)
	}
}

func main() {
	cmd := &cobra.Command{
		Use:   "gopher-ssg",
		Short: "Create wonderfully simple static websites with nothing more than you need",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "build",
		Short: "Builds the static website once",
		Run: func(cmd *cobra.Command, args []string) {
			conf := setupConfig()
			compile(conf)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "serve",
		Short: "Watches for file changes auto-compiles and serves the output",
		Run: func(cmd *cobra.Command, args []string) {
			conf := setupConfig()
			watchAndServe(conf)
		},
	})

	if err := fang.Execute(context.Background(), cmd); err != nil {
		os.Exit(1)
	}

}
