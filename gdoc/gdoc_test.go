package gdoc

import (
	"os"
	"path"
	"testing"
)

func TestGDocParse(t *testing.T) {

	entries, err := os.ReadDir("test_files")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() == ".gitignore" {
			continue
		}
		f, err := os.Open(path.Join("test_files", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		g := Doc{
			Id:        "",
			Links:     make([]string, 0),
			ImageUrls: make([]string, 0),
		}
		g.WithProvenance(e.Name())
		err = g.ParseHtml(f)
		if err != nil {
			t.Fatal(err)
		}
		if g.Id == "" {
			t.Errorf("%s: doc id should be present", g.Provenance)
		}
		if g.Timestamp == "" {
			t.Errorf("%s: timestamp should be present", g.Provenance)
		}
		if len(g.Content) == 0 {
			t.Fatalf("%s: should have some content", g.Provenance)
		}
		if len(g.PageTitle) == 0 {
			t.Fatalf("%s: should have some page title", g.Provenance)
		}
		if g.Revision == 0 {
			t.Fatalf("%s: should have a non-zero Revision", g.Provenance)

		}
	}
}
