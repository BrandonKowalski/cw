package library_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/BrandonKowalski/cw/internal/library"
)

func TestScanFiltersAndSorts(t *testing.T) {
	r := t.TempDir()
	for _, p := range []string{"B/x/02 b.flac", "A/y/01 a.FLAC", "A/y/cover.jpg", ".hidden/z.flac", "A/y/._01 a.flac", "Music/Music Library.musiclibrary/Application.musicdb"} {
		if err := os.MkdirAll(filepath.Join(r, filepath.Dir(p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(r, p), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	es, err := library.Scan(r)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, e := range es {
		got = append(got, e.RelPath)
	}
	if !reflect.DeepEqual(got, []string{"A/y/01 a.FLAC", "B/x/02 b.flac"}) {
		t.Fatal(got)
	}
}

func TestScanMissingRoot(t *testing.T) {
	if _, err := library.Scan(filepath.Join(t.TempDir(), "gone")); err == nil {
		t.Fatal("expected error for missing root")
	}
}
