package apply_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/BrandonKowalski/cw/internal/apply"
	"github.com/BrandonKowalski/cw/internal/device"
	"github.com/BrandonKowalski/cw/internal/itunesdb/itunesdbtest"
	"github.com/BrandonKowalski/cw/internal/library"
	"github.com/BrandonKowalski/cw/internal/plan"
	"github.com/BrandonKowalski/cw/internal/state"
	"github.com/BrandonKowalski/cw/internal/testutil"
	"github.com/BrandonKowalski/cw/internal/transcode"
)

type recorder struct {
	mu     sync.Mutex
	phases []string
	steps  []string
	fails  []string
	done   int
}

func (r *recorder) Phase(name string, total int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.phases = append(r.phases, name)
}

func (r *recorder) Step(label string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.steps = append(r.steps, label)
}

func (r *recorder) Fail(label string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fails = append(r.fails, label)
}

func (r *recorder) Done() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.done++
}

type env struct {
	mount string
	lib   string
	tmp   string
	now   time.Time
}

func newEnv(t *testing.T) *env {
	t.Helper()
	testutil.RequireFFmpeg(t)
	e := &env{mount: t.TempDir(), lib: t.TempDir(), tmp: t.TempDir(), now: time.Unix(1750000000, 0)}
	if err := os.MkdirAll(filepath.Join(e.mount, "iPod_Control"), 0o755); err != nil {
		t.Fatal(err)
	}
	return e
}

func (e *env) flac(t *testing.T, name, title string, freq int, cover bool) string {
	t.Helper()
	dir := filepath.Join(e.lib, filepath.Dir(name))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := testutil.MakeFLAC(t, dir, filepath.Base(name), testutil.FLACOpts{
		Seconds:  0.5,
		Rate:     44100,
		Bits:     16,
		Freq:     freq,
		CoverPNG: cover,
		Tags:     map[string]string{"TITLE": title, "ARTIST": "Artist", "ALBUM": "Album", "TRACKNUMBER": "1"},
	})
	if cover {
		if err := os.Remove(p + ".cover.png"); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func (e *env) build(t *testing.T) (*device.Device, *state.State, *plan.Plan) {
	t.Helper()
	d, err := device.Open(e.mount)
	if err != nil {
		t.Fatal(err)
	}
	st, err := state.Load(e.mount)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := library.Scan(e.lib)
	if err != nil {
		t.Fatal(err)
	}
	files, err := d.MusicFiles()
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(e.lib)
	if err != nil {
		t.Fatal(err)
	}
	p := plan.Build(plan.Input{
		Entries:       entries,
		State:         st,
		DeviceFiles:   files,
		Free:          1 << 40,
		Threshold:     20000,
		EncodeVersion: transcode.Version,
		Load:          func(en library.Entry) (*library.Track, error) { return library.Load(root, en) },
	})
	return d, st, p
}

func (e *env) sync(t *testing.T, jobs int) (apply.Report, *recorder) {
	t.Helper()
	d, st, p := e.build(t)
	rec := &recorder{}
	rep, err := apply.Apply(context.Background(), d, st, p, e.lib, apply.Options{Jobs: jobs, TempDir: e.tmp, Now: func() time.Time { return e.now }}, rec)
	if err != nil {
		t.Fatal(err)
	}
	return rep, rec
}

func (e *env) state(t *testing.T) *state.State {
	t.Helper()
	st, err := state.Load(e.mount)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func (e *env) db(t *testing.T) []itunesdbtest.Track {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(e.mount, "iPod_Control", "iTunes", "iTunesDB"))
	if err != nil {
		t.Fatal(err)
	}
	tracks, err := itunesdbtest.Read(b)
	if err != nil {
		t.Fatal(err)
	}
	return tracks
}

func (e *env) exists(t *testing.T, rel string) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join(e.mount, filepath.FromSlash(rel)))
	if err == nil {
		return true
	}
	if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return false
}

func (e *env) write(t *testing.T, rel, data string) {
	t.Helper()
	p := filepath.Join(e.mount, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func listing(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		out = append(out, rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

func bump(t *testing.T, path string) {
	t.Helper()
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
}

func TestApplyFirstSyncUploadsAndRemovesUnmanaged(t *testing.T) {
	e := newEnv(t)
	e.flac(t, "A/one.flac", "One", 440, true)
	e.flac(t, "A/two.flac", "Two", 660, true)
	e.write(t, "iPod_Control/Music/F00/OLD1.mp3", "old")
	e.write(t, "iPod_Control/Music/F01/._junk", "dot")
	e.write(t, "iPod_Control/cw/art/deadbeef-1028.rgb", "stale")
	e.write(t, "iPod_Control/cw/art/deadbeef-1029.rgb", "stale")

	rep, rec := e.sync(t, 2)
	if rep.Uploaded != 2 || rep.Unmanaged != 1 || len(rep.Failures) != 0 {
		t.Fatalf("report %+v", rep)
	}
	if e.exists(t, "iPod_Control/Music/F00/OLD1.mp3") || e.exists(t, "iPod_Control/Music/F01/._junk") {
		t.Fatal("unmanaged file not removed")
	}
	if e.exists(t, "iPod_Control/cw/art/deadbeef-1028.rgb") || e.exists(t, "iPod_Control/cw/art/deadbeef-1029.rgb") {
		t.Fatal("stale art cache not removed")
	}
	st := e.state(t)
	if len(st.Tracks) != 2 || st.LastSync != e.now.Unix() {
		t.Fatalf("state %+v", st)
	}
	dbids := map[uint64]string{}
	var artID string
	for rel, tr := range st.Tracks {
		if !e.exists(t, tr.DevicePath) {
			t.Fatalf("%s: device file %s missing", rel, tr.DevicePath)
		}
		if tr.DBID == 0 || tr.DateAdded != e.now.Unix() || tr.EncodeVersion != transcode.Version || tr.FileSize <= 0 {
			t.Fatalf("%s: bad state %+v", rel, tr)
		}
		if tr.ArtID == "" || tr.ArtSize == 0 {
			t.Fatalf("%s: no art %+v", rel, tr)
		}
		artID = tr.ArtID
		dbids[tr.DBID] = tr.DevicePath
	}
	for _, f := range []string{"-1028.rgb", "-1029.rgb"} {
		if !e.exists(t, "iPod_Control/cw/art/"+artID+f) {
			t.Fatalf("art cache %s missing", f)
		}
	}
	tracks := e.db(t)
	if len(tracks) != 2 {
		t.Fatalf("db has %d tracks", len(tracks))
	}
	for _, tr := range tracks {
		if _, ok := dbids[tr.DBID]; !ok {
			t.Fatalf("db track dbid %d not in state", tr.DBID)
		}
		if tr.MHIILink == 0 {
			t.Fatalf("db track %d has no artwork link", tr.ID)
		}
		if tr.ID < 52 {
			t.Fatalf("db track id %d below 52", tr.ID)
		}
	}
	if !e.exists(t, "iPod_Control/Artwork/ArtworkDB") {
		t.Fatal("ArtworkDB missing")
	}
	if len(rec.phases) == 0 || rec.phases[0] != "transcoding" {
		t.Fatalf("phases %v", rec.phases)
	}
}

func TestApplyTagUpdateKeepsFileAndDBID(t *testing.T) {
	e := newEnv(t)
	p := e.flac(t, "A/one.flac", "One", 440, true)
	e.sync(t, 1)
	before := e.state(t).Tracks["A/one.flac"]

	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	e.flac(t, "A/one.flac", "One Renamed", 440, true)
	bump(t, p)
	rep, _ := e.sync(t, 1)
	if rep.TagUpdated != 1 || rep.Uploaded != 0 || rep.AudioUpdated != 0 {
		t.Fatalf("report %+v", rep)
	}
	after := e.state(t).Tracks["A/one.flac"]
	if after.DevicePath != before.DevicePath || after.DBID != before.DBID || after.DateAdded != before.DateAdded {
		t.Fatalf("before %+v after %+v", before, after)
	}
	if after.Tags.Title != "One Renamed" || after.TagFP == before.TagFP || after.SourceMTime == before.SourceMTime {
		t.Fatalf("state not updated %+v", after)
	}
	tracks := e.db(t)
	if len(tracks) != 1 || tracks[0].Title != "One Renamed" || tracks[0].DBID != before.DBID {
		t.Fatalf("db %+v", tracks)
	}
	if !e.exists(t, after.DevicePath) {
		t.Fatal("device file removed")
	}
}

func TestApplyAudioUpdateReplacesFileKeepsDBID(t *testing.T) {
	e := newEnv(t)
	p := e.flac(t, "A/one.flac", "One", 440, true)
	e.sync(t, 1)
	before := e.state(t).Tracks["A/one.flac"]

	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	e.flac(t, "A/one.flac", "One", 880, true)
	bump(t, p)
	e.now = e.now.Add(time.Hour)
	rep, _ := e.sync(t, 1)
	if rep.AudioUpdated != 1 || rep.Uploaded != 0 {
		t.Fatalf("report %+v", rep)
	}
	after := e.state(t).Tracks["A/one.flac"]
	if after.DevicePath == before.DevicePath || after.DBID != before.DBID || after.DateAdded != before.DateAdded || after.AudioFP == before.AudioFP {
		t.Fatalf("before %+v after %+v", before, after)
	}
	if e.exists(t, before.DevicePath) {
		t.Fatal("old device file still present")
	}
	if !e.exists(t, after.DevicePath) {
		t.Fatal("new device file missing")
	}
	tracks := e.db(t)
	if len(tracks) != 1 || tracks[0].DBID != before.DBID || tracks[0].Path != ":"+strings.ReplaceAll(after.DevicePath, "/", ":") {
		t.Fatalf("db %+v", tracks)
	}
}

func TestApplyDeleteRemovesFile(t *testing.T) {
	e := newEnv(t)
	e.flac(t, "A/one.flac", "One", 440, true)
	p := e.flac(t, "A/two.flac", "Two", 660, false)
	e.sync(t, 2)
	gone := e.state(t).Tracks["A/two.flac"]
	if gone == nil || gone.ArtID != "" || gone.ArtSize != 0 {
		t.Fatalf("state %+v", gone)
	}

	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	rep, _ := e.sync(t, 2)
	if rep.Deleted != 1 {
		t.Fatalf("report %+v", rep)
	}
	if e.exists(t, gone.DevicePath) {
		t.Fatal("deleted track file still present")
	}
	st := e.state(t)
	if len(st.Tracks) != 1 || st.Tracks["A/one.flac"] == nil {
		t.Fatalf("state %+v", st.Tracks)
	}
	if tracks := e.db(t); len(tracks) != 1 || tracks[0].DBID != st.Tracks["A/one.flac"].DBID {
		t.Fatalf("db %+v", tracks)
	}
}

func TestApplyBadFLACContinues(t *testing.T) {
	e := newEnv(t)
	e.flac(t, "good.flac", "Good", 440, false)
	if err := os.WriteFile(filepath.Join(e.lib, "bad.flac"), []byte("not a flac"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, _ := e.sync(t, 2)
	if rep.Uploaded != 1 || len(rep.Failures) != 1 || !strings.Contains(rep.Failures[0].RelPath, "bad.flac") {
		t.Fatalf("report %+v", rep)
	}
	if tracks := e.db(t); len(tracks) != 1 {
		t.Fatalf("db %+v", tracks)
	}
}

func TestApplyTranscodeFailureLeavesStateAndContinues(t *testing.T) {
	e := newEnv(t)
	e.flac(t, "good.flac", "Good", 440, false)
	bad := e.flac(t, "broken.flac", "Broken", 440, false)
	d, st, p := e.build(t)
	if err := os.WriteFile(bad, []byte("gone"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := &recorder{}
	rep, err := apply.Apply(context.Background(), d, st, p, e.lib, apply.Options{Jobs: 2, TempDir: e.tmp, Now: func() time.Time { return e.now }}, rec)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Uploaded != 1 || len(rep.Failures) != 1 || rep.Failures[0].RelPath != "broken.flac" {
		t.Fatalf("report %+v", rep)
	}
	if msg := rep.Failures[0].Err.Error(); !strings.HasPrefix(msg, "broken.flac: ") || strings.Contains(msg, e.lib) {
		t.Fatalf("failure error %q", msg)
	}
	if len(rec.fails) != 1 || rec.fails[0] != "broken.flac" {
		t.Fatalf("progress fails %v", rec.fails)
	}
	saved := e.state(t)
	if len(saved.Tracks) != 1 || saved.Tracks["good.flac"] == nil {
		t.Fatalf("state %+v", saved.Tracks)
	}
	files, err := d.MusicFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("music files %v", files)
	}
	tmp, err := os.ReadDir(e.tmp)
	if err != nil {
		t.Fatal(err)
	}
	if len(tmp) != 0 {
		t.Fatalf("temp dir not cleaned: %v", tmp)
	}
}

func TestApplyManyTracksConcurrent(t *testing.T) {
	e := newEnv(t)
	for i := 0; i < 8; i++ {
		e.flac(t, filepath.Join("A", string(rune('a'+i))+".flac"), string(rune('A'+i)), 300+i*50, true)
	}
	rep, _ := e.sync(t, 8)
	if rep.Uploaded != 8 || len(rep.Failures) != 0 {
		t.Fatalf("report %+v", rep)
	}
	st := e.state(t)
	paths := map[string]bool{}
	for _, tr := range st.Tracks {
		if paths[tr.DevicePath] {
			t.Fatalf("duplicate device path %s", tr.DevicePath)
		}
		paths[tr.DevicePath] = true
	}
	art, err := os.ReadDir(filepath.Join(e.mount, "iPod_Control", "cw", "art"))
	if err != nil {
		t.Fatal(err)
	}
	if len(art) != 2 {
		t.Fatalf("art cache %v", art)
	}
	tracks := e.db(t)
	if len(tracks) != 8 {
		t.Fatalf("db has %d tracks", len(tracks))
	}
	ids := map[uint32]bool{}
	for _, tr := range tracks {
		ids[tr.ID] = true
	}
	for id := uint32(52); id < 60; id++ {
		if !ids[id] {
			t.Fatalf("missing track id %d in %v", id, ids)
		}
	}
}

func TestApplyBlockedPlanTouchesNothing(t *testing.T) {
	e := newEnv(t)
	e.flac(t, "one.flac", "One", 440, true)
	e.write(t, "iPod_Control/Music/F00/OLD1.mp3", "old")
	e.write(t, "iPod_Control/iTunes/Play Counts", "pc")
	d, st, p := e.build(t)
	p.Blockers = append(p.Blockers, "blocked for test")
	before := listing(t, e.mount)
	_, err := apply.Apply(context.Background(), d, st, p, e.lib, apply.Options{Jobs: 1, TempDir: e.tmp}, &recorder{})
	if err == nil || !strings.Contains(err.Error(), "blocked for test") {
		t.Fatalf("err %v", err)
	}
	if after := listing(t, e.mount); strings.Join(after, "\n") != strings.Join(before, "\n") {
		t.Fatalf("device changed:\n%v\n%v", before, after)
	}
}

func TestApplyCancelledWritesNothing(t *testing.T) {
	e := newEnv(t)
	e.flac(t, "one.flac", "One", 440, true)
	d, st, p := e.build(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := apply.Apply(ctx, d, st, p, e.lib, apply.Options{Jobs: 1, TempDir: e.tmp}, &recorder{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v", err)
	}
	if e.exists(t, "iPod_Control/cw/state.json") || e.exists(t, "iPod_Control/iTunes/iTunesDB") {
		t.Fatal("state or db written after cancel")
	}
	files, err := d.MusicFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("music files after cancel %v", files)
	}
}

func TestApplyRemovesPlayCounts(t *testing.T) {
	e := newEnv(t)
	e.flac(t, "one.flac", "One", 440, false)
	e.write(t, "iPod_Control/iTunes/Play Counts", "pc")
	e.sync(t, 1)
	if e.exists(t, "iPod_Control/iTunes/Play Counts") {
		t.Fatal("Play Counts still present")
	}
}

func corruptCover(t *testing.T, path string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	i := bytes.Index(b, []byte("\x89PNG"))
	if i < 0 {
		t.Fatal("no embedded png")
	}
	copy(b[i:], bytes.Repeat([]byte{0xAB}, 64))
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func hasWarning(ws []string, want string) bool {
	for _, w := range ws {
		if strings.HasPrefix(w, want) {
			return true
		}
	}
	return false
}

func TestApplyBadCoverSyncsWithoutArt(t *testing.T) {
	e := newEnv(t)
	p := e.flac(t, "A/one.flac", "One", 440, true)
	corruptCover(t, p)
	rep, rec := e.sync(t, 1)
	if rep.Uploaded != 1 || len(rep.Failures) != 0 || len(rec.fails) != 0 {
		t.Fatalf("report %+v", rep)
	}
	if !hasWarning(rep.Warnings, "no cover art for A/one.flac: ") {
		t.Fatalf("warnings %v", rep.Warnings)
	}
	tr := e.state(t).Tracks["A/one.flac"]
	if tr == nil || tr.ArtID != "" || tr.ArtSize != 0 {
		t.Fatalf("state %+v", tr)
	}
	if tracks := e.db(t); len(tracks) != 1 || tracks[0].MHIILink != 0 {
		t.Fatalf("db %+v", tracks)
	}
}

func TestApplyBadCoverOnTagUpdateDropsArt(t *testing.T) {
	e := newEnv(t)
	p := e.flac(t, "A/one.flac", "One", 440, true)
	e.sync(t, 1)
	before := e.state(t).Tracks["A/one.flac"]
	if before.ArtID == "" {
		t.Fatal("no art after first sync")
	}
	corruptCover(t, p)
	bump(t, p)
	rep, _ := e.sync(t, 1)
	if rep.TagUpdated != 1 || len(rep.Failures) != 0 || !hasWarning(rep.Warnings, "no cover art for A/one.flac: ") {
		t.Fatalf("report %+v", rep)
	}
	after := e.state(t).Tracks["A/one.flac"]
	if after.ArtID != "" || after.ArtSize != 0 || after.TagFP == before.TagFP || after.DevicePath != before.DevicePath {
		t.Fatalf("state %+v", after)
	}
	art, err := os.ReadDir(filepath.Join(e.mount, "iPod_Control", "cw", "art"))
	if err != nil {
		t.Fatal(err)
	}
	if len(art) != 0 {
		t.Fatalf("unreferenced art kept %v", art)
	}
}

func TestApplyMissingArtCacheWarns(t *testing.T) {
	e := newEnv(t)
	e.flac(t, "A/one.flac", "One", 440, true)
	e.sync(t, 1)
	tr := e.state(t).Tracks["A/one.flac"]
	if err := os.Remove(filepath.Join(e.mount, "iPod_Control", "cw", "art", tr.ArtID+"-1029.rgb")); err != nil {
		t.Fatal(err)
	}
	rep, _ := e.sync(t, 1)
	if !hasWarning(rep.Warnings, "cover art cache missing for A/one.flac") {
		t.Fatalf("warnings %v", rep.Warnings)
	}
	if tracks := e.db(t); len(tracks) != 1 || tracks[0].MHIILink != 0 {
		t.Fatalf("db %+v", tracks)
	}
}

func TestWriteDatabasesReportsWrongSizeCache(t *testing.T) {
	e := newEnv(t)
	e.flac(t, "A/one.flac", "One", 440, true)
	e.sync(t, 1)
	st := e.state(t)
	tr := st.Tracks["A/one.flac"]
	e.write(t, "iPod_Control/cw/art/"+tr.ArtID+"-1028.rgb", "short")
	d, err := device.Open(e.mount)
	if err != nil {
		t.Fatal(err)
	}
	ws, err := apply.WriteDatabases(d, st)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 1 || ws[0] != "A/one.flac" {
		t.Fatalf("missing %v", ws)
	}
}

func TestApplyCleanupFailureIsWarning(t *testing.T) {
	e := newEnv(t)
	e.flac(t, "one.flac", "One", 440, false)
	e.write(t, "iPod_Control/Music/F00/OLD1.mp3", "old")
	dir := filepath.Join(e.mount, "iPod_Control", "Music", "F00")
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	d, st, p := e.build(t)
	rep, err := apply.Apply(context.Background(), d, st, p, e.lib, apply.Options{Jobs: 1, TempDir: e.tmp, Now: func() time.Time { return e.now }}, &recorder{})
	if err != nil {
		t.Fatal(err)
	}
	if !hasWarning(rep.Warnings, "could not remove iPod_Control/Music/F00/OLD1.mp3: ") {
		t.Fatalf("warnings %v", rep.Warnings)
	}
	if rep.Elapsed < 0 || rep.Uploaded != 1 {
		t.Fatalf("report %+v", rep)
	}
}

func TestApplyClearsDBPending(t *testing.T) {
	e := newEnv(t)
	e.flac(t, "one.flac", "One", 440, false)
	e.sync(t, 1)
	if e.state(t).DBPending {
		t.Fatal("db_pending left set after apply")
	}
}

func TestRecoverRewritesPendingDB(t *testing.T) {
	e := newEnv(t)
	e.flac(t, "one.flac", "One", 440, true)
	e.sync(t, 1)
	db := filepath.Join(e.mount, "iPod_Control", "iTunes", "iTunesDB")
	if err := os.Remove(db); err != nil {
		t.Fatal(err)
	}
	st := e.state(t)
	st.DBPending = true
	if err := state.Save(e.mount, st); err != nil {
		t.Fatal(err)
	}
	d, err := device.Open(e.mount)
	if err != nil {
		t.Fatal(err)
	}
	st = e.state(t)
	ok, err := apply.Recover(d, st)
	if err != nil || !ok {
		t.Fatalf("recover %v %v", ok, err)
	}
	if tracks := e.db(t); len(tracks) != 1 || tracks[0].MHIILink == 0 {
		t.Fatalf("db %+v", tracks)
	}
	if e.state(t).DBPending {
		t.Fatal("flag not cleared on disk")
	}
	ok, err = apply.Recover(d, e.state(t))
	if err != nil || ok {
		t.Fatalf("second recover %v %v", ok, err)
	}
}

func TestApplyCleanupMatchesDevicePathsCaseInsensitively(t *testing.T) {
	e := newEnv(t)
	e.flac(t, "one.flac", "One", 440, false)
	e.sync(t, 1)
	st := e.state(t)
	tr := st.Tracks["one.flac"]
	real := tr.DevicePath
	tr.DevicePath = strings.Replace(real, "/Music/F", "/Music/f", 1)
	if err := state.Save(e.mount, st); err != nil {
		t.Fatal(err)
	}
	e.flac(t, "two.flac", "Two", 550, false)
	rep, _ := e.sync(t, 1)
	if rep.Uploaded != 1 || rep.AudioUpdated != 0 || rep.Unmanaged != 0 {
		t.Fatalf("report %+v", rep)
	}
	if !e.exists(t, real) {
		t.Fatalf("%s removed", real)
	}
}

func TestApplyRestoresMissingDeviceFile(t *testing.T) {
	e := newEnv(t)
	e.flac(t, "one.flac", "One", 440, false)
	e.sync(t, 1)
	before := e.state(t).Tracks["one.flac"]
	e.flac(t, "two.flac", "Two", 550, false)
	e.sync(t, 1)
	if err := os.Remove(filepath.Join(e.mount, filepath.FromSlash(before.DevicePath))); err != nil {
		t.Fatal(err)
	}
	rep, _ := e.sync(t, 1)
	if rep.AudioUpdated != 1 || rep.Uploaded != 0 {
		t.Fatalf("report %+v", rep)
	}
	after := e.state(t).Tracks["one.flac"]
	if after.DBID != before.DBID || after.DateAdded != before.DateAdded || !e.exists(t, after.DevicePath) {
		t.Fatalf("before %+v after %+v", before, after)
	}
}
