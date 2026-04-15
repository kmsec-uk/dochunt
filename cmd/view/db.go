package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/dustin/go-humanize"
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
	SizeOnDisk       atomic.Value
	NumProcessedDocs atomic.Uint32
	stmtStats        *sql.Stmt
}

// return the database size
func (d *DB) dbSize() error {
	f, err := os.Stat(d.path)
	if err != nil {
		return err
	}
	d.SizeOnDisk.Store(humanize.Bytes(uint64(f.Size())))
	var numProcessedDocs uint32
	err = d.stmtStats.QueryRow().Scan(&numProcessedDocs)
	if err != nil {
		return err
	}
	d.NumProcessedDocs.Store(numProcessedDocs)
	return nil
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

	stmtStats, err := db.Prepare(`SELECT COUNT(*) FROM processed`)
	if err != nil {
		return nil, err
	}
	DB.stmtStats = stmtStats
	err = DB.dbSize()
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

// DocumentRow represents a single row in the view
type DocumentRow struct {
	Timestamp   string `json:"timestamp"`
	Source      string `json:"source"`
	DocID       string `json:"id"`
	DocTitle    string `json:"title"`
	Description string `json:"snippet"`
	Revision    string `json:"revision"`
}

// PageData holds the data for the HTML template
type PageData struct {
	Title            string
	SearchQuery      string
	Rows             []DocumentRow
	NextTS           string
	SizeOnDisk       string
	NumProcessedDocs uint32
}

// reference query:
// WITH expanded AS
// (
//        SELECT Json_extract(value, '$.ts')  AS timestamp,
//               Json_extract(value, '$.src') AS source,
//               docs.id                      AS id,
//               docs.revision                AS revision,
//               docs.og_title                AS doc_title,
//               docs.og_description          AS description
//        FROM   docs,
//               json_each(docs.sources) )
// SELECT   expanded.id,
//          expanded.revision,
//          title,
//          description,
//          timestamp,
//          source
// FROM     docs_fts f
// JOIN     expanded
// ON       f.id = expanded.id
// AND      f.revision = expanded.revision
//          --    WHERE docs_fts MATCH "*"
// ORDER BY rank ASC,
//          timestamp DESC
//          LIMIT 100

func (d *DB) ExplorePage(ts, ftsQuery string) (*PageData, error) {

	rows, err := d.doExploreQuery(ts, ftsQuery)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var pageData PageData
	pageData.Title = "Explore"
	pageData.NumProcessedDocs = d.NumProcessedDocs.Load()
	pageData.SizeOnDisk = d.SizeOnDisk.Load().(string)
	pageData.SearchQuery = ftsQuery
	var lastTS string

	for rows.Next() {
		var row DocumentRow
		if err := rows.Scan(&row.DocID, &row.Revision, &row.DocTitle, &row.Description, &row.Timestamp, &row.Source); err != nil {
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

func (d *DB) ExploreJSON(ts, ftsQuery string, w http.ResponseWriter) error {
	rows, err := d.doExploreQuery(ts, ftsQuery)
	if err != nil {
		return err
	}

	defer rows.Close()

	for rows.Next() {
		var row DocumentRow
		if err := rows.Scan(&row.DocID, &row.Revision, &row.DocTitle, &row.Description, &row.Timestamp, &row.Source); err != nil {
			return fmt.Errorf("Row scan error: %w", err)
		}
		b, err := json.Marshal(row)
		if err != nil {
			return fmt.Errorf("json.Marshal error: %w", err)
		}
		if _, err := w.Write(b); err != nil {
			return err
		}
		if _, err := w.Write([]byte("\r\n")); err != nil {
			return err
		}
	}
	return nil
}

// helper function to create and perform the sql query
// as there are an exponential number of different ways
func (d *DB) doExploreQuery(ts, ftsQuery string) (*sql.Rows, error) {

	baseSelect := `
		SELECT 
			d.id,
			d.revision,
			d.og_title AS doc_title,
			d.og_description AS description,
			json_extract(j.value, '$.ts') AS timestamp,
			json_extract(j.value, '$.src') AS source
		FROM docs d
		JOIN json_each(d.sources) j
	`

	if ts == "" && ftsQuery == "" {
		query := baseSelect + ` ORDER BY timestamp DESC LIMIT 100`
		return d.db.Query(query)
	}

	if ts == "" && ftsQuery != "" {
		query := `
			SELECT 
				d.id, d.revision, d.og_title AS doc_title, d.og_description AS description,
				json_extract(j.value, '$.ts') AS timestamp, json_extract(j.value, '$.src') AS source
			FROM docs_fts f
			JOIN docs d ON f.id = d.id AND f.revision = d.revision
			JOIN json_each(d.sources) j
			WHERE docs_fts MATCH ?
			ORDER BY rank, timestamp DESC
			LIMIT 100
		`
		return d.db.Query(query, ftsQuery)
	}

	if ts != "" && ftsQuery != "" {
		query := `
			SELECT 
				d.id, d.revision, d.og_title AS doc_title, d.og_description AS description,
				json_extract(j.value, '$.ts') AS timestamp, json_extract(j.value, '$.src') AS source
			FROM docs_fts f
			JOIN docs d ON f.id = d.id AND f.revision = d.revision
			JOIN json_each(d.sources) j
			WHERE docs_fts MATCH ? AND json_extract(j.value, '$.ts') < ?
			ORDER BY rank, timestamp DESC
			LIMIT 100
		`
		return d.db.Query(query, ftsQuery, ts)
	}

	if ts != "" && ftsQuery == "" {
		query := baseSelect + ` WHERE timestamp < ? ORDER BY timestamp DESC LIMIT 100`
		return d.db.Query(query, ts)
	}

	return nil, errors.New("unreachable code reached")
}
