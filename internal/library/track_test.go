package library_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/BrandonKowalski/cw/internal/flac"
	"github.com/BrandonKowalski/cw/internal/library"
	"github.com/BrandonKowalski/cw/internal/testutil"
)

func TestLoadFingerprintsStableAndTagSensitive(t *testing.T) {
	dir := t.TempDir()
	opts := testutil.FLACOpts{Seconds: 1, Rate: 44100, Bits: 16, CoverPNG: true, Tags: map[string]string{"TITLE": "One", "ARTIST": "A"}}
	testutil.MakeFLAC(t, dir, "a.flac", opts)
	e := library.Entry{RelPath: "a.flac", Size: 1, ModTime: time.Unix(1, 0)}
	a, err := library.Load(dir, e)
	if err != nil {
		t.Fatal(err)
	}
	b, err := library.Load(dir, e)
	if err != nil {
		t.Fatal(err)
	}
	if a.TagFP != b.TagFP || a.AudioFP != b.AudioFP || a.AudioFP == "" || a.Entry != e {
		t.Fatal("unstable")
	}
	opts.Tags = map[string]string{"TITLE": "Two", "ARTIST": "A"}
	testutil.MakeFLAC(t, dir, "b.flac", opts)
	c, err := library.Load(dir, library.Entry{RelPath: "b.flac"})
	if err != nil {
		t.Fatal(err)
	}
	if c.AudioFP != a.AudioFP || c.TagFP == a.TagFP || c.Cover == nil {
		t.Fatal("fingerprints")
	}
}

func TestLoadFolderImageFallback(t *testing.T) {
	dir := t.TempDir()
	testutil.MakeFLAC(t, dir, "a.flac", testutil.FLACOpts{Seconds: 1, Rate: 44100, Bits: 16})
	tr, err := library.Load(dir, library.Entry{RelPath: "a.flac"})
	if err != nil || tr.Cover != nil {
		t.Fatal(err, tr.Cover)
	}
	os.WriteFile(filepath.Join(dir, "folder.png"), []byte("png"), 0o644)
	os.WriteFile(filepath.Join(dir, "cover.jpg"), []byte("jpg"), 0o644)
	tr, err = library.Load(dir, library.Entry{RelPath: "a.flac"})
	if err != nil || string(tr.Cover) != "jpg" {
		t.Fatal(err, string(tr.Cover))
	}
}

func TestPickCover(t *testing.T) {
	pics := []flac.Picture{{Type: 0, Data: []byte("x")}, {Type: 3, Data: []byte("front")}}
	if string(library.PickPicture(pics)) != "front" {
		t.Fatal()
	}
	if string(library.PickPicture(pics[:1])) != "x" {
		t.Fatal()
	}
	if library.PickPicture(nil) != nil {
		t.Fatal()
	}
}

func TestAudioFPZeroMD5(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.flac")
	os.WriteFile(p, []byte("headAUDIO"), 0o644)
	fp, err := library.AudioFingerprint(p, flac.StreamInfo{}, 4)
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(t.TempDir(), "y.flac")
	os.WriteFile(other, []byte("zzzzAUDIO"), 0o644)
	fp2, _ := library.AudioFingerprint(other, flac.StreamInfo{}, 4)
	if fp != fp2 || len(fp) != 64 {
		t.Fatal(fp, fp2)
	}
}
