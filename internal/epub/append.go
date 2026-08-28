// Append-style EPUB updates: read an existing EPUB back (byte-preserving)
// and merge new chapters/volumes after it.
package epub

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// RawChapter is one chapter of an existing EPUB, kept byte-for-byte.
type RawChapter struct {
	Title string
	Href  string // path inside the zip, e.g. "chapters/ch00005.xhtml"
	Body  []byte // original XHTML document
}

// RawImage is an image carried over from an existing EPUB.
type RawImage struct {
	Href string
	Data []byte
}

// RawEntry is any other file carried over verbatim (styles, fonts, …).
type RawEntry struct {
	Name string
	Data []byte
}

// RawBook is an existing EPUB's content, read back for append-style updates.
type RawBook struct {
	ID        string
	Title     string
	Author    string
	Cover     []byte // cover-image bytes; nil when absent
	CoverHref string // path inside the zip
	Chapters  []RawChapter
	Images    []RawImage
	Other     []RawEntry // verbatim files (cover.xhtml, styles, fonts, …)
	Manifest  []manifestItem
	Spine     []string
}

// ReadBundle reads an EPUB into a RawBook. Navigation, table of contents and
// the package document are NOT part of Other: they are represented
// separately (Manifest/Spine) or regenerated on append.
func ReadBundle(path string) (*RawBook, error) {
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

	book := &RawBook{
		ID:       strings.TrimSpace(opf.Metadata.Identifier),
		Title:    strings.TrimSpace(opf.Metadata.Title),
		Author:   strings.TrimSpace(opf.Metadata.Creator),
		Manifest: nil,
		Spine:    nil,
	}
	for _, item := range opf.Manifest.Items {
		base := filepath.Base(strings.ToLower(item.Href))
		if strings.EqualFold(item.ID, "nav") || strings.EqualFold(item.ID, "ncx") ||
			base == "nav.xhtml" || base == "toc.ncx" {
			continue // regenerated on every append
		}
		extra := ""
		if props := strings.TrimSpace(item.Properties); props != "" {
			extra = ` properties="` + props + `"`
		}
		book.Manifest = append(book.Manifest, manifestItem{
			ID: item.ID, Href: item.Href, MediaType: item.MediaType, Extra: extra,
		})
		if strings.Contains(item.Properties, "cover-image") {
			book.CoverHref = joinPath(pathDir(opfPath), item.Href)
			if data, err := read(book.CoverHref); err == nil {
				book.Cover = data
			}
		}
	}
	for _, ref := range opf.Spine.Itemrefs {
		book.Spine = append(book.Spine, ref.IDRef)
	}

	byID := map[string]string{} // id -> href
	for _, item := range opf.Manifest.Items {
		byID[item.ID] = item.Href
	}

	baseDir := pathDir(opfPath)
	for _, item := range opf.Manifest.Items {
		mt := strings.ToLower(item.MediaType)
		if !strings.Contains(mt, "image/") || strings.Contains(item.Properties, "cover-image") {
			continue
		}
		href := joinPath(baseDir, item.Href)
		data, err := read(href)
		if err != nil {
			continue // tolerate missing images
		}
		book.Images = append(book.Images, RawImage{Href: href, Data: data})
	}

	for i, itemref := range opf.Spine.Itemrefs {
		href, ok := byID[itemref.IDRef]
		if !ok {
			continue
		}
		base := filepath.Base(strings.ToLower(href))
		if base == "cover.xhtml" || base == "cover.htm" || base == "cover.html" {
			continue
		}
		full := joinPath(baseDir, href)
		data, err := read(full)
		if err != nil {
			continue // tolerate missing spine files
		}
		book.Chapters = append(book.Chapters, RawChapter{
			Title: chapterTitle(data, i),
			Href:  full,
			Body:  data,
		})
	}
	if len(book.Chapters) == 0 {
		return nil, fmt.Errorf("epub has no readable chapters")
	}

	// Everything else is carried over verbatim except regenerated content
	// and manifest-listed files (chapters/images/cover). Legacy EPUBs whose
	// images were never manifest-listed are still preserved as other files.
	carrySet := map[string]bool{}
	for _, img := range book.Images {
		carrySet[img.Href] = true
	}
	if book.CoverHref != "" {
		carrySet[book.CoverHref] = true
	}
	chaptersSet := map[string]bool{}
	for _, ch := range book.Chapters {
		chaptersSet[ch.Href] = true
	}
	for _, f := range zr.File {
		name := f.Name
		if name == "mimetype" || name == "META-INF/container.xml" || name == opfPath ||
			name == "OEBPS/nav.xhtml" || name == "OEBPS/toc.ncx" {
			continue
		}
		if chaptersSet[name] || carrySet[name] {
			continue
		}
		data, err := read(name)
		if err != nil {
			continue
		}
		book.Other = append(book.Other, RawEntry{Name: name, Data: data})
	}
	return book, nil
}

// chapterTitle extracts the <title> of a chapter XHTML document.
func chapterTitle(data []byte, idx int) string {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return fallbackChapterTitle(idx)
	}
	var title string
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if title != "" {
			return
		}
		if n.Type == html.ElementNode && n.Data == "title" {
			title = strings.TrimSpace(nodeText(n))
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if title == "" {
		return fallbackChapterTitle(idx)
	}
	return title
}

func fallbackChapterTitle(idx int) string { return fmt.Sprintf("第 %d 章", idx+1) }

// Append merges new chapters after an existing book and writes the result
// to path atomically (tmp + rename). Old content is preserved verbatim;
// nav/toc/package are regenerated to cover the merged book. allVolumes is
// the COMPLETE merged volume layout (old split + new parts).
func Append(path string, old *RawBook, b Bundle, allVolumes []VolumeRef) error {
	nOld := len(old.Chapters)

	// ---- new image registry: src -> file name inside the zip ----
	imageMap := map[string]imageEntry{}
	var imageOrder []string
	for _, img := range b.Images {
		if _, ok := imageMap[img.Src]; ok {
			continue
		}
		ext, media := mediaForSrc(img.Src)
		name := fmt.Sprintf("images/img%05d%s", len(old.Images)+len(imageMap)+1, ext)
		imageMap[img.Src] = imageEntry{name, media}
		imageOrder = append(imageOrder, img.Src)
	}

	// ---- merged nav points (titles only; body irrelevant for nav) ----
	navBundle := Bundle{ID: old.ID, Title: old.Title, Author: old.Author, Volumes: allVolumes}
	for _, oc := range old.Chapters {
		navBundle.Chapters = append(navBundle.Chapters, ChapterRef{Title: oc.Title})
	}
	for _, nc := range b.Chapters {
		navBundle.Chapters = append(navBundle.Chapters, ChapterRef{Title: nc.Title})
	}
	points := buildNavPoints(navBundle)

	// ---- merged package document ----
	meta := manifestMeta{ID: old.ID, Title: old.Title, Author: old.Author,
		Modified: time.Now().UTC().Format("2006-01-02T15:04:05Z")}
	meta.items = append(meta.items, old.Manifest...)
	meta.spine = append(meta.spine, old.Spine...)
	for i := range b.Chapters {
		file := fmt.Sprintf("chapters/ch%05d.xhtml", nOld+i+1)
		id := fmt.Sprintf("ch%05d", nOld+i+1)
		meta.items = append(meta.items, manifestItem{ID: id, Href: file, MediaType: xhtmlType, Extra: ""})
		meta.spine = append(meta.spine, id)
	}
	for _, src := range imageOrder {
		if img := b.findImage(src); img != nil {
			meta.items = append(meta.items, manifestItem{
				ID: "img_" + imageMap[src].name, Href: imageMap[src].name,
				MediaType: imageMap[src].media, Extra: ""})
		}
	}

	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)

	// mimetype first, stored (uncompressed) per EPUB spec.
	fw, err := zw.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	if err != nil {
		return err
	}
	if _, err := fw.Write([]byte(mimetypeStr)); err != nil {
		return err
	}
	if err := writeZip(zw, "META-INF/container.xml", containerXML()); err != nil {
		return err
	}

	for _, e := range old.Other {
		if err := writeZipBytes(zw, e.Name, e.Data); err != nil {
			return err
		}
	}
	if old.Cover != nil && old.CoverHref != "" {
		if err := writeZipBytes(zw, old.CoverHref, old.Cover); err != nil {
			return err
		}
	}
	for _, img := range old.Images {
		if err := writeZipBytes(zw, img.Href, img.Data); err != nil {
			return err
		}
	}
	for _, ch := range old.Chapters {
		if err := writeZipBytes(zw, ch.Href, ch.Body); err != nil {
			return err
		}
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
	for i, ch := range b.Chapters {
		htmlText := ch.HTML
		for _, src := range imageOrder {
			if strings.Contains(htmlText, src) {
				htmlText = strings.ReplaceAll(htmlText, src, "../"+imageMap[src].name)
			}
		}
		file := fmt.Sprintf("chapters/ch%05d.xhtml", nOld+i+1)
		if err := writeZip(zw, "OEBPS/"+file, chapterXHTML(ch.Title, htmlText)); err != nil {
			return err
		}
	}

	meta.nav = navEntry{File: "nav.xhtml", Props: ` properties="nav"`}
	meta.ncx = navEntry{File: "toc.ncx"}
	if err := writeZip(zw, "OEBPS/nav.xhtml", navXHTML(old.Title, points)); err != nil {
		return err
	}
	if err := writeZip(zw, "OEBPS/toc.ncx", ncxXML(meta.ID, old.Title, points)); err != nil {
		return err
	}
	if err := writeZip(zw, "OEBPS/content.opf", opfXML(meta)); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}

	tmp := path + ".append.tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func containerXML() string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`
}
