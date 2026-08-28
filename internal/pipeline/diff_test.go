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
