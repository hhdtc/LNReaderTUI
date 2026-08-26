package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"lnreadertui/internal/epub"
	"lnreadertui/internal/model"
)

// testBook builds a store with one real imported EPUB of n chapters and
// returns the parsed book for the reader.
func testBook(t *testing.T, n int) (*model.Store, *model.Book, *epub.Book) {
	t.Helper()
	dir := t.TempDir()
	store, err := model.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Long paragraphs so each chapter occupies several viewport pages.
	longPara := func(k int) string {
		var sb strings.Builder
		for i := 0; i < 60; i++ {
			sb.WriteString("これはペーストスト用の長い本文段落です。")
		}
		_ = k
		return sb.String()
	}
	parsed := &epub.Book{Title: "测试书", Author: "作者"}
	for i := 0; i < n; i++ {
		paras := make([]string, 0, 30)
		for k := 0; k < 30; k++ {
			paras = append(paras, longPara(k))
		}
		parsed.Chapters = append(parsed.Chapters, epub.Chapter{
			Title: "第X章", Paragraphs: paras,
		})
	}
	src := filepath.Join(dir, "src.epub")
	bundle := epub.Bundle{
		ID: "test-id", Title: "测试书", Author: "作者",
		Volumes:  []epub.VolumeRef{{Title: "v", Count: n}},
		Chapters: make([]epub.ChapterRef, 0, n),
	}
	for i := 0; i < n; i++ {
		bundle.Chapters = append(bundle.Chapters, epub.ChapterRef{
			Title: "第X章",
			HTML:  "<p>" + longPara(i) + "</p><p>" + longPara(i) + "</p>"})
	}
	if err := epub.Write(src, bundle); err != nil {
		t.Fatal(err)
	}
	books, err := store.ImportFile(src)
	if err != nil {
		t.Fatal(err)
	}
	return store, books[0], parsed
}

// Left/right arrows flip pages within the chapter; at the chapter edge they
// roll over to the previous/next chapter.
func TestReaderArrowPageNav(t *testing.T) {
	store, b, parsed := testBook(t, 3)
	v := newReaderView(b, parsed, store)
	v.resize(120, 40)

	v.openChapter(0, 0)

	// Right pages down within the chapter (chapter unchanged).
	start := v.vp.YOffset
	v, _ = v.Update(keyArrow(tea.KeyRight))
	if v.chapter != 0 {
		t.Fatalf("right mid-chapter changed chapter: %d", v.chapter)
	}
	if v.vp.YOffset <= start {
		t.Fatalf("right did not page down: offset %d -> %d", start, v.vp.YOffset)
	}

	// Left pages back up.
	mid := v.vp.YOffset
	v, _ = v.Update(keyArrow(tea.KeyLeft))
	if v.chapter != 0 || v.vp.YOffset >= mid {
		t.Fatalf("left did not page up: chapter %d offset %d (was %d)",
			v.chapter, v.vp.YOffset, mid)
	}

	// At the very bottom, right rolls to the next chapter (top).
	v.vp.GotoBottom()
	v, cmd := v.Update(keyArrow(tea.KeyRight))
	if v.chapter != 1 {
		t.Fatalf("right at bottom: chapter %d, want 1", v.chapter)
	}
	if v.vp.YOffset != 0 {
		t.Fatalf("next chapter should start at top, offset %d", v.vp.YOffset)
	}
	if cmd == nil {
		t.Fatal("no save command on chapter roll")
	}
	_ = cmd()
	if p := store.Find(b.ID).Progress; p.ChapterIndex != 1 {
		t.Fatalf("progress not saved: %+v", p)
	}

	// At the very top, left rolls to the previous chapter (at its bottom).
	v.vp.GotoTop()
	v, _ = v.Update(keyArrow(tea.KeyLeft))
	if v.chapter != 0 {
		t.Fatalf("left at top: chapter %d, want 0", v.chapter)
	}
	if v.vp.YOffset == 0 {
		t.Fatal("previous chapter should open at its last page")
	}

	// Left at the first chapter's top clamps.
	v, _ = v.Update(keyArrow(tea.KeyLeft))
	if v.chapter != 0 {
		t.Fatalf("left at first chapter top: chapter %d", v.chapter)
	}
}

// 's' toggles the chapter sidebar; enter jumps to the selected chapter.
func TestReaderSidebarToggleAndJump(t *testing.T) {
	store, b, parsed := testBook(t, 4)
	v := newReaderView(b, parsed, store)
	v.resize(120, 40)

	v, _ = v.Update(keyRunes("s"))
	if !v.showSide {
		t.Fatal("s did not open sidebar")
	}
	// The sidebar list owns navigation now.
	v, _ = v.Update(keyRunes("j"))
	if v.chapter != 0 {
		t.Fatalf("j should not change chapter directly: %d", v.chapter)
	}
	// Select chapter 2 and jump.
	v.chList.Select(2)
	_, cmd := v.Update(keyArrow(tea.KeyEnter))
	if v.chapter != 2 {
		t.Fatalf("jump: chapter %d, want 2", v.chapter)
	}
	_ = cmd
	// Esc closes the sidebar (and only that).
	v, _ = v.Update(keyArrow(tea.KeyEsc))
	if v.showSide {
		t.Fatal("esc did not close sidebar")
	}
}

// Progress is auto-saved: chapter switches persist without any manual save
// key.
func TestReaderAutoSave(t *testing.T) {
	store, b, parsed := testBook(t, 3)
	v := newReaderView(b, parsed, store)
	v.resize(120, 40)

	// openChapter persists via its save command (executed by the tea loop).
	openCmd := v.openChapter(0, 0)
	_ = openCmd()
	p := store.Find(b.ID).Progress
	if p.ChapterIndex != 0 || p.LastReadAt == "" {
		t.Fatalf("progress not persisted: %+v", p)
	}
	// Switching chapters persists immediately (no manual save involved).
	_, cmd := v.Update(keyRunes("n"))
	_ = cmd()
	if p := store.Find(b.ID).Progress; p.ChapterIndex != 1 {
		t.Fatalf("auto-save on chapter switch missing: %+v", p)
	}
}

// The rendered reader includes the chapter sidebar when toggled.
func TestReaderViewRendersSidebar(t *testing.T) {
	store, b, parsed := testBook(t, 3)
	v := newReaderView(b, parsed, store)
	v.resize(120, 40)
	v.openChapter(0, 0)
	v.showSide = true
	v.resize(120, 40)
	if !strings.Contains(v.View(), "Chapters") {
		t.Fatalf("sidebar title missing:\n%s", v.View())
	}
}

// Scroll position must persist: page down a few times, save (as the tick or
// exit does), and a fresh reader session restores the same offset.
func TestReaderAutoSaveOffset(t *testing.T) {
	store, b, parsed := testBook(t, 3)
	v := newReaderView(b, parsed, store)
	v.resize(120, 40)
	_ = v.openChapter(0, 0)

	// Page down twice via arrows.
	v, _ = v.Update(keyArrow(tea.KeyRight))
	v, _ = v.Update(keyArrow(tea.KeyRight))
	if v.vp.YOffset == 0 {
		t.Fatal("no scroll happened")
	}
	savedOffset := v.vp.YOffset

	// The save that runs on quit/tick persists the offset.
	if err := v.save(); err != nil {
		t.Fatal(err)
	}
	p := store.Find(b.ID).Progress
	if p.ChapterIndex != 0 || p.Offset <= 0 {
		t.Fatalf("offset not persisted: %+v", p)
	}

	// Re-enter the book: a NEW reader view restores from the store.
	v2 := newReaderView(b, parsed, store)
	v2.resize(120, 40)
	_ = v2.openChapter(p.ChapterIndex, p.Offset)
	if v2.vp.YOffset != savedOffset {
		t.Fatalf("offset not restored: got %d, want %d", v2.vp.YOffset, savedOffset)
	}
}

// save() must be a no-op when the position has not moved.
func TestReaderSaveIdempotentForUnmoved(t *testing.T) {
	store, b, parsed := testBook(t, 2)
	v := newReaderView(b, parsed, store)
	v.resize(120, 40)
	_ = v.openChapter(0, 0)
	if err := v.save(); err != nil {
		t.Fatal(err)
	}
	first := store.Find(b.ID).Progress
	if err := v.save(); err != nil {
		t.Fatal(err)
	}
	second := store.Find(b.ID).Progress
	if first.LastReadAt != second.LastReadAt {
		t.Fatal("unmoved save wrote again")
	}
}

// Saves record the current chapter title for the library display.
func TestProgressCarriesChapterTitle(t *testing.T) {
	store, b, parsed := testBook(t, 3)
	v := newReaderView(b, parsed, store)
	v.resize(120, 40)
	openCmd := v.openChapter(0, 0)
	_ = openCmd() // persists chapter + title
	p := store.Find(b.ID).Progress
	if p.ChapterTitle != "第X章" {
		t.Fatalf("chapter title not recorded: %+v", p)
	}
}

// h and l perform the same page flips as the arrows.
func TestReaderHLPageKeys(t *testing.T) {
	store, b, parsed := testBook(t, 3)
	v := newReaderView(b, parsed, store)
	v.resize(120, 40)
	_ = v.openChapter(0, 0)

	start := v.vp.YOffset
	v, _ = v.Update(keyRunes("l")) // page down within chapter
	if v.chapter != 0 || v.vp.YOffset <= start {
		t.Fatalf("l did not page: chapter %d offset %d (was %d)", v.chapter, v.vp.YOffset, start)
	}
	v, _ = v.Update(keyRunes("h")) // page back up
	if v.vp.YOffset != start {
		t.Fatalf("h did not page back: offset %d", v.vp.YOffset)
	}
	// At bottom, l rolls to the next chapter like right.
	v.vp.GotoBottom()
	v, _ = v.Update(keyRunes("l"))
	if v.chapter != 1 {
		t.Fatalf("l at bottom: chapter %d, want 1", v.chapter)
	}
}
