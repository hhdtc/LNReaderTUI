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
// topRow is the 0-based terminal row where the item rows begin. Items can
// have variable heights (wrapped descriptions), so itemLines reports the
// rendered row count for an item index; clicks walk the cumulative heights.
// Returns true when the message was consumed.
func mouseList(msg tea.MouseMsg, m *list.Model, topRow int,
	itemLines func(idx int) int) bool {
	if msg.Action != tea.MouseActionPress {
		return false
	}
	if msg.Button == tea.MouseButtonWheelUp {
		if m.Cursor() > 0 || !m.Paginator.OnFirstPage() {
			m.CursorUp()
		}
		return true
	}
	if msg.Button == tea.MouseButtonWheelDown {
		if m.Cursor() < m.Paginator.PerPage-1 || !m.Paginator.OnLastPage() {
			m.CursorDown()
		}
		return true
	}
	if msg.Button == tea.MouseButtonLeft && msg.Y >= topRow {
		y := msg.Y - topRow
		first := m.Paginator.Page * m.Paginator.PerPage
		total := len(m.Items())
		for i := first; i < first+m.Paginator.PerPage && i < total; i++ {
			h := max(itemLines(i), 1)
			if y < h {
				m.Select(i)
				return true
			}
			y -= h
		}
		if total > 0 {
			m.Select(total - 1)
		}
		return true
	}
	return false
}

// itemLayout measures the real item geometry (start row + stride) from the
// rendered list output, so clicks stay correct under any terminal font,
// spacing or status-bar layout.
func itemLayout(itemTitles []string, listOut string) (startRow, stride int) {
	stride = 2
	if len(itemTitles) == 0 {
		return 2, stride
	}
	lines := strings.Split(listOut, "\n")
	first := -1
	second := -1
	for i, l := range lines {
		if first < 0 && strings.Contains(l, itemTitles[0]) {
			first = i
			continue
		}
		if first >= 0 && second < 0 && len(itemTitles) > 1 &&
			strings.Contains(l, itemTitles[1]) {
			second = i
			break
		}
	}
	if first < 0 {
		return 2, stride // fallback: list starts after title+status rows
	}
	if second > first {
		stride = max(second-first, 1)
	}
	// +1: the list output begins on the frame row after the tab bar.
	startRow = first + 1
	return startRow, stride
}
