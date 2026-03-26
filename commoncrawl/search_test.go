package commoncrawl

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"testing"
)

const (
	warcHeader = iota
	httpHeader
	contentBlock
)

func TestSearch(t *testing.T) {
	c := NewClient()
	//  https://index.commoncrawl.org/CC-MAIN-2026-08-index?url=https://dprk-research.kmsec.uk*&output=json
	results, err := c.Search("CC-MAIN-2026-08", "url=https://dprk-research.kmsec.uk*")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for range results {
		count = count + 1
	}
	if count != 2 {
		t.Fatalf("expected 2 results, got %d", count)
	}
	for r := range results {
		if r.Error != nil {
			t.Fatalf("record error: %v", err)
		}
		if r.Record.Encoding == "" {
			t.Error("expected some encoding")
		}
		if r.Record.URL == "" {
			t.Error("expected some URL")
		}
		if r.Record.Filename == "" {
			t.Error("expected some Filename")
		}
		if r.Record.MimeDetected == "" {
			t.Error("expected some MimeDetected")
		}
		if r.Record.Status == "" {
			t.Error("expected some Status")
		}
	}
}

func TestReadWARCResponse(t *testing.T) {
	content := `{"urlkey": "uk,kmsec,dprk-research)/", "timestamp": "20260212191712", "url": "https://dprk-research.kmsec.uk/", "mime": "text/html", "mime-detected": "text/html", "status": "200", "digest": "62RJY6M7QI7IVE2AGKM6543W5CCAX3YK", "length": "12579", "offset": "153149978", "filename": "crawl-data/CC-MAIN-2026-08/segments/1770395506235.44/warc/CC-MAIN-20260212175836-20260212205836-00647.warc.gz", "encoding": "UTF-8"}`
	var r CDXRecord
	if err := json.Unmarshal([]byte(content), &r); err != nil {
		t.Fatal(err)
	}
	res, err := r.FetchWARCItem(http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Close()
	reader, err := gzip.NewReader(res)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for {
		if !scanner.Scan() {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
}
