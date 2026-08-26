// Package epub assembles and reads EPUB/TXT books.
package epub

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"mime"
	"os"
	"strings"
	"time"
)

// Bundle is the input for EPUB assembly (mirrors LNReader's build_epub).
type Bundle struct {
	ID       string // e.g. "bilinovel-12345"
	Title    string
	Author   string
	Cover    []byte // raw cover bytes; nil when absent
	Volumes  []VolumeRef
	Chapters []ChapterRef
	Images   []Image // images referenced by chapter HTML <img src="...">
}

// VolumeRef groups a contiguous run of chapters.
type VolumeRef struct {
	Title string
	Count int
}

// ChapterRef is one chapter HTML fragment.
type ChapterRef struct {
	Title string
	HTML  string
}

// Image is a downloaded image, keyed by the source URL it appears as
// inside chapter HTML.
type Image struct {
	Src  string
	Data []byte
}

const (
	opfNS       = "http://www.idpf.org/2007/opf"
	dcNS        = "http://purl.org/dc/elements/1.1/"
	ncxNS       = "http://www.daisy.org/z3986/2005/ncx/"
	epubNS      = "http://www.idpf.org/2007/ops"
	xhtmlType   = "application/xhtml+xml"
	coverExt    = ".jpg"
	coverMime   = "image/jpeg"
	mimetypeStr = "application/epub+zip"
)

// Write assembles an EPUB at path compatible with common readers
// (spine + nav + ncx, cover item when present).
func Write(outPath string, b Bundle) error {
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)

	// mimetype MUST be first and stored (uncompressed) per EPUB spec.
	fw, err := zw.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	if err != nil {
		return err
	}
	if _, err := fw.Write([]byte(mimetypeStr)); err != nil {
		return err
	}

	container := `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`
	if err := writeZip(zw, "META-INF/container.xml", container); err != nil {
		return err
	}

	// ---- image registry: src -> (file name, media type) ----
	imageMap := map[string]imageEntry{}
	var imageOrder []string
	for _, img := range b.Images {
		if _, ok := imageMap[img.Src]; ok {
			continue
		}
		ext, media := mediaForSrc(img.Src)
		name := fmt.Sprintf("images/img%05d%s", len(imageMap)+1, ext)
		imageMap[img.Src] = imageEntry{name, media}
		imageOrder = append(imageOrder, img.Src)
	}
	for _, src := range imageOrder {
		img := b.findImage(src)
		if img == nil {
			continue
		}
		if err := writeZipBytes(zw, "OEBPS/"+imageMap[src].name, img.Data); err != nil {
			return err
		}
	}

	meta := manifestMeta{ID: b.ID, Title: b.Title, Author: b.Author,
		Modified: time.Now().UTC().Format("2006-01-02T15:04:05Z")}

	if len(b.Cover) > 0 {
		if err := writeZipBytes(zw, "OEBPS/images/cover"+coverExt, b.Cover); err != nil {
			return err
		}
		if err := writeZip(zw, "OEBPS/cover.xhtml", coverXHTML(b.Title, imagesCoverPath())); err != nil {
			return err
		}
		meta.items = append(meta.items, manifestItem{"cover", "cover.xhtml", xhtmlType, ""})
		meta.items = append(meta.items, manifestItem{"coverimg", "images/cover" + coverExt,
			coverMime, ` properties="cover-image"`})
		meta.spine = append(meta.spine, "cover")
	}

	for idx, ch := range b.Chapters {
		htmlText := ch.HTML
		// Rewrite image srcs to relative EPUB paths.
		for _, src := range imageOrder {
			if strings.Contains(htmlText, src) {
				htmlText = strings.ReplaceAll(htmlText, src, "../"+imageMap[src].name)
			}
		}
		file := fmt.Sprintf("chapters/ch%05d.xhtml", idx+1)
		if err := writeZip(zw, "OEBPS/"+file, chapterXHTML(ch.Title, htmlText)); err != nil {
			return err
		}
		id := fmt.Sprintf("ch%05d", idx+1)
		meta.items = append(meta.items, manifestItem{id, file, xhtmlType, ""})
		meta.spine = append(meta.spine, id)
	}

	meta.nav = navEntry{File: "nav.xhtml", Props: ` properties="nav"`}
	meta.ncx = navEntry{File: "toc.ncx"}

	points := buildNavPoints(b)
	if err := writeZip(zw, "OEBPS/nav.xhtml", navXHTML(b.Title, points)); err != nil {
		return err
	}
	if err := writeZip(zw, "OEBPS/toc.ncx", ncxXML(meta.ID, b.Title, points)); err != nil {
		return err
	}
	if err := writeZip(zw, "OEBPS/content.opf", opfXML(meta)); err != nil {
		return err
	}

	if err := zw.Close(); err != nil {
		return err
	}
	return os.WriteFile(outPath, buf.Bytes(), 0o644)
}

func (b *Bundle) findImage(src string) *Image {
	for i := range b.Images {
		if b.Images[i].Src == src {
			return &b.Images[i]
		}
	}
	return nil
}

func mediaForSrc(src string) (ext, media string) {
	lower := strings.ToLower(src)
	for _, cand := range []string{".png", ".gif", ".webp", ".jpeg"} {
		if strings.Contains(lower, cand) {
			ext = cand
			break
		}
	}
	if ext == "" {
		return coverExt, coverMime
	}
	if m := mime.TypeByExtension(ext); m != "" {
		return ext, m
	}
	switch ext {
	case ".png":
		return ext, "image/png"
	case ".gif":
		return ext, "image/gif"
	case ".webp":
		return ext, "image/webp"
	default:
		return ".jpg", coverMime
	}
}

func imagesCoverPath() string { return "images/cover" + coverExt }

type imageEntry struct {
	name  string
	media string
}

type navEntry struct {
	File  string
	Props string
}

type manifestItem struct {
	ID        string
	Href      string
	MediaType string
	Extra     string
}

type manifestMeta struct {
	ID       string
	Title    string
	Author   string
	Modified string
	items    []manifestItem
	spine    []string
	nav      navEntry
	ncx      navEntry
}

type navPoint struct {
	ID          string
	Label       string
	Content     string
	PlayOrder   int
	Children    []navPoint
	IsContainer bool // volume group; no content of its own
}

// buildNavPoints groups chapters under volume sections exactly like
// LNReader's TOC code (volumes flatten to sections only when > 1 volume).
func buildNavPoints(b Bundle) []navPoint {
	var nav []navPoint
	order := 0
	pos := 0
	for _, vol := range b.Volumes {
		count := vol.Count
		group := b.Chapters[pos:min(pos+count, len(b.Chapters))]
		pos += count
		if len(group) == 0 {
			continue
		}
		add := func(g []ChapterRef, base int) []navPoint {
			out := make([]navPoint, 0, len(g))
			for i, ch := range g {
				order++
				out = append(out, navPoint{
					ID:      fmt.Sprintf("ch%05d", base+i+1),
					Label:   ch.Title,
					Content: fmt.Sprintf("chapters/ch%05d.xhtml", base+i+1),
				})
			}
			return out
		}
		if vol.Title != "" && len(b.Volumes) > 1 {
			children := add(group, pos-len(group))
			last := children[len(children)-1]
			order = last.PlayOrder
			nav = append(nav, navPoint{Label: vol.Title, Children: children, IsContainer: true})
		} else {
			nav = append(nav, add(group, pos-len(group))...)
		}
	}
	// Reassign play orders after grouping (containers take the last child's).
	order = 0
	var assign func([]navPoint)
	assign = func(pts []navPoint) {
		for i := range pts {
			if pts[i].IsContainer {
				assign(pts[i].Children)
			} else {
				order++
				pts[i].PlayOrder = order
			}
		}
	}
	assign(nav)
	return nav
}

func coverXHTML(title, coverPath string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>` + xmlEscape(title) + `</title>
<style>body{margin:0;padding:0}img{display:block;margin:0 auto;max-width:100%;max-height:100vh}</style>
</head>
<body><div><img src="` + coverPath + `" alt="cover"/></div></body>
</html>`
}

func chapterXHTML(title, body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>` + xmlEscape(title) + `</title></head>
<body><h1>` + xmlEscape(title) + `</h1>` + body + `</body>
</html>`
}

func navXHTML(title string, points []navPoint) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="` + epubNS + `">
<head><title>` + xmlEscape(title) + `</title></head>
<body><nav epub:type="toc"><h1>` + xmlEscape(title) + `</h1><ol>`)
	writeNavOL(&sb, points)
	sb.WriteString(`</ol></nav></body>
</html>`)
	return sb.String()
}

func writeNavOL(sb *strings.Builder, points []navPoint) {
	for _, p := range points {
		sb.WriteString("<li>")
		if p.IsContainer {
			sb.WriteString("<span>" + xmlEscape(p.Label) + "</span><ol>")
			writeNavOL(sb, p.Children)
			sb.WriteString("</ol>")
		} else {
			sb.WriteString(`<a href="` + p.Content + `">` + xmlEscape(p.Label) + "</a>")
		}
		sb.WriteString("</li>")
	}
}

func ncxXML(id, title string, points []navPoint) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<ncx xmlns="` + ncxNS + `" version="2005-1">
  <head>
    <meta name="dtb:uid" content="` + xmlEscape(id) + `"/>
    <meta name="dtb:depth" content="2"/>
  </head>
  <docTitle><text>` + xmlEscape(title) + `</text></docTitle>
  <navMap>`)
	writeNavPoints(&sb, points)
	sb.WriteString(`  </navMap>
</ncx>`)
	return sb.String()
}

func writeNavPoints(sb *strings.Builder, points []navPoint) {
	for _, p := range points {
		if p.IsContainer {
			writeNavPoints(sb, p.Children) // container points carry no content
			continue
		}
		sb.WriteString(`    <navPoint id="` + p.ID + `" playOrder="` +
			fmt.Sprint(p.PlayOrder) + `">
      <navLabel><text>` + xmlEscape(p.Label) + `</text></navLabel>
      <content src="` + p.Content + `"/>
    </navPoint>` + "\n")
	}
}

func opfXML(m manifestMeta) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="` + opfNS + `" version="3.0" unique-identifier="book-id">
  <metadata xmlns:dc="` + dcNS + `">
    <dc:identifier id="book-id">` + xmlEscape(m.ID) + `</dc:identifier>
    <dc:title>` + xmlEscape(m.Title) + `</dc:title>
    <dc:language>zh</dc:language>`)
	if m.Author != "" && m.Author != "Unknown" {
		sb.WriteString("\n    <dc:creator>" + xmlEscape(m.Author) + "</dc:creator>")
	}
	sb.WriteString("\n    <meta property=\"dcterms:modified\">" + m.Modified + "</meta>\n  </metadata>\n  <manifest>\n")
	sb.WriteString(`    <item id="nav" href="` + m.nav.File + `" media-type="application/xhtml+xml"` +
		m.nav.Props + `/>` + "\n")
	sb.WriteString(`    <item id="ncx" href="` + m.ncx.File + `" media-type="application/x-dtbncx+xml"/>` + "\n")
	for _, it := range m.items {
		sb.WriteString(`    <item id="` + it.ID + `" href="` + it.Href + `" media-type="` +
			it.MediaType + `"` + it.Extra + `/>` + "\n")
	}
	sb.WriteString("  </manifest>\n  <spine toc=\"ncx\">\n")
	for _, idref := range m.spine {
		sb.WriteString(`    <itemref idref="` + idref + `"/>` + "\n")
	}
	sb.WriteString("  </spine>\n</package>")
	return sb.String()
}

func writeZip(zw *zip.Writer, name, content string) error {
	return writeZipBytes(zw, name, []byte(content))
}

func writeZipBytes(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func xmlEscape(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}
