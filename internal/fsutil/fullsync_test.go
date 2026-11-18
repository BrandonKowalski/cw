package fsutil

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestFullSync(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "x"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("data")); err != nil {
		t.Fatal(err)
	}
	if err := FullSync(f); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := FullSync(f); err == nil {
		t.Fatal("expected error on closed file")
	}
}

func TestFullSyncUnsupported(t *testing.T) {
	for _, err := range []error{unix.ENOTSUP, unix.EOPNOTSUPP, unix.EINVAL, unix.ENOTTY} {
		if !unsupported(err) {
			t.Errorf("%v should be ignored", err)
		}
	}
	if unsupported(unix.EIO) {
		t.Error("EIO must not be ignored")
	}
}
