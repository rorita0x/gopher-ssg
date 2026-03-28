# Super Simple Static Site Generator

Currently this only supports handlebar, but I'm planning to integrate a few more.

![CLI Demo](/docs/demo.gif)

Sample Project Structure:
![Project Structure](/docs/sample-directory-layout.png)

Default Configuration would look like this:
```toml
output_path = "dist"
static_path = "static"
layouts_path = "layouts"
templates_path = "templates"
pages_path = "pages"
layout_embed_value = "embed"

[path_layout_map]
  "." = "default"
```