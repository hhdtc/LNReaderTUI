// Package pipeline orchestrates the bilinovel download → EPUB assembly flow.
package pipeline

import (
	"context"
	"fmt"
	"strings"

	"lnreadertui/internal/epub"
	"lnreadertui/internal/site"
)

// Progress reports download progress to the UI.
type Progress struct {
	Done   int // chapters fetched so far
	Total  int
	Title  string // current chapter title
	Volume string // current volume ("" for single-volume books)
	Err    error  // terminal error, when non-nil
}

// FetchNovel downloads the entire novel (meta, catalog, every chapter and
// its images) and assembles an EPUB at outPath.
// onProgress is invoked from inside the download goroutine.
func FetchNovel(ctx context.Context, d *site.Downloader, novelID, srcURL string,
	onProgress func(Progress), outPath string) (*epub.Bundle, error) {

	if onProgress != nil {
		onProgress(Progress{Title: "novel page"})
	}
	meta, err := d.FetchNovel(ctx, novelID)
	if err != nil {
		return nil, err
	}
	if onProgress != nil {
		onProgress(Progress{Title: "catalog"})
	}
	volumes, err := d.FetchCatalog(ctx, novelID)
	if err != nil {
		return nil, err
	}
	total := 0
	for _, v := range volumes {
		total += len(v.Chapters)
	}

	chapters := make([]epub.ChapterRef, 0, total)
	imgSet := map[string]bool{}
	var imgOrder []string
	pos := 0
	for _, vol := range volumes {
		for _, ref := range vol.Chapters {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			url := ref.Href
			if url == "" {
				url, err = d.ResolveChapterURL(ctx, volumes, pos)
				if err != nil {
					return nil, fmt.Errorf("chapter %d (%s): %w", pos+1, ref.Title, err)
				}
			}
			ch, err := d.FetchChapter(ctx, url)
			if err != nil {
				return nil, fmt.Errorf("chapter %d (%s): %w", pos+1, ref.Title, err)
			}
			if ch.Title == "" {
				ch.Title = ref.Title
			}
			chapters = append(chapters, epub.ChapterRef{Title: ch.Title, HTML: ch.HTML})
			for _, src := range ch.ImageURLs {
				if !imgSet[src] {
					imgSet[src] = true
					imgOrder = append(imgOrder, src)
				}
			}
			if onProgress != nil {
				onProgress(Progress{Done: pos + 1, Total: total, Title: ch.Title, Volume: vol.Title})
			}
			pos++
		}
	}

	// Download chapter images (rate-limited by the downloader).
	var images []epub.Image
	for i, src := range imgOrder {
		if onProgress != nil && i%4 == 0 {
			onProgress(Progress{Done: total, Total: total,
				Title: fmt.Sprintf("images %d/%d", i, len(imgOrder))})
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		data, err := d.DownloadImage(ctx, src)
		if err != nil {
			return nil, fmt.Errorf("image %s: %w", src, err)
		}
		images = append(images, epub.Image{Src: src, Data: data})
	}

	bundle := &epub.Bundle{
		ID:       "bilinovel-" + novelID,
		Title:    meta.Title,
		Author:   meta.Author,
		Volumes:  toVolumeRefs(volumes),
		Chapters: chapters,
		Images:   images,
	}
	if meta.CoverURL != "" {
		if !imgSet[meta.CoverURL] {
			if data, err := d.DownloadImage(ctx, meta.CoverURL); err == nil {
				bundle.Cover = data
			}
		} else {
			for _, img := range images {
				if img.Src == meta.CoverURL {
					bundle.Cover = img.Data
					break
				}
			}
		}
	}

	if err := epub.Write(outPath, *bundle); err != nil {
		return nil, err
	}
	return bundle, nil
}

func toVolumeRefs(volumes []site.VolumeRef) []epub.VolumeRef {
	out := make([]epub.VolumeRef, 0, len(volumes))
	for _, v := range volumes {
		out = append(out, epub.VolumeRef{Title: v.Title, Count: len(v.Chapters)})
	}
	return out
}

// UpdateResult reports what an update appended.
type UpdateResult struct {
	OldTotal      int
	NewTotal      int
	AddedChapters []AddedChapter
}

// AddedChapter is one newly fetched chapter.
type AddedChapter struct {
	Volume string
	Title  string
}

// UpdateNovel appends newly published chapters/volumes to the existing EPUB
// at path. Old content is preserved byte-for-byte; the file is rewritten
// atomically. The reading-progress indexes stay valid because new content
// is strictly appended after the last existing chapter.
func UpdateNovel(ctx context.Context, d *site.Downloader, novelID, path string,
	onProgress func(Progress)) (*UpdateResult, error) {

	if onProgress != nil {
		onProgress(Progress{Title: "novel page"})
	}
	if _, err := d.FetchNovel(ctx, novelID); err != nil {
		return nil, err
	}
	if onProgress != nil {
		onProgress(Progress{Title: "catalog"})
	}
	volumes, err := d.FetchCatalog(ctx, novelID)
	if err != nil {
		return nil, err
	}

	old, err := epub.ReadBundle(path)
	if err != nil {
		return nil, fmt.Errorf("read local book: %w", err)
	}

	start, addedFlat, mergedVols, err := diffCatalog(oldTitles(old), volumes)
	if err != nil {
		return nil, err
	}
	res := &UpdateResult{OldTotal: len(old.Chapters), NewTotal: len(old.Chapters) + len(addedFlat)}
	if len(addedFlat) == 0 {
		return res, nil
	}

	chapters := make([]epub.ChapterRef, 0, len(addedFlat))
	imgSet := map[string]bool{}
	var imgOrder []string
	for i, ref := range addedFlat {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		pos := start + i
		url := ref.Href
		if url == "" {
			url, err = d.ResolveChapterURL(ctx, volumes, pos)
			if err != nil {
				return nil, fmt.Errorf("chapter %d (%s): %w", pos+1, ref.Title, err)
			}
		}
		ch, err := d.FetchChapter(ctx, url)
		if err != nil {
			return nil, fmt.Errorf("chapter %d (%s): %w", pos+1, ref.Title, err)
		}
		if ch.Title == "" {
			ch.Title = ref.Title
		}
		chapters = append(chapters, epub.ChapterRef{Title: ch.Title, HTML: ch.HTML})
		for _, src := range ch.ImageURLs {
			if !imgSet[src] {
				imgSet[src] = true
				imgOrder = append(imgOrder, src)
			}
		}
		if onProgress != nil {
			onProgress(Progress{Done: i + 1, Total: len(addedFlat),
				Title: ch.Title, Volume: volumeAt(volumes, pos)})
		}
		res.AddedChapters = append(res.AddedChapters,
			AddedChapter{Volume: volumeAt(volumes, pos), Title: ch.Title})
	}

	var images []epub.Image
	for i, src := range imgOrder {
		if onProgress != nil && i%4 == 0 {
			onProgress(Progress{Done: len(addedFlat), Total: len(addedFlat),
				Title: fmt.Sprintf("images %d/%d", i, len(imgOrder))})
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		data, err := d.DownloadImage(ctx, src)
		if err != nil {
			return nil, fmt.Errorf("image %s: %w", src, err)
		}
		images = append(images, epub.Image{Src: src, Data: data})
	}

	bundle := &epub.Bundle{
		ID:       "bilinovel-" + novelID,
		Title:    old.Title,
		Author:   old.Author,
		Chapters: chapters,
		Images:   images,
	}
	if err := epub.Append(path, old, *bundle, mergedVols); err != nil {
		return nil, fmt.Errorf("append epub: %w", err)
	}
	return res, nil
}

func oldTitles(old *epub.RawBook) []string {
	out := make([]string, len(old.Chapters))
	for i, ch := range old.Chapters {
		out[i] = ch.Title
	}
	return out
}

// volumeAt returns the volume title containing chapter pos.
func volumeAt(volumes []site.VolumeRef, pos int) string {
	offset := 0
	for _, v := range volumes {
		if pos < offset+len(v.Chapters) {
			return v.Title
		}
		offset += len(v.Chapters)
	}
	return ""
}

// diffCatalog compares the local chapter titles against the remote volume
// tree. It returns:
//   - start: the first chapter index to fetch (== len(localTitles))
//   - added: the chapters to append
//   - mergedVols: the COMPLETE volume layout (old split + new parts)
//
// The remote catalog must be a strict prefix extension of the local book;
// any divergence aborts (the reader-side index match would be unreliable).
func diffCatalog(localTitles []string, remote []site.VolumeRef) (int, []site.ChapterRef, []epub.VolumeRef, error) {
	var flat []site.ChapterRef
	for _, v := range remote {
		flat = append(flat, v.Chapters...)
	}
	if len(flat) < len(localTitles) {
		return 0, nil, nil, fmt.Errorf(
			"site catalog has %d chapters; local book has %d — cannot append",
			len(flat), len(localTitles))
	}
	for i, t := range localTitles {
		expect := strings.TrimSpace(flat[i].Title)
		if expect == "" {
			expect = fmt.Sprintf("第 %d 章", i+1)
		}
		if strings.TrimSpace(t) != expect {
			return 0, nil, nil, fmt.Errorf(
				"site catalog changed at chapter %d (%q vs local %q); update aborted",
				i+1, expect, t)
		}
	}
	// The volume layout is the remote one: chapters are contiguous, so the
	// nav grouping walks volumes by count regardless of the local/new split.
	return len(localTitles), flat[len(localTitles):], toVolumeRefs(remote), nil
}
