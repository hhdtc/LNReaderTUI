package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/cursor"
	tea "github.com/charmbracelet/bubbletea"

	"lnreadertui/internal/model"
	"lnreadertui/internal/site"
)

// fakeSearch is an injectable searcher; blocked emulates an in-flight call.
type fakeSearch struct {
	results map[string]*site.Result
	errs    map[string]error
	queries []string
	blocked bool
}

func (f *fakeSearch) call(ctx context.Context, q string) (*site.Result, error) {
	f.queries = append(f.queries, q)
	if f.blocked {
		<-ctx.Done()
		return nil, ctx.Err() // cancel signals a newer query took over
	}
	if e, ok := f.errs[q]; ok {
		return nil, e
	}
	return f.results[q], nil
}

func searchViewTestApp(t *testing.T, f *fakeSearch) *App {
	t.Helper()
	store, err := model.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := New(store, nil, t.TempDir())
	m, _ := a.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	a = m.(*App)
	m, _ = a.Update(keyArrow(tea.KeyRight)) // Search tab
	a = m.(*App)
	a.search.searchFn = f.call
	return a
}

func oneResult(title string) *site.Result {
	return &site.Result{Total: 1, Books: []site.Book{{Title: title, URL: "https://www.bilinovel.com/novel/1.html"}}}
}

// drain feeds a message and then executes any returned commands
// synchronously (as the tea program would), re-feeding results until nothing
// remains pending. Nested BatchMsgs are unwrapped recursively.
func drain(a *App, msg tea.Msg) *App {
	pending := []tea.Msg{msg}
	for len(pending) > 0 {
		m := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		m2, cmd := a.Update(m)
		a = m2.(*App)
		for cmd != nil {
			switch r := cmd().(type) {
			case tea.BatchMsg:
				var flatten func(c tea.Cmd)
				flatten = func(c tea.Cmd) {
					sub := c()
					if b2, ok := sub.(tea.BatchMsg); ok {
						for _, c2 := range b2 {
							flatten(c2)
						}
						return
					}
					if sub != nil {
						pending = append(pending, sub)
					}
				}
				for _, c := range r {
					flatten(c)
				}
				cmd = nil
			case nil:
				cmd = nil
			case cursor.BlinkMsg:
				// The textinput cursor blink re-schedules itself forever; a
				// real program absorbs it, so just let the input consume it.
				m2b, _ := a.Update(r)
				a = m2b.(*App)
				cmd = nil
			default:
				m2b, c2 := a.Update(r)
				a = m2b.(*App)
				cmd = c2
			}
		}
	}
	return a
}

func fireDebounce(t *testing.T, a *App) *App {
	t.Helper()
	return drain(a, typeaheadTickMsg{})
}

// Typing schedules a debounced live search; the tick fires only when the
// input still holds the scheduled query.
func TestTypeaheadFiresDebounce(t *testing.T) {
	f := &fakeSearch{results: map[string]*site.Result{"苍穹": oneResult("苍穹女武神")}}
	a := searchViewTestApp(t, f)

	a = drain(a, keyRunes("苍"))
	a = drain(a, keyRunes("穹"))

	a = fireDebounce(t, a) // tick: "苍穹" is pending and unchanged
	if len(f.queries) != 1 || f.queries[0] != "苍穹" {
		t.Fatalf("live search not fired once for 苍穹: %v", f.queries)
	}
	if a.search.state != searchResults {
		t.Fatalf("state = %d, want results", a.search.state)
	}
	if len(a.search.list.Items()) != 1 {
		t.Fatalf("results not applied: %d items", len(a.search.list.Items()))
	}
}

// Rapid keystrokes coalesce: every keystroke supersedes the previous tick,
// so only the final value is searched once.
func TestTypeaheadCoalesces(t *testing.T) {
	f := &fakeSearch{results: map[string]*site.Result{}}
	a := searchViewTestApp(t, f)

	a = drain(a, keyRunes("苍"))
	a = drain(a, keyRunes("龙"))
	a = fireDebounce(t, a)
	if len(f.queries) != 1 || f.queries[0] != "苍龙" {
		t.Fatalf("coalesced search = %v, want one for 苍龙", f.queries)
	}
}

// One-char queries are not live-searched, but Enter still runs them.
func TestTypeaheadMinimumLength(t *testing.T) {
	f := &fakeSearch{results: map[string]*site.Result{}}
	a := searchViewTestApp(t, f)

	a = drain(a, keyRunes("苍"))
	a = fireDebounce(t, a)
	if len(f.queries) != 0 {
		t.Fatalf("1-rune query live-searched: %v", f.queries)
	}

	a = drain(a, keyArrow(tea.KeyEnter))
	if len(f.queries) != 1 || f.queries[0] != "苍" {
		t.Fatalf("enter did not search 苍: %v", f.queries)
	}
}

// Results from a superseded query are ignored; only the current generation
// is applied.
func TestTypeaheadLatestWins(t *testing.T) {
	f := &fakeSearch{results: map[string]*site.Result{"苍穹": oneResult("苍穹女武神")}}
	a := searchViewTestApp(t, f)

	a = drain(a, keyRunes("苍"))
	a = drain(a, keyRunes("穹"))
	a = fireDebounce(t, a) // gen 1 applied
	if a.search.state != searchResults {
		t.Fatalf("state after first result = %d, want results", a.search.state)
	}
	firstGen := a.search.seq

	// A stale result (older generation) must not replace the list.
	a = drain(a, searchResultMsg{gen: firstGen - 1, q: "苍穹", res: oneResult("旧")})
	if len(a.search.list.Items()) != 1 {
		t.Fatalf("stale result replaced list: %d items", len(a.search.list.Items()))
	}
	if it := a.search.list.Items()[0].(searchItem); it.book.Title != "苍穹女武神" {
		t.Fatalf("stale result applied: %q", it.book.Title)
	}
	// The current generation applies (newer search).
	a = drain(a, searchResultMsg{gen: a.search.seq, q: "苍穹", res: oneResult("新")})
	it := a.search.list.Items()[0].(searchItem)
	if it.book.Title != "新" {
		t.Fatalf("current result not applied: %q", it.book.Title)
	}
}

// Enter blurs the input (so Enter on the list downloads) and searches
// immediately.
func TestEnterIsImmediateKeepsTyping(t *testing.T) {
	f := &fakeSearch{results: map[string]*site.Result{"苍穹女武神": oneResult("苍穹女武神")}}
	a := searchViewTestApp(t, f)

	a = drain(a, keyRunes("苍穹女武神"))
	a = drain(a, keyArrow(tea.KeyEnter))
	if len(f.queries) == 0 || f.queries[len(f.queries)-1] != "苍穹女武神" {
		t.Fatalf("enter did not search the full query: %v", f.queries)
	}
	if !a.search.input.Focused() {
		t.Fatal("input must stay focused after enter so typing can continue")
	}
	// Typing must go right back into the input after the enter search.
	a = drain(a, keyRunes("神"))
	if a.search.input.Value() != "苍穹女武神神" {
		t.Fatalf("typing after enter did not continue in input: %q", a.search.input.Value())
	}
}

// esc clears the query and results entirely.
func TestEscClears(t *testing.T) {
	f := &fakeSearch{results: map[string]*site.Result{}}
	a := searchViewTestApp(t, f)

	a = drain(a, keyRunes("苍穹"))
	a = drain(a, keyArrow(tea.KeyEsc))
	if a.search.input.Value() != "" || a.search.state != searchIdle {
		t.Fatalf("esc did not clear: value=%q state=%d", a.search.input.Value(), a.search.state)
	}
}

// The full focus loop: enter keeps typing focus, ↓ switches to the result
// list, esc returns to the input.
func TestFocusLoop(t *testing.T) {
	f := &fakeSearch{results: map[string]*site.Result{"苍穹": oneResult("苍穹女武神")}}
	a := searchViewTestApp(t, f)

	a = drain(a, keyRunes("苍穹"))
	a = drain(a, keyArrow(tea.KeyEnter)) // immediate search, input stays focused
	if !a.search.input.Focused() {
		t.Fatal("input not focused after enter")
	}
	a = drain(a, keyArrow(tea.KeyDown)) // to result list
	if a.search.input.Focused() {
		t.Fatal("input still focused after down")
	}
	if a.search.list.SelectedItem() == nil {
		t.Fatalf("no item selected on the list")
	}
	a = drain(a, keyArrow(tea.KeyEsc)) // back to input
	if !a.search.input.Focused() {
		t.Fatal("esc did not return focus to input")
	}
	a = drain(a, keyRunes("神")) // keep typing
	if a.search.input.Value() != "苍穹神" {
		t.Fatalf("typing after focus loop: %q", a.search.input.Value())
	}
}

// Enter on a result goes to a detail page (with chapter count), not straight
// to download; Enter on the detail downloads, esc returns to the list.
func TestSearchDetailFlow(t *testing.T) {
	f := &fakeSearch{results: map[string]*site.Result{"苍穹女武神": oneResult("苍穹女武神")}}
	a := searchViewTestApp(t, f)
	a = drain(a, keyRunes("苍穹女武神"))
	a = drain(a, typeaheadTickMsg{})
	a = drain(a, keyArrow(tea.KeyDown)) // blur to list
	a = drain(a, keyArrow(tea.KeyEnter))
	if a.search.detail == nil {
		t.Fatal("enter on result did not open detail")
	}
	if a.search.detail.book.Title != "苍穹女武神" {
		t.Fatalf("detail book = %q", a.search.detail.book.Title)
	}
	// Chapter count arrives via detailLoadedMsg.
	a = drain(a, detailLoadedMsg{book: a.search.detail.book, total: 42})
	if a.search.detail.totalChapters != 42 {
		t.Fatalf("chapters = %d, want 42", a.search.detail.totalChapters)
	}
	if !strings.Contains(a.View(), "章节数: 42") {
		t.Fatalf("detail view missing chapter count:\n%s", a.View())
	}

	// Enter on detail starts the download.
	m, cmd := a.Update(keyArrow(tea.KeyEnter))
	a = m.(*App)
	if cmd == nil {
		t.Fatal("no download command from detail")
	}
	msg := cmd().(startDownloadMsg)
	if msg.book.Title != "苍穹女武神" {
		t.Fatalf("download started for %q", msg.book.Title)
	}

	// esc returns to the list.
	a = drain(a, keyArrow(tea.KeyEsc))
	if a.search.detail != nil {
		t.Fatal("esc did not close detail")
	}
}

// A staggered chapter-count result must not leak into a different book's
// detail page.
func TestSearchDetailIgnoresStale(t *testing.T) {
	f := &fakeSearch{results: map[string]*site.Result{"苍穹女武神": oneResult("苍穹女武神")}}
	a := searchViewTestApp(t, f)
	a = drain(a, keyRunes("苍穹女武神"))
	a = drain(a, typeaheadTickMsg{})
	a = drain(a, keyArrow(tea.KeyDown))
	a = drain(a, keyArrow(tea.KeyEnter))
	first := a.search.detail.book.URL
	// User navigated back, then we get a late result for the old detail.
	a = drain(a, keyArrow(tea.KeyEsc))
	a = drain(a, detailLoadedMsg{book: site.Book{URL: first}, total: 7})
	if a.search.detail != nil {
		t.Fatalf("stale detail applied: %+v", a.search.detail)
	}
}

// Per-volume chapter titles are capped on the detail page.
func TestDetailCapsChapterPreview(t *testing.T) {
	f := &fakeSearch{results: map[string]*site.Result{"长书": oneResult("长书")}}
	a := searchViewTestApp(t, f)
	a = drain(a, keyRunes("长书"))
	a = drain(a, typeaheadTickMsg{})
	a = drain(a, keyArrow(tea.KeyDown))
	a = drain(a, keyArrow(tea.KeyEnter))
	chapters := make([]site.ChapterRef, 20)
	for i := range chapters {
		chapters[i] = site.ChapterRef{Title: "第X章"}
	}
	a = drain(a, detailLoadedMsg{book: a.search.detail.book, total: 20,
		volumes: []site.VolumeRef{{Title: "卷一", Chapters: chapters}}})
	v := a.View()
	if !strings.Contains(v, "… 还有 14 章") {
		t.Fatalf("missing truncation note:\n%s", v)
	}
}

// The detail page lists volumes with their chapter counts.
func TestDetailShowsVolumes(t *testing.T) {
	f := &fakeSearch{results: map[string]*site.Result{"玩乐关系": oneResult("玩乐关系")}}
	a := searchViewTestApp(t, f)
	a = drain(a, keyRunes("玩乐关系"))
	a = drain(a, typeaheadTickMsg{})
	a = drain(a, keyArrow(tea.KeyDown))
	a = drain(a, keyArrow(tea.KeyEnter))
	a = drain(a, detailLoadedMsg{
		book: a.search.detail.book, total: 3,
		volumes: []site.VolumeRef{
			{Title: "第一卷", Chapters: []site.ChapterRef{{Title: "第一章 开幕"}, {Title: "第二章 风波"}}},
			{Title: "第二卷", Chapters: []site.ChapterRef{{Title: "第三章 终局"}}},
		},
	})
	v := a.View()
	if !strings.Contains(v, "卷数: 2") {
		t.Fatalf("missing volume count in detail:\n%s", v)
	}
	if !strings.Contains(v, "第一卷 — 2 章") || !strings.Contains(v, "第二卷 — 1 章") {
		t.Fatalf("missing volume lines:\n%s", v)
	}
	// Chapter titles appear under their volume.
	if !strings.Contains(v, "第一章 开幕") || !strings.Contains(v, "第三章 终局") {
		t.Fatalf("missing chapter titles:\n%s", v)
	}
}

// Up at the first result refocuses the query input; up elsewhere just moves
// the selection.
func TestUpArrowReturnsToInput(t *testing.T) {
	f := &fakeSearch{results: map[string]*site.Result{
		"苍穹女武神": {Total: 2, Books: []site.Book{
			{Title: "苍穹女武神", URL: "https://www.bilinovel.com/novel/1.html"},
			{Title: "苍穹之战神", URL: "https://www.bilinovel.com/novel/2.html"},
		}},
	}}
	a := searchViewTestApp(t, f)
	a = drain(a, keyRunes("苍穹女武神"))
	a = drain(a, typeaheadTickMsg{})
	a = drain(a, keyArrow(tea.KeyDown)) // blur to list at cursor 0
	if a.search.input.Focused() {
		t.Fatal("down did not blur")
	}
	a = drain(a, keyArrow(tea.KeyUp)) // at first result -> refocus
	if !a.search.input.Focused() {
		t.Fatal("up at first result did not refocus input")
	}

	// Down again, move selection down, then up must only move the cursor.
	a = drain(a, keyArrow(tea.KeyDown))
	a = drain(a, keyArrow(tea.KeyDown)) // cursor 1
	if a.search.list.Cursor() != 1 {
		t.Fatalf("cursor = %d, want 1", a.search.list.Cursor())
	}
	a = drain(a, keyArrow(tea.KeyUp)) // cursor 0, still list-focused
	if a.search.input.Focused() {
		t.Fatal("up from cursor 1 refocused input (should stay in list)")
	}
	if a.search.list.Cursor() != 0 {
		t.Fatalf("cursor after up = %d, want 0", a.search.list.Cursor())
	}
}

// Long detail content scrolls: j/up move the viewport, g returns to top.
func TestDetailScrolls(t *testing.T) {
	f := &fakeSearch{results: map[string]*site.Result{"长书": oneResult("长书")}}
	a := searchViewTestApp(t, f)
	a = drain(a, keyRunes("长书"))
	a = drain(a, typeaheadTickMsg{})
	a = drain(a, keyArrow(tea.KeyDown))
	a = drain(a, keyArrow(tea.KeyEnter))
	volumes := make([]site.VolumeRef, 6)
	for vi := range volumes {
		chapters := make([]site.ChapterRef, 20)
		for i := range chapters {
			chapters[i] = site.ChapterRef{Title: "第X章"}
		}
		volumes[vi] = site.VolumeRef{Title: "卷一", Chapters: chapters}
	}
	a = drain(a, detailLoadedMsg{book: a.search.detail.book, total: 120, volumes: volumes})
	// First render builds the content.
	_ = a.View()
	if a.search.detailVp.TotalLineCount() <= a.search.detailVp.Height {
		t.Fatal("detail content not tall enough to scroll")
	}
	a = drain(a, keyArrow(tea.KeyDown)) // scroll 1 line
	if a.search.detailVp.YOffset != 1 {
		t.Fatalf("down: offset %d, want 1", a.search.detailVp.YOffset)
	}
	a = drain(a, keyRunes("g")) // back to top
	if a.search.detailVp.YOffset != 0 {
		t.Fatalf("g: offset %d, want 0", a.search.detailVp.YOffset)
	}
	// Enter still downloads while scrolled; esc leaves.
	a = drain(a, keyRunes("j"))
	_, cmd := a.Update(keyArrow(tea.KeyEnter))
	if cmd == nil {
		t.Fatal("enter in scrolled detail did not download")
	}
}

// The detail page shows a clear "Enter: download" instruction.
func TestDetailShowsEnterInstruction(t *testing.T) {
	f := &fakeSearch{results: map[string]*site.Result{"玩乐关系": oneResult("玩乐关系")}}
	a := searchViewTestApp(t, f)
	a = drain(a, keyRunes("玩乐关系"))
	a = drain(a, typeaheadTickMsg{})
	a = drain(a, keyArrow(tea.KeyDown))
	a = drain(a, keyArrow(tea.KeyEnter))
	v := a.View()
	if !strings.Contains(v, "Enter: download") || !strings.Contains(v, "Esc: back") {
		t.Fatalf("missing action instructions in detail:\n%s", v)
	}
}
