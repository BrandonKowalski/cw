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

func fixtureBytes(t *testing.T) []byte {
	t.Helper()
	p := testutil.MakeFLAC(t, t.TempDir(), "f.flac", testutil.FLACOpts{Seconds: 0.2, Rate: 44100, Bits: 16,
		Tags: map[string]string{"TITLE": "T", "ARTIST": "A"}, CoverPNG: true})
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func blockStarts(t *testing.T, b []byte) map[byte]int {
	t.Helper()
	starts := map[byte]int{}
	off := 4
	for {
		typ := b[off] & 0x7f
		last := b[off]&0x80 != 0
		starts[typ] = off
		off += 4 + (int(b[off+1])<<16 | int(b[off+2])<<8 | int(b[off+3]))
		if last {
			return starts
		}
	}
}

func readBytes(t *testing.T, b []byte) (*flac.Metadata, error) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "t.flac")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return flac.ReadFile(p)
}

func TestReadFileTruncated(t *testing.T) {
	b := fixtureBytes(t)
	starts := blockStarts(t, b)
	cases := map[string]int{
		"empty":              0,
		"inside magic":       2,
		"inside header":      6,
		"inside streaminfo":  starts[0] + 4 + 10,
		"inside vorbis":      starts[4] + 4 + 6,
		"vorbis end minus1":  starts[6] - 1,
		"inside picture":     starts[6] + 4 + 20,
		"picture end minus1": starts[6] + 4 + (int(b[starts[6]+1])<<16 | int(b[starts[6]+2])<<8 | int(b[starts[6]+3])) - 1,
	}
	for name, n := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := readBytes(t, b[:n]); err == nil {
				t.Fatalf("want error at %d", n)
			}
		})
	}
}

func TestReadFileVorbisLengthExceedsBlock(t *testing.T) {
	b := fixtureBytes(t)
	body := []byte{0, 0, 0, 0, 1, 0, 0, 0, 0xe8, 0x03, 0, 0, 'A', '=', 'b'}
	out := append([]byte{}, b[:42]...)
	out[4] &= 0x7f
	out = append(out, 0x84, 0, 0, byte(len(body)))
	out = append(out, body...)
	if _, err := readBytes(t, out); err == nil {
		t.Fatal("want error")
	}
}

func TestReadFileID3Prefix(t *testing.T) {
	b := fixtureBytes(t)
	base, err := readBytes(t, b)
	if err != nil {
		t.Fatal(err)
	}
	for _, footer := range []bool{false, true} {
		size := 300
		hdr := []byte{'I', 'D', '3', 4, 0, 0, 0, 0, byte(size >> 7), byte(size & 0x7f)}
		total := 10 + size
		if footer {
			hdr[5] = 0x10
			total += 10
		}
		prefix := append(hdr, make([]byte, size)...)
		if footer {
			prefix = append(prefix, make([]byte, 10)...)
		}
		m, err := readBytes(t, append(prefix, b...))
		if err != nil {
			t.Fatalf("footer=%v: %v", footer, err)
		}
		if m.AudioOffset != base.AudioOffset+int64(total) {
			t.Fatalf("footer=%v offset %d want %d", footer, m.AudioOffset, base.AudioOffset+int64(total))
		}
		if m.Stream != base.Stream || len(m.Pictures) != 1 || m.Tags["TITLE"][0] != "T" {
			t.Fatalf("footer=%v mismatch", footer)
		}
	}
}
