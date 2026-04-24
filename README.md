# Google Docs hunting dataset

This repository holds the code that supports [dochunt](https://dochunt.kmsec.uk).

Before reading the code, you may want to [read the blog post](https://kmsec.uk/blog/parsing-google-docs) that introduces the problem 
statement and my approach.

The star of the show is the code to parse Google Docs content from HTML. It is very simple and
is hosted in `/gdoc/`.

Everything else here supports the dataset viewable at [dochunt](https://dochunt.kmsec.uk).

* `/db/` - the database writer implementation that underpins the sqlite database
* `/commoncrawl/` - a basic search+retrieve commoncrawl client used in `/cmd/get/`
* `/warc/` - a very specific WARC file parser specifically for my use-case (specific!)

`/cmd/view` contains the web server, templates, etc. for [dochunt](https://dochunt.kmsec.uk)
and a distinct database reader implementation.

## Example program

If you want to parse Google Docs content from HTML, here's a minimal approach:

```go
func main() {
	// open a file for reading
	f, err := os.Open("example.html")
	if err != nil {
		panic(err)
	}
	// create a blank Doc - sometimes it's better to pass in a
	// known Google Docs ID in advance to the NewDoc parameter
	g := gdoc.NewDoc("")

	// the Google Docs struct has an optional Provenance field,
	// which is useful for documenting *where* you found a DOM.
	g = g.WithProvenance("example.html")

	// parse the html from the file stream
	if err := g.ParseHtml(f); err != nil {
		panic(err)
	}
	b, err := json.MarshalIndent(g, "", " ")
	if err != nil {
		panic(err)
	}
	fmt.Println(string(b))
	/*
		Outputs:
			{
			"id": "1J1YwL0a94brFB8v_-zh8WVGlNxtZmjmRiRP5Gbpv9YI",
			"revision": 751,
			"provenance": "example.html",
			"timestamp": "2026-02-04T23:19:13Z",
			"page_title": "Test Requirement - Google Docs",
			"og_title": "Test Requirement",
			"og_description": "※ You can find another test in the left tab of this document  Blockchain developer test  This is the test project:  https://bitbucket.org/workspace052/testing/src/dev/                          * You should perform tests on this project and send a PR to BitBucket after completing the tests.*     T...",
			"og_image": "https://lh7-us.googleusercontent.com/docs/AHkbwyJgNSeUYHVi1oly1xonvHrffHjtmDLl8boILmc_jDO3AnEa8IgLDBiFcSdnhxwMFt1nuz88ycGsuPW9dB9W2AV_0fRc53LvbvE4kAm0BchlfQ89qZVC=w1200-h630-p",
			"created": "2025-04-23T16:24:56.629+01:00",
			"content": "※ You can find another test....<snipped!>",
			"links": [
			"https://bitbucket.org/workspace052/testing/src/dev/"
			],
			"image_urls": [
			"https://lh7-rt.googleusercontent.com/docsz/AD_4nXcbQk6SOuUh3HgJihU_kuy03T9ff67viXO7_1VVmD4fk2_DqiXDsFXrUp2_moGkf0kmlc7clKsrk3PFrVWZVJPSXUopbe-zp8VQ89jQ5k3-GuYONkxZwum227-ip_GmmnuqKiAAQi9_N4BZZTFWHQ?key=MvlX0wMJ8MA3FUQmKoAwlita"
			]
			}
	*/
}
```
The example above is is shown in `/cmd/example/main.go`. 
FYI, the document in the example above has been taken down, which is why it's useful to be able to parse HTML from archived snapshots. Neat!