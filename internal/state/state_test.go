package state_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/BrandonKowalski/cw/internal/library"
	"github.com/BrandonKowalski/cw/internal/state"
)

func newFakeMount(t *testing.T) string {
	t.Helper()
	m := t.TempDir()
	if err := os.MkdirAll(filepath.Join(m, "iPod_Control"), 0o755); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestStateRoundTripAndMissing(t *testing.T) {
	m := newFakeMount(t)
	s, err := state.Load(m)
	if err != nil || s.Tracks == nil || len(s.Tracks) != 0 || s.Version != 1 {
		t.Fatal(err)
	}
	s.Tracks["A/b.flac"] = &state.Track{DBID: 7, Tags: library.Tags{Title: "b"}}
	if err := state.Save(m, s); err != nil {
		t.Fatal(err)
	}
	s2, err := state.Load(m)
	if err != nil {
		t.Fatal(err)
	}
	if s2.Tracks["A/b.flac"].DBID != 7 || s2.Tracks["A/b.flac"].Tags.Title != "b" {
		t.Fatal()
	}
	if _, err := os.Stat(filepath.Join(m, "iPod_Control/cw/state.json.tmp")); !os.IsNotExist(err) {
		t.Fatal("tmp left")
	}
}

func TestLoadCorrupt(t *testing.T) {
	m := newFakeMount(t)
	dir := filepath.Join(m, "iPod_Control", "cw")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := state.Load(m); err == nil {
		t.Fatal("expected error")
	}
}

func TestNewDBID(t *testing.T) {
	s := &state.State{Version: 1, Tracks: map[string]*state.Track{}}
	seen := map[uint64]bool{}
	for i := 0; i < 1000; i++ {
		id := s.NewDBID()
		if id == 0 || seen[id] {
			t.Fatal(id)
		}
		seen[id] = true
		s.Tracks[string(rune('a'+i%26))+string(rune(i))] = &state.Track{DBID: id}
	}
}
