package warc

import (
	"errors"
	"os"
	"testing"

	"github.com/kmsec-uk/dochunt/gdoc"
)

func TestParseGzippedWarcItem(t *testing.T) {
	doc := &gdoc.Doc{
		Id: "tesst",
	}
	f, err := os.Open("test_files/test.gz")
	if err != nil {
		t.Fatal(err)
	}
	err = ParseGzippedWarcGDoc(f, doc)
	if err != nil {
		// since the gdoc parser is more guarded now,
		// it will reject a non-Google Docs page.
		// for this test, we just want to make sure
		// that the gzip -> warc item -> html parsing
		// logic is correct.
		if !errors.Is(err, gdoc.ErrDocParsing) {
			t.Fatal(err)
		}
	}
	// despite rejecting the file, it will have hopefully set the PageTitle attribute
	if got, want := doc.PageTitle, "feed | DPRK research"; got != want {
		t.Fatalf("parsed page title - want %s, got %s", want, got)
	}
}
