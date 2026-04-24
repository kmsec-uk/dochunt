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

// PageData holds the data for the base HTML template
type PageData struct {
	Title            string `json:"-"`
	SizeOnDisk       string `json:"-"`
	NumProcessedDocs uint32 `json:"-"`
}

type FAQ struct {
	Heading template.HTML
	Content template.HTML
}

type AboutPage struct {
	PageData
	FAQs []FAQ
}

var AboutFAQs = []FAQ{
	{
		Heading: "What is this website?",
		Content: `
		<p>This is a public dataset of Google Docs that was used to test my Google Docs HTML parser.
		This was a stab at extracting metadata and content from a snapshot of a document's HTML rather than 
		visiting the document in a browser. As Google Docs is an interactive, multiplayer application, a document's properties
		are held within minified script tags.<p>
		<p>It turns out, you can extract a lot of interesting information like:</p>
		<ul>
			<li>Title</li>
			<li>Creation date</li>
			<li>Number of revisions</li>
			<li>Snippets of the documents content</li>
			<li>Links that are embedded in the document</li>
			<li>Image urls that are embedded in the document (although these are always hosted on Google's servers and unique for <i>every</i> image.)</li>
		</ul>
		<p>You can <a target="_blank" href="https://kmsec.uk/blog/parsing-google-docs/">read my blog post</a> on this effort.</p>`,
	},
	{
		Heading: "How do you collect this data?",
		Content: `
		<p>This dataset leverages <a target="_blank" href="https://commoncrawl.org/" target="_blank">commoncrawl</a>
		and <a target="_blank" href="https://urlscan.io/">urlscan</a> as the primary sources of public Google Docs.</p>
		<p>Both commoncrawl and urlscan archive a page's HTML on their servers.
		HTML is parsed from these third parties to extract a document's metadata and content.</p>
		<p><b>No data is scraped directly from Google servers, and all data held here is already publicly 
		available on the internet in an unparsed format.</b></p>`,
	},
	{
		Heading: "How does this work under the hood?",
		Content: `<p>Raw Google Docs pages' HTML are retrieved from commoncrawl and urlscan, parsed, and then ingested into a SQLITE database.
		It's all written in Go. Check out the <a target="_blank" href="https://github.com/kmsec-uk/dochunt">source code</a>.</p>
		<p>At the moment, all collection requires a manual trigger.</p>
		<p>The web server and database is hosted on a computer in my home in the UK, so availability and performance is not guaranteed.</p>`,
	},
	{
		Heading: "Do you have an API?",
		Content: `
		<p>A basic one yes. Both the query page and document view can return JSON.</p>
		<p>The query page is newline-delimited JSON (NDJSON).</p>
		<p>Simply add a <code>format=json</code> parameter to your requests. For example 
		<br>
		<a href="/?q=%E7%A2%BA%E8%AA%8D%E3%81%99%E3%82%8B&format=json"><code>https://dochunt.kmsec.uk/?q=%E7%A2%BA%E8%AA%8D%E3%81%99%E3%82%8B&format=json</code></a></p>
		The query page is newline-delimited JSON (NDJSON).</p>
		<p>Adding <code>?format=json</code> to a document endpoint returns a 
		single JSON object that discloses all instances of this document in the 
		corpus. For example:
		<br>
		<a href="/d/1_G0BCG2pd-6JvmGVeflBM5xVovAPzHniwKygSAttG48?format=json"><code>https://dochunt.kmsec.uk/d/1_G0BCG2pd-6JvmGVeflBM5xVovAPzHniwKygSAttG48?format=json</code></a></p>`,
	},
	{
		Heading: "Is the data accurate?",
		Content: `<p>Part of my motivation here was to test whether the parser works against a large corpus of raw HTML documents. It works 98% of the time (that number is based on vibes. Completely made up).</p>
		<p>However, when parsing document content, there is no 
		guarantee that it is complete or in the correct order.</p>
		<p>This is due to my preference for "good enough" rather than perfect parsing (see also: <i>"skill issue"</i>).</p>
		`,
	},
	{
		Heading: "What is a <i>revision</i>?",
		Content: `<p>On a document's page, you may see several revision numbers.
		Throughout the lifetime of a document, it may undergo several revisions by editors.
		A larger revision number means more edits to a document.</p>
        <p>Observers (commoncrawl, urlscan, your eyes) may see different revisions of the same document when visiting it over
        time, so we can use the revision as a chronological marker, which helps inform us of the document's lifecycle.</p>`,
	},
	{
		Heading: "Why did you do this?",
		Content: `<p>I'm not a developer, but as a threat intelligence analyst, I love turning raw data into actionable content.</p>
		<p>On parental leave I had the unique combination of limited, sporadic focus time, extra mental capacity, and a mind free to pursue something completely
		meritless. This was something I could pick up and work on when I needed a bit of cognitive stimulation.</p>
		<p>I had also only recently discovered commoncrawl (which truly is a modern treasure trove), and I wanted to get familiar with it.</p>`,
	},
	{
		Heading: "Is this vibe coded?",
		Content: `95% of the code is written by me, and any professional Go developer would see that from examining (aka roasting) my code.
		I use AI to get over implementation humps or to scratchpad, but this was a hobby project and I truly enjoy the struggle of creation.`,
	},
	{
		Heading: "Do you accept takedowns?",
		Content: `<p>Yes. This is a hobby project of mine, however I am able to conduct manual takedowns in as timely manner as I can.</p>
		<p>Be mindful that <b>this data is merely a refinement of what can be found on third party websites</b> (namely commoncrawl, urlscan, or Google Docs if the document is still publicly accessible).</p>
		<p>Check out the contact details on the footer of <a target="_blank" href="https://kmsec.uk">my website</a> or <a target="_blank" href="https://github.com/kmsec-uk/dochunt/issues/new?title=Takedown%20request&body=Please%20remove%20document%20[document%20ID%20here]%20as%20it%20contains%20[xyz]">create an issue on GitHub</a>.</p>
		<p>Please provide the document ID and come prepared with the justification for takedown -- for example the presence of sensitive personally identifiable information (PII), illegal, offensive, or harmful content.</p>`,
	},
}
