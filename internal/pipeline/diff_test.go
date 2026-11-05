package pipeline

import (
	"testing"

	"lnreadertui/internal/epub"
	"lnreadertui/internal/site"
)

func ch(title string) site.ChapterRef { return site.ChapterRef{Title: title} }

func vol(title string, refs ...site.ChapterRef) site.VolumeRef {
	return site.VolumeRef{Title: title, Chapters: refs}
}

// TestDiffCatalogAppendAcrossVolumes: new chapters land after the local
// prefix, and the volume layout splits at the boundary.
func TestDiffCatalogAppendAcrossVolumes(t *testing.T) {
	remote := []site.VolumeRef{
		vol("卷一", ch("01"), ch("02")),
		vol("卷二", ch("03"), ch("04"), ch("05"), ch("06")),
	}
	start, added, merged, err := diffCatalog([]string{"01", "02", "03", "04"}, remote)
	if err != nil {
		t.Fatal(err)
	}
	if start != 4 {
		t.Fatalf("start = %d, want 4", start)
	}
	if len(added) != 2 || added[0].Title != "05" || added[1].Title != "06" {
		t.Fatalf("added = %+v", added)
	}
	want := []epub.VolumeRef{{Title: "卷一", Count: 2}, {Title: "卷二", Count: 4}}
	if len(merged) != len(want) {
		t.Fatalf("merged = %+v", merged)
	}
	for i := range want {
		if merged[i].Title != want[i].Title || merged[i].Count != want[i].Count {
			t.Fatalf("merged[%d] = %+v, want %+v", i, merged[i], want[i])
		}
	}
}

// TestDiffCatalogNoNewChapters.
func TestDiffCatalogNoNewChapters(t *testing.T) {
	remote := []site.VolumeRef{vol("卷", ch("01"), ch("02"))}
	start, added, merged, err := diffCatalog([]string{"01", "02"}, remote)
	if err != nil {
		t.Fatal(err)
	}
	if start != 2 || len(added) != 0 {
		t.Fatalf("start=%d added=%d", start, len(added))
	}
	if len(merged) != 1 || merged[0].Count != 2 {
		t.Fatalf("merged = %+v", merged)
	}
}

// TestDiffCatalogMismatchAborts: a mid-book title change must abort.
func TestDiffCatalogMismatchAborts(t *testing.T) {
	remote := []site.VolumeRef{vol("卷", ch("01"), ch("02b"))}
	if _, _, _, err := diffCatalog([]string{"01", "02a"}, remote); err == nil {
		t.Fatal("expected abort on catalog mismatch")
	}
}

// TestDiffCatalogLocalAheadAborts.
func TestDiffCatalogLocalAheadAborts(t *testing.T) {
	remote := []site.VolumeRef{vol("卷", ch("01"))}
	if _, _, _, err := diffCatalog([]string{"01", "02"}, remote); err == nil {
		t.Fatal("expected abort when local book has more chapters")
	}
}

// TestDiffCatalogSplitVolumeWithinOneVolume: local ends mid-volume.
func TestDiffCatalogSplitVolumeWithinOneVolume(t *testing.T) {
	remote := []site.VolumeRef{vol("卷", ch("01"), ch("02"), ch("03"))}
	_, added, merged, err := diffCatalog([]string{"01"}, remote)
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 2 {
		t.Fatalf("added = %d, want 2", len(added))
	}
	want := []epub.VolumeRef{{Title: "卷", Count: 3}}
	if len(merged) != len(want) {
		t.Fatalf("merged = %+v", merged)
	}
	if merged[0].Count != 3 {
		t.Fatalf("merged count = %d, want 3", merged[0].Count)
	}
}

// TestDiffCatalogToleratesSubstitutedGlyph: the listing page can serve one
// substituted character (女 -> N) while the chapter page keeps the real title,
// as observed on 玩乐关系 chapter 3 ("概要N王的败北" vs "概要女王的败北").
// The chapter heading is what both sides store, so this is not a divergence.
func TestDiffCatalogToleratesSubstitutedGlyph(t *testing.T) {
	local := []string{"插图", "【第一话】掷骰问题（Day176）", "【第二话】概要女王的败北（Day58）"}
	remote := []site.VolumeRef{vol("玩乐关系 1",
		ch("插图"),
		ch("【第一话】掷骰问题（Day176）"),
		ch("【第二话】概要N王的败北（Day58）"),
		ch("【第三话】隐藏真身的两难（Day93）"),
	)}
	start, added, _, err := diffCatalog(local, remote)
	if err != nil {
		t.Fatalf("substituted glyph aborted the update: %v", err)
	}
	if start != 3 || len(added) != 1 || added[0].Title != "【第三话】隐藏真身的两难（Day93）" {
		t.Fatalf("start=%d added=%+v", start, added)
	}
}

// TestDiffCatalogToleratesSpacing: listing spacing is not a title change.
func TestDiffCatalogToleratesSpacing(t *testing.T) {
	remote := []site.VolumeRef{vol("卷", ch("第 1 章 开始"), ch("第2章\u3000继续"))}
	if _, _, _, err := diffCatalog([]string{"第1章开始", "第2章 继续"}, remote); err != nil {
		t.Fatalf("whitespace aborted the update: %v", err)
	}
}

// TestDiffCatalogReorderedAborts: reordering keeps the count but must abort.
func TestDiffCatalogReorderedAborts(t *testing.T) {
	remote := []site.VolumeRef{vol("卷",
		ch("【第一话】掷骰问题（Day176）"),
		ch("【第三话】隐藏真身的两难（Day93）"),
		ch("【第二话】概要女王的败北（Day58）"),
	)}
	local := []string{
		"【第一话】掷骰问题（Day176）",
		"【第二话】概要女王的败北（Day58）",
		"【第三话】隐藏真身的两难（Day93）",
	}
	if _, _, _, err := diffCatalog(local, remote); err == nil {
		t.Fatal("expected abort on reordered catalog")
	}
}

// TestDiffCatalogSkipsUntitledListingEntry: an untitled listing entry (an
// image-only chapter, for example) cannot confirm its position, so it must not
// abort the update by comparison against a synthesized name.
func TestDiffCatalogSkipsUntitledListingEntry(t *testing.T) {
	remote := []site.VolumeRef{vol("卷", ch("插图"), ch(""), ch("【第二话】概要女王的败北（Day58）"))}
	if _, _, _, err := diffCatalog([]string{"插图", "第 2 章", "【第二话】概要女王的败北（Day58）"}, remote); err != nil {
		t.Fatalf("untitled listing entry aborted the update: %v", err)
	}
}

// TestDiffCatalogUntitledCatalogAborts: when the listing titles nothing, the
// prefix is unverifiable and a blind append must not happen.
func TestDiffCatalogUntitledCatalogAborts(t *testing.T) {
	remote := []site.VolumeRef{vol("卷", ch(""), ch(""), ch(""))}
	if _, _, _, err := diffCatalog([]string{"插图", "概述", "结语"}, remote); err == nil {
		t.Fatal("expected abort when the catalog titles nothing")
	}
}
