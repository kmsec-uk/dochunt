package commoncrawl

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
)

// CDXRecord matches the JSON output
// from the Common Crawl index
type CDXRecord struct {
	URL          string `json:"url"`
	Filename     string `json:"filename"`
	Offset       string `json:"offset"`
	Length       string `json:"length"`
	Status       string `json:"status"`
	MimeDetected string `json:"mime-detected"`
	Encoding     string `json:"encoding"`
}

// FetchWARCItem returns the http response body as-is
// for a given WARC offset.
//
// (note to self) don't forget to call .Close()!
func (r *CDXRecord) FetchWARCItem(client *http.Client) (io.ReadCloser, error) {
	offset, err := strconv.Atoi(r.Offset)
	if err != nil {
		return nil, fmt.Errorf("strconv.Atoi: %w", err)
	}
	length, err := strconv.Atoi(r.Length)
	if err != nil {
		return nil, fmt.Errorf("strconv.Atoi: %w", err)
	}
	rangeHeader := fmt.Sprintf("bytes=%d-%d", offset, offset+length-1)
	req, err := http.NewRequest("GET", "https://data.commoncrawl.org/"+r.Filename, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Add("user-agent", userAgent)
	req.Header.Add("range", rangeHeader)

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("doing request: %w", err)
	}
	if res.StatusCode != http.StatusPartialContent {
		res.Body.Close()
		return nil, fmt.Errorf("unexpected status code: %d", res.StatusCode)
	}
	return res.Body, nil
}
