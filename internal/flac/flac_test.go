package flac_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/BrandonKowalski/cw/internal/flac"
	"github.com/BrandonKowalski/cw/internal/testutil"
)

func TestReadFileStreamAndTags(t *testing.T) {
	p := testutil.MakeFLAC(t, t.TempDir(), "a.flac", testutil.FLACOpts{Seconds: 1, Rate: 44100, Bits: 16,
		Tags: map[string]string{"TITLE": "Hourglass", "ARTIST": "Sleep Theory", "TRACKNUMBER": "2"}, CoverPNG: true})
	m, err := flac.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if m.Stream.SampleRate != 44100 || m.Stream.BitsPerSample != 16 || m.Stream.Channels != 1 || m.Stream.TotalSamples != 44100 {
		t.Fatalf("%+v", m.Stream)
	}
	if m.Tags["TITLE"][0] != "Hourglass" || m.Tags["TRACKNUMBER"][0] != "2" {
		t.Fatalf("%v", m.Tags)
	}
	if len(m.Pictures) != 1 || m.Pictures[0].Type != 3 || m.Pictures[0].MIME != "image/png" || len(m.Pictures[0].Data) == 0 {
		t.Fatal("picture")
	}
	if m.AudioOffset <= 0 {
		t.Fatal("offset")
	}
}

func TestReadFile24Bit48k(t *testing.T) {
	p := testutil.MakeFLAC(t, t.TempDir(), "b.flac", testutil.FLACOpts{Seconds: 0.5, Rate: 48000, Bits: 24})
	m, err := flac.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if m.Stream.SampleRate != 48000 || m.Stream.BitsPerSample != 24 {
		t.Fatalf("%+v", m.Stream)
	}
}

func TestReadFileRejectsNonFLAC(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.flac")
	if err := os.WriteFile(p, []byte("ID3 not flac"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := flac.ReadFile(p); err == nil {
		t.Fatal("want error")
	}
}
