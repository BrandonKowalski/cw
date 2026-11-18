package config_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/BrandonKowalski/cw/internal/config"
)

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	c, err := config.Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if c.TrackWarnThreshold != 20000 || c.Jobs != runtime.NumCPU() || c.Library != "" {
		t.Fatalf("%+v", c)
	}
}

func TestLoadExpandsTildeAndOverrides(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.toml")
	os.WriteFile(p, []byte("library = \"~/Music\"\njobs = 3\n"), 0o644)
	c, _ := config.Load(p)
	home, _ := os.UserHomeDir()
	if c.Library != filepath.Join(home, "Music") || c.Jobs != 3 || c.TrackWarnThreshold != 20000 {
		t.Fatalf("%+v", c)
	}
}
