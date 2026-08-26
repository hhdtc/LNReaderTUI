package site

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	fhttp "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

// Port of LNReader's bilinovel_downloader.py — itself a port of
// github.com/montaro2017/bili_novel_packer:
//   - rate-limited plain HTTP (Chrome TLS impersonation, no cookies)
//   - catalog parsing with volume grouping
//   - chapter URL resolution via next/prev chapter probing
//   - multi-page chapters (url_previous/url_next + #footlink)
//   - anti-scrape cleanup (junk tags, lazy-load images, unicode host obfuscation)
//   - paragraph-shuffle restore driven by chapterlog.js template parameters

const downloadDomain = "https://www.bilinovel.com"

const downloadUA = "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 " +
	"(KHTML, like Gecko) Chrome/124.0.0.0 Mobile Safari/537.36"

// Site rate limits (same as the packer): 15 text req/min, 10 image req/min.
const (
	TextInterval  = 4.0
	ImageInterval = 6.0
)

var (
	contentSelectors = []string{"#acontent", ".bcontent"}
	removeSelectors  = []string{"div", "ins", "figure", "fig", "br", "script", ".tp", ".bd"}
	junkTagRe        = regexp.MustCompile(`[a-z]\d{4}`)
	pageURLRe        = regexp.MustCompile(`url_previous:'(.*?)',url_next:'(.*?)'`)

	prevPageTexts    = map[string]bool{"上一页": true, "上一頁": true}
	nextPageTexts    = map[string]bool{"下一页": true, "下一頁": true}
	maxProbeCount    = 20
	novelIDRe        = regexp.MustCompile(`(?:linovelib|bilinovel)\.com/(?:novel|download)/(\d+)`)
	chapterIDRe      = regexp.MustCompile(`chapterid:'(\d+)'`)
	chapterlogSrcRe  = regexp.MustCompile(`src="([^"]*chapterlog\.js[^"]*)"`)
	imgAttrWhitelist = map[string]bool{"alt": true, "class": true, "dir": true, "height": true,
		"id": true, "ismap": true, "lang": true, "longdesc": true, "style": true,
		"title": true, "usemap": true, "width": true, "src": true, "xml:lang": true}
)

// The site hides image hostnames with a mathematical-bold "b" (U+1D623).
const unicodeB = "\U0001d623"

// DownloadError is a download failure.
type DownloadError struct{ msg string }

func (e *DownloadError) Error() string { return e.msg }

func derrf(format string, args ...any) error {
	return &DownloadError{fmt.Sprintf(format, args...)}
}

// RateLimiter paces requests like the site's server-side throttle expects.
type RateLimiter struct {
	interval time.Duration
	mu       sync.Mutex
	last     time.Time
}

func newRateLimiter(interval float64) *RateLimiter {
	return &RateLimiter{interval: time.Duration(interval * float64(time.Second))}
}

func (r *RateLimiter) wait(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	wait := r.last.Add(r.interval).Sub(time.Now())
	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	r.last = time.Now()
	return nil
}

// ChapterRef is a catalog entry; Href is "" when the catalog hides the URL
// (javascript: href) and the URL must be probed.
type ChapterRef struct {
	Title string
	Href  string
}

// VolumeRef groups chapters under a volume title.
type VolumeRef struct {
	Title    string
	Chapters []ChapterRef
}

// NovelMeta describes the detail page of a novel.
type NovelMeta struct {
	NovelID     string
	Title       string
	Author      string
	CoverURL    string
	Description string
	Status      string
}

// Chapter is one fetched chapter, in reading order.
type Chapter struct {
	Title     string
	HTML      string
	ImageURLs []string
}

// NovelBundle is a complete downloaded novel, ready for EPUB assembly.
type NovelBundle struct {
	Meta     NovelMeta
	Volumes  []VolumeRef
	Chapters []Chapter
}

// Downloader fetches novel content and images from bilinovel.com.
type Downloader struct {
	client       tlsclient.HttpClient
	textLimiter  *RateLimiter
	imageLimiter *RateLimiter
	mu           sync.Mutex
	templates    map[string]*shuffleTemplate
}

// NewDownloader creates a downloader with a Chrome-impersonating session.
func NewDownloader() (*Downloader, error) {
	c, err := tlsclient.NewHttpClient(tlsclient.NewNoopLogger(),
		tlsclient.WithClientProfile(profiles.Chrome_124),
		tlsclient.WithForceHttp1(),
		tlsclient.WithTimeoutSeconds(20),
		tlsclient.WithCookieJar(tlsclient.NewCookieJar()),
		tlsclient.WithDefaultHeaders(fhttp.Header{
			"accept":          {"*/*"},
			"accept-language": {"zh-CN,zh;q=0.9"},
			"cookie":          {"night=0"},
			"referer":         {downloadDomain},
			"user-agent":      {downloadUA},
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("tls client: %w", err)
	}
	return &Downloader{
		client:       c,
		textLimiter:  newRateLimiter(TextInterval),
		imageLimiter: newRateLimiter(ImageInterval),
		templates:    map[string]*shuffleTemplate{},
	}, nil
}

const requestTimeout = 30 * time.Second

func (d *Downloader) get(ctx context.Context, rawURL string) (string, error) {
	if err := d.textLimiter.wait(ctx); err != nil {
		return "", err
	}
	// The client timeout only covers the dial; a server that accepts the
	// connection and stalls the response would hang us forever otherwise.
	reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := fhttp.NewRequestWithContext(reqCtx, fhttp.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 || len(body) == 0 {
		return "", derrf("request failed: %s (status %d)", rawURL, resp.StatusCode)
	}
	return string(body), nil
}

func (d *Downloader) getBytes(ctx context.Context, rawURL string) ([]byte, error) {
	if err := d.imageLimiter.wait(ctx); err != nil {
		return nil, err
	}
	reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := fhttp.NewRequestWithContext(reqCtx, fhttp.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 || len(data) == 0 {
		return nil, derrf("image download failed: %s", rawURL)
	}
	return data, nil
}

// ---------- novel & catalog ----------

// FetchNovel returns the novel detail metadata for a novel id.
func (d *Downloader) FetchNovel(ctx context.Context, novelID string) (NovelMeta, error) {
	html, err := d.get(ctx, fmt.Sprintf("%s/novel/%s.html", downloadDomain, novelID))
	if err != nil {
		return NovelMeta{}, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return NovelMeta{}, err
	}
	title := strings.TrimSpace(doc.Find(".book-title").First().Text())
	if title == "" {
		return NovelMeta{}, derrf("novel page missing title (id %s)", novelID)
	}
	meta := NovelMeta{NovelID: novelID, Title: title, Author: "Unknown"}
	if a := doc.Find(".book-rand-a span").First(); a.Length() > 0 {
		meta.Author = strings.TrimSpace(a.Text())
	}
	if img := doc.Find(".book-layout img").First(); img.Length() > 0 {
		meta.CoverURL, _ = img.Attr("src")
	}
	if metaEl := doc.Find(`meta[name="description"]`).First(); metaEl.Length() > 0 {
		desc, _ := metaEl.Attr("content")
		meta.Description = strings.TrimSpace(desc)
		if i := strings.Index(meta.Description, "内容简介："); i >= 0 {
			meta.Description = strings.TrimSpace(meta.Description[i+len("内容简介："):])
		}
	}
	doc.Find("p.book-meta").EachWithBreak(func(_ int, m *goquery.Selection) bool {
		text := strings.TrimSpace(m.Text())
		if strings.Contains(text, "字") {
			for _, token := range []string{"连载", "完结"} {
				if strings.Contains(text, token) {
					meta.Status = token
					break
				}
			}
			return false
		}
		return true
	})
	return meta, nil
}

// FetchCatalog parses the volume/chapter tree for a novel id.
func (d *Downloader) FetchCatalog(ctx context.Context, novelID string) ([]VolumeRef, error) {
	html, err := d.get(ctx, fmt.Sprintf("%s/novel/%s/catalog", downloadDomain, novelID))
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}
	var volumes []VolumeRef
	var current *VolumeRef
	found := false
	doc.Find(".volume-chapters > li").Each(func(_ int, li *goquery.Selection) {
		classes, _ := li.Attr("class")
		switch {
		case strings.Contains(classes, "chapter-bar"):
			if current != nil {
				volumes = append(volumes, *current)
			}
			v := VolumeRef{Title: strings.TrimSpace(li.Text())}
			current = &v
		case strings.Contains(classes, "jsChapter") && current != nil:
			link := li.Find("a").First()
			href, _ := link.Attr("href")
			title := strings.TrimSpace(link.Text())
			if title == "" {
				title = fmt.Sprintf("Chapter %d", len(current.Chapters)+1)
			}
			ref := ChapterRef{Title: title}
			switch {
			case strings.HasPrefix(href, "javascript"):
				ref.Href = ""
			case href != "":
				if !strings.HasPrefix(href, "http") {
					href = downloadDomain + href
				}
				ref.Href = href
			}
			current.Chapters = append(current.Chapters, ref)
			found = true
		}
	})
	if current != nil {
		volumes = append(volumes, *current)
	}
	if !found || len(volumes) == 0 {
		return nil, derrf("no chapters found (id %s)", novelID)
	}
	return volumes, nil
}

// ---------- chapter URL resolution ----------

func (d *Downloader) allChapters(volumes []VolumeRef) []ChapterRef {
	var out []ChapterRef
	for _, v := range volumes {
		out = append(out, v.Chapters...)
	}
	return out
}

// ResolveChapterURL resolves a hidden chapter URL (catalog href was
// javascript:). Mirrors the packer: probe the next chapter's page for its
// "previous chapter" link; otherwise walk the previous chapter's pages to
// its final page and take the "next chapter" link.
func (d *Downloader) ResolveChapterURL(ctx context.Context, volumes []VolumeRef, pos int) (string, error) {
	chapters := d.allChapters(volumes)

	// Next chapter first.
	if pos+1 < len(chapters) {
		if nxt := chapters[pos+1]; nxt.Href != "" {
			page, err := d.fetchPage(ctx, nxt.Href)
			if err != nil {
				return "", err
			}
			if page.prevChapterURL != "" {
				return page.prevChapterURL, nil
			}
		}
	}

	// Then walk the previous chapter's pages.
	if pos > 0 {
		if prev := chapters[pos-1]; prev.Href != "" {
			page, err := d.fetchPage(ctx, prev.Href)
			if err != nil {
				return "", err
			}
			for range maxProbeCount {
				if page.nextPageURL != "" {
					page, err = d.fetchPage(ctx, page.nextPageURL)
					if err != nil {
						return "", err
					}
					continue
				}
				if page.nextChapterURL != "" {
					return page.nextChapterURL, nil
				}
				break
			}
		}
	}
	return "", derrf("could not resolve chapter URL")
}

// ---------- chapter content ----------

// FetchChapter fetches a full chapter (all pages). Returns title, cleaned
// HTML and image URLs.
//
// The site's chapterlog.js shuffles each page's paragraphs independently
// (same chapterId seed, per-page DOM), so restore happens per page — never
// across the concatenated chapter.
func (d *Downloader) FetchChapter(ctx context.Context, rawURL string) (Chapter, error) {
	page, err := d.fetchPage(ctx, rawURL)
	if err != nil {
		return Chapter{}, err
	}
	ch := Chapter{Title: page.title}
	chapterID := page.chapterID
	scriptURLs := page.scriptURLs
	for {
		content := page.content
		if chapterID != nil && len(scriptURLs) > 0 {
			content = d.restoreIfShuffled(content, *chapterID, scriptURLs[0])
		}
		ch.HTML += content
		ch.ImageURLs = append(ch.ImageURLs, page.imageURLs...)
		if page.nextPageURL == "" {
			break
		}
		page, err = d.fetchPage(ctx, page.nextPageURL)
		if err != nil {
			return Chapter{}, err
		}
	}
	return ch, nil
}

// DownloadImage fetches an image (or decodes a data: URI).
func (d *Downloader) DownloadImage(ctx context.Context, src string) ([]byte, error) {
	if strings.HasPrefix(src, "data:image") {
		rest := strings.SplitN(src, ",", 2)
		if len(rest) < 2 {
			return nil, derrf("malformed data image")
		}
		return base64.StdEncoding.DecodeString(rest[1])
	}
	u := strings.ReplaceAll(src, unicodeB, "b")
	u = strings.ReplaceAll(u, "https://https://", "https://")
	if !strings.HasPrefix(u, "http") {
		u = downloadDomain + u
	}
	return d.getBytes(ctx, u)
}

type chapterPage struct {
	title string
	// content is the cleaned inner HTML of the content container.
	content   string
	imageURLs []string

	prevPageURL    string
	nextPageURL    string
	prevChapterURL string
	nextChapterURL string
	chapterID      *int
	scriptURLs     []string
}

func (d *Downloader) fetchPage(ctx context.Context, rawURL string) (*chapterPage, error) {
	html, err := d.get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	return parsePage(html, rawURL)
}

// parsePage extracts a chapter page from already-fetched HTML (also used by
// the offline golden tests).
func parsePage(html, rawURL string) (*chapterPage, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	page := &chapterPage{}
	if !strings.Contains(rawURL, "_") {
		if at := doc.Find("#atitle").First(); at.Length() > 0 {
			page.title = strings.TrimSpace(at.Text())
		}
	}

	var contentSel *goquery.Selection
	for _, sel := range contentSelectors {
		contentSel = doc.Find(sel).First()
		if contentSel.Length() > 0 {
			break
		}
	}
	if contentSel == nil || contentSel.Length() == 0 {
		return nil, derrf("chapter content missing: %s", rawURL)
	}

	// Anti-scrape junk: remove elements whose tag name looks like a marker.
	contentSel.Find("*").Each(func(_ int, el *goquery.Selection) {
		if junkTagRe.MatchString(goquery.NodeName(el)) {
			el.Remove()
		}
	})
	for _, sel := range removeSelectors {
		contentSel.Find(sel).Each(func(_ int, el *goquery.Selection) {
			el.Remove()
		})
	}

	// Paragraph-shuffle restore happens on the full chapter, but we need the
	// chapter id + script urls from this page.
	if m := chapterIDRe.FindStringSubmatch(html); m != nil {
		if id, e := strconv.Atoi(m[1]); e == nil {
			page.chapterID = &id
		}
	}
	for _, m := range chapterlogSrcRe.FindAllStringSubmatch(html, -1) {
		page.scriptURLs = append(page.scriptURLs, m[1])
	}

	// Navigation.
	nav := pageURLRe.FindStringSubmatch(html)
	var prevURL, nextURL string
	if nav != nil {
		prevURL, nextURL = nav[1], nav[2]
	}
	prevLink := doc.Find("#footlink a.prevlink").First()
	nextLink := doc.Find("#footlink a.nextlink").First()
	if prevLink.Length() > 0 && prevURL != "" {
		if prevPageTexts[strings.TrimSpace(prevLink.Text())] {
			page.prevPageURL = downloadDomain + prevURL
		} else {
			page.prevChapterURL = downloadDomain + prevURL
		}
	}
	if nextLink.Length() > 0 && nextURL != "" {
		if nextPageTexts[strings.TrimSpace(nextLink.Text())] {
			page.nextPageURL = downloadDomain + nextURL
		} else {
			page.nextChapterURL = downloadDomain + nextURL
		}
	}

	// Images: lazy-load data-src -> src, protocol-relative, junk removal.
	contentSel.Find("img").Each(func(_ int, img *goquery.Selection) {
		src, _ := img.Attr("data-src")
		if src == "" {
			src, _ = img.Attr("src")
		}
		if src == "" || strings.Contains(src, "<") {
			img.Remove()
			return
		}
		if strings.HasPrefix(src, "//") {
			src = "https:" + src
		}
		img.SetAttr("src", src)
		page.imageURLs = append(page.imageURLs, src)
		for _, attr := range img.Nodes[0].Attr {
			if !imgAttrWhitelist[attr.Key] {
				img.RemoveAttr(attr.Key)
			}
		}
		img.SetAttr("alt", "")
	})

	inner, err := contentSel.Html()
	if err != nil {
		return nil, err
	}
	page.content = inner
	return page, nil
}

// restoreIfShuffled reorders paragraphs of a content fragment using the
// chapterlog.js shuffle template parameters for the given chapter id.
func (d *Downloader) restoreIfShuffled(html string, chapterID int, scriptURL string) string {
	tmpl := d.getTemplate(scriptURL)
	if tmpl == nil {
		return html
	}
	params := tmpl.paramsFor(chapterID)
	children := parseFragmentChildren(html)
	restored := restoreParagraphs(children, params)
	if restored == nil {
		return html
	}
	return serializeNodes(restored)
}

func (d *Downloader) getTemplate(scriptURL string) *shuffleTemplate {
	d.mu.Lock()
	tmpl, ok := d.templates[scriptURL]
	d.mu.Unlock()
	if ok {
		return tmpl
	}
	fullURL := scriptURL
	if !strings.HasPrefix(scriptURL, "http") {
		fullURL = downloadDomain + scriptURL
	}
	var parsed *shuffleTemplate
	// Use a detached context; the chapterlog resource is small and its
	// failure must not abort the whole download (just leave paragraphs
	// shuffled, mirrored from the Python reference).
	js, err := d.get(context.Background(), fullURL)
	if err != nil {
		parsed = nil
	} else {
		parsed = parseShuffleTemplate(js)
	}
	d.mu.Lock()
	d.templates[scriptURL] = parsed
	d.mu.Unlock()
	return parsed
}
