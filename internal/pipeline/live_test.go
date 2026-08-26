package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lnreadertui/internal/epub"
	"lnreadertui/internal/site"
)

// Live test: RUN_LIVE=1 go test ./internal/pipeline/ -run TestLivePipeline.
func TestLivePipeline(t *testing.T) {
	if os.Getenv("RUN_LIVE") == "" {
		t.Skip("RUN_LIVE not set")
	}
	dl, err := site.NewDownloader()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	out := filepath.Join(os.TempDir(), "live-novel.epub")
	last := time.Now()
	var lastDone int
	bundle, err := FetchNovel(ctx, dl, "1202", "", func(p Progress) {
		if p.Done != lastDone || p.Total != lastDone {
			t.Logf("progress %d/%d %s", p.Done, p.Total, p.Title)
			lastDone = p.Done
		}
		if time.Since(last) > 20*time.Second {
			t.Logf("... still fetching (%d/%d %s)", p.Done, p.Total, p.Title)
			last = time.Now()
		}
	}, out)
	if err != nil {
		t.Fatalf("FetchNovel: %v", err)
	}
	if len(bundle.Chapters) == 0 {
		t.Fatal("no chapters in bundle")
	}

	book, err := epub.ReadFile(out)
	if err != nil {
		t.Fatalf("read produced epub: %v", err)
	}
	if len(book.Chapters) != len(bundle.Chapters) {
		t.Fatalf("epub chapters = %d, bundle = %d", len(book.Chapters), len(bundle.Chapters))
	}
	paras := book.Chapters[len(book.Chapters)/2].Paragraphs
	if len(paras) == 0 {
		t.Fatal("middle chapter is empty")
	}
	t.Logf("OK: %q by %q, %d chapters, epub %d bytes; middle chapter %q has %d paragraphs",
		book.Title, book.Author, len(book.Chapters), fileSize(out), book.Chapters[len(book.Chapters)/2].Title, len(paras))
}

func fileSize(p string) int64 {
	fi, err := os.Stat(p)
	if err != nil {
		return 0
	}
	return fi.Size()
}

// A focused assembly proof: one illustration chapter and one text chapter
// (images included) → EPUB → re-parse. Exercises image downloading without
// waiting for an entire novel.
func TestLiveAssembleSmall(t *testing.T) {
	if os.Getenv("RUN_LIVE") == "" {
		t.Skip("RUN_LIVE not set")
	}
	dl, err := site.NewDownloader()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	volumes, err := dl.FetchCatalog(ctx, "1202")
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, v := range volumes {
		total += len(v.Chapters)
	}

	// Pick the first 插图 chapter (illustrations) and one text chapter.
	var illus, text *site.ChapterRef
outer:
	for _, v := range volumes {
		for i := range v.Chapters {
			ch := &v.Chapters[i]
			if ch.Title == "插图" {
				illus = ch
			}
			if text == nil && ch.Title != "插图" && !strings.HasPrefix(ch.Title, "后记") {
				text = ch
			}
			if illus != nil && text != nil {
				break outer
			}
		}
	}
	if illus == nil || text == nil {
		t.Fatal("catalog has no illustration/text chapters")
	}

	var chapters []epub.ChapterRef
	var images []epub.Image
	var imgOrder []string
	imgSet := map[string]bool{}
	collect := func(ref *site.ChapterRef) {
		ch, err := dl.FetchChapter(ctx, ref.Href)
		if err != nil {
			t.Fatalf("fetch %q: %v", ref.Title, err)
		}
		chapters = append(chapters, epub.ChapterRef{Title: ch.Title, HTML: ch.HTML})
		for _, src := range ch.ImageURLs {
			if !imgSet[src] {
				imgSet[src] = true
				imgOrder = append(imgOrder, src)
			}
		}
	}
	collect(text)
	collect(illus)
	for _, src := range imgOrder {
		if err := ctx.Err(); err != nil {
			t.Fatal(err)
		}
		data, err := dl.DownloadImage(ctx, src)
		if err != nil {
			t.Fatalf("image %s: %v", src, err)
		}
		images = append(images, epub.Image{Src: src, Data: data})
	}
	if len(images) == 0 {
		t.Fatal("no images collected")
	}

	meta, err := dl.FetchNovel(ctx, "1202")
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(os.TempDir(), "live-assembled.epub")
	bundle := &epub.Bundle{
		ID: "bilinovel-" + meta.NovelID, Title: meta.Title, Author: meta.Author,
		Volumes:  []epub.VolumeRef{{Title: volumes[0].Title, Count: len(chapters)}},
		Chapters: chapters, Images: images,
	}
	if meta.CoverURL != "" {
		if data, err := dl.DownloadImage(ctx, meta.CoverURL); err == nil {
			bundle.Cover = data
		}
	}
	if err := epub.Write(out, *bundle); err != nil {
		t.Fatalf("write: %v", err)
	}
	book, err := epub.ReadFile(out)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if len(book.Chapters) != 2 {
		t.Fatalf("chapters = %d, want 2", len(book.Chapters))
	}
	if len(book.Chapters[1].Paragraphs) == 0 {
		t.Fatal("illustration chapter produced no content")
	}
	t.Logf("OK: %q by %q, %d images, epub %d bytes; chapter1 has %d paragraphs",
		book.Title, book.Author, len(images), fileSize(out), len(book.Chapters[1].Paragraphs))
}
