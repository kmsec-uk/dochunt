package gdoc

import (
	"fmt"
	"io"
	"log"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var (
	// https://docs.google.com/document/d/19gTpFORRvKgI0oIghM-GF-CFQS3R_ySp_ltzeCe6iHg/
	// /                               ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
	googleDocIdRegex = regexp.MustCompile(`\/d\/[\w_\-\d]+\b`)

	// creation time from the script tag
	// config['dct'] = 1.759713531746E12;
	// /               ^^^^^^^^^^^^^^^^^
	docCreationTime = regexp.MustCompile(`config\['dct'\]\s+\=\s+([\d\.E]+)`)
	imageBlobUrls   = regexp.MustCompile(`"s-blob-v1-IMAGE[\w\-]+":("https:\/\/.+?")`)
	embeddedLinks   = regexp.MustCompile(`{"lnk_type":0,"ulnk_url":("http.+?")}`)
	embeddedText    = regexp.MustCompile(`{"ty":"is","ibi":\d+,"s":(".+?")}`)
)

type Doc struct {
	Id            string    `json:"id"`
	Timestamp     time.Time `json:"timestamp"`
	PageTitle     string    `json:"page_title"`
	OgTitle       string    `json:"og_title"`
	OgDescription string    `json:"og_description"`
	OgImageUrl    string    `json:"og_image"`
	Created       time.Time `json:"created"`
	Content       string    `json:"content"`
	Links         []string  `json:"links"`
	ImageUrls     []string  `json:"image_urls"`
}

func (d *Doc) ParseHtml(reader io.Reader) error {

	doc, err := html.Parse(reader)
	if err != nil {
		return err
	}

	snippets := make([]string, 0)

	for n := range doc.Descendants() {
		// page.title attribute
		if n.Type == html.ElementNode && n.DataAtom == atom.Title {
			d.PageTitle = n.FirstChild.Data
		}
		// meta.og:title
		// meta.og:description
		// meta.og:image
		if n.Type == html.ElementNode && n.DataAtom == atom.Meta {
			var foundDescription bool
			var foundTitle bool
			var foundOgImage bool
			for _, a := range n.Attr {
				if a.Key == "property" {
					switch a.Val {
					case "og:title":
						foundTitle = true
					case "og:description":
						foundDescription = true
					case "og:image":
						foundOgImage = true
					}
				}
			}
			if foundDescription {
				for _, a := range n.Attr {
					if a.Key == "content" {
						d.OgDescription = a.Val
						break
					}
				}
			}
			if foundTitle {
				for _, a := range n.Attr {
					if a.Key == "content" {
						d.OgTitle = a.Val
						break
					}
				}
			}
			if foundOgImage {
				for _, a := range n.Attr {
					if a.Key == "content" {
						d.OgImageUrl = a.Val
						break
					}
				}
			}
		}
		// script content
		if n.Type == html.ElementNode && n.DataAtom == atom.Script {
			if n.FirstChild == nil || n.FirstChild.Type != html.TextNode {
				continue
			}
			if strings.HasPrefix(n.FirstChild.Data, "DOCS_timing['sac']") {
				// first get the doc creation time
				m := docCreationTime.FindStringSubmatch(n.FirstChild.Data)
				if len(m) < 2 {
					return fmt.Errorf("creation time (from `config['dct']`) not found in script")
				}
				t, err := stringToDate(m[1])
				if err != nil {
					return fmt.Errorf("parsing creation time from `%s`: %w", m[1], err)
				} else {
					// fmt.Println("creation time:", t.UTC())
					d.Created = t
				}
				// image blob urls
				imageBlobMatches := imageBlobUrls.FindAllStringSubmatch(n.FirstChild.Data, -1)
				if len(imageBlobMatches) == 0 {
					log.Printf("%s: no image blob urls found", d.Id)
					continue
				}
				for _, m := range imageBlobMatches {
					if len(m[1]) == 0 {
						continue
					}
					s, err := strconv.Unquote(m[1])
					if err != nil {
						log.Printf("%s: error strconv unquote `%s`: %v", d.Id, m[1], err)
					}
					d.ImageUrls = append(d.ImageUrls, s)
				}
			}
			if strings.HasPrefix(n.FirstChild.Data, "DOCS_modelChunk = {") {
				// text blobs
				embeddedTextMatches := embeddedText.FindAllStringSubmatch(n.FirstChild.Data, -1)
				for _, m := range embeddedTextMatches {
					t, err := strconv.Unquote(m[1])
					if err != nil {
						log.Printf("%s: error strconv unquote `%s...`: %v", d.Id, m[1][:30], err)

					}
					t = strings.TrimSpace(t)
					if len(t) == 0 {
						continue
					}
					snippets = append(snippets, t)
				}
				// links
				embeddedLinkMatches := embeddedLinks.FindAllStringSubmatch(n.FirstChild.Data, -1)
				for _, m := range embeddedLinkMatches {
					t, err := strconv.Unquote(m[1])
					if err != nil {
						log.Printf("%s: error strconv unquote `%s...`: %v", d.Id, m[1][:30], err)

					}
					t = strings.TrimSpace(t)
					if len(t) == 0 {
						continue
					}
					if slices.Contains(d.Links, t) {
						continue
					}
					d.Links = append(d.Links, t)
				}

			}

		}
	}
	d.Content = strings.Join(snippets, "\n")
	return nil
}

// stringToDate converts E notation
// string to time.Time
// e.g. "1.759713531746E12" -> time.Time
func stringToDate(s string) (time.Time, error) {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return time.Time{}, err
	}
	// convert from seconds to nanoseconds
	t := time.UnixMilli(int64(f))
	return t, nil
}
