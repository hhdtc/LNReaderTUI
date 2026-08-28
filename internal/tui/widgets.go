package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

var (
	tabActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("232")).
			Background(lipgloss.Color("81")).
			Padding(0, 1)
	tabInactive = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Padding(0, 1)
	tabSep = lipgloss.NewStyle().
		Foreground(lipgloss.Color("237"))
	tabHint = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	footerHint = lipgloss.NewStyle().
			Foreground(lipgloss.Color("246")).
			Faint(true)

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214"))

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("229"))

	progressBar = lipgloss.NewStyle().
			Foreground(lipgloss.Color("78"))
)

// wrapWidth wraps text to the given display width, honoring CJK wide runes.
func wrapWidth(text string, width int) []string {
	if width <= 0 {
		return []string{""}
	}
	var lines []string
	var cur strings.Builder
	curW := 0
	for _, r := range []rune(text) {
		if r == '\n' {
			lines = append(lines, cur.String())
			cur.Reset()
			curW = 0
			continue
		}
		w := runewidth.RuneWidth(r)
		if w == 0 {
			// Combining/zero-width: append to current position.
			cur.WriteRune(r)
			continue
		}
		if curW+w > width {
			lines = append(lines, strings.TrimRight(cur.String(), " "))
			cur.Reset()
			curW = 0
			if w > width {
				// Oversized rune (very wide glyph): emit alone on a line.
				cur.WriteRune(r)
				lines = append(lines, cur.String())
				cur.Reset()
				curW = 0
				continue
			}
		}
		cur.WriteRune(r)
		curW += w
	}
	lines = append(lines, cur.String())
	return lines
}

// wrapParagraphs plain-text paragraphs into display lines for a viewport.
func wrapParagraphs(paras []string, width int) string {
	lines := make([]string, 0, len(paras))
	for _, p := range paras {
		lines = append(lines, wrapWidth(p, width)...)
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// limitLines keeps the first maxLines lines of a rendered view, so a
// slightly-tall body can never push the tab bar or footer off-screen.
func limitLines(s string, maxLines int) string {
	if maxLines <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= maxLines {
		return s
	}
	return strings.Join(lines[:maxLines], "\n")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// mouseList implements click-to-select and wheel for a bubbles list.
// topRow is the 0-based terminal row where the item rows begin; items render
// itemHeight rows each (default delegate: 2 = title + description).
// Returns true when the message was consumed.
func mouseList(msg tea.MouseMsg, m *list.Model, topRow, itemHeight int) bool {
	if msg.Action != tea.MouseActionPress {
		return false
	}
	if msg.Button == tea.MouseButtonWheelUp {
		if m.Cursor() > 0 || !m.Paginator.OnFirstPage() {
			m.CursorUp()
		}
		_ = m
		return true
	}
	if msg.Button == tea.MouseButtonWheelDown {
		if m.Cursor() < m.Paginator.PerPage-1 || !m.Paginator.OnLastPage() {
			m.CursorDown()
		}
		return true
	}
	if msg.Button == tea.MouseButtonLeft && msg.Y > topRow {
		rel := (msg.Y - topRow) / max(itemHeight, 1)
		if rel >= m.Paginator.PerPage {
			rel = m.Paginator.PerPage - 1
		}
		abs := m.Paginator.Page*m.Paginator.PerPage + rel
		if total := len(m.Items()); abs >= total {
			abs = total - 1
		}
		if abs >= 0 {
			m.Select(abs)
		}
		return true
	}
	return false
}
