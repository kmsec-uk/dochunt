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
	"reflect"
	"sync/atomic"
	"time"

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
	stmtDocsById     *sql.Stmt
}

// return the database size
func (d *DB) DbSize() error {
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
	err = DB.DbSize()
	if err != nil {
		return nil, err
	}
	stmtDocsById, err := db.Prepare(`SELECT id, revision, created, og_title, og_description, content, links, image_urls, sources FROM docs WHERE id = ? ORDER BY revision DESC`)
	if err != nil {
		return nil, err
	}
	DB.stmtDocsById = stmtDocsById

	return DB, nil
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

// PageData holds the data for the base HTML template
type PageData struct {
	Title            string
	SizeOnDisk       string
	NumProcessedDocs uint32
}
type IndexPage struct {
	PageData
	SearchQuery string
	Rows        []DocumentRow
	NextTS      string
}
type JsonArrayString []string

func (s *JsonArrayString) Scan(i any) error {
	if reflect.TypeOf(i).String() != "string" {
		return errors.New("JsonArrayString: Scan(): unexpected type")
	}
	if err := json.Unmarshal([]byte(i.(string)), &s); err != nil {
		return err
	}
	return nil
}

type JsonArraySource []Source

func (s *JsonArraySource) Scan(i any) error {
	if reflect.TypeOf(i).String() != "string" {
		return errors.New("JsonArraySource: Scan(): unexpected type")
	}
	if err := json.Unmarshal([]byte(i.(string)), &s); err != nil {
		return err
	}
	return nil
}

type DocInstance struct {
	Id          string          `json:"id"`
	Title       string          `json:"title"`
	Revision    string          `json:"revision"`
	Created     string          `json:"created"`
	Description string          `json:"description"`
	Content     string          `json:"content"`
	Links       JsonArrayString `json:"links"`
	ImageUrls   JsonArrayString `json:"images"`
	Sources     JsonArraySource `json:"sources"`
}

// DocPageData holds the data for a specific document
type DocPageData struct {
	PageData
	Id        string        `json:"id"`
	LastSeen  string        `json:"last_seen"`
	Instances []DocInstance `json:"instances"`
}
type Timestamp struct {
	time.Time
}

func (t *Timestamp) MarshalJSON() ([]byte, error) {
	return []byte(`"` + t.UTC().Format(time.RFC3339) + `"`), nil
}

type Source struct {
	Src       string    `json:"src"` // source - e.g. commoncrawl
	Id        string    `json:"id"`  // id that narrows down the source - e.g. the commoncrawl dataset name
	Timestamp Timestamp `json:"ts"`  // timestamp - the time the doc was observed
}

// 1qk9pmMy966ue1iSSqGQ-61f19s99dBQ5NMo_Rz9UBY0
func (d *DB) DocPage(id string) (*DocPageData, error) {
	p := &DocPageData{
		Instances: make([]DocInstance, 0),
	}
	p.SizeOnDisk = d.SizeOnDisk.Load().(string)
	p.NumProcessedDocs = d.NumProcessedDocs.Load()
	rows, err := d.stmtDocsById.Query(id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var doc DocInstance
		//`SELECT id, revision, created, og_title, og_description, content, links, image_urls, sources FROM docs WHERE id = ? ORDER BY revision DESC`
		if err := rows.Scan(&doc.Id, &doc.Revision, &doc.Created, &doc.Title, &doc.Description, &doc.Content, &doc.Links, &doc.ImageUrls, &doc.Sources); err != nil {
			return nil, err
		}
		p.Instances = append(p.Instances, doc)
	}
	if len(p.Instances) == 0 {
		return nil, sql.ErrNoRows
	}
	latest := p.Instances[0]
	p.Id = latest.Id
	p.Title = latest.Title
	lastSeen := time.Time{}
	for _, s := range latest.Sources {
		if s.Timestamp.After(lastSeen) {
			lastSeen = s.Timestamp.Time
		}
	}
	p.LastSeen = lastSeen.UTC().Format(time.RFC3339)
	return p, nil
}

// ExplorePage creates the PageData for the index page (the query
// view)
func (d *DB) ExplorePage(ts, ftsQuery string) (*IndexPage, error) {

	rows, err := d.doExploreQuery(ts, ftsQuery)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var pageData IndexPage
	pageData.Title = "Explore"
	pageData.NumProcessedDocs = d.NumProcessedDocs.Load()
	pageData.SizeOnDisk = d.SizeOnDisk.Load().(string)
	pageData.SearchQuery = ftsQuery
	var lastTS string

	for rows.Next() {
		var row DocumentRow
		if err := rows.Scan(&row.DocID, &row.Revision, &row.DocTitle, &row.Description, &row.Timestamp, &row.Source); err != nil {
			log.Printf("error scanning row: %v", err)
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

// The query view's (index page) JSON interface
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
