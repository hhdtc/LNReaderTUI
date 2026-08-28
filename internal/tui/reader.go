package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"lnreadertui/internal/epub"
	"lnreadertui/internal/model"
)

type readerView struct {
	store   *model.Store
	book    *model.Book
	parsed  *epub.Book
	chapter int

	vp       viewport.Model
	chList   list.Model
	showSide bool

	width, height int
	err           string
	lastSaved     time.Time

	// Last persisted position, for dedupe: scroll moves no longer need a
	// dirty flag — save() compares the live position against this pair.
	lastSavedChapter int
	lastSavedOffset  int
}

type chItem struct {
	title string
	idx   int
}

func (c chItem) Title() string       { return c.title }
func (c chItem) Description() string { return "" }
func (c chItem) FilterValue() string { return c.title }

// sidebarWidth is the chapter panel width while open.
const sidebarWidth = 36

func newReaderView(book *model.Book, parsed *epub.Book, store *model.Store) *readerView {
	items := make([]list.Item, 0, len(parsed.Chapters))
	for i, ch := range parsed.Chapters {
		items = append(items, chItem{title: ch.Title, idx: i})
	}
	cl := list.New(items, list.NewDefaultDelegate(), sidebarWidth, 20)
	cl.Title = "Chapters"
	return &readerView{
		store:     store,
		book:      book,
		parsed:    parsed,
		chList:    cl,
		lastSaved: time.Now(),
	}
}

func (v *readerView) resize(width, height int) {
	v.width, v.height = width, height
	contentW := max(width-2, 1)
	if v.showSide {
		contentW = max(width-sidebarWidth-2, 1)
	}
	v.vp.Width = contentW
	v.vp.Height = max(height-4, 1)
	v.vp.SetYOffset(v.vp.YOffset) // re-clamp after resize
	v.chList.SetSize(sidebarWidth, max(height-4, 1))
}

// openChapter loads the chapter at idx, restoring the given offset ratio.
func (v *readerView) openChapter(idx int, ratio float64) tea.Cmd {
	if idx < 0 || idx >= len(v.parsed.Chapters) {
		idx = 0
	}
	v.chapter = idx
	v.loadChapter(ratio)
	return v.saveCmd()
}

func (v *readerView) loadChapter(ratio float64) {
	ch := v.parsed.Chapters[v.chapter]
	content := wrapParagraphs(ch.Paragraphs, max(v.vp.Width-2, 10))
	v.vp.SetContent(content)
	maxOffset := max(0, v.vp.TotalLineCount()-v.vp.Height)
	v.vp.SetYOffset(int(ratio * float64(maxOffset)))
	if v.showSide {
		v.chList.Select(v.chapter)
	}
}

func (v *readerView) save() error {
	maxY := max(1, v.vp.TotalLineCount()-v.vp.Height)
	if v.lastSavedChapter == v.chapter && v.lastSavedOffset == v.vp.YOffset {
		return nil
	}
	v.lastSavedChapter = v.chapter
	v.lastSavedOffset = v.vp.YOffset
	v.lastSaved = time.Now()
	return v.store.SetProgress(v.book.ID, v.chapter,
		float64(v.vp.YOffset)/float64(maxY), v.parsed.Chapters[v.chapter].Title)
}

func (v *readerView) saveCmd() tea.Cmd {
	v.lastSavedChapter = v.chapter
	v.lastSavedOffset = v.vp.YOffset
	v.lastSaved = time.Now()
	id, chapter := v.book.ID, v.chapter
	parsedTitle := v.parsed.Chapters[v.chapter].Title
	maxY := max(1, v.vp.TotalLineCount()-v.vp.Height)
	ratio := float64(v.vp.YOffset) / float64(maxY)
	return func() tea.Msg {
		_ = v.store.SetProgress(id, chapter, ratio, parsedTitle)
		return nil
	}
}

func (v *readerView) gotoChapter(idx int, ratio float64) {
	if idx < 0 {
		idx = 0
	}
	if idx >= len(v.parsed.Chapters) {
		idx = len(v.parsed.Chapters) - 1
	}
	if idx == v.chapter {
		return
	}
	v.chapter = idx
	v.loadChapter(ratio)
}

func (v *readerView) Update(msg tea.Msg) (*readerView, tea.Cmd) {
	switch m := msg.(type) {
	case tea.KeyMsg:
		if v.showSide {
			switch m.String() {
			case "enter":
				if item := v.chList.SelectedItem(); item != nil {
					idx := item.(chItem).idx
					_ = v.save()
					v.gotoChapter(idx, 0)
					return v, v.saveCmd()
				}
				return v, nil
			case "j", "k", "up", "down", "g", "G", "pgup", "pgdown",
				"home", "end", "ctrl+d", "ctrl+u":
				// The open sidebar owns list navigation.
				var cmd tea.Cmd
				v.chList, cmd = v.chList.Update(msg)
				return v, cmd
			}
		}
		switch m.String() {
		case "q":
			return v, func() tea.Msg { return exitReaderMsg{} }
		case "esc":
			if v.showSide {
				v.showSide = false
				v.resize(v.width, v.height)
				return v, nil
			}
			return v, func() tea.Msg { return exitReaderMsg{} }
		case "s", "o", "c":
			v.showSide = !v.showSide
			if v.showSide {
				v.chList.Select(v.chapter)
			}
			v.resize(v.width, v.height)
			return v, nil
		case "left", "right", "h", "l":
			// Page flipping: right/l (and left/h) scroll within the chapter;
			// at the chapter edge they roll over to the next/previous
			// chapter.
			if m.String() == "right" || m.String() == "l" {
				return v.pageRight()
			}
			return v.pageLeft()
		case "n", "p":
			next := v.chapter
			if m.String() == "n" {
				next = v.chapter + 1
			} else {
				next = v.chapter - 1
			}
			if next < 0 || next >= len(v.parsed.Chapters) {
				return v, nil
			}
			_ = v.save()
			v.gotoChapter(next, 0)
			return v, v.saveCmd()
		case "j":
			v.vp.ScrollDown(3)
			return v, nil
		case "k":
			v.vp.ScrollUp(3)
			return v, nil
		case "g":
			v.vp.GotoTop()
			return v, nil
		case "G":
			v.vp.GotoBottom()
			return v, nil
		}
	case tea.MouseMsg:
		// Click the content area: left third pages back, right third pages
		// forward (same as the arrow keys, with chapter roll-over).
		if m.Action == tea.MouseActionPress && m.Button == tea.MouseButtonLeft {
			if m.Y >= 2 && m.Y < v.height-2 && v.width > 2 {
				third := v.width / 3
				switch {
				case m.X < third:
					return v.pageLeft()
				case m.X >= 2*third:
					return v.pageRight()
				}
			}
			return v, nil // middle third / outside content: ignore
		}
	}
	var cmd tea.Cmd
	v.vp, cmd = v.vp.Update(msg)
	return v, cmd
}

// pageRight flips one page forward (rolls to the next chapter at the bottom).
func (v *readerView) pageRight() (*readerView, tea.Cmd) {
	if v.vp.AtBottom() {
		if v.chapter+1 < len(v.parsed.Chapters) {
			_ = v.save()
			v.gotoChapter(v.chapter+1, 0)
			return v, v.saveCmd()
		}
		return v, nil
	}
	v.vp.PageDown()
	return v, nil
}

// pageLeft flips one page back (rolls to the previous chapter's last page at
// the top).
func (v *readerView) pageLeft() (*readerView, tea.Cmd) {
	if v.vp.AtTop() {
		if v.chapter > 0 {
			_ = v.save()
			v.gotoChapter(v.chapter-1, 1)
			return v, v.saveCmd()
		}
		return v, nil
	}
	v.vp.PageUp()
	return v, nil
}

func (v *readerView) View() string {
	ch := v.parsed.Chapters[v.chapter]

	header := titleStyle.Render(v.book.Title+" — "+ch.Title) + "  " +
		lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(v.percent())
	sep := dimmed(strings.Repeat("─", v.width))
	status := lipgloss.NewStyle().Foreground(lipgloss.Color("246")).Faint(true).Render(
		"←/→: page · n/p: chapter · j/k: scroll · s: chapters · q: back")
	if v.err != "" {
		status = failedStyle.Render(v.err)
	}

	body := v.vp.View()
	if v.showSide {
		side := v.chList.View()
		side = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, false, false, true).
			Render(side)
		body = lipgloss.JoinHorizontal(lipgloss.Left, side, body)
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, sep, body, sep, status)
}

func (v *readerView) percent() string {
	if v.book.Chapters <= 0 {
		return "0%"
	}
	total := max(v.book.Chapters, len(v.parsed.Chapters))
	return fmt.Sprintf("%d/%d · %.0f%%", v.chapter+1, total,
		100*float64(v.chapter+1)/float64(total))
}
