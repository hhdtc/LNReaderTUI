package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"lnreadertui/internal/model"
)

type libraryView struct {
	store     *model.Store
	list      list.Model
	filter    textinput.Model
	filtering bool
}

type libItem struct {
	book *model.Book
}

func (i libItem) Title() string { return i.book.Title }

func (i libItem) Description() string {
	p := i.book.Progress
	if p.LastReadAt == "" && p.ChapterIndex == 0 {
		return fmt.Sprintf("%s · %d chapters · unread",
			orUnknown(i.book.Author), i.book.Chapters)
	}
	line := fmt.Sprintf("%s · %d/%d chapters", orUnknown(i.book.Author),
		p.ChapterIndex+1, i.book.Chapters)
	if p.ChapterTitle != "" {
		line += fmt.Sprintf(" · %s %.0f%%", p.ChapterTitle, p.Offset*100)
	}
	return line + " · " + shortDate(p.LastReadAt)
}

func (i libItem) FilterValue() string { return i.book.Title + " " + i.book.Author }

func orUnknown(s string) string {
	if s == "" {
		return "Unknown"
	}
	return s
}

func shortDate(rfc string) string {
	t, err := time.Parse(time.RFC3339, rfc)
	if err != nil {
		return ""
	}
	return t.Local().Format("2006-01-02")
}

func newLibraryView(store *model.Store) *libraryView {
	v := &libraryView{store: store}
	v.list = list.New(nil, list.NewDefaultDelegate(), 80, 20)
	v.list.Title = "Library"
	v.filter = textinput.New()
	v.filter.Placeholder = "filter library…"
	v.refresh()
	return v
}

func (v *libraryView) refresh() {
	books := v.store.Books()
	items := make([]list.Item, 0, len(books))
	for _, b := range books {
		items = append(items, libItem{book: b})
	}
	v.list.SetItems(items)
}

func (v *libraryView) selected() *model.Book {
	if v.list.SelectedItem() == nil {
		return nil
	}
	return v.list.SelectedItem().(libItem).book
}

func (v *libraryView) resize(width, height int) {
	// One extra row for the filter input when filtering.
	listH := height
	if v.filtering {
		listH = max(height-1, 1)
	}
	v.list.SetSize(width, listH)
	v.list.SetWidth(width)
}

func (v *libraryView) Update(msg tea.Msg) (*libraryView, tea.Cmd) {
	switch m := msg.(type) {
	case tea.MouseMsg:
		if m.Y == 0 {
			return v, nil // tab bar is handled by the app
		}
		topRow := 3
		if v.filtering {
			topRow = 4
		}
		// The default delegate truncates descriptions to one line, so every
		// item renders exactly 2 rows (title + description).
		if mouseList(m, &v.list, topRow, func(int) int { return 2 }) {
			return v, nil
		}
		return v, nil
	case tea.KeyMsg:
		switch m.String() {
		case "/", "f":
			v.filtering = true
			v.filter.Focus()
			return v, nil
		case "esc":
			if v.filtering {
				v.filtering = false
				v.filter.Blur()
				v.filter.SetValue("")
				v.applyFilter("")
				return v, nil
			}
		case "enter":
			if v.filtering {
				v.applyFilter(v.filter.Value())
				v.filtering = false
				v.filter.Blur()
				return v, nil
			}
			if b := v.selected(); b != nil {
				return v, func() tea.Msg { return openBookMsg{book: b} }
			}
			return v, nil
		}
	}
	if v.filtering {
		var cmd tea.Cmd
		v.filter, cmd = v.filter.Update(msg)
		return v, cmd
	}
	var cmd tea.Cmd
	v.list, cmd = v.list.Update(msg)
	return v, cmd
}

func (v *libraryView) applyFilter(q string) {
	books := v.store.Books()
	items := make([]list.Item, 0, len(books))
	q = strings.ToLower(strings.TrimSpace(q))
	for _, b := range books {
		if q != "" && !strings.Contains(strings.ToLower(b.Title), q) &&
			!strings.Contains(strings.ToLower(b.Author), q) {
			continue
		}
		items = append(items, libItem{book: b})
	}
	v.list.SetItems(items)
}

func (v *libraryView) View(width, height int) string {
	if v.filtering {
		return lipgloss.JoinVertical(lipgloss.Left,
			titleStyle.Render("Library — filter"),
			v.filter.View(),
			v.list.View(),
		)
	}
	return v.list.View()
}
