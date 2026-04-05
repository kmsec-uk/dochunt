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
	// Initialize your DB here (using the path to your existing database file)
	// Example: db, err := sql.Open("sqlite", "file:data.db?_pragma=journal_mode(WAL)")
	DB, err := NewDB(*dbPath)
	if err != nil {
		panic(err)
	}
	l, err := content.ReadDir("static")
	if err != nil {
		fmt.Println(err)
	}
	for _, i := range l {
		fmt.Println(i.Name())
	}
	mux := http.NewServeMux()
	// favicon, css
	mux.Handle("GET /static/", http.FileServerFS(content))
	mux.HandleFunc("GET /static/{$}", http.NotFound)
	// index
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {

		cursorTS := r.URL.Query().Get("ts")

		p, err := DB.ExplorePage(cursorTS)
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
	mux.HandleFunc("/", http.NotFound)

	log.Println("Starting server on :8080...")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
