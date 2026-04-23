package db

import (
	"database/sql"
	json "encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kmsec-uk/dochunt/gdoc"
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
	db              *sql.DB
	upsertDoc       *sql.Stmt
	insertProcessed *sql.Stmt
	existsProcessed *sql.Stmt
	stmtGetState    *sql.Stmt
	stmtSetState    *sql.Stmt
}

// query for inserting a document. sepearated out for readability
var upsertQuery string = `
INSERT INTO docs (
	id, revision, sources, created,
	page_title, og_title, og_description, og_image,
	content, links, image_urls
) VALUES (
	?, ?, json_array(json(?)), ?,
	?, ?, ?, ?,
	?, json(?), json(?)
)
ON CONFLICT(id, revision) DO UPDATE SET
	sources = json_insert(sources, '$[#]', json(?));`

var dsn string = "?_pragma=journal_mode(WAL)&" +
	"_pragma=foreign_keys(ON)&" +
	"_pragma=busy_timeout(5000)&" +
	"_pragma=synchronous(FULL)"

var ftsCreate string = `
CREATE VIRTUAL TABLE IF NOT EXISTS docs_fts USING fts5(
    id UNINDEXED,
    revision UNINDEXED,
    title,
    content,
	links
);`

var ftsBackfill string = `
INSERT INTO docs_fts(id, revision, title, content, links)
SELECT id, revision, og_title, content, links FROM docs;
`
var ftsInsertTrigger string = `
CREATE TRIGGER IF NOT EXISTS docs_fts_insert AFTER INSERT ON docs BEGIN
	INSERT INTO docs_fts(id, revision, title, content, links)
    VALUES (new.id, new.revision, new.og_title, new.content, new.links);
END;
`
var ftsDeleteTrigger string = `
CREATE TRIGGER IF NOT EXISTS docs_fts_delete AFTER DELETE ON docs BEGIN
    DELETE FROM docs_fts WHERE id = old.id AND revision = old.revision;
END;
`
var createUrlscanTable string = `
CREATE TABLE IF NOT EXISTS urlscan_state (
id INTEGER PRIMARY KEY CHECK (id = 1),
ts TEXT NOT NULL);`

var upsertUrlscanState string = `
INSERT INTO urlscan_state (id, ts) VALUES (1, ?) 
ON CONFLICT(id) DO UPDATE SET ts = excluded.ts;`

func NewDB(dbPath string) (*DB, error) {
	ensureDirExists(filepath.Dir(dbPath))
	db, err := sql.Open("sqlite", "file:"+dbPath+dsn)
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS docs (
		id TEXT NOT NULL,
		revision INTEGER NOT NULL,
		sources TEXT NOT NULL,
		created TEXT NOT NULL,
		page_title TEXT NOT NULL,
		og_title TEXT NOT NULL,
		og_description TEXT NOT NULL,
		og_image TEXT NOT NULL,
		content TEXT NOT NULL,
		links TEXT NOT NULL,
		image_urls TEXT NOT NULL,
		PRIMARY KEY (id, revision)
		);
		CREATE TABLE IF NOT EXISTS processed (digest TEXT PRIMARY KEY);`)

	if err != nil {
		return nil, fmt.Errorf("setting up db: %w", err)
	}
	//fts
	_, err = db.Exec(ftsCreate + ftsInsertTrigger + ftsDeleteTrigger)
	if err != nil {
		return nil, fmt.Errorf("setting up fts: %w", err)
	}
	// check if we need to backfill
	var countFts int
	err = db.QueryRow("SELECT COUNT(*) FROM docs_fts;").Scan(&countFts)
	if err != nil {
		return nil, fmt.Errorf("checking fts row count: %w", err)
	}

	if countFts == 0 {
		_, err = db.Exec(ftsBackfill)
		if err != nil {
			return nil, fmt.Errorf("backfilling fts: %w", err)
		}
	}

	// urlscan state
	_, err = db.Exec(createUrlscanTable)
	if err != nil {
		return nil, fmt.Errorf("setting up db: creating urlscan table: %w", err)
	}
	db.SetMaxOpenConns(1)
	DB := &DB{db: db}

	stmtInsertProcessed, err := db.Prepare(`INSERT INTO processed (digest) VALUES (?)`)
	if err != nil {
		return nil, err
	}
	DB.insertProcessed = stmtInsertProcessed

	stmtExistsProcessed, err := db.Prepare(`SELECT EXISTS(SELECT 1 FROM processed WHERE digest = ?)`)
	if err != nil {
		return nil, err
	}
	DB.existsProcessed = stmtExistsProcessed

	stmtUpsertDoc, err := db.Prepare(upsertQuery)
	if err != nil {
		return nil, err
	}
	DB.upsertDoc = stmtUpsertDoc

	stmtGetState, err := db.Prepare("SELECT ts FROM urlscan_state WHERE id = 1;")
	if err != nil {
		return nil, err
	}
	DB.stmtGetState = stmtGetState

	// updateState
	stmtSetState, err := db.Prepare(upsertUrlscanState)
	if err != nil {
		return nil, err
	}
	DB.stmtSetState = stmtSetState
	return DB, nil
}

// IsProcessed checks if a document has been ingested into
// the DB. The parameter is called digest, which initially
// referred to commoncrawl digest, but urlscan IDs are also
// used as the unique marker.
func (DB *DB) IsProcessed(digest string) (bool, error) {
	var exists bool
	err := DB.existsProcessed.QueryRow(digest).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

// LastUrlscanSearchTime retrieves the timestamp from the last search
func (DB *DB) LastUrlscanSearchTime() (string, error) {
	var timestamp string
	err := DB.stmtGetState.QueryRow().Scan(&timestamp)
	if err != nil {
		return "", err
	}
	return timestamp, nil
}

// UpdateLastUrlscanSearchTime sets the timestamp t
// as the contents of the `urlscan_state` table.
func (DB *DB) UpdateLastUrlscanSearchTime(t string) error {
	_, err := DB.stmtSetState.Exec(t)
	if err != nil {
		return err
	}
	return nil
}

type Source struct {
	Src       string `json:"src"` // source - e.g. commoncrawl
	Id        string `json:"id"`  // id that narrows down the source - e.g. the commoncrawl dataset name
	Timestamp string `json:"ts"`  // timestamp - the time the doc was observed
}

// returns a new Source object in []byte] format ready for
// insertion into db
func NewSource(src, id, ts string) ([]byte, error) {

	s := &Source{
		Src:       src,
		Id:        id,
		Timestamp: ts,
	}
	j, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	return j, nil

}

func (DB *DB) AddDoc(doc *gdoc.Doc, srcName, digest string) error {
	src, err := NewSource(srcName, doc.Provenance, doc.Timestamp)
	if err != nil {
		return fmt.Errorf("creating source object: %w", err)
	}
	links, err := json.Marshal(doc.Links)
	if err != nil {
		return fmt.Errorf("creating links array: %w", err)
	}
	images, err := json.Marshal(doc.ImageUrls)
	if err != nil {
		return fmt.Errorf("creating links array: %w", err)
	}

	tx, err := DB.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Stmt(DB.upsertDoc).Exec(
		doc.Id,
		doc.Revision,
		src,
		doc.Created.UTC().Format(time.RFC3339),
		doc.PageTitle,
		doc.OgTitle,
		doc.OgDescription,
		doc.OgImageUrl,
		doc.Content,
		links,
		images,
		src,
	)
	if err != nil {
		return fmt.Errorf("inserting doc: %w", err)
	}
	_, err = tx.Stmt(DB.insertProcessed).Exec(digest)
	if err != nil {
		return fmt.Errorf("inserting digest %s: %w", digest, err)
	}

	return tx.Commit()
}

func (DB *DB) Close() error {
	return DB.db.Close()
}
