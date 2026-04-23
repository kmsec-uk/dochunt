package main

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/kmsec-uk/ccdocs/gdoc"
)

func TestUrlscanGetDom(t *testing.T) {
	u := NewUrlscanClient("")
	res, err := u.GetDom(t.Context(), "019d9d10-37b7-70ac-8610-bf515bbe3ab2")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Close()
	d := gdoc.Doc{}
	err = d.ParseHtml(res)
	if err != nil {
		t.Fatal(err)
	}
	if d.Id == "" {
		t.Fatal("empty id")
	}
	fmt.Println(d.Id, d.Created, d.OgDescription)
}

// this one should fail GDOC parsing
func TestUrlscanGetDomShouldFail(t *testing.T) {
	u := NewUrlscanClient("").WithRateLimit(2 * time.Second)
	res, err := u.GetDom(t.Context(), "019da473-ca63-756e-a624-e2e5c7710b2d")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Close()
	d := gdoc.Doc{}
	err = d.ParseHtml(res)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUrlscanSearch(t *testing.T) {
	u := NewUrlscanClient("").WithRateLimit(2 * time.Second)
	res, err := u.Search(t.Context(), "page.domain:lseg.com AND date:>1768889818", "")
	if err != nil {
		t.Fatal(err)
	}
	for r := range res {
		if r.Error != nil {
			t.Errorf("TestUrlscanSearch: range channel: %v", r.Error)
			continue
		}
		// fmt.Println(r.Item.Id)
	}
}
func TestEndToEnd(t *testing.T) {
	ctx := t.Context()
	u := NewUrlscanClient("").WithRateLimit(2 * time.Second)
	res, err := u.Search(ctx, `page.url:"https://docs.google.com/document/d/1hp-_YxStN-THXfFkWV08eGC3OrWVnXRNZWcBOJacyHk/edit*"`, "")
	if err != nil {
		t.Fatal(err)
	}
	for r := range res {
		if r.Error != nil {
			t.Errorf("TestEndToEnd: range channel: %v", r.Error)
		}
		reader, err := u.GetDom(ctx, r.Item.Id)
		if err != nil {
			t.Fatalf("TestEndToEnd: getting dom: %v", err)
		}
		defer reader.Close()
		g := gdoc.NewDoc("").WithProvenance("test!")
		if err := g.ParseHtml(reader); err != nil {
			t.Fatalf("TestEndtoEnd: parsing gdoc: %v", err)
		}
		b, err := json.MarshalIndent(g, "", " ")
		if err != nil {
			t.Fatalf("TestEndtoEnd: marshalling gdoc: %v", err)
		}
		fmt.Println(string(b))
	}
}
