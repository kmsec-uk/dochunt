package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	_ "modernc.org/sqlite"
)

func init() {
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "Google Docs CommonCrawl scraper!")
		flag.PrintDefaults()
	}
}

func main() {

	dbPath := flag.String("db", "../data/store.db", "path to database")
	flag.Parse()

	DB, err := NewDB(*dbPath)
	if err != nil {
		panic(err)
	}
	mux := http.NewServeMux()
	// favicon, css
	mux.Handle("GET /static/", http.FileServerFS(content))
	mux.HandleFunc("GET /static/{$}", func(w http.ResponseWriter, r *http.Request) {
		// prevent dir browsing
		p := &PageData{
			Title:            "Not found",
			SizeOnDisk:       DB.SizeOnDisk.Load().(string),
			NumProcessedDocs: DB.NumProcessedDocs.Load(),
		}
		if err := notfound.Execute(w, p); err != nil {
			log.Printf("Template execution error: %v", err)
		}
	})
	// index
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {

		cursorTS := r.URL.Query().Get("ts")
		ftsQuery := r.URL.Query().Get("q")
		wants := r.URL.Query().Get("format")
		if wants == "json" {
			err := DB.ExploreJSON(cursorTS, ftsQuery, w)
			if err != nil {
				log.Printf("error producing json results: %w", err)
			}
			return
		}
		p, err := DB.ExplorePage(cursorTS, ftsQuery)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := index.Execute(w, p); err != nil {
			log.Printf("Template execution error: %v", err)
		}
	})
	// 404
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		p := &PageData{
			Title:            "Not found",
			SizeOnDisk:       DB.SizeOnDisk.Load().(string),
			NumProcessedDocs: DB.NumProcessedDocs.Load(),
		}
		if err := notfound.Execute(w, p); err != nil {
			log.Printf("Template execution error: %v", err)
		}
	})

	log.Println("Starting server on :8080...")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
