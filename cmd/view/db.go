package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// ensure a directory exists.
func ensureDirExists(dir string) {
	err := os.MkdirAll(dir, 0750)
	if err != nil {
		panic(err)
	}

}

type DB struct {
	db               *sql.DB
	path             string
	SizeOnDisk       string
	NumProcessedDocs int32
	stmtStats        *sql.Stmt
}

// query for inserting a document. sepearated out for readability

func (d *DB) dbSize() {
	f, err := os.Stat(d.path)
	if err != nil {
		panic(err)
	}
	// b / kb / mb
	i := f.Size() / 1024 / 1024
	d.SizeOnDisk = fmt.Sprintf("%dMB", i)
}
func NewDB(dbPath string) (*DB, error) {
	ensureDirExists(filepath.Dir(dbPath))
	// the view server opens it in read-only mode.
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	DB := &DB{db: db,
		path: dbPath,
	}

	DB.dbSize()

	stmtStats, err := db.Prepare(`SELECT COUNT(*) FROM processed`)
	if err != nil {
		return nil, err
	}
	DB.stmtStats = stmtStats

	err = DB.stmtStats.QueryRow().Scan(&DB.NumProcessedDocs)
	if err != nil {
		return nil, err
	}

	return DB, nil
}

type Source struct {
	Src       string `json:"src"` // source - e.g. commoncrawl
	Id        string `json:"id"`  // id that narrows down the source - e.g. the commoncrawl dataset name
	Timestamp string `json:"ts"`  // timestamp - the time the doc was observed
}

// DocumentRow represents a single row in our paginated view
type DocumentRow struct {
	Timestamp   string
	Source      string
	DocID       string
	DocTitle    string
	Description string
}

// PageData holds the data for the HTML template
type PageData struct {
	Title            string
	Rows             []DocumentRow
	NextTS           string
	SizeOnDisk       string
	NumProcessedDocs int32
}

func (d *DB) ExplorePage(ts string) (*PageData, error) {
	var rows *sql.Rows
	var queryErr error

	// Base query uses a CTE to unnest the sources array and extract the fields
	// We use standard string comparison for keyset pagination on RFC3339 timestamps
	baseQuery := `
			WITH expanded AS (
				SELECT 
					json_extract(value, '$.ts') as timestamp,
					json_extract(value, '$.src') as source,
					docs.id as doc_id,
					docs.og_title as doc_title,
					docs.og_description as description
				FROM docs, json_each(docs.sources)
			)
			SELECT timestamp, source, doc_id, doc_title, description 
			FROM expanded
		`

	if ts != "" {
		query := baseQuery + ` 
				WHERE timestamp < ?
				ORDER BY timestamp DESC
				LIMIT 100`
		rows, queryErr = d.db.Query(query, ts)
	} else {
		query := baseQuery + ` 
				ORDER BY timestamp DESC
				LIMIT 100`
		rows, queryErr = d.db.Query(query)
	}

	if queryErr != nil {
		return nil, queryErr
	}

	defer rows.Close()

	var pageData PageData
	pageData.Title = "Explore"
	pageData.NumProcessedDocs = d.NumProcessedDocs
	pageData.SizeOnDisk = d.SizeOnDisk
	var lastTS string

	for rows.Next() {
		var row DocumentRow
		if err := rows.Scan(&row.Timestamp, &row.Source, &row.DocID, &row.DocTitle, &row.Description); err != nil {
			log.Printf("Row scan error: %v", err)
			continue
		}
		pageData.Rows = append(pageData.Rows, row)

		// Track the last seen row to use as the pagination key for the next page
		lastTS = row.Timestamp
	}

	// If we fetched exactly 100 rows, assume there is a next page
	if len(pageData.Rows) == 100 {
		pageData.NextTS = lastTS
	}
	return &pageData, nil
}
