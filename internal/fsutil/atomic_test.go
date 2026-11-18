package fsutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/BrandonKowalski/cw/internal/fsutil"
)

func TestWriteAtomic(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "f.json")
	if err := fsutil.WriteAtomic(p, []byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := fsutil.WriteAtomic(p, []byte("two")); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil || string(b) != "two" {
		t.Fatal(string(b), err)
	}
	if _, err := os.Stat(p + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("tmp left")
	}
}
