package model

import (
	"os"
	"path/filepath"
	"testing"
)

// writeStubEPUB writes a file that fails epub parsing on purpose: the
// registration paths must still work, falling back to call arguments.
func writeStubEPUB(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("PK\x03\x04not-a-real-zip"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRegisterDownloadedNaming covers <title>-<novelID> names (sanitized).
func TestRegisterDownloadedNaming(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "staging.epub")
	writeStubEPUB(t, src)
	b, err := s.RegisterDownloaded(src, "测试/书名", "作者", "9876", "https://x/novel/9876", 300)
	if err != nil {
		t.Fatal(err)
	}
	if got := filepath.Base(b.File); got != "测试_书名-9876.epub" {
		t.Fatalf("file = %q, want 测试_书名-9876.epub", got)
	}
	if _, err := os.Stat(filepath.Join(dir, b.File)); err != nil {
		t.Fatalf("registered file missing: %v", err)
	}
}

// TestRegisterDownloadedReplace verifies re-download of the same novel
// replaces the entry in place and preserves id/progress.
func TestRegisterDownloadedReplace(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "a.epub")
	writeStubEPUB(t, src)
	first, err := s.RegisterDownloaded(src, "书A", "作者", "1234", "u1", 100)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetProgress(first.ID, 10, 0.5, "第十章"); err != nil {
		t.Fatal(err)
	}

	oldFile := first.File
	second, err := s.RegisterDownloaded(src, "书A", "作者", "1234", "u2", 120)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Fatalf("id changed: %s != %s", second.ID, first.ID)
	}
	if second.Progress.ChapterIndex != 10 || second.Progress.Offset != 0.5 {
		t.Fatalf("progress lost: %+v", second.Progress)
	}
	if second.Chapters != 120 {
		t.Fatalf("chapters = %d, want 120", second.Chapters)
	}
	if second.SourceURL != "u2" {
		t.Fatalf("sourceUrl = %q", second.SourceURL)
	}
	// Deterministic naming: the re-download overwrites the same path.
	if oldFile != second.File {
		t.Fatalf("file changed on re-download: %s != %s", oldFile, second.File)
	}
	if _, err := os.Stat(filepath.Join(dir, second.File)); err != nil {
		t.Fatalf("registered file missing: %v", err)
	}
	if len(s.Books()) != 1 {
		t.Fatalf("books = %d, want 1", len(s.Books()))
	}
}

// TestRegisterDownloadedDifferentNovels verifies two different novels with
// the same title stay separate.
func TestRegisterDownloadedDifferentNovels(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "a.epub")
	writeStubEPUB(t, src)
	if _, err := s.RegisterDownloaded(src, "同名", "作者", "1", "u", 10); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RegisterDownloaded(src, "同名", "作者", "2", "u", 20); err != nil {
		t.Fatal(err)
	}
	if got := s.FindBySource("1"); got == nil {
		t.Fatal("source 1 not found")
	}
	if got := s.FindBySource("2"); got == nil {
		t.Fatal("source 2 not found")
	}
	if len(s.Books()) != 2 {
		t.Fatalf("books = %d, want 2", len(s.Books()))
	}
}

// TestUpdateBookIndex verifies chapter count refresh.
func TestUpdateBookIndex(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "a.epub")
	writeStubEPUB(t, src)
	b, err := s.RegisterDownloaded(src, "书B", "作者", "55", "u", 10)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateBookIndex(b.ID, 42, "", ""); err != nil {
		t.Fatal(err)
	}
	if got := s.Find(b.ID).Chapters; got != 42 {
		t.Fatalf("chapters = %d, want 42", got)
	}
	if err := s.UpdateBookIndex("nope", 1, "", ""); err == nil {
		t.Fatal("expected error for unknown id")
	}
}

// TestSafeFileName sanity.
func TestSafeFileName(t *testing.T) {
	cases := map[string]string{
		"正常的书名":                 "正常的书名",
		"a/b\\c:d*e?f\"g<h>i|j": "a_b_c_d_e_f_g_h_i_j",
		"  lead+trail  ":        "lead+trail",
		"":                      "",
	}
	for in, want := range cases {
		if got := safeFileName(in); got != want {
			t.Errorf("safeFileName(%q) = %q, want %q", in, got, want)
		}
	}
}
