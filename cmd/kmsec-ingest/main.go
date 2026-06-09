package main

import (
	"crypto/sha1"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kmsec-uk/dochunt/db"
	"github.com/kmsec-uk/dochunt/gdoc"
)

type Processed struct {
	hash string
	doc  gdoc.Doc
}

func main() {

	DB, err := db.NewDB("../data/store.db")
	if err != nil {
		panic(err)
	}
	files, err := filepath.Glob("docs/*.html")
	if err != nil {
		panic(err)
	}
	var processed []Processed
	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			panic(err)
		}
		defer f.Close()
		doc := gdoc.NewDoc("").WithProvenance("manual")
		doc.ParseHtml(f)
		var alreadyProcessed bool
		for _, p := range processed {
			if p.doc.Id == doc.Id && p.doc.Revision == doc.Revision {
				alreadyProcessed = true
				break
			}
		}
		if alreadyProcessed {
			continue
		}
		if _, err := f.Seek(0, 0); err != nil {
			panic(err)
		}
		h := sha1.New()

		if _, err := io.Copy(h, f); err != nil {
			panic(err)
		}

		processed = append(processed, Processed{doc: *doc, hash: fmt.Sprintf("%x", h.Sum(nil))})
		fmt.Println(doc.Id, doc.Revision, doc.Timestamp, doc.Links, fmt.Sprintf("%x", h.Sum(nil)))
	}
	for _, p := range processed {
		if err := DB.AddDoc(&p.doc, "kmsec.uk", p.hash); err != nil {
			panic(err)
		}
	}
}
