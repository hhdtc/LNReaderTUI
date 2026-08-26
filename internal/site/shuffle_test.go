package site

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func pNode(text string) *html.Node {
	return &html.Node{Type: html.ElementNode, Data: "p",
		FirstChild: &html.Node{Type: html.TextNode, Data: text}}
}

func TestParsePlainTemplate(t *testing.T) {
	js := `
var x = 1;
function restore() {
  var paras = [];
  if (len > 20) {
    // shuffle
    var seed = Null | 0;
    seed = Number(chapterId) * 4 + 6;
    var t = (seed * 5 + 3) % 97;
    // swap loop
  }
}
`
	tmpl := parseShuffleTemplate(js)
	if tmpl == nil {
		t.Fatal("template not parsed")
	}
	if tmpl.fixedLength != 20 {
		t.Fatalf("fixedLength = %d, want 20", tmpl.fixedLength)
	}
	if tmpl.seedMultiplier != 4 || tmpl.seedOffset != 6 {
		t.Fatalf("seed params = (%d,%d), want (4,6)", tmpl.seedMultiplier, tmpl.seedOffset)
	}
	if tmpl.a != 5 || tmpl.c != 3 || tmpl.mod != 97 {
		t.Fatalf("lcg params = (%d,%d,%d), want (5,3,97)", tmpl.a, tmpl.c, tmpl.mod)
	}
	p := tmpl.paramsFor(15)
	if p.seed != 15*4+6 {
		t.Fatalf("seed = %d, want %d", p.seed, 15*4+6)
	}
}

func TestEvalJSArith(t *testing.T) {
	cases := map[string]int{
		"20":    20,
		"(3*7)": 21,
		"6*2+1": 13,
		"1<<4":  16,
		"~0":    -1,
		"7%3":   1,
	}
	for expr, want := range cases {
		got, ok := evalInt(expr)
		if !ok || got != want {
			t.Errorf("evalInt(%q) = %d,%v; want %d,true", expr, got, ok, want)
		}
	}
	if got, ok := evalExprWithVars("(seed*5+3)%97", map[string]int{"seed": 66}); !ok || got != 42 {
		t.Errorf("evalExprWithVars seed=66 = %d,%v; want 42,true", got, ok)
	}
	if _, ok := evalInt("foo(1)"); ok {
		t.Error("evalInt accepted a function call")
	}
}

// The shuffle must be a deterministic permutation preserving the first
// fixedLength paragraphs, then the LCG swaps per the reference algorithm.
func TestRestoreParagraphs(t *testing.T) {
	paras := make([]*html.Node, 7)
	for i := range paras {
		paras[i] = pNode(strings.Repeat("x", i+1))
	}
	params := shuffleParams{fixedLength: 2, seed: 5, a: 3, c: 7, mod: 10}

	out := restoreParagraphs(paras, params)
	if out == nil {
		t.Fatal("restore returned nil")
	}
	// Verify exact permutation against the reference algorithm.
	var shuffled = []int{2, 3, 4, 5, 6}
	seed := 5
	for i := len(shuffled) - 1; i > 0; i-- {
		seed = (seed*3 + 7) % 10
		j := int(float64(seed) / float64(10) * float64(i+1))
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	}
	order := append([]int{0, 1}, shuffled...)
	for i, target := range order {
		if got := nodeText(out[target]); got != strings.Repeat("x", i+1) {
			t.Fatalf("position %d holds %q, want paragraph %d", target, got, i)
		}
	}
}
