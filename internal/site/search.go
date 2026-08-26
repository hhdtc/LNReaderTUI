// Package site implements the bilinovel.com (linovelib mirror) search and
// download pipeline, ported from LNReader's backend/services/linovelib.py
// and bilinovel_downloader.py.
//
// The search form is protected by the Jieqi "search guard" cookie chain
// (/search.html?search_guard=css|js|redeem). It is fully replicable with
// plain HTTP using a Chrome TLS-fingerprint session — no browser or
// user-supplied cookies needed.
package site

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	fhttp "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

const (
	SearchURL = "https://www.bilinovel.com/search.html"
	BaseURL   = "https://www.bilinovel.com"

	searchUA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/151.0.0.0 Safari/537.36"

	placeholderCover = "book-cover-no.svg"
)

var novelURLRe = regexp.MustCompile(`(?:linovelib|bilinovel)\.com/(?:novel|download)/\d+`)

// Book is a search result (or a novel detail page, when the query matched an
// exact title and the site redirected to /novel/<id>.html).
type Book struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Author      string `json:"author"`
	Publisher   string `json:"publisher"`
	CoverURL    string `json:"cover_url"`
	Status      string `json:"status"`
	Rating      string `json:"rating"`
	Description string `json:"description"`
	Tags        string `json:"tags"`
}

// Result is a search response.
type Result struct {
	Total      int
	Books      []Book
	Suggestion string
}

// Error describes a failed search.
type Error struct{ msg string }

func (e *Error) Error() string { return e.msg }

func errf(format string, args ...any) error { return &Error{fmt.Sprintf(format, args...)} }

// NewSearchClient returns an HTTP client impersonating Chrome over HTTP/1.1.
func NewSearchClient() (tlsclient.HttpClient, error) {
	return tlsclient.NewHttpClient(tlsclient.NewNoopLogger(),
		tlsclient.WithClientProfile(profiles.Chrome_146),
		tlsclient.WithForceHttp1(),
		tlsclient.WithTimeoutSeconds(20),
		tlsclient.WithCookieJar(tlsclient.NewCookieJar()),
		tlsclient.WithDefaultHeaders(fhttp.Header{
			"accept-language": {"zh-CN,zh;q=0.9,en;q=0.8,ja;q=0.7"},
			"user-agent":      {searchUA},
		}),
	)
}

// runGuard completes the Jieqi search-guard dance on the session.
// The css/js guard endpoints issue jieqiSearchCss / jieqiSearchJs (the js
// value arrives inline in the script body), and a redeem request issues
// jieqiSearchTicket. The search POST then passes.
func runGuard(ctx context.Context, c tlsclient.HttpClient) error {
	page, err := doGet(ctx, c, SearchURL, map[string]string{"accept": "text/html"})
	if err != nil {
		return err
	}
	if page.StatusCode != 200 || page.Body == "" {
		return errf("bilinovel search page is unreachable")
	}

	if _, err := doGet(ctx, c, SearchURL+"?search_guard=css", map[string]string{
		"accept":  "text/css,*/*;q=0.1",
		"referer": SearchURL,
	}); err != nil {
		return err
	}

	js, err := doGet(ctx, c, SearchURL+"?search_guard=js", map[string]string{
		"accept":  "*/*",
		"referer": SearchURL,
	})
	if err != nil {
		return err
	}
	if m := regexp.MustCompile(`jieqiSearchJs=([^;"]+)`).FindStringSubmatch(js.Body); m != nil {
		if u, err := url.Parse(BaseURL); err == nil {
			c.GetCookieJar().SetCookies(u, []*fhttp.Cookie{{
				Name: "jieqiSearchJs", Value: m[1], Path: "/", Domain: u.Host,
			}})
		}
	}

	_, err = doGet(ctx, c, fmt.Sprintf("%s?search_guard=redeem&r=%d", SearchURL,
		time.Now().UnixMilli()), map[string]string{
		"accept":  "*/*",
		"referer": SearchURL,
	})
	return err
}

type response struct {
	StatusCode int
	Body       string
	FinalURL   string
}

func doGet(ctx context.Context, c tlsclient.HttpClient, u string, headers map[string]string) (*response, error) {
	reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := fhttp.NewRequestWithContext(reqCtx, fhttp.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	final := u
	if resp.Request != nil && resp.Request.URL != nil {
		final = resp.Request.URL.String()
	}
	return &response{StatusCode: resp.StatusCode, Body: string(body), FinalURL: final}, nil
}

// doSearchOnce runs one guard dance + search POST on a fresh session.
// The Jieqi ticket is single-use and the site enforces a 5-second minimum
// between searches per session; a fresh session per attempt keeps repeated
// searches legal without sleeping.
//
// Line handles the context expiration.
func doSearchOnce(ctx context.Context, query string) (*Result, error) {
	c, err := NewSearchClient()
	if err != nil {
		return nil, err
	}
	if err := runGuard(ctx, c); err != nil {
		return nil, err
	}

	form := url.Values{"searchkey": {query}}
	reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := fhttp.NewRequestWithContext(reqCtx, fhttp.MethodPost, SearchURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/x-www-form-urlencoded")
	req.Header.Set("accept", "text/html,application/xhtml+xml,application/xml;q=0.9")
	req.Header.Set("origin", BaseURL)
	req.Header.Set("referer", SearchURL)

	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 || len(body) == 0 {
		return nil, errf("bilinovel returned an empty response for the search; " +
			"the site may have changed its anti-bot measures")
	}
	text := string(body)
	final := SearchURL
	if resp.Request != nil && resp.Request.URL != nil {
		final = resp.Request.URL.String()
	}
	// Exact-title matches redirect to the novel page instead of a list.
	if novelURLRe.MatchString(final) {
		book, ok := parseNovelPage(text, final)
		if !ok {
			return nil, errf("bilinovel redirected to a novel page that could not be parsed")
		}
		return &Result{Total: 1, Books: []Book{book}}, nil
	}
	return parseResults(text), nil
}

// Search searches bilinovel.com, falling back to relaxed substring queries
// when the first search yields no results.
func Search(ctx context.Context, query string) (*Result, error) {
	result, err := doSearchOnce(ctx, query)
	if err != nil {
		return nil, err
	}
	if result.Total == 0 {
		if fb := fuzzyFallback(ctx, query); fb != nil {
			return fb, nil
		}
	}
	return result, nil
}

// parseResults extracts search result cards (li.book-li).
func parseResults(html string) *Result {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return &Result{}
	}
	books := make([]Book, 0)
	doc.Find("li.book-li").Each(func(_ int, item *goquery.Selection) {
		layout := item.Find("a.book-layout").First()
		href, _ := layout.Attr("href")
		u := href
		if u != "" && !strings.HasPrefix(u, "http") {
			u = BaseURL + u
		}
		// Search also matches authors/translators; only novels are useful.
		if !novelURLRe.MatchString(u) {
			return
		}
		b := Book{Title: item.Find("h4.book-title").First().Text(), URL: u}

		img := item.Find(".book-cover img").First()
		if src, ok := img.Attr("data-src"); ok && src != "" {
			coverSrc(src, &b)
		} else if src, ok := img.Attr("src"); ok {
			coverSrc(src, &b)
		}

		authorEl := item.Find(".book-author").First()
		authorEl.Find("svg").Remove()
		b.Author = strings.TrimSpace(authorEl.Text())

		item.Find(".tag-small").Each(func(_ int, em *goquery.Selection) {
			text := strings.TrimSpace(em.Text())
			if text == "" {
				return
			}
			cls, _ := em.Attr("class")
			switch {
			case strings.Contains(cls, "red"):
				b.Tags = text
			case strings.Contains(cls, "yellow"):
				b.Publisher = text
			case strings.Contains(cls, "gray"):
				b.Status = text
			}
		})

		b.Rating = strings.TrimSpace(item.Find(".corner em").First().Text())
		b.Description = strings.TrimSpace(item.Find("p.book-desc").First().Text())
		books = append(books, b)
	})
	return &Result{Total: len(books), Books: books}
}

func coverSrc(src string, b *Book) {
	src = strings.Split(src, "?")[0]
	if src == "" || strings.Contains(src, placeholderCover) {
		return
	}
	if !strings.HasPrefix(src, "http") {
		src = BaseURL + src
	}
	b.CoverURL = src
}

// parseNovelPage parses a novel detail page (the redirect target of an
// exact-title search) into a single-result Book.
func parseNovelPage(html, url string) (Book, bool) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return Book{}, false
	}
	title := strings.TrimSpace(doc.Find("h1.book-title").First().Text())
	if title == "" {
		return Book{}, false
	}
	b := Book{Title: title, URL: url}

	if a := doc.Find(".book-rand-a .authorname a").First(); a.Length() > 0 {
		b.Author = strings.TrimSpace(a.Text())
	}
	img := doc.Find(".module-item-cover img").First()
	if src, ok := img.Attr("data-src"); ok && src != "" {
		coverSrc(src, &b)
	} else if src, ok := img.Attr("src"); ok {
		coverSrc(src, &b)
	}

	if meta, ok := doc.Find(`meta[name="description"]`).First().Attr("content"); ok {
		desc := strings.TrimSpace(meta)
		if i := strings.Index(desc, "内容简介："); i >= 0 {
			desc = strings.TrimSpace(desc[i+len("内容简介："):])
		}
		b.Description = desc
	}

	doc.Find("p.book-meta").EachWithBreak(func(_ int, m *goquery.Selection) bool {
		text := strings.TrimSpace(m.Text())
		if strings.Contains(text, "字") {
			for _, token := range []string{"连载", "完结"} {
				if strings.Contains(text, token) {
					b.Status = token
					break
				}
			}
			return false
		}
		return true
	})
	return b, true
}

// ---- fuzzy fallback ----
//
// Jieqi search LIKE-matches titles/authors, so a misspelled query often
// still contains a clean run of characters (入见人间 -> 人间). Each candidate
// substring is searched and books whose title/author resembles the original
// query are kept, with the closest match surfaced as a suggestion.

const (
	fallbackMinRatio   = 0.6
	fallbackMaxTries   = 6
	fallbackMaxCandLen = 12
)

func fuzzyFallback(ctx context.Context, query string) *Result {
	n := len([]rune(query))
	if n < 3 {
		return nil
	}
	runes := []rune(query)
	seen := map[string]bool{}
	tried := 0
	for length := n - 1; length > 1; length-- {
		for start := 0; start+length <= n; start++ {
			sub := string(runes[start : start+length])
			if seen[sub] {
				continue
			}
			seen[sub] = true
			if tried >= fallbackMaxTries {
				return nil
			}
			tried++
			if tried > fallbackMaxCandLen+fallbackMaxTries {
				// Defensive cap; original caps via candidate iteration.
			}
			subCtx, cancel := withTimeout(ctx, 15*time.Second)
			result, err := doSearchOnce(subCtx, sub)
			cancel()
			if err != nil || result.Total == 0 {
				continue
			}
			filtered := make([]Book, 0)
			for _, b := range result.Books {
				if closeEnough(query, b) {
					filtered = append(filtered, b)
				}
			}
			if len(filtered) > 0 {
				return &Result{Total: len(filtered), Books: filtered,
					Suggestion: suggestFor(query, filtered)}
			}
			// Nothing passes the similarity gate, but the site's suggestions
			// can still be usable: keep books whose title/author contains a
			// tried substring. Zero-result searches are worse than loose ones.
			containing := make([]Book, 0)
			for _, b := range result.Books {
				if containsEnough(query, b) {
					containing = append(containing, b)
				}
			}
			if len(containing) > 0 {
				return &Result{Total: len(containing), Books: containing,
					Suggestion: suggestFor(query, containing)}
			}
		}
	}
	return nil
}

// containsEnough matches books whose title/author contains any 2+-rune
// substring of the query — a looser gate than closeEnough.
func containsEnough(query string, b Book) bool {
	qr := []rune(query)
	for _, cand := range []string{b.Title, b.Author} {
		cr := []rune(cand)
		for i := 0; i+2 <= len(qr); i++ {
			if strings.Contains(string(cr), string(qr[i:i+2])) {
				return true
			}
		}
	}
	return false
}

func closeEnough(query string, b Book) bool {
	for _, cand := range []string{b.Title, b.Author} {
		if len([]rune(cand)) >= 2 && similarity(query, cand) >= fallbackMinRatio {
			return true
		}
	}
	return false
}

func suggestFor(query string, books []Book) string {
	best, bestScore := "", 0.0
	for _, b := range books {
		for _, cand := range []string{b.Author, b.Title} {
			if s := similarity(query, cand); s > bestScore {
				best, bestScore = cand, s
			}
		}
	}
	return best
}

// similarity returns a SequenceMatcher-like ratio: 2*LCS(runes) / (len(a)+len(b)).
func similarity(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 || lb == 0 {
		return 0
	}
	dp := make([]int, lb+1)
	for i := 1; i <= la; i++ {
		prev := 0
		for j := 1; j <= lb; j++ {
			cur := dp[j]
			if ra[i-1] == rb[j-1] {
				dp[j] = prev + 1
			} else if dp[j] < dp[j-1] {
				dp[j] = dp[j-1]
			}
			prev = cur
		}
	}
	return 2 * float64(dp[lb]) / float64(la+lb)
}

// SearchNovelID extracts the novel id from a search URL.
func SearchNovelID(u string) string {
	m := regexp.MustCompile(`(?:linovelib|bilinovel)\.com/(?:novel|download)/(\d+)`).
		FindStringSubmatch(u)
	if m == nil {
		return ""
	}
	return m[1]
}
