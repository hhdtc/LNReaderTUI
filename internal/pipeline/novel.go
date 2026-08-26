// Package pipeline orchestrates the bilinovel download → EPUB assembly flow.
package pipeline

import (
	"context"
	"fmt"

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
