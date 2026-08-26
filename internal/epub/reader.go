package epub

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// Chapter is one readable chapter of a book: title plus plain-text paragraphs.
type Chapter struct {
	Title      string
	Paragraphs []string
}

// Book is a parsed local book.
type Book struct {
	Title    string
	Author   string
	Chapters []Chapter
}

// ReadFile parses a .epub or .txt file by extension.
func ReadFile(path string) (*Book, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".epub":
		return readEPUB(path)
	case ".txt":
		return readTXT(path)
	default:
		return nil, fmt.Errorf("unsupported file type: %s", filepath.Ext(path))
	}
}

// ---- EPUB ----

type containerDoc struct {
	Rootfiles []struct {
		Rootfile []struct {
			FullPath string `xml:"full-path,attr"`
		} `xml:"rootfile"`
	} `xml:"rootfiles"`
}

type opfDoc struct {
	Metadata struct {
		Title   string `xml:"http://purl.org/dc/elements/1.1/ title"`
		Creator string `xml:"http://purl.org/dc/elements/1.1/ creator"`
	} `xml:"metadata"`
	Manifest struct {
		Items []struct {
			ID         string `xml:"id,attr"`
			Href       string `xml:"href,attr"`
			MediaType  string `xml:"media-type,attr"`
			Properties string `xml:"properties,attr"`
		} `xml:"item"`
	} `xml:"manifest"`
	Spine struct {
		Itemrefs []struct {
			IDRef string `xml:"idref,attr"`
		} `xml:"itemref"`
	} `xml:"spine"`
}

func readEPUB(path string) (*Book, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open epub: %w", err)
	}
	defer zr.Close()

	read := func(name string) ([]byte, error) {
		for _, f := range zr.File {
			if f.Name == name {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return io.ReadAll(rc)
			}
		}
		return nil, fmt.Errorf("missing %s", name)
	}

	containerBytes, err := read("META-INF/container.xml")
	if err != nil {
		return nil, err
	}
	var container containerDoc
	if err := xml.Unmarshal(containerBytes, &container); err != nil {
		return nil, fmt.Errorf("container.xml: %w", err)
	}
	if len(container.Rootfiles) == 0 || len(container.Rootfiles[0].Rootfile) == 0 {
		return nil, fmt.Errorf("container.xml has no rootfile")
	}
	opfPath := container.Rootfiles[0].Rootfile[0].FullPath

	opfBytes, err := read(opfPath)
	if err != nil {
		return nil, err
	}
	var opf opfDoc
	if err := xml.Unmarshal(opfBytes, &opf); err != nil {
		return nil, fmt.Errorf("opf: %w", err)
	}

	book := &Book{
		Title:  strings.TrimSpace(opf.Metadata.Title),
		Author: strings.TrimSpace(opf.Metadata.Creator),
	}
	if book.Title == "" {
		book.Title = filepath.Base(strings.TrimSuffix(path, filepath.Ext(path)))
	}

	// Only spine-referenced XHTML documents are chapters; nav/ncx and
	// non-document items must never be parsed as reading content.
	byID := map[string]string{} // id -> href
	for _, item := range opf.Manifest.Items {
		mt := strings.ToLower(item.MediaType)
		if strings.Contains(mt, "css") || strings.Contains(mt, "image/") ||
			strings.Contains(mt, "ncx") || strings.Contains(mt, "oebps-package") ||
			strings.Contains(mt, "font") || strings.Contains(mt, "javascript") {
			continue
		}
		lower := strings.ToLower(item.Href)
		if strings.HasSuffix(lower, "nav.xhtml") || strings.HasSuffix(lower, "toc.ncx") {
			continue
		}
		base := filepath.Base(lower)
		if strings.Contains(item.Properties, "cover-image") ||
			base == "cover.xhtml" || base == "cover.htm" || base == "cover.html" {
			continue
		}
		byID[item.ID] = item.Href
	}

	baseDir := pathDir(opfPath)
	for i, itemref := range opf.Spine.Itemrefs {
		href, ok := byID[itemref.IDRef]
		if !ok {
			continue
		}
		full := joinPath(baseDir, href)
		data, err := read(full)
		if err != nil {
			continue // tolerate missing spine files
		}
		ch, ok := extractChapter(data, full, i)
		if !ok {
			continue
		}
		book.Chapters = append(book.Chapters, ch)
	}
	if len(book.Chapters) == 0 {
		return nil, fmt.Errorf("epub has no readable chapters")
	}
	return book, nil
}

func pathDir(p string) string {
	i := strings.LastIndex(p, "/")
	if i < 0 {
		return ""
	}
	return p[:i]
}

func joinPath(dir, href string) string {
	href = strings.Split(href, "#")[0]
	href = strings.Split(href, "?")[0]
	if dir == "" {
		return href
	}
	return dir + "/" + href
}

var blockElements = map[string]bool{
	"p": true, "div": true, "h1": true, "h2": true, "h3": true, "h4": true,
	"h5": true, "h6": true, "li": true, "blockquote": true, "pre": true,
	"section": true, "article": true, "table": true, "tr": true, "td": true,
	"th": true, "ul": true, "ol": true, "dl": true, "figure": true,
	"figcaption": true, "aside": true, "header": true, "footer": true,
	"main": true, "dd": true, "dt": true, "form": true, "fieldset": true,
	"center": true, "body": true, "nav": true,
}

// extractChapter converts an XHTML chapter into title + paragraphs.
func extractChapter(data []byte, name string, idx int) (Chapter, bool) {
	doc, err := html.Parse(strings.NewReader(string(data)))
	if err != nil {
		return Chapter{}, false
	}
	var paraBuf []string
	var cur strings.Builder
	var title string
	foundTitle := false

	flush := func(force bool) {
		text := strings.TrimSpace(cur.String())
		if text != "" {
			paraBuf = append(paraBuf, text)
			cur.Reset()
		} else if force {
			cur.Reset()
		}
	}

	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.DocumentNode {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
			return
		}
		if n.Type == html.ElementNode {
			tag := n.Data
			switch tag {
			case "script", "style", "head", "noscript":
				return
			case "br":
				flush(true)
				return
			case "img":
				cur.WriteString("[图]")
				return
			}
			if !foundTitle && (tag == "h1" || tag == "h2" || tag == "h3") {
				t := strings.TrimSpace(nodeText(n))
				if t != "" {
					title = t
					foundTitle = true
				}
				return
			}
			if blockElements[tag] {
				if cur.Len() > 0 {
					flush(true)
				}
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c)
				}
				flush(true)
				return
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
			return
		}
		if n.Type == html.TextNode {
			cur.WriteString(n.Data)
		}
	}
	walk(doc)

	flush(true)
	if title == "" {
		title = chapterFallbackTitle(name, idx)
	}
	return Chapter{Title: title, Paragraphs: paraBuf}, len(paraBuf) > 0
}

func nodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

// chapterFallbackTitle comes from the manifest href (e.g. ch00042.xhtml) or
// the default chapter numbering.
func chapterFallbackTitle(name string, idx int) string {
	base := filepath.Base(name)
	if pre := strings.TrimSuffix(base, filepath.Ext(base)); pre != "" && pre != "ch" {
		return pre
	}
	return fmt.Sprintf("Chapter %d", idx+1)
}

// ---- TXT ----

var txtChapterRe = regexp.MustCompile(`(?m)^(第[一二三四五六七八九十百千\d]+[章話节節回]|Chapter\s*\d+|CHAPTER\s*\d+|第\d+章)`)

func readTXT(path string) (*Book, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	title := filepath.Base(strings.TrimSuffix(path, filepath.Ext(path)))
	book := &Book{Title: title, Author: "Unknown"}

	matches := txtChapterRe.FindAllStringSubmatchIndex(text, -1)
	if len(matches) >= 2 {
		for i, m := range matches {
			start := m[0]
			end := len(text)
			if i+1 < len(matches) {
				end = matches[i+1][0]
			}
			chText := strings.TrimSpace(text[start:end])
			var paras []string
			for _, p := range strings.Split(chText, "\n\n") {
				p = strings.TrimSpace(p)
				if p != "" {
					paras = append(paras, p)
				}
			}
			book.Chapters = append(book.Chapters, Chapter{
				Title:      strings.TrimSpace(matchOf(text, m)),
				Paragraphs: paras,
			})
		}
		return book, nil
	}

	// No chapter markers: split into chunks of ~3000 chars.
	const chunkSize = 3000
	runes := []rune(text)
	for i := 0; i < len(runes); i += chunkSize {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunk := strings.TrimSpace(string(runes[i:end]))
		if chunk == "" {
			continue
		}
		book.Chapters = append(book.Chapters, Chapter{
			Title:      fmt.Sprintf("Page %d", len(book.Chapters)+1),
			Paragraphs: strings.Split(chunk, "\n"),
		})
	}
	if len(book.Chapters) == 0 {
		return nil, fmt.Errorf("txt has no readable content")
	}
	return book, nil
}

func matchOf(text string, m []int) string {
	if m[0] < 0 || m[1] > len(text) {
		return ""
	}
	return text[m[0]:m[1]]
}
