package library_test

import (
	"testing"

	"github.com/BrandonKowalski/cw/internal/library"
)

func TestTagsFromVorbisPicard(t *testing.T) {
	g := library.TagsFromVorbis(map[string][]string{
		"TITLE": {"Bad Blood"}, "ARTIST": {"Taylor Swift"}, "ALBUMARTIST": {"Taylor Swift"},
		"ALBUM": {"1989"}, "TRACKNUMBER": {"8"}, "TOTALTRACKS": {"21"}, "DISCNUMBER": {"1/2"},
		"DATE": {"2023-10-27"}, "RELEASETYPE": {"album", "compilation"}, "ALBUMARTISTSORT": {"Swift, Taylor"},
	}, "1-08 Bad Blood")
	want := library.Tags{Title: "Bad Blood", Artist: "Taylor Swift", AlbumArtist: "Taylor Swift", Album: "1989",
		TrackNumber: 8, TrackTotal: 21, DiscNumber: 1, DiscTotal: 2, Year: 2023, SortAlbumArtist: "Swift, Taylor"}
	if g != want {
		t.Fatalf("%+v", g)
	}
}

func TestTagsFromVorbisMissing(t *testing.T) {
	g := library.TagsFromVorbis(map[string][]string{"ARTIST": {"A", "B"}}, "07 Afterglow")
	if g.Title != "07 Afterglow" || g.Artist != "A, B" || g.AlbumArtist != "A, B" || g.TrackNumber != 0 || g.Genre != "" {
		t.Fatalf("%+v", g)
	}
}

func TestCompilationOnlyFromTag(t *testing.T) {
	if !library.TagsFromVorbis(map[string][]string{"COMPILATION": {"1"}}, "x").Compilation {
		t.Fatal()
	}
	if library.TagsFromVorbis(map[string][]string{"COMPILATION": {"0"}}, "x").Compilation {
		t.Fatal()
	}
}

func TestTotalFromSlashAndOriginalYear(t *testing.T) {
	g := library.TagsFromVorbis(map[string][]string{"TRACKNUMBER": {"3/12"}, "ORIGINALYEAR": {"1999"}}, "x")
	if g.TrackNumber != 3 || g.TrackTotal != 12 || g.Year != 1999 {
		t.Fatalf("%+v", g)
	}
}
