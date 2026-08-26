package site

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Live tests hit bilinovel.com; run explicitly with RUN_LIVE=1 go test ./internal/site/.

func liveEnabled(t *testing.T) {
	t.Helper()
	if os.Getenv("RUN_LIVE") == "" {
		t.Skip("RUN_LIVE not set")
	}
}

func TestLiveSearch(t *testing.T) {
	liveEnabled(t)
	res, err := Search(context.Background(), "斗破苍穹")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if res.Total == 0 {
		t.Fatal("no results")
	}
	b := res.Books[0]
	if b.Title == "" {
		t.Fatal("empty title")
	}
	if SearchNovelID(b.URL) == "" {
		t.Fatalf("no novel id in URL: %s", b.URL)
	}
	t.Logf("first: %q by %q [%s] %s rating=%s %d total results",
		b.Title, b.Author, b.Status, b.URL, b.Rating, res.Total)
}

func TestLiveFetchAndShuffle(t *testing.T) {
	liveEnabled(t)
	dl, err := NewDownloader()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	meta, err := dl.FetchNovel(ctx, "1202")
	if err != nil {
		t.Fatalf("fetch novel: %v", err)
	}
	t.Logf("novel: %q by %q cover=%s", meta.Title, meta.Author, meta.CoverURL)

	volumes, err := dl.FetchCatalog(ctx, meta.NovelID)
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	total := 0
	for _, v := range volumes {
		total += len(v.Chapters)
	}
	if total == 0 {
		t.Fatal("empty catalog")
	}
	t.Logf("catalog: %d volumes, %d chapters; first volume %q", len(volumes), total, volumes[0].Title)

	// Resolve + fetch the first text chapter (skip leading front matter).
	idx := 0
	for i, c := range volumes[0].Chapters {
		idx = i
		if c.Title != "插图" {
			break
		}
	}
	ref := volumes[0].Chapters[idx]
	url := ref.Href
	if url == "" {
		url, err = dl.ResolveChapterURL(ctx, volumes, 0)
		if err != nil {
			t.Fatalf("resolve chapter: %v", err)
		}
	}
	ch, err := dl.FetchChapter(ctx, url)
	if err != nil {
		t.Fatalf("fetch chapter: %v", err)
	}
	if ch.Title == "" {
		t.Fatal("empty chapter title")
	}
	text := stripTags(ch.HTML)
	if len([]rune(text)) < 50 {
		t.Fatalf("chapter too short (%d runes): %q", len([]rune(text)), text[:min(80, len(text))])
	}
	t.Logf("chapter %q: %d runes, %d images, html %d bytes",
		ch.Title, len([]rune(text)), len(ch.ImageURLs), len(ch.HTML))
}

func stripTags(html string) string {
	// Crude tag strip for smoke assertions; not for production use.
	var sb strings.Builder
	inTag := false
	for _, r := range html {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			sb.WriteRune(r)
		}
	}
	return sb.String()
}
