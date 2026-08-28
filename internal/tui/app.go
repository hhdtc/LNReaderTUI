// Package tui is the bubbletea terminal interface for the library, online
// search, downloads and reading.
package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"lnreadertui/internal/epub"
	"lnreadertui/internal/model"
	"lnreadertui/internal/site"
)

type viewKind int

const (
	viewLibrary viewKind = iota
	viewSearch
	viewJobs
	viewReader
)

// App is the root bubbletea model.
type App struct {
	store   *model.Store
	dl      *site.Downloader
	dataDir string

	width, height int
	view          viewKind

	library *libraryView
	search  *searchView
	jobs    *jobsView
	rview   *readerView

	modal  *modal
	status string
}

// New wires the root model.
func New(store *model.Store, dl *site.Downloader, dataDir string) *App {
	return &App{
		store:   store,
		dl:      dl,
		dataDir: dataDir,
		view:    viewLibrary,
		library: newLibraryView(store),
		search:  newSearchView(dl),
		jobs:    newJobsView(store),
	}
}

// Init implements tea.Model.
func (a *App) Init() tea.Cmd {
	return tea.Tick(10*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// ---- messages ----

type tickMsg time.Time

// searchResultMsg carries an online search outcome.
type searchResultMsg struct {
	gen int
	q   string
	res *site.Result
	err error
}

// jobProgressMsg reports download progress or completion.
type jobProgressMsg struct {
	id         string
	done       int
	total      int
	title      string
	volume     string
	finished   bool
	err        error
	bundlePath string
}

// importDoneMsg reports the outcome of an async path import.
type importDoneMsg struct {
	books []*model.Book
	err   error
}

// bookLoadedMsg carries a parsed book for the reader.
type bookLoadedMsg struct {
	id     string
	parsed *epub.Book
	err    error
}

// jobImportedMsg reports a finished download registered into the library.
type jobImportedMsg struct {
	id   string
	book *model.Book
	err  error
}

// openBookMsg requests the reader for a library book.
type openBookMsg struct{ book *model.Book }

// resetProgressMsg clears a book's reading progress.
type resetProgressMsg struct{ id string }

// deleteBookMsg requests deleting a book.
type deleteBookMsg struct{ id string }

// importPathMsg starts an import of a path.
type importPathMsg struct{ path string }

// startDownloadMsg starts a download job from a search result.
type startDownloadMsg struct{ book site.Book }

// exitReaderMsg asks the root to close the reader (saving progress).
type exitReaderMsg struct{}

// noneMsg is a quiet poll tick (no observable effect).
type noneMsg struct{}

// ---- lifecycle ----

// Update implements tea.Model.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = m.Width, m.Height
		a.resizeViews()
		return a, nil

	case tickMsg:
		// Periodic autosave while reading (save() no-ops when nothing moved).
		var cmds []tea.Cmd
		if a.rview != nil {
			a.saveReaderProgress()
		}
		cmds = append(cmds, tea.Tick(10*time.Second,
			func(t time.Time) tea.Msg { return tickMsg(t) }))
		return a, tea.Batch(cmds...)

	case detailLoadedMsg:
		a.search.applyDetail(m)
		return a, nil

	case searchResultMsg:
		if m.gen != a.search.seq {
			return a, nil // stale (a newer query replaced it)
		}
		if m.err != nil {
			a.search.setError(m.err)
		} else {
			a.search.setResult(m.res)
			a.status = ""
		}
		return a, nil

	case typeaheadTickMsg:
		// Debounce fires no matter which tab is active.
		return a.updateSearch(msg)

	case jobProgressMsg:
		if icmd := a.jobs.apply(m); icmd != nil {
			// Finished download: auto-import while keeping events flowing.
			return a, tea.Batch(a.pollJobs(), icmd)
		}
		return a, a.pollJobs()

	case noneMsg:
		// Nothing was ready; keep polling so later events are still drained.
		return a, a.pollJobs()

	case importDoneMsg:
		if m.err != nil {
			a.status = fmt.Sprintf("import failed: %v", m.err)
		} else {
			a.status = fmt.Sprintf("imported %d book(s)", len(m.books))
			a.library.refresh()
		}
		return a, nil

	case bookLoadedMsg:
		if m.err != nil {
			a.status = fmt.Sprintf("open failed: %v", m.err)
			return a, nil
		}
		book := a.store.Find(m.id)
		if book == nil {
			a.status = "book not found"
			return a, nil
		}
		a.view = viewReader
		a.rview = newReaderView(book, m.parsed, a.store)
		a.resizeViews()
		return a, a.rview.openChapter(
			book.Progress.ChapterIndex, book.Progress.Offset)

	case jobImportedMsg:
		a.jobs.markImported(m.id)
		if m.err != nil {
			a.status = fmt.Sprintf("import failed: %v", m.err)
		} else {
			a.status = fmt.Sprintf("imported %s", m.book.Title)
			a.library.refresh()
		}
		return a, nil

	case openBookMsg:
		return a, loadBookCmd(a.store, m.book)

	case resetProgressMsg:
		if err := a.store.ResetProgress(m.id); err != nil {
			a.status = fmt.Sprintf("reset failed: %v", err)
		} else {
			a.status = "reading progress reset"
			a.library.refresh()
		}
		a.modal = nil
		return a, nil

	case deleteBookMsg:
		b := a.store.Find(m.id)
		if err := a.store.Delete(m.id); err != nil {
			a.status = fmt.Sprintf("delete failed: %v", err)
		} else if b != nil {
			a.status = fmt.Sprintf("deleted %s", b.Title)
			a.library.refresh()
		}
		a.modal = nil
		return a, nil

	case importPathMsg:
		return a, func() tea.Msg {
			books, err := a.store.ImportFile(strings.TrimSpace(m.path))
			return importDoneMsg{books: books, err: err}
		}

	case startDownloadMsg:
		a.jobs.start(m.book, a.dl)
		a.view = viewJobs
		a.resizeViews()
		return a, a.pollJobs()

	case exitReaderMsg:
		a.backToLibrary()
		return a, nil

	case tea.MouseMsg:
		// Tab bar clicks (row 0) switch tabs; everything else falls through
		// to the active view (list clicks, wheel scrolling, …).
		if m.Action == tea.MouseActionPress && m.Button == tea.MouseButtonLeft &&
			a.view != viewReader && m.Y == 0 {
			if idx := a.tabAtX(m.X); idx >= 0 {
				a.view = viewKind(idx)
				a.afterViewChange()
				return a, nil
			}
		}
	case tea.KeyMsg:
		// A fast typist (or a paste) can arrive as ONE KeyMsg holding many
		// runes. Shortcuts match single runes, so split the burst up before
		// any view sees it.
		if len(m.Runes) > 1 {
			var cmds []tea.Cmd
			for _, r := range m.Runes {
				mm, c := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				a = mm.(*App)
				if c != nil {
					cmds = append(cmds, c)
				}
			}
			return a, tea.Batch(cmds...)
		}
		if a.modal != nil {
			return a.updateModal(m)
		}
		// Global keys.
		switch m.String() {
		case "tab", "shift+tab":
			if a.view != viewReader {
				a.cycleView()
				return a, nil
			}
		case "left", "right":
			// Arrows own the tab bar in every tab view (even while a text
			// input is focused); typing cursor moves via Home/End/ctrl-a/ctrl-e.
			if a.view != viewReader {
				if m.String() == "right" {
					a.view = (a.view + 1) % viewReader
				} else {
					a.view = (a.view + viewReader - 1) % viewReader
				}
				a.afterViewChange()
				return a, nil
			}
		case "1", "2", "3":
			if a.view != viewReader && !a.textInputActive() {
				if idx := int(m.String()[0] - '1'); idx >= 0 && idx < int(viewReader) {
					a.view = viewKind(idx)
					a.afterViewChange()
				}
				return a, nil
			}
		case "q":
			if a.view != viewReader && !a.textInputActive() {
				return a, tea.Quit
			}
		case "ctrl+c":
			// ctrl+c is a universal interrupt: always exits, saving the
			// reader's progress first.
			if a.rview != nil {
				a.saveReaderProgress()
			}
			return a, tea.Quit
		}
	}

	// Delegate to the active view.
	switch a.view {
	case viewLibrary:
		return a.updateLibrary(msg)
	case viewSearch:
		return a.updateSearch(msg)
	case viewJobs:
		return a.updateJobs(msg)
	case viewReader:
		return a.updateReader(msg)
	}
	return a, nil
}

func (a *App) updateLibrary(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m, ok := msg.(tea.KeyMsg); ok {
		switch m.String() {
		case "i":
			a.modal = newPathModal("Import — path to .epub/.txt file or directory",
				"Large files are parsed in the background.",
				func(v string) tea.Msg { return importPathMsg{path: v} })
			return a, nil
		case "d":
			if b := a.library.selected(); b != nil {
				target := b
				a.modal = newConfirmModal("Delete "+target.Title,
					fmt.Sprintf("This removes the book and its file (%d chapters).",
						target.Chapters),
					func() tea.Msg { return deleteBookMsg{id: target.ID} })
			}
			return a, nil
		case "x":
			if b := a.library.selected(); b != nil {
				target := b
				a.modal = newConfirmModal("Reset progress — "+target.Title,
					fmt.Sprintf("Marks %s as unread (chapter 1).", target.Title),
					func() tea.Msg { return resetProgressMsg{id: target.ID} })
			}
			return a, nil
		}
	}
	var cmd tea.Cmd
	a.library, cmd = a.library.Update(msg)
	return a, cmd
}

func (a *App) updateSearch(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	a.search, cmd = a.search.Update(msg)
	return a, cmd
}

func (a *App) updateJobs(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	a.jobs, cmd = a.jobs.Update(msg)
	return a, cmd
}

func (a *App) updateReader(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	a.rview, cmd = a.rview.Update(msg)
	return a, cmd
}

// textInputActive reports whether a focused text input owns the keyboard
// (typing q must not quit mid-query).
func (a *App) textInputActive() bool {
	switch a.view {
	case viewSearch:
		return a.search.input.Focused()
	case viewLibrary:
		return a.library.filtering && a.library.filter.Focused()
	}
	return false
}

func (a *App) cycleView() {
	a.view = (a.view + 1) % viewReader
	a.afterViewChange()
}

// afterViewChange runs after any tab switch: the search input regains focus
// so typing is immediately possible.
func (a *App) afterViewChange() {
	a.resizeViews()
	if a.view == viewSearch {
		a.search.input.Focus()
		// The input owns the keyboard again: hide the selection highlight
		// until ↓ selects something.
		a.search.syncDelegate()
	}
}

func (a *App) backToLibrary() {
	if a.rview != nil {
		_ = a.rview.save()
		a.rview = nil
	}
	a.view = viewLibrary
	a.resizeViews()
}

func (a *App) saveReaderProgress() {
	if a.rview == nil {
		return
	}
	if err := a.rview.save(); err != nil {
		a.status = fmt.Sprintf("save failed: %v", err)
	}
}

// pollJobs drains one pending download event (or nothing after 200 ms).
// Re-issued after every jobProgressMsg so the channel keeps flowing.
func (a *App) pollJobs() tea.Cmd {
	return func() tea.Msg {
		select {
		case m := <-a.jobs.events:
			return m
		case <-time.After(200 * time.Millisecond):
			return noneMsg{}
		}
	}
}

func (a *App) resizeViews() {
	if a.width == 0 || a.height == 0 {
		return
	}
	h := tabContentHeight(a.height)
	a.library.resize(a.width, h)
	a.search.resize(a.width, h)
	a.jobs.resize(a.width, h)
	if a.rview != nil {
		a.rview.resize(a.width, a.height)
	}
}

func tabContentHeight(h int) int {
	if h < 4 {
		return 1
	}
	return h - 3 // tab bar + footer + status line
}

// ---- rendering ----

// View implements tea.Model.
func (a *App) View() string {
	if a.width == 0 {
		return ""
	}
	if p := os.Getenv("OMTUI_FRAME_DUMP"); p != "" {
		_ = os.WriteFile(p, []byte(a.render()), 0o644)
		st := fmt.Sprintf("view=%d reader=%v\n", a.view, a.rview != nil)
		st += fmt.Sprintf("width=%d height=%d\n", a.width, a.height)
		_ = os.WriteFile(p+".state", []byte(st), 0o644)
	}
	return a.render()
}

// render assembles the complete frame.
func (a *App) render() string {
	if a.view == viewReader && a.rview != nil {
		return a.rview.View()
	}
	h := tabContentHeight(a.height)
	var body string
	switch a.view {
	case viewLibrary:
		body = limitLines(a.library.View(a.width, h), h)
	case viewSearch:
		body = limitLines(a.search.View(a.width, h), h)
	case viewJobs:
		body = limitLines(a.jobs.View(a.width, h), h)
	}
	if a.modal != nil {
		body = a.modal.render(a.width, h, body)
	}
	return padFrame(lipgloss.JoinVertical(lipgloss.Left,
		a.tabBar(), body, footerBar(a, a.view), statusLine(a.status)), a.height)
}

// padFrame makes every frame exactly terminal-height: a shorter frame would
// otherwise leave stale rows on screen (the renderer only rewrites lines it
// is told about).
func padFrame(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	for len(lines) < n {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// tabAtX maps a terminal column to a tab index (tab bar rows are y=1;
// the renderer outputs a leading newline on some setups, so the caller
// passes the row-based coordinate and expects 0-based offsets).
func (a *App) tabAtX(x int) int {
	names := []string{"Library", "Search", "Downloads"}
	x -= 1 // left margin of the tab bar content
	pos := 0
	for i, name := range names {
		label := fmt.Sprintf("%d %s", i+1, name)
		var w int
		if viewKind(i) == a.view {
			w = lipgloss.Width(tabActive.Render(" " + label + " "))
		} else {
			w = lipgloss.Width(tabInactive.Render(" " + label + " "))
		}
		if x >= pos && x < pos+w {
			return i
		}
		pos += w
		if i < len(names)-1 {
			pos += lipgloss.Width(tabSep.Render("│"))
		}
	}
	return -1
}

func (a *App) tabBar() string {
	names := []string{"Library", "Search", "Downloads"}
	var b strings.Builder
	for i, name := range names {
		if i > 0 {
			b.WriteString(tabSep.Render("│"))
		}
		label := fmt.Sprintf("%d %s", i+1, name)
		if viewKind(i) == a.view {
			b.WriteString(tabActive.Render(" " + label + " "))
		} else {
			b.WriteString(tabInactive.Render(" " + label + " "))
		}
	}
	hint := tabHint.Render("←/→ or Tab: switch · q: quit")
	tabs := b.String()
	if a.width > lipgloss.Width(tabs)+2 {
		hint = lipgloss.PlaceHorizontal(a.width-lipgloss.Width(tabs), lipgloss.Right, hint)
		return tabs + hint
	}
	return tabs + "  " + hint
}

func footerBar(a *App, v viewKind) string {
	switch v {
	case viewLibrary:
		return footerHint.Render("enter: read · i: import · d: delete · x: reset progress · /: filter")
	case viewSearch:
		// The detail page renders its own action footer; the generic search
		// hint would stack confusingly below it.
		if a.search.detail != nil {
			return ""
		}
		return footerHint.Render("type to search live · enter: immediate · esc: clear")
	case viewJobs:
		return footerHint.Render("x: cancel · enter: import to library · d: dismiss")
	}
	return ""
}

func statusLine(s string) string {
	if s == "" {
		return ""
	}
	return statusStyle.Render(s)
}

func loadBookCmd(store *model.Store, b *model.Book) tea.Cmd {
	return func() tea.Msg {
		parsed, err := epub.ReadFile(store.FilePath(b))
		return bookLoadedMsg{id: b.ID, parsed: parsed, err: err}
	}
}

// ---- modal ----

type modal struct {
	title   string
	body    string
	input   *textinput.Model
	confirm func() tea.Msg
	async   func(string) tea.Msg
}

func newConfirmModal(title, body string, confirm func() tea.Msg) *modal {
	return &modal{title: title, body: body, confirm: confirm}
}

func newPathModal(title, body string, async func(string) tea.Msg) *modal {
	in := textinput.New()
	in.Placeholder = "/path/to/novel.epub or /dir"
	in.Prompt = "> "
	in.Focus()
	return &modal{title: title, body: body, input: &in, async: async}
}

func (a *App) updateModal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m := a.modal
	switch msg.String() {
	case "esc":
		a.modal = nil
		return a, nil
	case "enter":
		a.modal = nil
		if m.input != nil {
			if v := strings.TrimSpace(m.input.Value()); v != "" && m.async != nil {
				return a, func() tea.Msg { return m.async(v) }
			}
			return a, nil
		}
		if m.confirm != nil {
			return a, func() tea.Msg { return m.confirm() }
		}
		return a, nil
	}
	if m.input != nil {
		var cmd tea.Cmd
		*m.input, cmd = m.input.Update(msg)
		return a, cmd
	}
	return a, nil
}

func (m *modal) render(width, height int, bg string) string {
	body := m.body + "\n"
	if m.input != nil {
		body += m.input.View() + "\n"
	}
	body += "enter: confirm · esc: cancel"
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1, 2).
		Width(min(width-4, 60)).
		Render(m.title + "\n\n" + body)
	lines := strings.Split(bg, "\n")
	if len(lines) > 0 {
		lines[len(lines)-1] = "" // clear last line under overlay
	}
	plain := strings.Join(lines, "\n")
	return lipgloss.Place(width, max(height, len(strings.Split(plain, "\n"))),
		lipgloss.Center, lipgloss.Center, box)
}
