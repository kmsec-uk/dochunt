package warc

import (
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
		t.Fatal(err)
	}
	if got, want := doc.PageTitle, "feed | DPRK research"; got != want {
		t.Fatalf("parsed page title - want %s, got %s", want, got)
	}
}
