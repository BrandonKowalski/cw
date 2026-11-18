package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/BrandonKowalski/cw/internal/cli"
	"github.com/BrandonKowalski/cw/internal/state"
	"github.com/BrandonKowalski/cw/internal/testutil"
)

func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}

func run(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := cli.Run(args, strings.NewReader(stdin), &out, &errb)
	return code, out.String(), errb.String()
}

type env struct {
	mount, lib string
}

func setup(t *testing.T, tracks int) env {
	t.Helper()
	testutil.RequireFFmpeg(t)
	t.Setenv("HOME", t.TempDir())
	mount := t.TempDir()
	if err := os.MkdirAll(filepath.Join(mount, "iPod_Control"), 0o755); err != nil {
		t.Fatal(err)
	}
	lib := t.TempDir()
	dir := filepath.Join(lib, "Artist", "Album")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	names := []string{"01 One.flac", "02 Two.flac"}
	for i := 0; i < tracks; i++ {
		testutil.MakeFLAC(t, dir, names[i], testutil.FLACOpts{Seconds: 1, Rate: 44100, Tags: map[string]string{
			"TITLE": strings.TrimSuffix(names[i], ".flac"), "ARTIST": "Artist", "ALBUM": "Album", "TRACKNUMBER": string(rune('1' + i)),
		}})
	}
	return env{mount: mount, lib: lib}
}

func (e env) flags() []string {
	return []string{"--device", e.mount, "--library", e.lib}
}

func statePath(e env) string {
	return filepath.Join(e.mount, "iPod_Control", "cw", "state.json")
}

func TestRunUnknownCommand(t *testing.T) {
	code, _, errs := run(t, "", "bogus")
	if code != 2 || !strings.Contains(errs, "usage: cw") {
		t.Fatal(code, errs)
	}
}

func TestCLIDiffDoesNotTouchDevice(t *testing.T) {
	e := setup(t, 2)
	code, out, errs := run(t, "", append([]string{"diff"}, e.flags()...)...)
	if code != 0 || !strings.Contains(out, "upload") {
		t.Fatal(code, out, errs)
	}
	if _, err := os.Stat(statePath(e)); !os.IsNotExist(err) {
		t.Fatalf("state exists: %v", err)
	}
}

func TestCLISyncYesEndToEnd(t *testing.T) {
	e := setup(t, 2)
	code, out, errs := run(t, "", append([]string{"sync", "-y"}, e.flags()...)...)
	if code != 0 || !strings.Contains(out, "Done in") {
		t.Fatal(code, out, errs)
	}
	st, err := state.Load(e.mount)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Tracks) != 2 {
		t.Fatal(len(st.Tracks))
	}
	code, out, errs = run(t, "", append([]string{"diff"}, e.flags()...)...)
	if code != 0 || out != "Already in sync.\n" {
		t.Fatal(code, out, errs)
	}
}

func TestCLISyncDeclinePrompt(t *testing.T) {
	e := setup(t, 2)
	code, out, errs := run(t, "n\n", append([]string{"sync"}, e.flags()...)...)
	if code != 0 || !strings.Contains(out, "Apply? [y/N] ") || !strings.Contains(out, "Nothing changed.") {
		t.Fatal(code, out, errs)
	}
	if _, err := os.Stat(statePath(e)); !os.IsNotExist(err) {
		t.Fatalf("state exists: %v", err)
	}
}

func TestCLISyncBlockedWithYesStillRefuses(t *testing.T) {
	e := setup(t, 0)
	st := &state.State{Version: 1, Tracks: map[string]*state.Track{"Artist/Album/01 One.flac": {DBID: 1, DevicePath: "iPod_Control/Music/F00/AAAA.m4a"}}}
	if err := state.Save(e.mount, st); err != nil {
		t.Fatal(err)
	}
	code, out, errs := run(t, "", append([]string{"sync", "-y"}, e.flags()...)...)
	if code != 1 || !strings.Contains(out, "refusing to wipe") {
		t.Fatal(code, out, errs)
	}
}

func TestCLIJobsFlag(t *testing.T) {
	e := setup(t, 2)
	code, out, errs := run(t, "", append([]string{"sync", "-y", "-j", "1"}, e.flags()...)...)
	if code != 0 || !strings.Contains(out, "Done in") {
		t.Fatal(code, out, errs)
	}
	code, _, errs = run(t, "", append([]string{"diff", "-j", "0"}, e.flags()...)...)
	if code != 2 || !strings.Contains(errs, "--jobs must be at least 1") {
		t.Fatal(code, errs)
	}
}

func TestCLIConfigJobsInvalid(t *testing.T) {
	e := setup(t, 0)
	home := os.Getenv("HOME")
	if err := os.MkdirAll(filepath.Join(home, ".config", "cw"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".config", "cw", "config.toml"), []byte("jobs = 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, errs := run(t, "", append([]string{"diff"}, e.flags()...)...)
	if code != 2 || !strings.Contains(errs, "--jobs must be at least 1") {
		t.Fatal(code, errs)
	}
}

func TestCLINoLibrary(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	mount := t.TempDir()
	if err := os.MkdirAll(filepath.Join(mount, "iPod_Control"), 0o755); err != nil {
		t.Fatal(err)
	}
	code, _, errs := run(t, "", "diff", "--device", mount)
	if code != 1 || !strings.Contains(errs, "no library set: pass --library or set library in ~/.config/cw/config.toml") {
		t.Fatal(code, errs)
	}
}

func TestCLIDiffWarnsWhenDBPending(t *testing.T) {
	e := setup(t, 0)
	if err := os.MkdirAll(filepath.Join(e.mount, "iPod_Control", "cw"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := state.Save(e.mount, &state.State{Version: 1, DBPending: true, Tracks: map[string]*state.Track{}}); err != nil {
		t.Fatal(err)
	}
	_, out, errs := run(t, "", append([]string{"diff"}, e.flags()...)...)
	if !strings.Contains(out, "interrupted") || !strings.Contains(out, "cw sync") {
		t.Fatal(out, errs)
	}
	st, err := state.Load(e.mount)
	if err != nil || !st.DBPending {
		t.Fatal(err, st)
	}
}

func TestCLIInfo(t *testing.T) {
	e := setup(t, 0)
	code, out, errs := run(t, "", append([]string{"info"}, e.flags()...)...)
	if code != 0 || !strings.Contains(out, e.mount) || !strings.Contains(out, "never") {
		t.Fatal(code, out, errs)
	}
}
