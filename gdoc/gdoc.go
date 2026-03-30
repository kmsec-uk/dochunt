package gdoc

import (
	"encoding/json"
	"errors"
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
	// The ID extracted from a URL
	// https://docs.google.com/document/d/19gTpFORRvKgI0oIghM-GF-CFQS3R_ySp_ltzeCe6iHg/
	// /                                 ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
	GoogleDocIdRegex = regexp.MustCompile(`docs\.google\.com\/document\/d\/(.+?)\/`)

	// creation time from the script tag
	// config['dct'] = 1.759713531746E12;
	// /               ^^^^^^^^^^^^^^^^^
	docCreationTime         = regexp.MustCompile(`config\['dct'\]\s+\=\s+([\d\.E]+)`)
	embeddedDocId           = regexp.MustCompile(`config\['bci'\]\s+=\s+'(.+?)'`)
	imageBlobUrls           = regexp.MustCompile(`"s-blob-v1-IMAGE[\w\-]+":("https:\/\/.+?")`)
	embeddedLinks           = regexp.MustCompile(`{"lnk_type":0,"ulnk_url":("http.+?")}`)
	embeddedText            = regexp.MustCompile(`{"ty":"is","ibi":\d+,"s":(".+?[^\\]")}`)
	embeddedRevision        = regexp.MustCompile(`"revision":(\d+)}`)
	embeddedServerTimestamp = regexp.MustCompile(`"server_time_ms":(\d+),"`)
)

var (
	ErrDocIdNotExtracted             = errors.New("document ID could not be extracted")
	ErrServerTimestampNotExtracted   = errors.New("server timestamp could not be extracted")
	ErrCreationTimestampNotExtracted = errors.New("creation timestamp could not be extracted")
)

type Doc struct {
	Id            string    `json:"id"`                     // the unique document ID obtained from the URL path
	Revision      uint32    `json:"revision"`               // the revision of the document obtained from script tags
	Provenance    string    `json:"provenance"`             // optional field for understanding how this document was observed
	Timestamp     string    `json:"timestamp"`              // the timestamp of when this document was observed
	PageTitle     string    `json:"page_title"`             // html.head.title attribute
	OgTitle       string    `json:"og_title"`               // meta.og_title attr
	OgDescription string    `json:"og_description"`         // ""
	OgImageUrl    string    `json:"og_image"`               // ""
	Created       time.Time `json:"created,format:RFC3339"` // created timestamp extracted from a script tag
	Content       string    `json:"content"`                // plaintext content extracted from script tags
	Links         []string  `json:"links"`                  // links that are embedded in the doc, similarly eztracted from script tags
	ImageUrls     []string  `json:"image_urls"`             // image assets that are used in the doc, e.g. banner pictures
}

// NewDoc returns a pointer to a blank Doc struct.
// If no document ID is known (e.g. you don't know the
// Google Doc ID in advance), you can parse an empty `id` "",
// and it will be populated by the parser.
func NewDoc(id string) *Doc {
	return &Doc{
		Id:        id,
		Links:     make([]string, 0),
		ImageUrls: make([]string, 0),
	}
}

// WithProvenance populates the optional
// Provenance field. This is useful for understanding
// where you found the document / html
func (d *Doc) WithProvenance(s string) *Doc {
	d.Provenance = s
	return d
}

// WithTimestamp populates the doc.Timestamp
// field string. This is useful if you want to use
// an alternative timestamp string, e.g. when the client
// observed the HTML, rather than what time the server
// reports.
// if omitted, we extract the server timestamp and it's populated
// with an RFC3339 UTC timestamp.
func (d *Doc) WithTimestamp(s string) *Doc {
	d.Timestamp = s
	return d
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
				if len(m) != 2 {
					return ErrCreationTimestampNotExtracted
				}
				t, err := stringToDate(m[1])
				if err != nil {
					return fmt.Errorf("parsing creation time from `%s`: %w", m[1], err)
				} else {
					d.Created = t
				}
				// embedded doc ID
				// only parse if initialised with empty Id
				if d.Id == "" {
					docIdMatches := embeddedDocId.FindStringSubmatch(n.FirstChild.Data)
					if len(docIdMatches) != 2 {
						return ErrDocIdNotExtracted
					}
					d.Id = m[1]
				}
				// image blob urls
				imageBlobMatches := imageBlobUrls.FindAllStringSubmatch(n.FirstChild.Data, -1)
				if len(imageBlobMatches) == 0 {
					continue
				}
				for _, m := range imageBlobMatches {
					if len(m[1]) == 0 {
						continue
					}
					var t string
					err := json.Unmarshal([]byte(m[1]), &t)
					if err != nil {
						log.Printf("%s: error: json unmarshall `%s...`: %v", d.Id, m[1][:30], err)
						continue
					}
					t = strings.TrimSpace(t)
					d.ImageUrls = append(d.ImageUrls, t)
				}

			}
			// timestamp only parsed if it's not populated
			if strings.HasPrefix(n.FirstChild.Data, "_docs_flag_initialData") && d.Timestamp == "" {
				m := embeddedServerTimestamp.FindStringSubmatch(n.FirstChild.Data)
				if len(m) != 2 {
					return ErrServerTimestampNotExtracted
				}
				ts, err := stringToDate(m[1])
				if err != nil {
					return fmt.Errorf("parsing creation time from `%s`: %w", m[1], err)

				}
				d.Timestamp = ts.UTC().Format(time.RFC3339)
			}
			if strings.HasPrefix(n.FirstChild.Data, "DOCS_modelChunk = {") {
				// text blobs
				embeddedTextMatches := embeddedText.FindAllStringSubmatch(n.FirstChild.Data, -1)
				for _, m := range embeddedTextMatches {
					var t string
					err := json.Unmarshal([]byte(m[1]), &t)
					if err != nil {
						log.Printf("%s: error: json unmarshall `%s...`: %v", d.Id, m[1][:30], err)
						continue
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
					var t string
					err := json.Unmarshal([]byte(m[1]), &t)
					if err != nil {
						log.Printf("%s: error: json unmarshall `%s...`: %v", d.Id, m[1][:30], err)
						continue
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
				// revision
				m := embeddedRevision.FindStringSubmatch(n.FirstChild.Data)
				if len(m) != 2 {
					continue
				}
				u64, err := strconv.ParseUint(m[1], 10, 32)
				if err != nil {
					continue // keep passing
				}
				if u32 := uint32(u64); d.Revision < u32 {
					d.Revision = u32
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
	// convert from ms
	t := time.UnixMilli(int64(f))
	return t, nil
}
