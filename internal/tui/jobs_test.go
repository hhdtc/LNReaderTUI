package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lnreadertui/internal/epub"
	"lnreadertui/internal/model"
)

// The Downloads tab's enter-import handoff: a finished job's epub lands in
// the library with parsed metadata and remains imported-only-once.
func TestJobsImportFlow(t *testing.T) {
	dir := t.TempDir()

	// Produce a real epub in the temp dir.
	epubPath := filepath.Join(dir, "downloaded.epub")
	if err := epub.Write(epubPath, epub.Bundle{
		ID: "bilinovel-999", Title: "苍穹女武神", Author: "橘公司",
		Volumes:  []epub.VolumeRef{{Title: "vol1", Count: 2}},
		Chapters: []epub.ChapterRef{{Title: "c1", HTML: "<p>a</p>"}, {Title: "c2", HTML: "<p>b</p>"}},
	}); err != nil {
		t.Fatal(err)
	}

	store, err := model.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	jv := newJobsView(store)
	j := &job{id: "job-1", novelID: "999", srcURL: "https://www.bilinovel.com/novel/999.html",
		title: "苍穹女武神", status: statusDone, outPath: epubPath, total: 2}
	jv.items = append(jv.items, j)

	// Finished event reflected the completed state (via channel/poll path).
	jv.apply(jobProgressMsg{id: "job-1", finished: true, bundlePath: epubPath})
	if j.status != statusDone || j.outPath != epubPath {
		t.Fatalf("job not marked done: status=%s", j.status)
	}

	// Enter on the finished job issues the import command.
	cmd := jv.importJob(j)
	if cmd == nil {
		t.Fatal("no import command")
	}
	msg := cmd()
	im := msg.(jobImportedMsg)
	if im.err != nil {
		t.Fatalf("import: %v", im.err)
	}
	jv.markImported(im.id)
	if !j.imported {
		t.Fatal("job not marked imported")
	}
	b := store.Find(im.book.ID)
	if b == nil {
		t.Fatal("book not registered")
	}
	if b.Title != "苍穹女武神" || b.Author != "橘公司" {
		t.Fatalf("metadata mismatch: %q by %q", b.Title, b.Author)
	}
	if b.SourceID != "999" || b.SourceURL != "https://www.bilinovel.com/novel/999.html" {
		t.Fatalf("source mismatch: %+v", b)
	}
	if b.Chapters != 2 {
		t.Fatalf("chapters = %d, want 2", b.Chapters)
	}
	if _, err := os.Stat(store.FilePath(b)); err != nil {
		t.Fatalf("book file missing: %v", err)
	}
}

// Progress events for an unknown job must not disturb the list.
func TestJobsApplyUnknownJob(t *testing.T) {
	dir := t.TempDir()
	store, err := model.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	jv := newJobsView(store)
	jv.apply(jobProgressMsg{id: "nope", done: 1})
	if len(jv.items) != 0 {
		t.Fatal("unknown job created an item")
	}
}

// Job events must keep flowing once the poll misses an empty channel: the
// poll re-arms on noneMsg (previously it stopped after the first miss, so a
// job froze at "fetching novel page…" forever).
func TestJobEventsKeepPolling(t *testing.T) {
	a := newTestApp(t)
	a.jobs.items = append(a.jobs.items, &job{id: "job-1", title: "月之珊瑚", status: statusRunning})
	a.jobs.refresh()

	// Poll cycle 1: channel empty at poll time -> noneMsg -> re-arm.
	m, _ := a.Update(noneMsg{})
	a = m.(*App)
	_, cmd := a.Update(noneMsg{})
	if cmd == nil {
		t.Fatal("poll not re-armed after idle cycle")
	}

	// Event 1 arrives after the cycle; the re-armed poll must deliver it.
	a.jobs.events <- jobProgressMsg{id: "job-1", done: 5, total: 10}
	mm, _ := a.Update(cmd())
	a = mm.(*App)
	if j := a.jobs.find("job-1"); j.done != 5 {
		t.Fatalf("progress not delivered: %+v", j)
	}

	// Event 2 (completion) arrives after another noneMsg round.
	_, cmd2 := a.Update(noneMsg{})
	if cmd2 == nil {
		t.Fatal("poll not re-armed after progress event")
	}
	a.jobs.events <- jobProgressMsg{id: "job-1", finished: true}
	mm2, _ := a.Update(cmd2())
	a = mm2.(*App)
	if j := a.jobs.find("job-1"); j.status != statusDone {
		t.Fatalf("completion not delivered: %+v", j)
	}
}

// A finished download auto-imports: apply returns the import command and the
// book lands in the library with no manual Enter.
func TestJobsAutoImportOnFinish(t *testing.T) {
	dir := t.TempDir()
	epubPath := filepath.Join(dir, "auto.epub")
	if err := epub.Write(epubPath, epub.Bundle{
		ID: "bilinovel-777", Title: "自动导入", Author: "作者",
		Volumes:  []epub.VolumeRef{{Title: "v", Count: 1}},
		Chapters: []epub.ChapterRef{{Title: "c1", HTML: "<p>x</p>"}},
	}); err != nil {
		t.Fatal(err)
	}
	store, err := model.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	jv := newJobsView(store)
	j := &job{id: "job-9", novelID: "777", srcURL: "u", title: "自动导入",
		status: statusRunning, outPath: epubPath, total: 1}
	jv.items = append(jv.items, j)

	// Completion triggers the auto-import command.
	cmd := jv.apply(jobProgressMsg{id: "job-9", finished: true, bundlePath: epubPath})
	if cmd == nil {
		t.Fatal("no auto-import command on completion")
	}
	msg := cmd().(jobImportedMsg)
	if msg.err != nil {
		t.Fatalf("auto import: %v", msg.err)
	}
	jv.markImported(msg.id)
	if b := store.Find(msg.book.ID); b == nil || b.Title != "自动导入" {
		t.Fatalf("book not in library: %+v", b)
	}
	if !j.imported {
		t.Fatal("job not marked imported")
	}
}

// The running job description shows the current volume alongside the chapter.
func TestJobDescriptionShowsVolume(t *testing.T) {
	j := &job{status: statusRunning, done: 5, total: 10,
		cur: "第一章", volume: "第一卷 风云再起"}
	desc := (jobItem{j: j}).Description()
	if !strings.Contains(desc, "第一卷 风云再起 · 第一章") {
		t.Fatalf("volume not in description: %q", desc)
	}
	if !strings.Contains(desc, "5/10") {
		t.Fatalf("progress missing in description: %q", desc)
	}
}
