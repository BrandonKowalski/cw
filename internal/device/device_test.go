package device_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/BrandonKowalski/cw/internal/device"
)

func newFakeDevice(t *testing.T) *device.Device {
	t.Helper()
	m := t.TempDir()
	if err := os.MkdirAll(filepath.Join(m, "iPod_Control"), 0o755); err != nil {
		t.Fatal(err)
	}
	d, err := device.Open(m)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestFindOne(t *testing.T) {
	v := t.TempDir()
	if err := os.MkdirAll(filepath.Join(v, "IPOD", "iPod_Control"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(v, "Other"), 0o755); err != nil {
		t.Fatal(err)
	}
	m, err := device.Find(v)
	if err != nil || m != filepath.Join(v, "IPOD") {
		t.Fatal(m, err)
	}
}

func TestFindNone(t *testing.T) {
	v := t.TempDir()
	_, err := device.Find(v)
	if err == nil || !strings.Contains(err.Error(), "no iPod found under "+v) {
		t.Fatal(err)
	}
}

func TestFindMany(t *testing.T) {
	v := t.TempDir()
	for _, n := range []string{"A", "B"} {
		if err := os.MkdirAll(filepath.Join(v, n, "iPod_Control"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	_, err := device.Find(v)
	if err == nil || !strings.Contains(err.Error(), "--device") ||
		!strings.Contains(err.Error(), filepath.Join(v, "A")) ||
		!strings.Contains(err.Error(), filepath.Join(v, "B")) {
		t.Fatal(err)
	}
}

func TestOpenMissing(t *testing.T) {
	if _, err := device.Open(t.TempDir()); err == nil {
		t.Fatal("expected error")
	}
}

func TestNewTrackPathShape(t *testing.T) {
	d := newFakeDevice(t)
	p, err := d.NewTrackPath(".m4a")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^iPod_Control/Music/F[0-4][0-9]/[A-Z0-9]{4}\.m4a$`).MatchString(p) {
		t.Fatal(p)
	}
	if st, err := os.Stat(filepath.Join(d.Mount, filepath.Dir(p))); err != nil || !st.IsDir() {
		t.Fatal(err)
	}
}

func TestMusicFiles(t *testing.T) {
	d := newFakeDevice(t)
	got, err := d.MusicFiles()
	if err != nil || len(got) != 0 {
		t.Fatal(got, err)
	}
	p, err := d.NewTrackPath(".m4a")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d.Mount, p), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = d.MusicFiles()
	if err != nil || len(got) != 1 || got[0] != p {
		t.Fatal(got, err)
	}
}

func TestFree(t *testing.T) {
	d := newFakeDevice(t)
	n, err := d.Free()
	if err != nil || n <= 0 {
		t.Fatal(n, err)
	}
}

func TestModelString(t *testing.T) {
	d := newFakeDevice(t)
	if d.ModelString() != "" {
		t.Fatal("expected empty")
	}
	dir := filepath.Join(d.Mount, "iPod_Control", "Device")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := "BoardHwName: iPod M25\nModelNumStr: xA002 \nFirewireGuid: 0x1\n"
	if err := os.WriteFile(filepath.Join(dir, "SysInfo"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := d.ModelString(); got != "xA002" {
		t.Fatal(got)
	}
}
