package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/kmsec-uk/dochunt/gdoc"
)

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
	fmt.Println(
		"[!] Congratulations! You have just parsed a malicious Google Doc",
		"that supports the DPRK's Contagious Interview campaign!",
	)
}
