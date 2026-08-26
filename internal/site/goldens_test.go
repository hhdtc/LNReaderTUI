package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// Fixtures captured 2026-08-12 from https://www.bilinovel.com, shared with
// LNReader's backend/tests/fixtures. They pin the obfuscated-template parser
// and the paragraph-restore against reference-verified constants.

const fixtureDir = "../../../LNReader/backend/tests/fixtures"

func loadFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixtureDir, name))
	if err != nil {
		t.Skipf("fixture not available: %v", err)
	}
	return string(data)
}

// The obfuscated live template must yield the constants independently
// verified by bili_novel_packer's v1006c1.3-era values.
func TestGoldenObfuscatedTemplate(t *testing.T) {
	js := loadFixture(t, "chapterlog_v1006c1.82.js")
	tpl := parseShuffleTemplate(js)
	if tpl == nil {
		t.Fatal("template not parsed")
	}
	if tpl.fixedLength != 20 || tpl.a != 9302 || tpl.c != 49397 ||
		tpl.mod != 233280 || tpl.seedMultiplier != 126 || tpl.seedOffset != 232 {
		t.Fatalf("template constants mismatch: fixed=%d a=%d c=%d mod=%d mult=%d off=%d",
			tpl.fixedLength, tpl.a, tpl.c, tpl.mod, tpl.seedMultiplier, tpl.seedOffset)
	}
	if got := tpl.paramsFor(307075).seed; got != 307075*126+232 {
		t.Fatalf("seed(307075) = %d, want %d", got, 307075*126+232)
	}
}

// A structural change must fail parsing rather than silently no-op.
func TestGoldenParseStructuralFailure(t *testing.T) {
	broken := strings.ReplaceAll(loadFixture(t, "chapterlog_v1006c1.82.js"), "Number(", "parseInt(")
	if tpl := parseShuffleTemplate(broken); tpl != nil {
		t.Fatal("broken template parsed")
	}
}

// Page 1 of chapter_330342 has 22 direct <p> paragraphs (> 20 fixed): the
// site shuffles it. Restoring must invert the shuffle — the first 20 stay
// untouched and the rest come back in original order.
func TestGoldenRestoreShuffledPage(t *testing.T) {
	html := loadFixture(t, "chapter_330342_p1.html")
	js := loadFixture(t, "chapterlog_v1006c1.82.js")

	page, err := parsePage(html, "https://www.bilinovel.com/novel/5289/330342.html")
	if err != nil {
		t.Fatal(err)
	}
	if page.chapterID == nil {
		t.Fatal("no chapter id parsed")
	}
	original := directParagraphTexts(page.content)
	params := parseShuffleTemplate(js).paramsFor(*page.chapterID)
	if len(original) <= params.fixedLength {
		t.Fatalf("fixture page has %d direct paragraphs, expected > %d", len(original), params.fixedLength)
	}

	// The site's forward shuffle (same LCG Fisher-Yates), then zero the p
	// texts for the shuffled order.
	shuffled := forwardShuffle(original, params)
	if strings.Join(shuffled, "|") == strings.Join(original, "|") {
		t.Fatal("fixture shuffle produced no change; fixture likely stale")
	}
	var sb strings.Builder
	for _, text := range shuffled {
		sb.WriteString("<p>" + text + "</p>")
	}

	// Inject the template into a fresh downloader (as the Python test does)
	// and run the production restore path offline.
	d := &Downloader{templates: map[string]*shuffleTemplate{}}
	d.templates["chapterlog.js"] = parseShuffleTemplate(js)
	restored := d.restoreIfShuffled(sb.String(), *page.chapterID, "chapterlog.js")

	got := directParagraphTexts(restored)
	if strings.Join(got, "|") != strings.Join(original, "|") {
		t.Fatalf("restore mismatch:\n original: %q\n got:      %q", original, got)
	}
}

// ---- helpers mirroring the Python reference test ----

func directParagraphTexts(fragment string) []string {
	nodes := parseFragmentChildren(fragment)
	var out []string
	for _, n := range nodes {
		if n.Type == html.ElementNode && n.Data == "p" {
			text := strings.Map(removeSpace, nodeText(n))
			if text != "" {
				out = append(out, text)
			}
		}
	}
	return out
}

func forwardShuffle(texts []string, p shuffleParams) []string {
	arr := append([]string(nil), texts...)
	if len(arr) <= p.fixedLength {
		return arr
	}
	zone := append([]string(nil), arr[p.fixedLength:]...)
	seed := p.seed
	for i := len(zone) - 1; i > 0; i-- {
		seed = (seed*p.a + p.c) % p.mod
		j := int(float64(seed) / float64(p.mod) * float64(i+1))
		zone[i], zone[j] = zone[j], zone[i]
	}
	return append(append([]string(nil), arr[:p.fixedLength]...), zone...)
}
