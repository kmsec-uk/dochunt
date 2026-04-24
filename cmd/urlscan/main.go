package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kmsec-uk/dochunt/db"
	"github.com/kmsec-uk/dochunt/gdoc"
)

var ErrApiKeyEmpty = errors.New("api file is empty")

var DocQuery string = `page.url:"https://docs.google.com/document/d/*" page.status:200`

// ParsedPayload is used to pass data safely from the concurrent
// workers to the single-threaded DB writer.
type ParsedPayload struct {
	Doc    *gdoc.Doc
	Digest string
}

func init() {
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "urlscan Google Docs scraper!")
		fmt.Fprintln(flag.CommandLine.Output(), "requires an apikey held inside a file")
		flag.PrintDefaults()
	}
}

// readApiKey returns the API key held within the
// apifile. Error returned if empty or nonexistent.
func readApiKey(filepath string) (string, error) {
	b, err := os.ReadFile(filepath)
	if err != nil {
		return "", err
	}
	apikey := strings.TrimSpace(string(b))
	if apikey == "" {
		return "", ErrApiKeyEmpty
	}
	return apikey, nil
}

func main() {

	dbPath := flag.String("db", "../data/store.db", "path to database")
	apifile := flag.String("apifile", "./.apikey", "path to urlscan api file")

	flag.Parse()

	apikey, err := readApiKey(*apifile)
	if err != nil {
		log.Fatalf("reading apifile `%s`: %v", *apifile, err)
	}
	u := NewUrlscanClient(apikey)

	db, err := db.NewDB(*dbPath)
	if err != nil {
		log.Fatalf("init db: %v", err)
	}
	defer db.Close()
	log.Println("database setup complete, starting collection")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	since, err := db.LastUrlscanSearchTime()
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			since = ""
		} else {
			log.Fatalf("error getting last timestamp: %v", err)
		}
	}
	currentTime := strconv.FormatInt(time.Now().UnixMilli(), 10)

	log.Printf("running query for results since %s", since)

	searchResults, err := u.Search(ctx, DocQuery, since)

	if err != nil {
		log.Fatalf("error running search: %v", err)
	}

	writerDone := make(chan struct{})
	resultsCh := make(chan ParsedPayload, 50)

	go func() {
		for p := range resultsCh {
			if err := db.AddDoc(p.Doc, "urlscan", p.Digest); err != nil {
				log.Printf("error: inserting doc %s (urlscan id %s): %v", err, p.Doc.Id, p.Doc.Provenance)
				continue
			}
			log.Printf("%s ingested into db", p.Doc.Id)
		}
		log.Println("closing db writer channel")
		close(writerDone)
	}()

	for res := range searchResults {
		// skip non-public scans
		if res.Item.Task.Visibility != "public" {
			log.Printf("%s: skipped (non-public scan): %s", res.Item.Id, res.Item.Page.Url)
			continue
		}
		// skip export endpoints
		if strings.Contains(res.Item.Page.Url, "/export") {
			log.Printf("%s: skipped (export): %s", res.Item.Id, res.Item.Page.Url)
			continue
		}
		// skip /pub (publish) endpoints
		// example: .../document/d/0B5LPyQqaZw-3cXRBT0kyNTh2TVU/pub
		if strings.Contains(res.Item.Page.Url, "/pub") {
			log.Printf("%s: skipped (publish): %s", res.Item.Id, res.Item.Page.Url)
			continue
		}
		exists, err := db.IsProcessed(res.Item.Id)
		if err != nil {
			log.Printf("error: checking digest in DB: %v", err)
			continue
		}
		if exists {
			log.Printf("%s: skipped (already ingested): %s", res.Item.Id, res.Item.Page.Url)
			continue
		}
		// grab unique doc id
		m := gdoc.GoogleDocIdRegex.FindStringSubmatch(res.Item.Page.Url)
		if m == nil || len(m) != 2 {
			log.Printf("%s: skipped (could not identify google doc id): %s", res.Item.Id, res.Item.Page.Url)
			continue
		}
		// we use the item ID as "provenance" field
		doc := gdoc.NewDoc(m[1]).WithProvenance(res.Item.Id)
		reader, err := u.GetDom(ctx, res.Item.Id)
		if err != nil {
			if errors.Is(err, ErrRateLimitExhausted) {
				log.Printf("critical error: %v", err)
				break
			}
			log.Printf("error: doc %s: fetching DOM for %s: %v\n", doc.Id, res.Item.Id, err)
			continue
		}
		defer reader.Close()
		if err := doc.ParseHtml(reader); err != nil {
			log.Printf("error: doc %s: parsing urlscan ID `%s` html stream: %v\n", doc.Id, res.Item.Id, err)
			continue
		}
		resultsCh <- ParsedPayload{doc, res.Item.Id}

	}
	log.Println("closing results channel")
	close(resultsCh)
	<-writerDone

	// update state
	err = db.UpdateLastUrlscanSearchTime(currentTime)
	if err != nil {
		log.Printf("Could not update state with timestamp %s: %v", currentTime, err)

	}

	log.Println("finished processing")
}
