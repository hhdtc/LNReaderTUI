package epub

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteReadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "novel.epub")
	bundle := Bundle{
		ID:     "bilinovel-12345",
		Title:  "测试小说",
		Author: "作者甲",
		Cover:  []byte{0xff, 0xd8, 0xff, 0xe0, 0x00}, // fake jpeg header
		Volumes: []VolumeRef{
			{Title: "第一卷", Count: 2},
			{Title: "第二章卷", Count: 1},
		},
		Chapters: []ChapterRef{
			{Title: "第一章 开始", HTML: `<p>第一段文字。</p><p><img src="https://www.bilinovel.com/files/a.jpg" alt=""/></p>`},
			{Title: "第二章 继续", HTML: `<p>第二段文字。</p>`},
			{Title: "第三章 结束", HTML: `<p>第三段文字。</p>`},
		},
		Images: []Image{
			{Src: "https://www.bilinovel.com/files/a.jpg", Data: []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a}},
		},
	}
	if err := Write(out, bundle); err != nil {
		t.Fatalf("write: %v", err)
	}

	// The produced file must be a valid zip with the stored mimetype first.
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 4 || string(data[:4]) != "PK\x03\x04" {
		t.Fatal("not a zip file")
	}

	book, err := ReadFile(out)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if book.Title != "测试小说" {
		t.Fatalf("title = %q", book.Title)
	}
	if book.Author != "作者甲" {
		t.Fatalf("author = %q", book.Author)
	}
	if len(book.Chapters) != 3 {
		t.Fatalf("chapters = %d, want 3", len(book.Chapters))
	}
	if book.Chapters[0].Title != "第一章 开始" {
		t.Fatalf("chapter0 title = %q", book.Chapters[0].Title)
	}
	joined := strings.Join(book.Chapters[0].Paragraphs, "\n")
	if !strings.Contains(joined, "第一段文字。") {
		t.Fatalf("missing paragraph text: %q", joined)
	}
	if !strings.Contains(joined, "[图]") {
		t.Fatalf("image placeholder missing: %q", joined)
	}
	if len(book.Chapters[0].Paragraphs) != 2 {
		t.Fatalf("paragraphs = %d, want 2", len(book.Chapters[0].Paragraphs))
	}
}

func TestReadTXTChunking(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "book.txt")
	content := strings.Repeat("第一章 开始\n这是正文。\n\n", 3) +
		"第二章 继续\n更多正文。\n\n" +
		"第三章 结束\n结尾。\n"
	if err := os.WriteFile(src, []byte(content+"\n第一章"), 0o644); err != nil {
		t.Fatal(err)
	}
	book, err := ReadFile(src)
	if err != nil {
		t.Fatalf("read txt: %v", err)
	}
	if len(book.Chapters) < 2 {
		t.Fatalf("chapters = %d, want >= 2", len(book.Chapters))
	}
	if !strings.Contains(book.Chapters[0].Paragraphs[0], "第一章 开始") {
		t.Fatalf("chapter title not detected: %q", book.Chapters[0].Title)
	}
}

// TestReadFixtureEPUB parses the LNReader test corpus when available.
func TestReadFixtureEPUB(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping fixture test in short mode")
	}
	for _, rel := range []string{
		"../../../LNReader/testepub/aozorabunko_43737.epub",
		"../../../LNReader/testepub/Minna no Nihongo Shokyu I Dai 2-Han Honsatsu Kanji-Kana ( PDFDrive ).epub",
	} {
		book, err := ReadFile(rel)
		if err != nil {
			t.Logf("%s: %v (skipping)", rel, err)
			continue
		}
		if len(book.Chapters) == 0 {
			t.Errorf("%s: no chapters parsed", rel)
		}
		paras := book.Chapters[0].Paragraphs
		if len(paras) == 0 {
			t.Errorf("%s: first chapter empty", rel)
		}
		t.Logf("%s: %q by %q, %d chapters, first para %d runes",
			rel, book.Title, book.Author, len(book.Chapters), len([]rune(paras[0])))
	}
}
