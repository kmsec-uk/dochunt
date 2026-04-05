package main

import (
	"embed"
	"html/template"
)

//go:embed static
//go:embed tmpl
var content embed.FS

var (
	layout = template.Must(template.ParseFS(content, "tmpl/layout.html"))
	index  = template.Must(template.Must(layout.Clone()).ParseFS(content, "tmpl/index.html"))
)
