package main

import (
	"context"
	"testing"

	"lnreadertui/internal/model"
	"lnreadertui/internal/site"
)

func TestFindBookEmptyLibrary(t *testing.T) {
	store, err := model.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := findBook(store, "nope"); err == nil {
		t.Fatal("empty library matched a book")
	}
}

func TestIsDigits(t *testing.T) {
	for _, c := range []struct {
		in   string
		want bool
	}{{"764", true}, {"1202.html", false}, {"", false}, {"a1", false}} {
		if got := isDigits(c.in); got != c.want {
			t.Fatalf("isDigits(%q)=%v want %v", c.in, got, c.want)
		}
	}
}

func TestResolveNovelURLForms(t *testing.T) {
	// URL and id forms resolve without any network.
	b, err := resolveNovel(context.Background(), "https://www.bilinovel.com/novel/764.html")
	if err != nil {
		t.Fatal(err)
	}
	if site.SearchNovelID(b.URL) != "764" {
		t.Fatalf("url resolve: %+v", b)
	}
	b, err = resolveNovel(context.Background(), "764")
	if err != nil || site.SearchNovelID(b.URL) != "764" {
		t.Fatalf("id resolve: %v %+v", err, b)
	}
}
