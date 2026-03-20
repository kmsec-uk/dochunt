package main

import (
	"fmt"
	//	"strings"
)

func main() {
	client := NewClient()
	ch, err := client.Search("CC-MAIN-2026-08-index", "url=https://docs.google.com/document/d/*&filter==status:200")
	if err != nil {
		panic(err)
	}
	for res := range ch {
		if err := res.Error; err != nil {
			fmt.Println(err.Error())
			continue
		}
		fmt.Println(res.Record.URL)
	}
}

// func main() {
// 	// Notice we added &output=json to get clean JSON lines instead of the mixed text format
// 	baseURL := "https://index.commoncrawl.org/CC-MAIN-2026-08-index?url=https://docs.google.com/document/d/*&filter==status:200&output=json&limit=5"

// 	// Paging loop (just doing page 0 for this example)
// 	for page := 0; page < 2; page++ {
// 		queryURL := fmt.Sprintf("%s&page=%d", baseURL, page)
// 		fmt.Printf("Fetching index page %d...\n", page)

// 		processIndexPage(queryURL)
// 	}
// }

// func processIndexPage(queryURL string) {
// 	resp, err := http.Get(queryURL)
// 	if err != nil {
// 		fmt.Println("Error fetching index:", err)
// 		return
// 	}
// 	defer resp.Body.Close()

// 	// The API returns JSON lines, so we read it line by line
// 	scanner := bufio.NewScanner(resp.Body)
// 	for scanner.Scan() {
// 		var record CDXRecord
// 		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
// 			continue
// 		}

// 		fmt.Printf("\n--- Processing Doc: %s ---\n", record.URL)
// 		fetchAndParseWARC(record)
// 	}
// }

// func fetchAndParseWARC(record CDXRecord) {
// 	// 1. Calculate the byte range
// 	offset, _ := strconv.Atoi(record.Offset)
// 	length, _ := strconv.Atoi(record.Length)

// 	// The range is inclusive, so we subtract 1
// 	rangeHeader := fmt.Sprintf("bytes=%d-%d", offset, offset+length-1)
// 	warcURL := "https://data.commoncrawl.org/" + record.Filename

// 	// 2. Make the Range Request
// 	req, _ := http.NewRequest("GET", warcURL, nil)
// 	req.Header.Set("Range", rangeHeader)

// 	client := &http.Client{}
// 	resp, err := client.Do(req)
// 	if err != nil || resp.StatusCode != http.StatusPartialContent {
// 		fmt.Println("Failed to fetch WARC chunk")
// 		return
// 	}
// 	defer resp.Body.Close()

// 	// 3. Unzip the chunk
// 	gzReader, err := gzip.NewReader(resp.Body)
// 	if err != nil {
// 		fmt.Println("Gzip error:", err)
// 		return
// 	}
// 	defer gzReader.Close()

// 	// 4. Skip WARC headers to get to the HTTP response
// 	bufReader := bufio.NewReader(gzReader)
// 	for {
// 		line, err := bufReader.ReadString('\n')
// 		if err != nil || line == "\r\n" || line == "\n" {
// 			// An empty line signifies the end of the WARC headers
// 			break
// 		}
// 	}

// 	// 5. Parse the embedded HTTP response
// 	// We pass a dummy request just to satisfy the function signature
// 	embeddedResp, err := http.ReadResponse(bufReader, nil)
// 	if err != nil {
// 		fmt.Println("Error parsing embedded HTTP response:", err)
// 		return
// 	}
// 	defer embeddedResp.Body.Close()

// 	// 6. Parse the HTML and extract meta tags
// 	extractMetaTags(embeddedResp.Body)
// }

// func extractMetaTags(htmlBody io.Reader) {
// 	tokenizer := html.NewTokenizer(htmlBody)

// 	for {
// 		tt := tokenizer.Next()

// 		switch tt {
// 		case html.ErrorToken:
// 			// End of document
// 			return
// 		case html.StartTagToken, html.SelfClosingTagToken:
// 			t := tokenizer.Token()

// 			// We only care about <meta> tags
// 			if t.Data == "meta" {
// 				var name, property, content string
// 				for _, attr := range t.Attr {
// 					if attr.Key == "name" {
// 						name = attr.Val
// 					} else if attr.Key == "property" {
// 						property = attr.Val
// 					} else if attr.Key == "content" {
// 						content = attr.Val
// 					}
// 				}

// 				key := name
// 				if property != "" {
// 					key = property
// 				}

// 				if key != "" && content != "" {
// 					fmt.Printf("%s: %s\n", key, content)
// 				}
// 			}

// 			// Optional optimization: Break early once we hit the <body> tag
// 			// since meta tags only live in the <head>
// 			if t.Data == "body" {
// 				return
// 			}
// 		}
// 	}
// }
