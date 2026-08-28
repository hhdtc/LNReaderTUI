// Command lnreadertui is a terminal-based EPUB library: search bilinovel.com
// for novels, download them as EPUBs, read them locally, and keep per-book
// reading progress.
//
// Besides the interactive TUI it exposes machine-friendly subcommands so
// agents can drive the full loop: search, detail, download, and local book
// management.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	term "github.com/charmbracelet/x/term"

	tea "github.com/charmbracelet/bubbletea"

	"lnreadertui/internal/model"
	"lnreadertui/internal/pipeline"
	"lnreadertui/internal/site"
	"lnreadertui/internal/tui"
)

// version is stamped at build time via -ldflags "-X main.version=<ver>".
var version = "dev"

func main() {
	dataDirFlag := flag.String("data-dir", defaultDataDir(),
		"directory for the library index and imported books")
	importFlag := flag.String("import", "",
		"legacy: import .epub/.txt files (or a directory) at startup, then exit")
	versionFlag := flag.Bool("version", false, "print version and exit")
	flag.Usage = usage
	flag.Parse()

	if *versionFlag {
		fmt.Printf("lnreadertui %s\n", version)
		return
	}

	// Legacy one-shot import flag.
	if *importFlag != "" {
		store, err := model.Open(*dataDirFlag)
		if err != nil {
			fatal(err)
		}
		if err := runImport(store, []string{*importFlag}, false); err != nil {
			fatal(err)
		}
		return
	}

	args := flag.Args()
	if len(args) == 0 {
		if !isTerminal(os.Stdout) {
			fmt.Fprintln(os.Stderr,
				"lnreadertui: interactive mode requires a VT terminal; use a subcommand (search|detail|download|books|import) or --help")
			return
		}
		store, err := model.Open(*dataDirFlag)
		if err != nil {
			fatal(err)
		}
		dl, err := site.NewDownloader()
		if err != nil {
			fatal(err)
		}
		p := tui.New(store, dl, *dataDirFlag)
		if err := runBubbletea(p); err != nil {
			fatal(err)
		}
		return
	}

	store, err := model.Open(*dataDirFlag)
	if err != nil {
		fatal(err)
	}

	switch args[0] {
	case "search":
		cmd := flag.NewFlagSet("search", flag.ExitOnError)
		asJSON := cmd.Bool("json", false, "print JSON instead of readable text")
		_ = cmd.Parse(reorderMixedFlags(cmd, args[1:]))
		query := strings.Join(cmd.Args(), " ")
		if query == "" {
			fatal(fmt.Errorf("usage: lnreadertui search <query> [--json]"))
		}
		runSearch(query, *asJSON)

	case "detail":
		cmd := flag.NewFlagSet("detail", flag.ExitOnError)
		asJSON := cmd.Bool("json", false, "print JSON instead of readable text")
		_ = cmd.Parse(reorderMixedFlags(cmd, args[1:]))
		key := strings.Join(cmd.Args(), " ")
		if key == "" {
			fatal(fmt.Errorf("usage: lnreadertui detail <query|novel-id|url> [--json]"))
		}
		runDetail(key, *asJSON)

	case "download":
		cmd := flag.NewFlagSet("download", flag.ExitOnError)
		asJSON := cmd.Bool("json", false, "print JSON instead of readable text")
		out := cmd.String("out", "", "write the EPUB here instead of the library")
		_ = cmd.Parse(reorderMixedFlags(cmd, args[1:]))
		key := strings.Join(cmd.Args(), " ")
		if key == "" {
			fatal(fmt.Errorf("usage: lnreadertui download <query|novel-id|url> [--out PATH] [--json]"))
		}
		runDownload(store, filepath.Dir(*dataDirFlag), key, *out, *asJSON)

	case "books":
		cmd := flag.NewFlagSet("books", flag.ExitOnError)
		asJSON := cmd.Bool("json", false, "print JSON instead of readable text")
		_ = cmd.Parse(reorderMixedFlags(cmd, args[1:]))
		sub := cmd.Arg(0)
		var key string
		if rest := cmd.Args(); len(rest) > 1 {
			key = strings.Join(rest[1:], " ")
		}
		runBooks(store, sub, key, *asJSON)

	case "import":
		cmd := flag.NewFlagSet("import", flag.ExitOnError)
		asJSON := cmd.Bool("json", false, "print JSON instead of readable text")
		_ = cmd.Parse(reorderMixedFlags(cmd, args[1:]))
		if len(cmd.Args()) == 0 {
			fatal(fmt.Errorf("usage: lnreadertui import <path...>"))
		}
		if err := runImport(store, cmd.Args(), *asJSON); err != nil {
			fatal(err)
		}

	default:
		usage()
		os.Exit(2)
	}
}

// isTerminal reports whether f is a terminal device. The bubbletea TUI cannot
// run in headless contexts (CI sandboxes, piped output): opening a TTY fails
// and the process exits non-zero, which breaks headless launch probes.
func isTerminal(f *os.File) bool {
	return term.IsTerminal(f.Fd())
}

// reorderMixedFlags lets flags appear anywhere on the command line (not just
// before the positional args, which is Go's default).
func reorderMixedFlags(fs *flag.FlagSet, args []string) []string {
	needsValue := map[string]bool{} // true: next arg is the flag's value
	known := map[string]bool{}
	fs.VisitAll(func(f *flag.Flag) {
		bv, ok := f.Value.(interface{ IsBoolFlag() bool })
		isBool := ok && bv.IsBoolFlag()
		needsValue["--"+f.Name] = !isBool
		known["--"+f.Name] = true
		if len(f.Name) == 1 {
			needsValue["-"+f.Name] = !isBool
			known["-"+f.Name] = true
		}
	})
	var pre, post []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if name, _, hasEq := strings.Cut(a, "="); hasEq && known[name] {
			pre = append(pre, a)
			continue
		}
		if needsValue[a] {
			pre = append(pre, a)
			if i+1 < len(args) {
				pre = append(pre, args[i+1])
				i++
			}
			continue
		}
		if known[a] { // boolean flag
			pre = append(pre, a)
			continue
		}
		post = append(post, a)
	}
	return append(pre, post...)
}

func usage() {
	fmt.Fprint(os.Stderr, `Usage of lnreadertui:

  lnreadertui [flags]                     start the interactive TUI
  lnreadertui search <query> [--json]     search bilinovel.com
  lnreadertui detail <query|id|url> [--json]
                                          book detail incl. volumes & chapters
  lnreadertui download <query|id|url> [--out PATH] [--json]
                                          download a novel as EPUB; the book is
                                          registered in the library unless --out
                                          is given (runs in the foreground,
                                          cancel with Ctrl+C)
  lnreadertui books list [--json]         list local books with progress
  lnreadertui books show <id|title>       show one local book
  lnreadertui books delete <id|title>     delete a local book (+file)
  lnreadertui books reset <id|title>      reset reading progress
  lnreadertui import <path...> [--json]   import .epub/.txt files or directories

Flags:
  -data-dir string   library directory (default `+defaultDataDir()+`)
  -import string     legacy one-shot import
`)
}

// runBubbletea is a seam for tests.
var runBubbletea = func(p *tui.App) error {
	return runApp(p)
}

// runApp wraps the bubbletea program; separated so tests can stub it.
var runApp = func(p *tui.App) error {
	prog := tea.NewProgram(p, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := prog.Run()
	return err
}

// ---------- search ----------

func runSearch(query string, asJSON bool) {
	ctx := ctxWithInterrupt()
	res, err := site.Search(ctx, query)
	if err != nil {
		fatal(err)
	}
	if asJSON {
		printJSON(res)
		return
	}
	if res.Total == 0 {
		fmt.Println("no results")
		return
	}
	if res.Suggestion != "" {
		fmt.Printf("did you mean: %s\n", res.Suggestion)
	}
	for i, b := range res.Books {
		fmt.Printf("%d. %s\n   %s | %s | ★%s | %s\n   %s\n",
			i+1, b.Title, b.Author, b.Status, b.Rating, b.URL, oneLine(b.Description))
	}
}

// ---------- detail ----------

func runDetail(key string, asJSON bool) {
	ctx := ctxWithInterrupt()
	book, err := resolveNovel(ctx, key)
	if err != nil {
		fatal(err)
	}
	dl, err := site.NewDownloader()
	if err != nil {
		fatal(err)
	}
	id := site.SearchNovelID(book.URL)
	// Id/url lookups skip search metadata; fetch the novel page.
	if book.Title == "" {
		if meta, err := dl.FetchNovel(ctx, id); err == nil {
			book.Title = meta.Title
			book.Author = meta.Author
			book.Description = meta.Description
			book.Status = meta.Status
		}
	}
	volumes, err := dl.FetchCatalog(ctx, id)
	if err != nil {
		fatal(err)
	}
	total := 0
	for _, v := range volumes {
		total += len(v.Chapters)
	}
	if asJSON {
		printJSON(struct {
			site.Book
			TotalChapters int              `json:"totalChapters"`
			Volumes       []site.VolumeRef `json:"volumes"`
		}{book, total, volumes})
		return
	}
	fmt.Printf("%s\n作者: %s | 状态: %s | 评分: %s | 出版社: %s | 标签: %s\n章节数: %d | 卷数: %d\n\n",
		book.Title, book.Author, book.Status, book.Rating, book.Publisher,
		book.Tags, total, len(volumes))
	if book.Description != "" {
		fmt.Printf("简介\n%s\n\n", book.Description)
	}
	for _, v := range volumes {
		fmt.Printf("・ %s — %d 章\n", v.Title, len(v.Chapters))
	}
}

// resolveNovel turns a query / novel id / URL into a search result book.
func resolveNovel(ctx context.Context, key string) (site.Book, error) {
	key = strings.TrimSpace(key)
	if site.SearchNovelID(key) != "" {
		return site.Book{URL: key}, nil
	}
	if isDigits(key) {
		return site.Book{URL: "https://www.bilinovel.com/novel/" + key + ".html"}, nil
	}
	res, err := site.Search(ctx, key)
	if err != nil {
		return site.Book{}, err
	}
	if res.Total == 0 {
		return site.Book{}, fmt.Errorf("no results for %q", key)
	}
	return res.Books[0], nil
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

// ---------- download ----------

func runDownload(store *model.Store, absDataDir, key, outPath string, asJSON bool) {
	ctx := ctxWithInterrupt()
	book, err := resolveNovel(ctx, key)
	if err != nil {
		fatal(err)
	}
	id := site.SearchNovelID(book.URL)
	if id == "" {
		fatal(fmt.Errorf("cannot extract novel id from %q", book.URL))
	}
	dl, err := site.NewDownloader()
	if err != nil {
		fatal(err)
	}

	if outPath == "" {
		// Temporary staging (same convention as the TUI), then register.
		outDir := filepath.Join(os.TempDir(), "lnreadertui-downloads")
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			fatal(err)
		}
		outPath = filepath.Join(outDir, "cli-"+id+".epub")
		deleteAfter := true
		defer func() {
			if deleteAfter {
				_ = os.Remove(outPath)
			}
		}()
		bundle, err := pipeline.FetchNovel(ctx, dl, id, book.URL,
			func(p pipeline.Progress) { progressLine(&p) }, outPath)
		if err != nil {
			fatal(err)
		}
		reg := "registered in library"
		b, err := store.RegisterDownloaded(outPath, bundle.Title, bundle.Author,
			id, book.URL, len(bundle.Chapters))
		if err != nil {
			fatal(err)
		}
		deleteAfter = true // already copied into the library
		_ = b
		_ = reg
		if asJSON {
			printJSON(struct {
				Title    string `json:"title"`
				Author   string `json:"author"`
				ID       string `json:"id"`
				Chapters int    `json:"chapters"`
				SavedTo  string `json:"savedTo"`
			}{bundle.Title, bundle.Author, b.ID, len(bundle.Chapters), store.FilePath(b)})
			return
		}
		fmt.Printf("saved %q by %q (%d chapters) to library [%s]\n",
			bundle.Title, bundle.Author, len(bundle.Chapters), store.FilePath(b))
		return
	}

	// Explicit output path: just write the EPUB, no library registration.
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		fatal(err)
	}
	bundle, err := pipeline.FetchNovel(ctx, dl, id, book.URL,
		func(p pipeline.Progress) { progressLine(&p) }, outPath)
	if err != nil {
		fatal(err)
	}
	if asJSON {
		printJSON(struct {
			Title    string `json:"title"`
			Author   string `json:"author"`
			Chapters int    `json:"chapters"`
			OutPath  string `json:"outPath"`
		}{bundle.Title, bundle.Author, len(bundle.Chapters), outPath})
		return
	}
	fmt.Printf("saved %q by %q (%d chapters) to %s\n",
		bundle.Title, bundle.Author, len(bundle.Chapters), outPath)
}

// progressLine prints a compact progress line.
func progressLine(p *pipeline.Progress) {
	if p.Err != nil {
		return
	}
	if p.Volume != "" && p.Title != "" && p.Done > 0 {
		fmt.Fprintf(os.Stderr, "\r[%3d/%d] %s · %s       ",
			p.Done, p.Total, p.Volume, p.Title)
		return
	}
	if p.Done > 0 {
		fmt.Fprintf(os.Stderr, "\r[%3d/%d] %s       ", p.Done, p.Total, p.Title)
		return
	}
	fmt.Fprintf(os.Stderr, "\r%s…       ", p.Title)
}

// ---------- books ----------

func runBooks(store *model.Store, sub, key string, asJSON bool) {
	switch sub {
	case "list":
		if asJSON {
			printJSON(store.Books())
			return
		}
		books := store.Books()
		if len(books) == 0 {
			fmt.Println("library is empty")
			return
		}
		for _, b := range books {
			p := b.Progress
			line := fmt.Sprintf("%s | %s | %d chapters", b.ID, b.Title, b.Chapters)
			if p.LastReadAt != "" {
				line += fmt.Sprintf(" | %d/%d | %s %.0f%% | %s",
					p.ChapterIndex+1, b.Chapters, p.ChapterTitle, p.Offset*100, p.LastReadAt)
			} else {
				line += " | unread"
			}
			fmt.Println(line)
		}
	case "show":
		b, err := findBook(store, key)
		if err != nil {
			fatal(err)
		}
		if asJSON {
			printJSON(b)
			return
		}
		p := b.Progress
		fmt.Printf("%s | %s | %s\nfile: %s\n", b.ID, b.Title, b.Author, store.FilePath(b))
		if p.LastReadAt != "" {
			fmt.Printf("progress: %d/%d %s %.0f%% last read %s\n",
				p.ChapterIndex+1, b.Chapters, p.ChapterTitle, p.Offset*100, p.LastReadAt)
		} else {
			fmt.Println("progress: unread")
		}
	case "delete":
		b, err := findBook(store, key)
		if err != nil {
			fatal(err)
		}
		if err := store.Delete(b.ID); err != nil {
			fatal(err)
		}
		if asJSON {
			printJSON(map[string]string{"deleted": b.ID})
			return
		}
		fmt.Printf("deleted %s\n", b.Title)
	case "reset":
		b, err := findBook(store, key)
		if err != nil {
			fatal(err)
		}
		if err := store.ResetProgress(b.ID); err != nil {
			fatal(err)
		}
		if asJSON {
			printJSON(map[string]string{"reset": b.ID})
			return
		}
		fmt.Printf("progress reset: %s\n", b.Title)
	default:
		fatal(fmt.Errorf("usage: lnreadertui books list|show|delete|reset [id|title]"))
	}
}

func findBook(store *model.Store, key string) (*model.Book, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, fmt.Errorf("usage: lnreadertui books show|delete|reset <id|title>")
	}
	if b := store.Find(key); b != nil {
		return b, nil
	}
	for _, b := range store.Books() {
		if strings.Contains(b.Title, key) {
			return b, nil
		}
	}
	return nil, fmt.Errorf("no book matching %q", key)
}

// ---------- import ----------

func runImport(store *model.Store, paths []string, asJSON bool) error {
	var books []*model.Book
	for _, p := range paths {
		bs, err := store.ImportFile(p)
		if err != nil {
			return err
		}
		books = append(books, bs...)
	}
	if asJSON {
		printJSON(books)
		return nil
	}
	for _, b := range books {
		fmt.Printf("imported %s — %s (%d chapters)\n", b.Title, orUnknown(b.Author), b.Chapters)
	}
	return nil
}

// ---------- helpers ----------

// ctxWithInterrupt returns a context cancelled by Ctrl+C (SIGINT) / SIGTERM,
// so CLI downloads cancel cleanly.
func ctxWithInterrupt() context.Context {
	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ctx.Done()
		stop()
	}()
	return ctx
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len([]rune(s)) > 120 {
		s = string([]rune(s)[:120]) + "…"
	}
	return s
}

func orUnknown(s string) string {
	if s == "" {
		return "Unknown"
	}
	return s
}

func defaultDataDir() string {
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "lnreadertui")
	}
	if runtime.GOOS == "windows" {
		if la := os.Getenv("LOCALAPPDATA"); la != "" {
			return filepath.Join(la, "lnreadertui")
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "data"
	}
	return filepath.Join(home, ".local", "share", "lnreadertui")
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "lnreadertui:", err)
	os.Exit(1)
}
