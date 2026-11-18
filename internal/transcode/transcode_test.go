package transcode_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BrandonKowalski/cw/internal/flac"
	"github.com/BrandonKowalski/cw/internal/testutil"
	"github.com/BrandonKowalski/cw/internal/transcode"
)

func TestArgsCDQualityNoFilter(t *testing.T) {
	a := strings.Join(transcode.Args("in.flac", "out.m4a", flac.StreamInfo{SampleRate: 44100, BitsPerSample: 16}), " ")
	if strings.Contains(a, "aresample") || !strings.Contains(a, "-c:a alac") || !strings.HasSuffix(a, "-f ipod out.m4a") {
		t.Fatal(a)
	}
}

func TestArgs24Bit48kKeepsRate(t *testing.T) {
	a := strings.Join(transcode.Args("i", "o", flac.StreamInfo{SampleRate: 48000, BitsPerSample: 24}), " ")
	if !strings.Contains(a, "aresample=osr=48000:osf=s16p:dither_method=triangular") {
		t.Fatal(a)
	}
}

func TestArgs96kResamples(t *testing.T) {
	a := strings.Join(transcode.Args("i", "o", flac.StreamInfo{SampleRate: 96000, BitsPerSample: 24}), " ")
	if !strings.Contains(a, "osr=44100") {
		t.Fatal(a)
	}
}

func TestOutputRate(t *testing.T) {
	if transcode.OutputRate(flac.StreamInfo{SampleRate: 48000}) != 48000 || transcode.OutputRate(flac.StreamInfo{SampleRate: 88200}) != 44100 {
		t.Fatal("rate")
	}
}

func TestRunProducesALAC(t *testing.T) {
	testutil.RequireFFmpeg(t)
	dir := t.TempDir()
	src := testutil.MakeFLAC(t, dir, "a.flac", testutil.FLACOpts{Seconds: 1, Rate: 48000, Bits: 24, CoverPNG: true})
	md, err := flac.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	si := md.Stream
	dst := filepath.Join(dir, "a.m4a")
	if err := transcode.Run(context.Background(), src, dst, si); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("ffprobe", "-v", "error", "-show_streams", dst).CombinedOutput()
	if err != nil {
		t.Fatal(err, string(out))
	}
	s := string(out)
	if strings.Count(s, "codec_type=audio") != 1 || strings.Contains(s, "codec_type=video") {
		t.Fatal(s)
	}
	for _, want := range []string{"codec_name=alac", "sample_rate=48000", "sample_fmt=s16p"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %s in %s", want, s)
		}
	}
	res, err := transcode.Inspect(dst, si)
	if err != nil {
		t.Fatal(err)
	}
	if res.DurationMS != 1000 || res.SampleRate != 48000 || res.Size <= 0 || res.BitrateKbps <= 0 {
		t.Fatalf("%+v", res)
	}
}

func TestInspectGuards(t *testing.T) {
	f := filepath.Join(t.TempDir(), "x")
	if err := os.WriteFile(f, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := transcode.Inspect(f, flac.StreamInfo{}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := transcode.Inspect(f, flac.StreamInfo{SampleRate: 44100, TotalSamples: 10}); err == nil {
		t.Fatal("expected error for zero duration")
	}
	if _, err := transcode.Inspect(filepath.Join(t.TempDir(), "missing"), flac.StreamInfo{SampleRate: 44100, TotalSamples: 44100}); err == nil {
		t.Fatal("expected stat error")
	}
}

func TestRunFailureRemovesOutput(t *testing.T) {
	testutil.RequireFFmpeg(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "bad.flac")
	if err := os.WriteFile(src, []byte("not audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "bad.m4a")
	err := transcode.Run(context.Background(), src, dst, flac.StreamInfo{SampleRate: 44100, BitsPerSample: 16})
	if err == nil {
		t.Fatal("expected error")
	}
	if _, statErr := os.Stat(dst); !os.IsNotExist(statErr) {
		t.Fatalf("dst exists: %v", statErr)
	}
}
