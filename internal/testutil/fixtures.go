package testutil

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"
)

type FLACOpts struct {
	Seconds  float64
	Rate     int
	Bits     int
	Freq     int
	Tags     map[string]string
	CoverPNG bool
}

func MakeFLAC(t *testing.T, dir, name string, opts FLACOpts) string {
	t.Helper()
	RequireFFmpeg(t)
	if opts.Freq == 0 {
		opts.Freq = 440
	}
	out := filepath.Join(dir, name)
	args := []string{"-y", "-loglevel", "error",
		"-f", "lavfi", "-i", fmt.Sprintf("sine=frequency=%d:sample_rate=%d:duration=%g", opts.Freq, opts.Rate, opts.Seconds)}
	if opts.CoverPNG {
		cover := filepath.Join(dir, name+".cover.png")
		img := image.NewRGBA(image.Rect(0, 0, 300, 300))
		for y := 0; y < 300; y++ {
			for x := 0; x < 300; x++ {
				img.Set(x, y, color.RGBA{uint8(x), uint8(y), 128, 255})
			}
		}
		f, err := os.Create(cover)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		args = append(args, "-i", cover, "-map", "0:a", "-map", "1", "-disposition:v", "attached_pic", "-metadata:s:v", "comment=Cover (front)")
	}
	if opts.Bits == 24 {
		args = append(args, "-sample_fmt", "s32", "-bits_per_raw_sample", "24")
	} else {
		args = append(args, "-sample_fmt", "s16")
	}
	keys := make([]string, 0, len(opts.Tags))
	for k := range opts.Tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "-metadata", k+"="+opts.Tags[k])
	}
	args = append(args, out)
	if b, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v\n%s", err, b)
	}
	return out
}
