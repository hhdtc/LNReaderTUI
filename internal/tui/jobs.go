package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"lnreadertui/internal/model"
	"lnreadertui/internal/pipeline"
	"lnreadertui/internal/site"
)

type jobStatus string

const (
	statusRunning jobStatus = "running"
	statusDone    jobStatus = "done"
	statusFailed  jobStatus = "failed"
	statusStopped jobStatus = "cancelled"
)

type job struct {
	id       string
	novelID  string
	srcURL   string
	title    string
	status   jobStatus
	done     int
	total    int
	cur      string
	volume   string
	err      string
	outPath  string
	imported bool
	cancel   context.CancelFunc
}

type jobItem struct {
	j *job
}

func (i jobItem) Title() string {
	st := string(i.j.status)
	switch i.j.status {
	case statusRunning:
		st = runningStyle.Render(st)
	case statusDone:
		st = doneStyle.Render(st)
	case statusFailed:
		st = failedStyle.Render(st)
	case statusStopped:
		st = dimmed(st)
	}
	return fmt.Sprintf("%-9s %s", st, i.j.title)
}

func (i jobItem) Description() string {
	j := i.j
	switch j.status {
	case statusRunning:
		if j.total > 0 {
			bar := progressBarText(j.done, j.total, 30)
			cur := j.cur
			if j.volume != "" {
				cur = j.volume + " · " + cur
			}
			return fmt.Sprintf("%s %3d/%d  %s", bar, j.done, j.total, cur)
		}
		if j.cur != "" {
			return "fetching " + j.cur + "…"
		}
		return "starting…"
	case statusDone:
		if j.imported {
			return "imported into library"
		}
		return "ready — press enter to import into your library"
	case statusFailed:
		return j.err
	case statusStopped:
		return "cancelled — press d to dismiss"
	}
	return ""
}

func (i jobItem) FilterValue() string { return i.j.title }

func progressBarText(done, total, width int) string {
	if total <= 0 {
		return strings.Repeat("·", width)
	}
	pct := float64(done) / float64(total)
	fill := int(pct * float64(width))
	if fill > width {
		fill = width
	}
	return progressBar.Render(strings.Repeat("█", fill)) +
		strings.Repeat("░", width-fill)
}

var (
	runningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("75"))
	doneStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("78"))
	failedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

type jobsView struct {
	store  *model.Store
	list   list.Model
	items  []*job
	seq    atomic.Int64
	events chan jobProgressMsg
}

func newJobsView(store *model.Store) *jobsView {
	v := &jobsView{store: store, events: make(chan jobProgressMsg, 128)}
	v.list = list.New(nil, list.NewDefaultDelegate(), 80, 20)
	v.list.Title = "Downloads"
	return v
}

func (v *jobsView) resize(width, height int) {
	v.list.SetSize(width, height)
	v.list.SetWidth(width)
}

func (v *jobsView) find(id string) *job {
	for _, j := range v.items {
		if j.id == id {
			return j
		}
	}
	return nil
}

func (v *jobsView) refresh() {
	items := make([]list.Item, 0, len(v.items))
	for _, j := range v.items {
		items = append(items, jobItem{j: j})
	}
	v.list.SetItems(items)
}

// start creates and launches a download job for a search result.
func (v *jobsView) start(b site.Book, dl *site.Downloader) {
	novelID := site.SearchNovelID(b.URL)
	if novelID == "" {
		j := &job{id: nextJobID(&v.seq), title: b.Title, status: statusFailed,
			err: "cannot extract novel id from URL"}
		v.items = append(v.items, j)
		v.refresh()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	j := &job{
		id:      nextJobID(&v.seq),
		novelID: novelID,
		srcURL:  b.URL,
		title:   b.Title,
		status:  statusRunning,
		cancel:  cancel,
	}
	v.items = append(v.items, j)
	v.refresh()

	outDir := filepath.Join(os.TempDir(), "lnreadertui-downloads")
	events := v.events
	go func() {
		_ = os.MkdirAll(outDir, 0o755)
		outPath := filepath.Join(outDir, j.id+".epub")
		_, err := pipeline.FetchNovel(ctx, dl, j.novelID, j.srcURL,
			func(p pipeline.Progress) {
				postJobEvent(ctx, events, jobProgressMsg{
					id: j.id, done: p.Done, total: p.Total,
					title: p.Title, volume: p.Volume})
			}, outPath)
		postJobEvent(ctx, events, jobProgressMsg{
			id: j.id, finished: true, err: err, bundlePath: outPath})
	}()
}

func nextJobID(seq *atomic.Int64) string {
	return fmt.Sprintf("job-%d", seq.Add(1))
}

// postJobEvent forwards progress into the channel; the root drains it with
// a poll command. Never touches bubbles state from the goroutine.
func postJobEvent(ctx context.Context, ch chan<- jobProgressMsg, m jobProgressMsg) {
	select {
	case ch <- m:
	case <-ctx.Done():
	}
}

// apply updates job state and returns a command when completion needs an
// action (auto-import of a finished download).
func (v *jobsView) apply(m jobProgressMsg) tea.Cmd {
	j := v.find(m.id)
	if j == nil {
		return nil
	}
	if m.finished {
		j.outPath = m.bundlePath
		if m.err != nil {
			j.status = statusFailed
			j.err = m.err.Error()
		} else {
			j.status = statusDone
		}
		v.refresh()
		// Auto-import: a completed download belongs in the library without
		// requiring a manual Enter.
		if m.err == nil && !j.imported {
			return v.importJob(j)
		}
		return nil
	}
	j.done = m.done
	j.total = m.total
	j.cur = m.title
	j.volume = m.volume
	v.refresh()
	return nil
}

// markImported records a successful registration (called when the import
// command's result lands).
func (v *jobsView) markImported(id string) {
	if j := v.find(id); j != nil {
		j.imported = true
		v.refresh()
	}
}

func (v *jobsView) Update(msg tea.Msg) (*jobsView, tea.Cmd) {
	switch m := msg.(type) {
	case tea.MouseMsg:
		if m.Y == 0 {
			return v, nil
		}
		if mouseList(m, &v.list, 3, 2) {
			return v, nil
		}
		return v, nil
	case tea.KeyMsg:
		switch m.String() {
		case "x":
			if item := v.list.SelectedItem(); item != nil {
				if j := item.(jobItem).j; j.status == statusRunning && j.cancel != nil {
					j.cancel()
					j.status = statusStopped
					v.refresh()
				}
			}
			return v, nil
		case "d":
			if item := v.list.SelectedItem(); item != nil {
				if j := item.(jobItem).j; j.status != statusRunning {
					v.remove(j)
				}
			}
			return v, nil
		case "enter":
			if item := v.list.SelectedItem(); item != nil {
				if j := item.(jobItem).j; j.status == statusDone && !j.imported {
					return v, v.importJob(j)
				}
			}
			return v, nil
		}
	}
	var cmd tea.Cmd
	v.list, cmd = v.list.Update(msg)
	return v, cmd
}

func (v *jobsView) remove(j *job) {
	for i, it := range v.items {
		if it == j {
			v.items = append(v.items[:i], v.items[i+1:]...)
			break
		}
	}
	v.refresh()
}

func (v *jobsView) importJob(j *job) tea.Cmd {
	return func() tea.Msg {
		b, err := v.store.RegisterDownloaded(j.outPath, "", "", j.novelID, j.srcURL, j.total)
		if err != nil {
			return jobImportedMsg{id: j.id, err: err}
		}
		return jobImportedMsg{id: j.id, book: b}
	}
}

func (v *jobsView) View(width, height int) string {
	if len(v.items) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left,
			titleStyle.Render("Downloads"),
			dimmed("No downloads yet — use the Search tab to find a novel."),
		)
	}
	return v.list.View()
}
