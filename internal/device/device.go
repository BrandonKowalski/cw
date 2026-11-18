package device

import (
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

const control = "iPod_Control"

type Device struct {
	Mount string
}

func Find(volumesDir string) (string, error) {
	entries, err := os.ReadDir(volumesDir)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", volumesDir, err)
	}
	var found []string
	for _, e := range entries {
		p := filepath.Join(volumesDir, e.Name())
		if st, err := os.Stat(filepath.Join(p, control)); err == nil && st.IsDir() {
			found = append(found, p)
		}
	}
	switch len(found) {
	case 0:
		return "", fmt.Errorf("no iPod found under %s", volumesDir)
	case 1:
		return found[0], nil
	}
	return "", fmt.Errorf("multiple iPods found, pick one with --device:\n  %s", strings.Join(found, "\n  "))
}

func Open(mount string) (*Device, error) {
	st, err := os.Stat(filepath.Join(mount, control))
	if err != nil {
		return nil, fmt.Errorf("open device %s: %w", mount, err)
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("open device %s: %s is not a directory", mount, control)
	}
	return &Device{Mount: mount}, nil
}

func (d *Device) MusicFiles() ([]string, error) {
	root := filepath.Join(d.Mount, control, "Music")
	var out []string
	err := filepath.WalkDir(root, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			if p == root && os.IsNotExist(err) {
				return filepath.SkipAll
			}
			return err
		}
		if e.Type().IsRegular() {
			rel, err := filepath.Rel(d.Mount, p)
			if err != nil {
				return err
			}
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list music files: %w", err)
	}
	return out, nil
}

func (d *Device) Free() (int64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(d.Mount, &st); err != nil {
		return 0, fmt.Errorf("statfs %s: %w", d.Mount, err)
	}
	return int64(st.Bavail) * int64(st.Bsize), nil
}

const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func (d *Device) NewTrackPath(ext string) (string, error) {
	for {
		folder := fmt.Sprintf("F%02d", rand.IntN(50))
		var name [4]byte
		for i := range name {
			name[i] = alphabet[rand.IntN(len(alphabet))]
		}
		rel := control + "/Music/" + folder + "/" + string(name[:]) + ext
		dir := filepath.Join(d.Mount, control, "Music", folder)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("create %s: %w", dir, err)
		}
		_, err := os.Stat(filepath.Join(d.Mount, filepath.FromSlash(rel)))
		if os.IsNotExist(err) {
			return rel, nil
		}
		if err != nil {
			return "", fmt.Errorf("stat %s: %w", rel, err)
		}
	}
}

func (d *Device) ModelString() string {
	b, err := os.ReadFile(filepath.Join(d.Mount, control, "Device", "SysInfo"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if ok && strings.TrimSpace(k) == "ModelNumStr" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
