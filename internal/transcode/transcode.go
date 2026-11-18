package transcode

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/BrandonKowalski/cw/internal/flac"
)

const Version = "alac-1"

type Result struct {
	DurationMS  int
	SampleRate  int
	BitrateKbps int
	Size        int64
}

func CheckFFmpeg() error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return errors.New("ffmpeg not found on PATH (brew install ffmpeg)")
	}
	return nil
}

func OutputRate(si flac.StreamInfo) int {
	if si.SampleRate <= 48000 {
		return si.SampleRate
	}
	return 44100
}

func Args(src, dst string, si flac.StreamInfo) []string {
	args := []string{"-nostdin", "-v", "error", "-y", "-i", src, "-map", "0:a:0", "-c:a", "alac"}
	if si.BitsPerSample > 16 || si.SampleRate > 48000 {
		args = append(args, "-af", "aresample=osr="+strconv.Itoa(OutputRate(si))+":osf=s16p:dither_method=triangular")
	}
	return append(args, "-sample_fmt", "s16p", "-map_metadata", "0", "-f", "ipod", dst)
}

func Run(ctx context.Context, src, dst string, si flac.StreamInfo) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", Args(src, dst, si)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		os.Remove(dst)
		return fmt.Errorf("ffmpeg %s: %w: %s", src, err, lastLine(stderr.String()))
	}
	return nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func Inspect(dst string, si flac.StreamInfo) (Result, error) {
	if si.SampleRate <= 0 {
		return Result{}, errors.New("inspect: sample rate is zero")
	}
	ms := int(si.TotalSamples * 1000 / uint64(si.SampleRate))
	if ms <= 0 {
		return Result{}, errors.New("inspect: duration is zero")
	}
	fi, err := os.Stat(dst)
	if err != nil {
		return Result{}, fmt.Errorf("inspect: %w", err)
	}
	size := fi.Size()
	kbps := int((float64(size)*8)/float64(ms) + 0.5)
	return Result{DurationMS: ms, SampleRate: OutputRate(si), BitrateKbps: kbps, Size: size}, nil
}
