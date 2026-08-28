package model

import (
	"path/filepath"
	"testing"

	"lnreadertui/internal/epub"
)

// A TUI download registers with an empty title (the job only learns the
// title later); the file name must fall back to the parsed EPUB title.
func TestRegisterDownloadedEmptyTitleUsesParsedTitle(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "staging.epub")
	if err := epub.Write(src, epub.Bundle{
		ID: "bilinovel-764", Title: "异世界の书", Author: "作家",
		Volumes:  []epub.VolumeRef{{Title: "v1", Count: 1}},
		Chapters: []epub.ChapterRef{{Title: "一章", HTML: "<p>内容</p>"}},
	}); err != nil {
		t.Fatal(err)
	}
	b, err := s.RegisterDownloaded(src, "", "", "764", "https://www.bilinovel.com/novel/764", 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := filepath.Base(b.File); got != "异世界の书-764.epub" {
		t.Fatalf("file = %q, want 异世界の书-764.epub", got)
	}
	// Parsed metadata also lands in the index entry.
	if b.Title != "异世界の书" {
		t.Fatalf("title = %q", b.Title)
	}
	if b.Author != "作家" {
		t.Fatalf("author = %q", b.Author)
	}
	if b.Chapters != 1 {
		t.Fatalf("chapters = %d", b.Chapters)
	}
}
