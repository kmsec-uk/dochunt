package main

import (
	"embed"
	"html/template"
)

//go:embed static
//go:embed tmpl
var content embed.FS

var (
	layout   = template.Must(template.ParseFS(content, "tmpl/layout.html"))
	index    = template.Must(template.Must(layout.Clone()).ParseFS(content, "tmpl/index.html"))
	notfound = template.Must(template.Must(layout.Clone()).ParseFS(content, "tmpl/notfound.html"))
	doc      = template.Must(template.Must(layout.Clone()).ParseFS(content, "tmpl/doc.html"))
	about    = template.Must(template.Must(layout.Clone()).ParseFS(content, "tmpl/about.html"))
)

type FAQ struct {
	Heading string
	Content template.HTML
}

var AboutFAQs = []FAQ{{
	Heading: "How do you collect this data?",
	Content: `This dataset leverages commoncrawl and urlscan as the primary sources of public Google Docs. No data is scraped directly from Google servers.`,
},
	{
		Heading: "How does this work under the hood?",
		Content: `Check out the <a href="https://github.com/kmsec-uk/dochunt">source code</a>`,
	},
	{
		Heading: "Is the content accurate?",
		Content: `<p>When parsing document content, there is no 
		guarantee that it is complete or in the correct order.</p>
		<p>This is due to my preference for "good enough" rather than perfect (see also: <i>skill issue</i>).</p>
		`,
	},
}

type AboutPage struct {
	PageData
	FAQs []FAQ
}
