package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"lnreadertui/internal/model"
	"lnreadertui/internal/site"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	store, err := model.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := New(store, nil, t.TempDir())
	m, _ := a.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return m.(*App)
}

// keyRunes emulates typing a character (e.g. a digit shortcut).
func keyRunes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// keyArrow emulates a navigation key (right/left/ctrl+c/...).
func keyArrow(t tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: t}
}

func update(t *testing.T, a *App, msg tea.Msg) *App {
	t.Helper()
	m, _ := a.Update(msg)
	return m.(*App)
}

func quits(t *testing.T, a *App, msg tea.Msg) bool {
	t.Helper()
	_, cmd := a.Update(msg)
	return cmd != nil &&
		reflect.ValueOf(cmd).Pointer() == reflect.ValueOf(tea.Quit).Pointer()
}

// Arrow keys move the tab selection (no text input focused).
func TestArrowNavigation(t *testing.T) {
	a := newTestApp(t)
	a = update(t, a, keyArrow(tea.KeyRight))
	if a.view != viewSearch {
		t.Fatalf("right from library: view = %d, want search", a.view)
	}
	a = update(t, a, keyArrow(tea.KeyRight))
	if a.view != viewJobs {
		t.Fatalf("right twice: view = %d, want jobs", a.view)
	}
	a = update(t, a, keyArrow(tea.KeyRight))
	if a.view != viewLibrary {
		t.Fatalf("right thrice: view = %d, want library (wrap)", a.view)
	}
	a = update(t, a, keyArrow(tea.KeyLeft))
	if a.view != viewJobs {
		t.Fatalf("left: view = %d, want jobs", a.view)
	}
	// Digits jump tabs only while no input is focused (typing a digit in the
	// query keeps it in the input).
	a = update(t, a, keyRunes("2"))
	if a.view != viewSearch {
		t.Fatalf("key 2: view = %d, want search", a.view)
	}
	// Typed digits go into the (auto-focused) query; to jump tabs the digit
	// must not be typing — blur first.
	a.search.input.Blur()
	a = update(t, a, keyRunes("3"))
	if a.view != viewJobs {
		t.Fatalf("key 3: view = %d, want jobs", a.view)
	}
}

// While a text input is focused, arrows edit the input and must not switch
// tabs; q/ctrl+c must not quit.
func TestTypingGuardsNavigation(t *testing.T) {
	a := newTestApp(t)
	a = update(t, a, keyArrow(tea.KeyRight)) // Search tab; input focused by default

	a = update(t, a, keyRunes("q"))
	if a.view != viewSearch {
		t.Fatalf("q with input focused changed view: %d", a.view)
	}
	if a.search.input.Value() != "q" {
		t.Fatalf("q not typed into input: %q", a.search.input.Value())
	}

	// Arrows navigate even while the input is focused (by design).
	a = update(t, a, keyArrow(tea.KeyRight))
	if a.view != viewJobs {
		t.Fatalf("right with input focused: view = %d, want jobs", a.view)
	}
	if a.search.input.Value() != "q" {
		t.Fatalf("input lost its typed value: %q", a.search.input.Value())
	}
	a = update(t, a, keyArrow(tea.KeyLeft))
	if a.view != viewSearch {
		t.Fatalf("left: view = %d, want search", a.view)
	}

	if !quits(t, a, keyArrow(tea.KeyCtrlC)) {
		t.Fatal("ctrl+c must always quit")
	}

	// Blur the input (as runSearch does), then q must quit.
	a.search.input.Blur()
	if !quits(t, a, keyRunes("q")) {
		t.Fatal("q did not quit with input blurred")
	}
}

// The navigation bar is rendered in every tab state, including mid-search.
func TestNavBarAlwaysVisible(t *testing.T) {
	a := newTestApp(t)
	a = update(t, a, keyArrow(tea.KeyRight)) // Search
	a.search.input.Blur()
	a = update(t, a, keyArrow(tea.KeyLeft))  // back to Library
	a = update(t, a, keyArrow(tea.KeyRight)) // Search again
	a.search.state = searchSearching
	v := a.View()
	for _, want := range []string{"1 Library", "2 Search", "3 Downloads", "←/→ or Tab: switch"} {
		if !strings.Contains(v, want) {
			t.Fatalf("nav bar missing %q in\n%s", want, v)
		}
	}
	if !strings.Contains(v, "searching") {
		t.Fatalf("search state not rendered:\n%s", v)
	}
}

// ctrl+c must quit from any tab view when no input owns the keyboard.
func TestCtrlCQuitsFromTabs(t *testing.T) {
	for _, v := range []viewKind{viewLibrary, viewSearch, viewJobs} {
		a := newTestApp(t)
		a.view = v
		_, cmd := a.Update(keyArrow(tea.KeyCtrlC))
		if cmd == nil || reflect.ValueOf(cmd).Pointer() != reflect.ValueOf(tea.Quit).Pointer() {
			t.Fatalf("ctrl+c from view %d did not quit", v)
		}
	}
}

// Every tab frame must fit the terminal: the nav bar and input rows can
// never overflow the screen.
func TestTabFramesFitTerminal(t *testing.T) {
	a := newTestApp(t) // 120x40
	for _, v := range []viewKind{viewLibrary, viewSearch, viewJobs} {
		a.view = v
		lines := strings.Split(a.View(), "\n")
		if len(lines) > 40 {
			t.Fatalf("view %d frame is %d lines, terminal is 40:\n%s",
				v, len(lines), a.View())
		}
		// Nav bar and footer must be present in every frame.
		if !strings.Contains(a.View(), "1 Library") ||
			!strings.Contains(a.View(), "2 Search") {
			t.Fatalf("view %d missing nav bar", v)
		}
	}
}

// After a live search fires, the input row and nav bar must still be
// visible with results below.
func TestSearchFrameAfterLiveSearch(t *testing.T) {
	f := &fakeSearch{results: map[string]*site.Result{"苍穹女武神": oneResult("苍穹女武神")}}
	a := searchViewTestApp(t, f)
	a = drain(a, keyRunes("苍穹女武神"))
	a = drain(a, typeaheadTickMsg{})
	if a.search.state != searchResults {
		t.Fatalf("state = %d, want results", a.search.state)
	}
	v := a.View()
	if !strings.Contains(v, "> 苍穹女武神") {
		t.Fatalf("input row missing after live search:\n%s", v)
	}
	if !strings.Contains(v, "1 Library") || !strings.Contains(v, "2 Search") {
		t.Fatalf("nav bar missing after live search:\n%s", v)
	}
	if !strings.Contains(v, "苍穹女武神") {
		t.Fatalf("results missing:\n%s", v)
	}
	if lines := strings.Split(v, "\n"); len(lines) > 40 {
		t.Fatalf("frame too tall: %d lines", len(lines))
	}
}

// Switching to the Search tab always refocuses the input.
func TestTabSwitchFocusesSearchInput(t *testing.T) {
	a := newTestApp(t)
	a = update(t, a, keyArrow(tea.KeyRight)) // Search; input focused
	a = update(t, a, keyArrow(tea.KeyDown))  // blur to list
	if a.search.input.Focused() {
		t.Fatal("input should be blurred after down")
	}
	a = update(t, a, keyArrow(tea.KeyRight)) // to Downloads
	a = update(t, a, keyArrow(tea.KeyLeft))  // back to Search
	if !a.search.input.Focused() {
		t.Fatal("input not focused on return to Search")
	}
	a = update(t, a, keyArrow(tea.KeyLeft)) // wrap to Downloads? left -> library? no: left from search = library
	if a.view != viewLibrary {
		t.Fatalf("left from search went to %d", a.view)
	}
	a = update(t, a, keyRunes("2")) // digit jump to Search
	if !a.search.input.Focused() {
		t.Fatal("input not focused after digit jump")
	}
}

// The library filter must open on '/' (via the app's full key path).
func TestLibraryFilterKeyOpens(t *testing.T) {
	a := newTestApp(t)
	m, _ := a.Update(keyRunes("/"))
	a = m.(*App)
	if !a.library.filtering {
		t.Fatal("'/' did not open the library filter")
	}
	if !a.library.filter.Focused() {
		t.Fatal("filter input not focused")
	}
	// Typing goes to the filter, enter applies it.
	m, _ = a.Update(keyRunes("长"))
	a = m.(*App)
	if a.library.filter.Value() != "长" {
		t.Fatalf("typing after '/' went elsewhere: value=%q", a.library.filter.Value())
	}
	m, _ = a.Update(keyRunes("文"))
	a = m.(*App)
	m, _ = a.Update(keyArrow(tea.KeyEnter))
	a = m.(*App)
	if a.library.filtering {
		t.Fatal("enter did not apply filter")
	}
}

// A pasted/fast-typed burst arrives as one KeyMsg with several runes; it must
// behave exactly like the individual keystrokes.
func TestRuneBurstSplits(t *testing.T) {
	a := newTestApp(t)
	// Burst "/长文" in library: '/' opens the filter and the rest lands in it.
	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/长文")})
	a = m.(*App)
	if !a.library.filtering {
		t.Fatal("burst '/' did not open filter")
	}
	if a.library.filter.Value() != "长文" {
		t.Fatalf("burst text not in filter: %q", a.library.filter.Value())
	}

	// Burst "jj" in the search input lands both chars in the input.
	a.view = viewSearch
	a.search.input.Focus()
	m, _ = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("jj")})
	a = m.(*App)
	if a.search.input.Value() != "jj" {
		t.Fatalf("burst in search input: %q", a.search.input.Value())
	}
}
