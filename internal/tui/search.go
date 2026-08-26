package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"lnreadertui/internal/site"
)

type searchState int

const (
	searchIdle searchState = iota
	searchSearching
	searchResults
	searchError
)

// typeaheadDelay is how long the query must be stable before a live search.
const typeaheadDelay = 500 * time.Millisecond

// minQueryRunes is the shortest live-searched query (Enter still works with
// any non-empty query).
const minQueryRunes = 2

// chapterPreview caps per-volume chapter titles shown on the detail page.
const chapterPreview = 6

type searchView struct {
	input   textinput.Model
	list    list.Model
	state   searchState
	spinner spinner.Model
	status  string
	errText string

	// detail is the book detail page shown when a result is selected (enter
	// no longer downloads directly).
	dl     *site.Downloader
	detail *searchDetail
	// detailVp renders the (possibly long) detail body with scrolling.
	detailVp     viewport.Model
	detailDirty  bool
	detailBuiltW int

	// typeahead state
	pending   string // last value scheduled for debounce
	lastValue string // value processed by the input
	tickGen   int    // newest scheduled debounce; stale ticks are ignored
	seq       int
	cancel    context.CancelFunc
	searchFn  func(context.Context, string) (*site.Result, error)
}

type typeaheadTickMsg struct {
	gen int
}

type searchItem struct {
	book site.Book
}

func (i searchItem) Title() string {
	extra := ""
	parts := []string{}
	parts = append(parts, i.book.Author)
	if i.book.Status != "" {
		parts = append(parts, i.book.Status)
	}
	if i.book.Rating != "" {
		parts = append(parts, "★"+i.book.Rating)
	}
	extra = strings.Join(parts, " · ")
	if extra == "" {
		return i.book.Title
	}
	return i.book.Title + "  " + dimmed(extra)
}

func (i searchItem) Description() string {
	desc := i.book.Description
	if len([]rune(desc)) > 90 {
		desc = string([]rune(desc)[:90]) + "…"
	}
	return desc
}

func (i searchItem) FilterValue() string { return i.book.Title + " " + i.book.Author }

func (v *searchView) detailView(width, height int) string {
	if v.detailDirty || v.detailBuiltW != v.detailVp.Width {
		v.detailVp.SetContent(strings.Join(v.detailBody(width), "\n"))
		v.detailDirty = false
		v.detailBuiltW = v.detailVp.Width
	}
	header := titleStyle.Render("Book detail") + "  " +
		dimmed("↑/↓ j/k: scroll · enter: download · esc: back")
	// Reserve header, blank and footer rows; the viewport must not push the
	// footer off-screen.
	body := limitLines(v.detailVp.View(), max(height-3, 1))
	footer := lipgloss.NewStyle().Bold(true).
		Foreground(lipgloss.Color("232")).
		Background(lipgloss.Color("78")).
		Padding(0, 1).
		Render("Enter: download") + "  " +
		dimmed("Esc: back to list")
	return lipgloss.JoinVertical(lipgloss.Left, header, body, "", footer)
}

// detailBody assembles the detail content lines (scrolled via the viewport).
func (v *searchView) detailBody(width int) []string {
	d := v.detail
	lines := []string{
		"",
		d.book.Title,
	}
	meta := []string{}
	if d.book.Author != "" {
		meta = append(meta, "作者: "+d.book.Author)
	}
	if d.book.Publisher != "" {
		meta = append(meta, "出版社: "+d.book.Publisher)
	}
	if d.book.Status != "" {
		meta = append(meta, "状态: "+d.book.Status)
	}
	if d.book.Rating != "" {
		meta = append(meta, "评分: "+d.book.Rating)
	}
	if d.book.Tags != "" {
		meta = append(meta, "标签: "+d.book.Tags)
	}
	chapters := ""
	switch {
	case d.loading:
		chapters = "章节数: fetching…"
	case d.totalChapters >= 0:
		chapters = fmt.Sprintf("章节数: %d", d.totalChapters)
	case d.err != "":
		chapters = "章节数: unknown (" + d.err + ")"
	}
	meta = append(meta, chapters)
	if len(d.volumes) > 0 {
		meta = append(meta, fmt.Sprintf("卷数: %d", len(d.volumes)))
	}
	lines = append(lines, dimmed(strings.Join(meta, " · ")))
	// Description first (above volumes).
	if d.book.Description != "" {
		lines = append(lines, "", "简介")
		lines = append(lines, wrapWidth(d.book.Description, width-2)...)
	}
	if len(d.volumes) > 0 {
		lines = append(lines, "", titleStyle.Render("Volumes & chapters"))
		for _, vol := range d.volumes {
			name := vol.Title
			if name == "" {
				name = "卷"
			}
			lines = append(lines, fmt.Sprintf("・ %s — %d 章", name, len(vol.Chapters)))
			shown := chapterPreview
			if len(vol.Chapters) < shown {
				shown = len(vol.Chapters)
			}
			for i := 0; i < shown; i++ {
				t := vol.Chapters[i].Title
				if t == "" {
					t = fmt.Sprintf("第 %d 章", i+1)
				}
				lines = append(lines, "   "+dimmed(t))
			}
			if len(vol.Chapters) > shown {
				lines = append(lines, fmt.Sprintf("   … 还有 %d 章", len(vol.Chapters)-shown))
			}
		}
	}
	return lines
}

// dimmed is a light ANSI style helper (avoid lipgloss dependency per item).
func dimmed(s string) string {
	return "\x1b[38;5;245m" + s + "\x1b[0m"
}

// searchDetail is the detail page for one search result; totalChapters is
// -1 while unknown/fetching.
type searchDetail struct {
	book          site.Book
	totalChapters int
	volumes       []site.VolumeRef
	loading       bool
	err           string
}

func newSearchView(dl *site.Downloader) *searchView {
	v := &searchView{state: searchIdle, searchFn: site.Search, dl: dl}
	v.input = textinput.New()
	v.input.Placeholder = "search bilinovel.com… (results appear as you type)"
	v.input.Prompt = "> "
	v.input.Width = 60
	v.input.Focus()
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	v.spinner = sp
	v.list = list.New(nil, list.NewDefaultDelegate(), 80, 20)
	v.list.Title = "Results"
	v.syncDelegate()
	return v
}

// syncDelegate makes the result-list highlight invisible while the query
// input owns focus; the highlight only appears once ↓ moves focus to the
// list (same rule as arriving from another tab).
func (v *searchView) syncDelegate() {
	d := list.NewDefaultDelegate()
	if !v.input.Focused() {
		v.list.SetDelegate(d)
		return
	}
	d.Styles.SelectedTitle = d.Styles.NormalTitle
	d.Styles.SelectedDesc = d.Styles.NormalDesc
	v.list.SetDelegate(d)
}

func (v *searchView) resize(width, height int) {
	// Title, input and status rows live above the list.
	v.list.SetSize(width, max(height-3, 1))
	v.list.SetWidth(width)
	v.detailVp.Width = max(width-1, 1)
	v.detailVp.Height = max(height-2, 1)
	v.detailVp.SetYOffset(v.detailVp.YOffset)
}

// onChange schedules a debounced live search for q.
func (v *searchView) onChange(q string) tea.Cmd {
	v.pending = q
	v.tickGen++
	gen := v.tickGen
	return tea.Tick(typeaheadDelay,
		func(t time.Time) tea.Msg { return typeaheadTickMsg{gen: gen} })
}

// runSearch fires a search for q. The input stays focused so the user can
// keep refining the query; ↓ switches to the result list (where Enter
// downloads).
func (v *searchView) runSearch(q string) tea.Cmd {
	v.seq++
	gen := v.seq
	if v.cancel != nil {
		v.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	v.cancel = cancel
	v.state = searchSearching
	v.status = ""
	v.errText = ""
	v.pending = ""
	return tea.Batch(v.spinner.Tick, func() tea.Msg {
		res, err := v.searchFn(ctx, q)
		return searchResultMsg{gen: gen, q: q, res: res, err: err}
	})
}

// openDetail shows the detail page for a book and kicks off the (async)
// catalog fetch for its chapter count.
func (v *searchView) openDetail(b site.Book) (*searchView, tea.Cmd) {
	v.detail = &searchDetail{book: b, totalChapters: -1, loading: v.dl != nil}
	v.detailDirty = true
	v.input.Blur()
	v.syncDelegate()
	if v.dl == nil {
		return v, nil
	}
	id := site.SearchNovelID(b.URL)
	return v, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		vols, err := v.dl.FetchCatalog(ctx, id)
		if err != nil {
			return detailLoadedMsg{book: b, total: -1, err: err}
		}
		total := 0
		for _, vol := range vols {
			total += len(vol.Chapters)
		}
		return detailLoadedMsg{book: b, total: total, volumes: vols}
	}
}

// detailLoadedMsg carries the fetched chapter count (and volume layout) for
// a detail page.
type detailLoadedMsg struct {
	book    site.Book
	total   int
	volumes []site.VolumeRef
	err     error
}

func (v *searchView) applyDetail(m detailLoadedMsg) {
	if v.detail == nil || v.detail.book.URL != m.book.URL {
		return // stale (user navigated away)
	}
	v.detail.loading = false
	if m.err != nil {
		v.detail.err = m.err.Error()
		return
	}
	v.detail.totalChapters = m.total
	v.detail.volumes = m.volumes
	v.detailDirty = true
}

func (v *searchView) setResult(res *site.Result) {
	if res == nil {
		v.state = searchError
		v.errText = "search returned no usable response"
		return
	}
	v.state = searchResults
	v.status = fmt.Sprintf("%d result(s)", res.Total)
	if res.Suggestion != "" {
		v.status += " · did you mean: " + res.Suggestion
	}
	items := make([]list.Item, 0, len(res.Books))
	for _, b := range res.Books {
		items = append(items, searchItem{book: b})
	}
	v.list.SetItems(items)
	v.list.Select(0)
	v.syncDelegate()
}

func (v *searchView) setError(err error) {
	v.state = searchError
	v.errText = err.Error()
	v.status = "search failed"
}

func (v *searchView) Update(msg tea.Msg) (*searchView, tea.Cmd) {
	switch m := msg.(type) {
	case typeaheadTickMsg:
		// Only the newest scheduled tick may fire — a keystroke after this
		// tick was scheduled already scheduled (and will fire) a newer one.
		if m.gen != v.tickGen {
			return v, nil
		}
		if v.input.Focused() && v.pending != "" &&
			v.input.Value() == v.pending &&
			len([]rune(v.pending)) >= minQueryRunes {
			return v, v.runSearch(v.pending)
		}
		return v, nil
	case tea.KeyMsg:
		// Detail page mode owns the keyboard except for its action keys.
		if v.detail != nil {
			switch m.String() {
			case "enter":
				return v, func() tea.Msg { return startDownloadMsg{book: v.detail.book} }
			case "esc", "q":
				v.detail = nil
				return v, nil
			case "j":
				v.detailVp.ScrollDown(3)
				return v, nil
			case "k":
				v.detailVp.ScrollUp(3)
				return v, nil
			case "up":
				v.detailVp.ScrollUp(1)
				return v, nil
			case "down":
				v.detailVp.ScrollDown(1)
				return v, nil
			case "pgdown", "ctrl+d":
				v.detailVp.PageDown()
				return v, nil
			case "pgup", "ctrl+u":
				v.detailVp.PageUp()
				return v, nil
			case "g":
				v.detailVp.GotoTop()
				return v, nil
			case "G":
				v.detailVp.GotoBottom()
				return v, nil
			}
			return v, nil
		}
		switch m.String() {
		case "enter":
			if v.input.Focused() {
				q := strings.TrimSpace(v.input.Value())
				if q == "" {
					return v, nil
				}
				return v, v.runSearch(q)
			}
			if v.list.SelectedItem() != nil && v.state == searchResults {
				item := v.list.SelectedItem().(searchItem)
				return v.openDetail(item.book)
			}
			return v, nil
		case "down":
			if v.input.Focused() {
				// Blur to the result list (typeahead pattern). The list
				// cursor restarts at the first item — like a fresh search.
				v.input.Blur()
				v.list.Select(0) // resets both cursor and paginator page
				v.syncDelegate()
				return v, nil
			}
		case "up":
			// At the very first result of the first page, up returns to the
			// query input. Cursor() is page-relative, so the page matters:
			// from page 2's first item, up must go to page 1's last item.
			if !v.input.Focused() && v.list.Cursor() == 0 &&
				v.list.Paginator.OnFirstPage() {
				v.input.Focus()
				v.syncDelegate()
				return v, nil
			}
		case "esc":
			if v.detail != nil {
				v.detail = nil
				return v, nil
			}
			if !v.input.Focused() {
				v.input.Focus()
				v.syncDelegate()
				return v, nil
			}
			v.input.SetValue("")
			v.lastValue = ""
			v.pending = ""
			v.tickGen++
			v.state = searchIdle
			v.status = ""
			v.errText = ""
			v.list.SetItems(nil)
			return v, nil
		}
	}

	if v.input.Focused() {
		var cmd tea.Cmd
		v.input, cmd = v.input.Update(msg)
		q := v.input.Value()
		if q != v.lastValue {
			v.lastValue = q
			return v, tea.Batch(v.onChange(q), cmd)
		}
		return v, cmd
	}
	var cmd tea.Cmd
	v.list, cmd = v.list.Update(msg)
	return v, cmd
}

func (v *searchView) View(width, height int) string {
	if v.detail != nil {
		return v.detailView(width, height)
	}
	parts := []string{}
	parts = append(parts, titleStyle.Render("Online search — bilinovel.com"))
	parts = append(parts, v.input.View())
	switch v.state {
	case searchSearching:
		parts = append(parts, v.spinner.View()+" searching… ")
		parts = append(parts, dimmed(v.status))
	case searchError:
		parts = append(parts, lipgloss.NewStyle().Foreground(lipgloss.Color("196")).
			Render(v.errText))
	default:
		if v.status != "" {
			parts = append(parts, dimmed(v.status))
		}
	}
	// The result list is always rendered (kept visible while a newer search
	// is still running).
	if v.state != searchIdle || len(v.list.Items()) > 0 {
		parts = append(parts, v.list.View())
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
