package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
)

const userAgent = "go: (kmsec.uk)"

// SearchMetadata represents the response from index
// when we request showNumPages=true.
// only care about number of pages tbh.
type SearchMetadata struct {
	Pages int `json:"pages"`
}

// CDXRecord matches the JSON output from the Common Crawl index
type CDXRecord struct {
	URL      string `json:"url"`
	Filename string `json:"filename"`
	Offset   string `json:"offset"`
	Length   string `json:"length"`
	Status   string `json:"status"`
}

type SearchResult struct {
	Record CDXRecord
	Error  error
}

// The Client interacts with
// CommonCrawl
type Client struct {
	client *http.Client
}

func NewClient() *Client {
	// in order to have relaxed body read timeouts but strict header
	// receipt timeout, we need to clone the default transport
	// the Client Timeouet is then set to 0
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = 2 * time.Second
	return &Client{
		client: &http.Client{Transport: t, Timeout: 0},
	}
}

func (c *Client) Search(collection, rawquery string) (<-chan SearchResult, error) {

	// get number of pages
	req, err := http.NewRequest("GET", "https://index.commoncrawl.org/"+collection+"?output=json&"+rawquery+"&showNumPages=true", nil)
	if err != nil {
		return nil, fmt.Errorf("creationg request: %w", err)
	}
	req.Header.Add("user-agent", userAgent)
	res, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("doing request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d", res.StatusCode)
	}
	var sm SearchMetadata
	err = json.NewDecoder(res.Body).Decode(&sm)
	if err != nil {
		return nil, err
	}
	if sm.Pages == 0 {
		return nil, fmt.Errorf("the index says there are 0 pages of results")
	}
	log.Printf("there are %d pages", sm.Pages)
	// actually start getting results
	ch := make(chan SearchResult, 15000)
	go func() {
		defer close(ch)
		for p := 0; p < sm.Pages; p++ {
			url := "https://index.commoncrawl.org/" + collection + "?output=json&page=" + strconv.Itoa(p) + "&" + rawquery
			log.Println(url)
			req, err := http.NewRequest("GET", url, nil)
			if err != nil {
				ch <- SearchResult{Error: fmt.Errorf("page %d: creating request: %w", p, err)}
				continue
			}
			req.Header.Add("user-agent", userAgent)

			res, err := c.client.Do(req)
			if err != nil {
				ch <- SearchResult{Error: fmt.Errorf("page %d: doing request: %w", p, err)}
				continue
			}

			if res.StatusCode != http.StatusOK {
				ch <- SearchResult{Error: fmt.Errorf("page %d: unexpected status code %d", p, res.StatusCode)}
				res.Body.Close()
				continue
			}

			scanner := bufio.NewScanner(res.Body)
			for scanner.Scan() {
				var record CDXRecord
				if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
					ch <- SearchResult{Error: err}
					continue
				}
				ch <- SearchResult{Record: record}
			}
			if err := scanner.Err(); err != nil {
				ch <- SearchResult{Error: err}
			}
			res.Body.Close()
		}
	}()

	return ch, nil
}
