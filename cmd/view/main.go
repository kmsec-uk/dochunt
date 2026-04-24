package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	_ "modernc.org/sqlite"
)

func init() {
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "Google Docs browser!")
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	t := time.NewTicker(time.Second)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				err := DB.DbSize()
				if err != nil {
					log.Printf("error checking database size: %v", err)
				}
			}
		}
	}()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /robots.txt", func(w http.ResponseWriter, r *http.Request) {
		// don't allow access to index document contents
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, `User-agent: *
Allow: /$
Allow: /about
Allow: /static/
Disallow: /d/`)
	})
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
			log.Printf("error in template execution: %v", err)
		}
	})
	// index
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {

		cursorTS := r.URL.Query().Get("ts")
		ftsQuery := r.URL.Query().Get("q")
		wants := r.URL.Query().Get("format")
		if wants == "json" {
			w.Header().Set("Content-Type", "application/json")
			err := DB.ExploreJSON(cursorTS, ftsQuery, w)
			if err != nil {
				log.Printf("error producing json results: %v", err)
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
			log.Printf("error in template execution: %v", err)
		}
	})
	mux.HandleFunc("GET /d/{doc}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("doc")
		p, err := DB.DocPage(id)
		if err != nil {
			if errors.Is(sql.ErrNoRows, err) {

				docNotFound := &PageData{
					Title:            "Document not found",
					SizeOnDisk:       DB.SizeOnDisk.Load().(string),
					NumProcessedDocs: DB.NumProcessedDocs.Load(),
				}
				w.WriteHeader(http.StatusNotFound)
				if err := notfound.Execute(w, docNotFound); err != nil {
					log.Printf("error in template execution: %v", err)
				}
				return
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		wants := r.URL.Query().Get("format")
		if wants == "json" {
			w.Header().Set("Content-Type", "application/json")

			if err := json.NewEncoder(w).Encode(p); err != nil {
				log.Printf("error producing json page: %v", err)
			}
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := doc.Execute(w, p); err != nil {
			log.Printf("error in template execution: %v", err)
		}
	})
	// about
	mux.HandleFunc("GET /about", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		pageData := &AboutPage{}
		pageData.FAQs = AboutFAQs
		pageData.Title = "About"
		pageData.NumProcessedDocs = DB.NumProcessedDocs.Load()
		pageData.SizeOnDisk = DB.SizeOnDisk.Load().(string)

		if err := about.Execute(w, pageData); err != nil {
			log.Printf("error in template execution: %v", err)
		}
	})
	// 404
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		p := &PageData{
			Title:            "Not found",
			SizeOnDisk:       DB.SizeOnDisk.Load().(string),
			NumProcessedDocs: DB.NumProcessedDocs.Load(),
		}
		w.WriteHeader(http.StatusNotFound)
		if err := notfound.Execute(w, p); err != nil {
			log.Printf("error in template execution: %v", err)
		}
	})

	log.Println("Starting server on :8080...")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
