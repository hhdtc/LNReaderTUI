package site

// Port of the chapterlog.js shuffle-template parsing and paragraph restore
// from LNReader's bilinovel_downloader.py (bili_novel_chapterlog.dart
// ported from bili_novel_packer).
//
// The site's anti-scrape mechanism shuffles the paragraphs of each page
// using deterministic LCG parameters embedded in a chapterlog.js script:
//   seed = chapterId * multiplier + offset   (fixed_length paragraphs stay)
//   for i in [len-1 .. 1]: seed = (seed*a + c) % mod; swap(i, seed/mod*(i+1))
//
// The template is either plain ("if (len > N)" ... "= ...Number(chapterId)...")
// or obfuscated; both variants are recognized below.

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"

	"github.com/dlclark/regexp2"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var (
	fixedLengthRe = regexp.MustCompile(`if\s*\(\s*[_$a-zA-Z0-9]+\s*>\s*`)
	seedRe        = regexp.MustCompile(`=\s*(.+?Number\s*\(\s*chapterId\s*\).+?)\s*;`)
	lcgRe         = regexp.MustCompile(`=\s*(\(\s*[_$a-zA-Z0-9]+\s*\*.+?\)\s*%\s*.+?)\s*;`)

	obfSeedRe = regexp.MustCompile(
		`var\s+[_$a-zA-Z0-9]+\s*=\s*[^;]*?Number\s*\(\s*[_$a-zA-Z0-9]+\s*\)\s*,\s*([^,)]+?)\s*\)\s*,\s*([^,)]+?)\s*\)\s*,`)
	// Backreference (unsupported by RE2) — use regexp2 for a faithful match.
	obfLcgRe = regexp2.MustCompile(
		`([_$a-zA-Z0-9]+)\s*=\s*[^;]*?\(\s*\1\s*,\s*([^,)]+?)\s*\)\s*,\s*([^,)]+?)\s*\)\s*,\s*([^;)]+?)\s*\)\s*;`,
		regexp2.None)

	safeExprRe = regexp.MustCompile(`^[0-9a-fA-FxX+\-*/%^<>&|~()\s]+$`)
	jsTokenRe  = regexp.MustCompile(`0[xX][0-9a-fA-F]+|\d+|<<|>>>|>>|[+\-*/%^<>&|~()]`)
)

type shuffleParams struct {
	fixedLength int
	seed        int
	a           int
	c           int
	mod         int
}

type shuffleTemplate struct {
	fixedLength    int
	seedMultiplier int
	seedOffset     int
	a              int
	c              int
	mod            int
}

func (t *shuffleTemplate) paramsFor(chapterID int) shuffleParams {
	return shuffleParams{
		fixedLength: t.fixedLength,
		seed:        chapterID*t.seedMultiplier + t.seedOffset,
		a:           t.a,
		c:           t.c,
		mod:         t.mod,
	}
}

// parseShuffleTemplate parses a chapterlog.js source; returns nil when the
// template cannot be recognized (paragraphs stay shuffled).
func parseShuffleTemplate(js string) *shuffleTemplate {
	if t, ok := parsePlainTemplate(js); ok {
		return t
	}
	return parseObfuscatedTemplate(js)
}

func parsePlainTemplate(js string) (*shuffleTemplate, bool) {
	m := fixedLengthRe.FindStringIndex(js)
	var fixedExpr string
	if m != nil {
		fixedExpr = extractTrailingExpression(js, m[1], ')')
	}
	seedM := seedRe.FindStringSubmatch(js)
	lcgM := lcgRe.FindStringSubmatch(js)
	if fixedExpr == "" || seedM == nil || lcgM == nil {
		return nil, false
	}
	fixed, ok := evalInt(stripOuterParens(fixedExpr))
	if !ok {
		return nil, false
	}
	seedMult, seedOff, ok := parseSeedExpr(seedM[1])
	if !ok {
		return nil, false
	}
	a, c, mod, ok := parseLcgExpr(lcgM[1])
	if !ok {
		return nil, false
	}
	return &shuffleTemplate{fixedLength: fixed, seedMultiplier: seedMult,
		seedOffset: seedOff, a: a, c: c, mod: mod}, true
}

func parseObfuscatedTemplate(js string) *shuffleTemplate {
	var seedMult, seedOff int
	seedOK := false
	for _, m := range obfSeedRe.FindAllStringSubmatch(js, -1) {
		mult, ok1 := evalInt(stripOuterParens(m[1]))
		off, ok2 := evalInt(stripOuterParens(m[2]))
		if ok1 && ok2 && mult > 0 && off >= 0 {
			seedMult, seedOff, seedOK = mult, off, true
			break
		}
	}
	if !seedOK {
		return nil
	}

	var a, c, mod int
	lcgOK := false
	if m, err := obfLcgRe.FindStringMatch(js); err == nil {
		for m != nil {
			av, e1 := evalInt(stripOuterParens(m.GroupByNumber(2).String()))
			ev, e2 := evalInt(stripOuterParens(m.GroupByNumber(3).String()))
			mv, e3 := evalInt(stripOuterParens(m.GroupByNumber(4).String()))
			if e1 && e2 && e3 && av > 0 && ev >= 0 && mv > av && mv > ev {
				a, c, mod = av, ev, mv
				lcgOK = true
				break
			}
			m, err = obfLcgRe.FindNextMatch(m)
		}
	}
	if !lcgOK {
		return nil
	}
	// Obfuscated templates always keep the first 20 paragraphs fixed.
	return &shuffleTemplate{fixedLength: 20, seedMultiplier: seedMult,
		seedOffset: seedOff, a: a, c: c, mod: mod}
}

// extractTrailingExpression scans source from start, honoring parenthesis
// depth, until the terminator (')' at depth 0 or a bare terminator char).
func extractTrailingExpression(source string, start int, terminator byte) string {
	depth := 0
	for i := start; i < len(source); i++ {
		ch := source[i]
		switch {
		case ch == '(':
			depth++
		case ch == ')':
			if depth == 0 && terminator == ')' {
				return strings.TrimSpace(source[start:i])
			}
			depth--
		case depth == 0 && ch == terminator:
			return strings.TrimSpace(source[start:i])
		}
	}
	return ""
}

// stripOuterParens removes one fully-wrapping paren layer at a time.
func stripOuterParens(expr string) string {
	value := strings.TrimSpace(expr)
	for strings.HasPrefix(value, "(") && strings.HasSuffix(value, ")") {
		depth := 0
		wraps := true
		for i := 0; i < len(value); i++ {
			switch value[i] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 && i != len(value)-1 {
					wraps = false
				}
			}
			if !wraps {
				break
			}
		}
		if !wraps {
			return value
		}
		value = strings.TrimSpace(value[1 : len(value)-1])
	}
	return value
}

func parseSeedExpr(expr string) (mult, offset int, ok bool) {
	off, ok1 := evalExprWithVars(expr, map[string]int{"chapterId": 0})
	one, ok2 := evalExprWithVars(expr, map[string]int{"chapterId": 1})
	if !ok1 || !ok2 {
		return 0, 0, false
	}
	return one - off, off, true
}

func parseLcgExpr(expr string) (a, c, mod int, ok bool) {
	parts := splitTopLevel(expr, "%")
	if len(parts) != 2 {
		return 0, 0, 0, false
	}
	m, ok := evalInt(stripOuterParens(parts[1]))
	if !ok {
		return 0, 0, 0, false
	}
	left := stripOuterParens(parts[0])
	ident := identRe(left)
	if ident == "" {
		return 0, 0, 0, false
	}
	cv, ok1 := evalExprWithVars(left, map[string]int{ident: 0})
	ov, ok2 := evalExprWithVars(left, map[string]int{ident: 1})
	if !ok1 || !ok2 {
		return 0, 0, 0, false
	}
	return ov - cv, cv, m, true
}

func identRe(expr string) string {
	for i := 0; i < len(expr); i++ {
		c := expr[i]
		if c == '$' || c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			start := i
			for i < len(expr) {
				c = expr[i]
				if c == '$' || c == '_' || (c >= 'a' && c <= 'z') ||
					(c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
					i++
					continue
				}
				break
			}
			return expr[start:i]
		}
	}
	return ""
}

func splitTopLevel(expr, op string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(expr); i++ {
		switch expr[i] {
		case '(':
			depth++
		case ')':
			depth--
		default:
			if depth == 0 && strings.HasPrefix(expr[i:], op) {
				parts = append(parts, strings.TrimSpace(expr[start:i]))
				start = i + len(op)
				i += len(op) - 1
			}
		}
	}
	return append(parts, strings.TrimSpace(expr[start:]))
}

func evalExprWithVars(expr string, vars map[string]int) (int, bool) {
	normalized := expr
	for key, value := range vars {
		strVal := strconv.Itoa(value)
		re := regexp.MustCompile(`Number\s*\(\s*` + regexp.QuoteMeta(key) + `\s*\)`)
		normalized = re.ReplaceAllString(normalized, strVal)
		re = regexp.MustCompile(`\b` + regexp.QuoteMeta(key) + `\b`)
		normalized = re.ReplaceAllString(normalized, strVal)
	}
	return evalInt(strings.TrimSpace(normalized))
}

// evalInt evaluates a pure-JS-integer expression (dec/hex literals, + - * /
// % ^ << >> >>> & | ~ and parens); ok=false when the expression contains
// anything else.
func evalInt(expr string) (int, bool) {
	expr = strings.TrimSpace(expr)
	if expr == "" || !safeExprRe.MatchString(expr) {
		return 0, false
	}
	v, err := evalJSArith(expr)
	if err != nil {
		return 0, false
	}
	return v, true
}

// ---- JS integer arithmetic evaluator ----

func evalJSArith(expr string) (int, error) {
	tokens := jsTokenRe.FindAllString(expr, -1)
	pos := 0
	peek := func() string {
		if pos < len(tokens) {
			return tokens[pos]
		}
		return ""
	}

	var parseXor func() (int, error)
	var parseShift func() (int, error)
	var parseAddSub func() (int, error)
	var parseMulDiv func() (int, error)
	var parseUnary func() (int, error)
	var parsePrimary func() (int, error)

	parseXor = func() (int, error) {
		value, err := parseShift()
		if err != nil {
			return 0, err
		}
		for peek() == "^" {
			pos++
			nv, err := parseShift()
			if err != nil {
				return 0, err
			}
			value ^= nv
		}
		return value, nil
	}
	parseShift = func() (int, error) {
		value, err := parseAddSub()
		if err != nil {
			return 0, err
		}
		for {
			switch peek() {
			case "<<":
				pos++
				nv, err := parseAddSub()
				if err != nil {
					return 0, err
				}
				value <<= uint(nv)
			case ">>", ">>>":
				pos++
				nv, err := parseAddSub()
				if err != nil {
					return 0, err
				}
				value >>= uint(nv)
			default:
				return value, nil
			}
		}
	}
	parseAddSub = func() (int, error) {
		value, err := parseMulDiv()
		if err != nil {
			return 0, err
		}
		for {
			switch peek() {
			case "+":
				pos++
				nv, err := parseMulDiv()
				if err != nil {
					return 0, err
				}
				value += nv
			case "-":
				pos++
				nv, err := parseMulDiv()
				if err != nil {
					return 0, err
				}
				value -= nv
			default:
				return value, nil
			}
		}
	}
	parseMulDiv = func() (int, error) {
		value, err := parseUnary()
		if err != nil {
			return 0, err
		}
		for {
			switch peek() {
			case "*":
				pos++
				nv, err := parseUnary()
				if err != nil {
					return 0, err
				}
				value *= nv
			case "/":
				pos++
				nv, err := parseUnary()
				if err != nil {
					return 0, err
				}
				value /= nv
			case "%":
				pos++
				nv, err := parseUnary()
				if err != nil {
					return 0, err
				}
				value %= nv
			default:
				return value, nil
			}
		}
	}
	parseUnary = func() (int, error) {
		switch peek() {
		case "+":
			pos++
			return parseUnary()
		case "-":
			pos++
			v, err := parseUnary()
			return -v, err
		case "~":
			pos++
			v, err := parseUnary()
			return ^v, err
		}
		return parsePrimary()
	}
	parsePrimary = func() (int, error) {
		if peek() == "(" {
			pos++
			value, err := parseXor()
			if err != nil {
				return 0, err
			}
			if peek() != ")" {
				return 0, errf("missing paren")
			}
			pos++
			return value, nil
		}
		tok := peek()
		if tok == "" || !isNumberToken(tok) {
			return 0, errf("unexpected token %q", tok)
		}
		pos++
		if len(tok) > 2 && (tok[0] == '0' && (tok[1] == 'x' || tok[1] == 'X')) {
			v, err := strconv.ParseInt(tok[2:], 16, 64)
			return int(v), err
		}
		v, err := strconv.Atoi(tok)
		return v, err
	}

	value, err := parseXor()
	if err != nil {
		return 0, err
	}
	if pos != len(tokens) {
		return 0, errf("trailing tokens")
	}
	return value, nil
}

func isNumberToken(tok string) bool {
	if len(tok) > 2 && tok[0] == '0' && (tok[1] == 'x' || tok[1] == 'X') {
		return true
	}
	for _, c := range tok {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(tok) > 0
}

// ---- paragraph restore on DOM trees ----

// parseFragmentChildren parses an HTML fragment into its top-level nodes.
func parseFragmentChildren(htmlSrc string) []*html.Node {
	ctx := &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := html.ParseFragment(strings.NewReader(htmlSrc), ctx)
	if err != nil {
		return nil
	}
	return nodes
}

// restoreParagraphs applies the shuffle params to direct <p> children.
// Returns nil when no change was made (fewer paragraphs than fixedLength).
func restoreParagraphs(nodes []*html.Node, params shuffleParams) []*html.Node {
	var slots []int
	var paragraphs []*html.Node
	for i, node := range nodes {
		if node.Type == html.ElementNode && node.Data == "p" {
			if text := strings.Map(removeSpace, nodeText(node)); text != "" {
				slots = append(slots, i)
				paragraphs = append(paragraphs, node)
			}
		}
	}
	if len(paragraphs) <= params.fixedLength {
		return nil
	}

	indices := make([]int, len(paragraphs))
	for i := range indices {
		indices[i] = i
	}
	fixed := indices[:params.fixedLength]
	shuffled := append([]int(nil), indices[params.fixedLength:]...)

	seed := params.seed
	for i := len(shuffled) - 1; i > 0; i-- {
		seed = (seed*params.a + params.c) % params.mod
		j := int(float64(seed) / float64(params.mod) * float64(i+1))
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	}
	order := append(append([]int(nil), fixed...), shuffled...)

	restored := make([]*html.Node, len(paragraphs))
	for i, target := range order {
		restored[target] = paragraphs[i]
	}
	out := append([]*html.Node(nil), nodes...)
	for i, slot := range slots {
		out[slot] = restored[i]
	}
	return out
}

func nodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			return
		}
		if n.Type == html.ElementNode && n.Data == "br" {
			b.WriteString("\n")
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

func removeSpace(r rune) rune {
	if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f' || r == '\v' ||
		r == '\u00a0' || r == '\u3000' {
		return -1
	}
	return r
}

func serializeNodes(nodes []*html.Node) string {
	var buf bytes.Buffer
	for _, n := range nodes {
		_ = html.Render(&buf, n)
	}
	return buf.String()
}
