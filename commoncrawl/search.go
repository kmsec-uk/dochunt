package commoncrawl

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

const userAgent = "go: kmsec.uk (contact: kmsec-uk@pm.me)"

// SearchMetadata represents the response from index
// when we request showNumPages=true.
// only care about number of pages tbh.
type SearchMetadata struct {
	Pages int `json:"pages"`
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

func (c *Client) HTTPClient() *http.Client {
	return c.client
}

// NewClient creates a new client to interact with commoncrawl
func NewClient() *Client {
	// in order to have relaxed body read timeouts but strict header
	// receipt timeout, we need to clone the default transport
	// the Client Timeouet is then set to 0
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = 5 * time.Second
	return &Client{
		client: &http.Client{Transport: t, Timeout: 0},
	}
}

// Search searches a commoncrawl collection via the index, with a given raw query string.
// For example `c.Search("CC-MAIN-2026-08", "url=https://example.com/*&filter==status:200")`
// will search the February 2026 dataset for results for example.com/*, filtering where the
// status is 200
func (c *Client) Search(collection, rawquery string) (<-chan SearchResult, error) {

	// get number of pages
	req, err := http.NewRequest("GET", "https://index.commoncrawl.org/"+collection+"-index?output=json&"+rawquery+"&showNumPages=true", nil)
	if err != nil {
		return nil, fmt.Errorf("retrieving number of pages: creationg request: %w", err)
	}
	req.Header.Add("user-agent", userAgent)
	res, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("retrieving number of pages: doing request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("retrieving number of pages: unexpected status code %d", res.StatusCode)
	}
	var sm SearchMetadata
	err = json.NewDecoder(res.Body).Decode(&sm)
	if err != nil {
		return nil, err
	}
	if sm.Pages == 0 {
		return nil, fmt.Errorf("the index says there are 0 pages of results")
	}
	// actually start getting results.
	// i've opted for a single goroutine for this as I don't want to hammer the index.
	ch := make(chan SearchResult, 15000)
	go func() {
		defer close(ch)
		for p := 0; p < sm.Pages; p++ {
			url := "https://index.commoncrawl.org/" + collection + "-index?output=json&page=" + strconv.Itoa(p) + "&" + rawquery
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
