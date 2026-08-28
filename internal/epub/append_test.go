package epub

import (
	"archive/zip"
	"path/filepath"
	"strings"
	"testing"
)

// TestAppendRoundTrip writes a book, appends a new chapter + image and
// verifies old content (including covers and chapter images) survives.
func TestAppendRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "novel.epub")

	img := []byte("fake-image-bytes-old")
	cover := []byte("fake-cover-bytes")
	orig := Bundle{
		ID: "bilinovel-123", Title: "旧书", Author: "作者",
		Volumes: []VolumeRef{{Title: "v1", Count: 3}},
		Chapters: []ChapterRef{
			{Title: "第一章", HTML: "<p>旧段落一。</p>"},
			{Title: "第二章", HTML: "<p>旧段落二。</p><img src=\"https://img.example.com/pic.png\"/>"},
			{Title: "第三章", HTML: "<p>旧段落三。</p>"},
		},
		Images: []Image{{Src: "https://img.example.com/pic.png", Data: img}},
		Cover:  cover,
	}
	if err := Write(path, orig); err != nil {
		t.Fatalf("write: %v", err)
	}

	old, err := ReadBundle(path)
	if err != nil {
		t.Fatalf("read bundle: %v", err)
	}
	if len(old.Chapters) != 3 {
		t.Fatalf("chapters = %d, want 3", len(old.Chapters))
	}
	if old.Title != "旧书" || old.Author != "作者" {
		t.Fatalf("meta = %q/%q", old.Title, old.Author)
	}
	if string(old.Cover) != string(cover) {
		t.Fatal("cover bytes lost")
	}
	if len(old.Images) != 1 || string(old.Images[0].Data) != string(img) {
		t.Fatal("image bytes lost")
	}

	newImg := []byte("fake-image-bytes-new")
	if err := Append(path, old, Bundle{
		ID: "bilinovel-123", Title: "旧书", Author: "作者",
		Volumes:  []VolumeRef{{Title: "v2", Count: 1}},
		Chapters: []ChapterRef{{Title: "第四章", HTML: "<p>新段落四。</p><img src=\"https://img.example.com/new.png\"/>"}},
		Images:   []Image{{Src: "https://img.example.com/new.png", Data: newImg}},
	}, []VolumeRef{{Title: "v1", Count: 3}, {Title: "v2", Count: 1}}); err != nil {
		t.Fatalf("append: %v", err)
	}

	book, err := ReadFile(path)
	if err != nil {
		t.Fatalf("read after append: %v", err)
	}
	if len(book.Chapters) != 4 {
		t.Fatalf("chapters = %d, want 4", len(book.Chapters))
	}
	if book.Chapters[0].Title != "第一章" {
		t.Fatalf("chapter0 title changed: %q", book.Chapters[0].Title)
	}
	if book.Chapters[3].Title != "第四章" {
		t.Fatalf("chapter3 title = %q", book.Chapters[3].Title)
	}
	joined0 := strings.Join(book.Chapters[0].Paragraphs, "\n")
	if !strings.Contains(joined0, "旧段落一。") {
		t.Fatal("old chapter 1 content lost")
	}
	joined2 := strings.Join(book.Chapters[1].Paragraphs, "\n")
	if !strings.Contains(joined2, "旧段落二。") {
		t.Fatal("old chapter 2 content lost")
	}
	if !strings.Contains(joined2, "[图]") {
		t.Fatal("old chapter image marker lost")
	}
	joined3 := strings.Join(book.Chapters[3].Paragraphs, "\n")
	if !strings.Contains(joined3, "新段落四。") {
		t.Fatal("new chapter content missing")
	}
	if !strings.Contains(joined3, "[图]") {
		t.Fatal("new chapter image marker missing")
	}

	// Cover and old image entries survive byte-for-byte.
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	found := map[string][]byte{}
	for _, f := range zr.File {
		if f.Name == "OEBPS/images/cover.jpg" || strings.HasPrefix(f.Name, "OEBPS/images/img") {
			rc, _ := f.Open()
			defer rc.Close()
			b := make([]byte, f.UncompressedSize64)
			rc.Read(b)
			found[f.Name] = b
		}
	}
	if string(found["OEBPS/images/cover.jpg"]) != string(cover) {
		t.Fatal("cover entry changed")
	}
	var oldImgFound, newImgFound bool
	for name, b := range found {
		if string(b) == string(img) {
			oldImgFound = true
		}
		if string(b) == string(newImg) {
			newImgFound = true
		}
		if strings.HasPrefix(name, "OEBPS/images/img") && name != "OEBPS/images/img00001.jpg" &&
			!strings.Contains(string(b), "new") && !strings.Contains(string(b), "old") {
			t.Logf("unexpected image entry %s (%d bytes)", name, len(b))
		}
	}
	if !oldImgFound || !newImgFound {
		t.Fatalf("image bytes missing (old=%v new=%v)", oldImgFound, newImgFound)
	}
}

// TestAppendUpdateSameFileTwice verifies two successive appends accumulate.
func TestAppendUpdateSameFileTwice(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "novel.epub")
	orig := Bundle{ID: "id", Title: "T", Author: "A",
		Volumes:  []VolumeRef{{Title: "v1", Count: 1}},
		Chapters: []ChapterRef{{Title: "一", HTML: "<p>1</p>"}}}
	if err := Write(path, orig); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		old, err := ReadBundle(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := Append(path, old, Bundle{
			ID: "id", Title: "T", Author: "A",
			Volumes:  []VolumeRef{{Title: "v2", Count: 1}},
			Chapters: []ChapterRef{{Title: "加一", HTML: "<p>n</p>"}},
		}, []VolumeRef{{Title: "v1", Count: 1}, {Title: "v2", Count: i + 1}}); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	book, err := ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(book.Chapters) != 3 {
		t.Fatalf("chapters = %d, want 3", len(book.Chapters))
	}
}
