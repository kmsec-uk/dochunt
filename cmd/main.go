package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"github.com/kmsec-uk/ccdocs/commoncrawl"
	"github.com/kmsec-uk/ccdocs/gdoc"
	"github.com/kmsec-uk/ccdocs/warc"
	"golang.org/x/sync/errgroup"
)

// ParsedPayload is used to pass data safely from the concurrent
// workers to the single-threaded DB writer.
type ParsedPayload struct {
	Doc    *gdoc.Doc
	Digest string
}

type Progress struct {
	skipped   atomic.Int32
	errors    atomic.Int32
	completed atomic.Int32
}

func (p *Progress) now() string {
	return fmt.Sprintf("completed: %d | errors: %d | skipped: %d", p.completed.Load(), p.errors.Load(), p.skipped.Load())
}

var spin = []string{
	"|",
	"/",
	"—",
	"\\",
}

func init() {
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "Google Docs CommonCrawl scraper!")
		flag.PrintDefaults()
	}
}

func main() {

	dbPath := flag.String("db", "data/store.db", "path to database")

	collection := flag.String("c", "CC-MAIN-2026-12", "commoncrawl collection to scrape")
	flag.Parse()

	db, err := NewDB(*dbPath)
	if err != nil {
		log.Fatalf("init db: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	progress := Progress{}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	go func() {
		idx := 0

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				idx = (idx + 1) % len(spin)
				fmt.Printf("\033[2K\r%s %s", spin[idx], progress.now())
			}
		}
	}()

	client := commoncrawl.NewClient()
	ch, err := client.Search(*collection, "url=https://docs.google.com/document/d/*&filter==status:200")

	if err != nil {
		log.Fatalf("running search : %v", err)
	}

	writerDone := make(chan struct{})
	resultsCh := make(chan ParsedPayload, 50)

	go func() {
		for p := range resultsCh {
			if err = db.AddDoc(p.Doc, "commoncrawl", p.Digest); err != nil {
				progress.errors.Add(1)
				log.Printf("error: inserting doc: %v", err)
				continue
			}
			progress.completed.Add(1)
		}
		close(writerDone)
	}()

	g, groupCtx := errgroup.WithContext(ctx)
	g.SetLimit(5)

	for res := range ch {
		if groupCtx.Err() != nil {
			break
		}
		if err := res.Error; err != nil {
			log.Printf("error: %v", err)
			if errors.Is(err, commoncrawl.ErrRateLimitExhausted) {
				log.Printf("cancelling all workers")
				cancel()
				break
			}
			continue
		}
		// skip Google Docs export endpoints
		if strings.Contains(res.Record.URL, "/export") {
			progress.skipped.Add(1)
			continue
		}
		// skip /pub (publish) endpoints
		// example: .../document/d/0B5LPyQqaZw-3cXRBT0kyNTh2TVU/pub
		if strings.Contains(res.Record.URL, "/pub") {
			progress.skipped.Add(1)
			continue
		}
		// exclute non-utf0
		if res.Record.Encoding != "UTF-8" {
			progress.skipped.Add(1)
			continue
		}
		// exclude non text/html response
		if res.Record.MimeDetected != "text/html" {
			progress.skipped.Add(1)
			continue
		}
		exists, err := db.IsProcessed(res.Record.Digest)
		if err != nil {
			progress.errors.Add(1)
			log.Printf("error: checking digest in DB: %v", err)
			continue
		}
		if exists {
			// skip
			progress.skipped.Add(1)
			continue
		}
		g.Go(func() error {
			// grab unique doc id
			m := gdoc.GoogleDocIdRegex.FindStringSubmatch(res.Record.URL)
			if m == nil || len(m) != 2 {
				return nil
			}
			doc := gdoc.NewDoc(m[1]).WithProvenance(*collection)

			reader, err := client.FetchWARCItem(&res.Record)
			if err != nil {
				progress.errors.Add(1)
				log.Printf("error: doc %s: fetching %s: %v\n", doc.Id, res.Record.Filename, err)
				return err
			}
			err = warc.ParseGzippedWarcGDoc(reader, doc)

			if err != nil {
				progress.errors.Add(1)
				log.Printf("error: doc %s: parsing gzipped html stream: %v\n", doc.Id, err)
				return nil
			}
			select {
			case <-groupCtx.Done():
				return groupCtx.Err()

			case resultsCh <- ParsedPayload{doc, res.Record.Digest}:
			}
			return nil
		})

	}

	if err := g.Wait(); err != nil {
		log.Fatalf("aborting: %v", err)
	}
	close(resultsCh)
	<-writerDone
	log.Println("finished processing")
}
