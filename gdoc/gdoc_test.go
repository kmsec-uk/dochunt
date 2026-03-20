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
		f, err := os.Open(path.Join("test_files", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		g := Doc{
			Id:        "Test -" + e.Name(),
			Links:     make([]string, 0),
			ImageUrls: make([]string, 0),
		}
		g.ParseHtml(f)

		if len(g.Content) == 0 {
			t.Fatal("should have some content")
		}
		if len(g.PageTitle) == 0 {
			t.Fatal("should have some page title")
		}
		fmt.Println(g.Content)
	}
}
