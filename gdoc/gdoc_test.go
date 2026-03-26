package gdoc

import (
	"fmt"
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
			Id:        "Test - " + e.Name(),
			Links:     make([]string, 0),
			ImageUrls: make([]string, 0),
		}
		g.ParseHtml(f)

		if len(g.Content) == 0 {
			t.Fatalf("%s: should have some content", e.Name())
		}
		if len(g.PageTitle) == 0 {
			t.Fatalf("%s: should have some page title", e.Name())
		}
		if g.Revision == 0 {
			t.Fatalf("%s: should have a non-zero Revision", e.Name())

		}
		fmt.Println(g.Links)
		fmt.Println(g.ImageUrls)
	}
}
