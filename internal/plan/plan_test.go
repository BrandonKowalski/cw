package plan

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/BrandonKowalski/cw/internal/library"
	"github.com/BrandonKowalski/cw/internal/state"
)

var t0 = time.Unix(1700000000, 5)

func entry(rel string, size int64, mt time.Time) library.Entry {
	return library.Entry{RelPath: rel, Size: size, ModTime: mt}
}

func track(e library.Entry, tagFP, audioFP string, disc, num int) *library.Track {
	return &library.Track{Entry: e, TagFP: tagFP, AudioFP: audioFP, Tags: library.Tags{DiscNumber: disc, TrackNumber: num}}
}

func st(rel string, e library.Entry, tagFP, audioFP string) *state.Track {
	return &state.Track{DevicePath: "iPod_Control/Music/F01/" + strings.ToUpper(rel) + ".m4a", SourceSize: e.Size, SourceMTime: e.ModTime.UnixNano(), TagFP: tagFP, AudioFP: audioFP, EncodeVersion: "alac-1", FileSize: 1000}
}

type loader struct {
	tracks map[string]*library.Track
	calls  []string
}

func (l *loader) load(e library.Entry) (*library.Track, error) {
	l.calls = append(l.calls, e.RelPath)
	tr, ok := l.tracks[e.RelPath]
	if !ok {
		return nil, errors.New("boom")
	}
	return tr, nil
}

func newState(m map[string]*state.Track) *state.State {
	return &state.State{Version: 1, Tracks: m}
}

func TestBuildCategories(t *testing.T) {
	old := t0
	nw := t0.Add(time.Hour)
	ea, eb, ec, ee, ef := entry("a", 10, old), entry("b", 10, nw), entry("c", 10, nw), entry("e", 10, nw), entry("f", 10, nw)
	s := newState(map[string]*state.Track{
		"a": st("a", ea, "ta", "aa"),
		"b": st("b", entry("b", 10, old), "tb", "ab"),
		"c": st("c", entry("c", 10, old), "tc", "ac"),
		"d": st("d", entry("d", 10, old), "td", "ad"),
		"e": st("e", entry("e", 10, old), "te", "ae"),
	})
	l := &loader{tracks: map[string]*library.Track{
		"b": track(eb, "tb2", "ab", 1, 1),
		"c": track(ec, "tc", "ac2", 1, 2),
		"e": track(ee, "te", "ae", 1, 3),
		"f": track(ef, "tf", "af", 1, 4),
	}}
	p := Build(Input{Entries: []library.Entry{ef, ee, ec, eb, ea}, State: s, Free: 1 << 40, Threshold: 20000, EncodeVersion: "alac-1", Load: l.load})
	if len(p.Items) != 3 {
		t.Fatalf("items = %d", len(p.Items))
	}
	want := []struct {
		rel  string
		kind Kind
	}{{"b", TagUpdate}, {"c", AudioUpdate}, {"f", Upload}}
	for i, w := range want {
		if p.Items[i].Track.Entry.RelPath != w.rel || p.Items[i].Kind != w.kind {
			t.Errorf("item %d = %s %v", i, p.Items[i].Track.Entry.RelPath, p.Items[i].Kind)
		}
	}
	if len(p.Deletes) != 1 || p.Deletes[0].RelPath != "d" || p.Deletes[0].Track != s.Tracks["d"] {
		t.Errorf("deletes = %+v", p.Deletes)
	}
	if len(p.Touched) != 1 {
		t.Fatalf("touched = %v", p.Touched)
	}
	if _, ok := p.Touched["e"]; !ok {
		t.Errorf("touched = %v", p.Touched)
	}
	for _, c := range l.calls {
		if c == "a" {
			t.Errorf("Load called for a")
		}
	}
	if p.TrackCountAfter != 5 {
		t.Errorf("TrackCountAfter = %d", p.TrackCountAfter)
	}
	if p.BytesFreed != 1000+1000 {
		t.Errorf("BytesFreed = %d", p.BytesFreed)
	}
	if p.BytesIn != 11+11 {
		t.Errorf("BytesIn = %d", p.BytesIn)
	}
	if p.Empty() {
		t.Errorf("plan should not be empty")
	}
}

func TestBuildEncodeVersionChangeForcesAudioUpdate(t *testing.T) {
	e := entry("a", 10, t0)
	s := newState(map[string]*state.Track{"a": st("a", e, "t", "x")})
	l := &loader{tracks: map[string]*library.Track{"a": track(e, "t", "x", 1, 1)}}
	p := Build(Input{Entries: []library.Entry{e}, State: s, Free: 1 << 40, Threshold: 20000, EncodeVersion: "alac-2", Load: l.load})
	if len(p.Items) != 1 || p.Items[0].Kind != AudioUpdate {
		t.Fatalf("items = %+v", p.Items)
	}
	if len(l.calls) != 1 {
		t.Errorf("load calls = %v", l.calls)
	}
}

func TestBuildEmptyLibraryBlocks(t *testing.T) {
	s := newState(map[string]*state.Track{})
	for _, k := range []string{"a", "b", "c"} {
		s.Tracks[k] = st(k, entry(k, 1, t0), "t", "a")
	}
	p := Build(Input{State: s, Free: 1 << 40, Threshold: 20000, EncodeVersion: "alac-1", Load: (&loader{}).load})
	if len(p.Blockers) != 1 || !strings.Contains(p.Blockers[0], "refusing to wipe") || !strings.Contains(p.Blockers[0], "3 tracks") {
		t.Errorf("blockers = %v", p.Blockers)
	}
	if len(p.Deletes) != 0 {
		t.Errorf("deletes = %v", p.Deletes)
	}
}

func TestBuildUnmanagedFiles(t *testing.T) {
	e := entry("a", 1, t0)
	tr := st("a", e, "t", "a")
	tr.DevicePath = "iPod_Control/Music/F02/KEEP.m4a"
	s := newState(map[string]*state.Track{"a": tr})
	p := Build(Input{
		Entries: []library.Entry{e}, State: s, Free: 1 << 40, Threshold: 20000, EncodeVersion: "alac-1", Load: (&loader{}).load,
		DeviceFiles: []string{
			"iPod_Control/Music/F01/AAAA.mp3",
			"iPod_Control/Music/F02/KEEP.m4a",
			"iPod_Control/cw/state.json",
			"iPod_Control/cw/art/x.jpg",
			"iPod_Control/Music/F01/._AAAA.mp3",
			"iPod_Control/Music/.DS_Store",
		},
	})
	if len(p.Unmanaged) != 1 || p.Unmanaged[0] != "iPod_Control/Music/F01/AAAA.mp3" {
		t.Errorf("unmanaged = %v", p.Unmanaged)
	}
	if p.Empty() {
		t.Errorf("plan with unmanaged is not empty")
	}
}

func TestBuildDuplicateWarning(t *testing.T) {
	e1 := entry("Sleep Theory/Afterglow/09 Parasite.flac", 1, t0)
	e2 := entry("Sleep Theory/Afterglow/09 Parasite (1).flac", 1, t0)
	l := &loader{tracks: map[string]*library.Track{e1.RelPath: track(e1, "a", "a", 1, 9), e2.RelPath: track(e2, "b", "b", 1, 9)}}
	p := Build(Input{Entries: []library.Entry{e1, e2}, State: newState(map[string]*state.Track{}), Free: 1 << 40, Threshold: 20000, EncodeVersion: "alac-1", Load: l.load})
	if len(p.Warnings) != 1 {
		t.Fatalf("warnings = %v", p.Warnings)
	}
	if !strings.Contains(p.Warnings[0], "09 Parasite.flac") || !strings.Contains(p.Warnings[0], "09 Parasite (1).flac") || !strings.HasPrefix(p.Warnings[0], "possible duplicate: Sleep Theory/Afterglow: ") {
		t.Errorf("warning = %q", p.Warnings[0])
	}
	if len(p.Items) != 2 {
		t.Errorf("items = %d", len(p.Items))
	}
}

func TestBuildDuplicateUsesStateTagsForUntouched(t *testing.T) {
	e1 := entry("d/one.flac", 1, t0)
	e2 := entry("d/two.flac", 1, t0)
	s1 := st("one", e1, "t", "a")
	s1.Tags = library.Tags{DiscNumber: 1, TrackNumber: 3}
	l := &loader{tracks: map[string]*library.Track{e2.RelPath: track(e2, "b", "b", 1, 3)}}
	p := Build(Input{Entries: []library.Entry{e1, e2}, State: newState(map[string]*state.Track{e1.RelPath: s1}), Free: 1 << 40, Threshold: 20000, EncodeVersion: "alac-1", Load: l.load})
	if len(p.Warnings) != 1 || !strings.Contains(p.Warnings[0], "one.flac") {
		t.Errorf("warnings = %v", p.Warnings)
	}
}

func TestBuildNoDuplicateForTrackZero(t *testing.T) {
	e1 := entry("d/one.flac", 1, t0)
	e2 := entry("d/two.flac", 1, t0)
	l := &loader{tracks: map[string]*library.Track{e1.RelPath: track(e1, "a", "a", 0, 0), e2.RelPath: track(e2, "b", "b", 0, 0)}}
	p := Build(Input{Entries: []library.Entry{e1, e2}, State: newState(map[string]*state.Track{}), Free: 1 << 40, Threshold: 20000, EncodeVersion: "alac-1", Load: l.load})
	if len(p.Warnings) != 0 {
		t.Errorf("warnings = %v", p.Warnings)
	}
}

func TestBuildNotEnoughSpaceBlocks(t *testing.T) {
	e := entry("a", 1<<30, t0)
	l := &loader{tracks: map[string]*library.Track{"a": track(e, "t", "a", 1, 1)}}
	p := Build(Input{Entries: []library.Entry{e}, State: newState(map[string]*state.Track{}), Free: 512 << 20, Threshold: 20000, EncodeVersion: "alac-1", Load: l.load})
	found := false
	for _, b := range p.Blockers {
		if strings.Contains(b, "not enough space on iPod: need ") {
			found = true
		}
	}
	if !found {
		t.Errorf("blockers = %v", p.Blockers)
	}
	if p.FreeAfter != 512<<20-(1<<30)*11/10 {
		t.Errorf("FreeAfter = %d", p.FreeAfter)
	}
}

func TestBuildLoadFailureKeepsExisting(t *testing.T) {
	e := entry("a", 10, t0.Add(time.Hour))
	s := newState(map[string]*state.Track{"a": st("a", entry("a", 10, t0), "t", "a")})
	l := &loader{tracks: map[string]*library.Track{}}
	p := Build(Input{Entries: []library.Entry{e}, State: s, Free: 1 << 40, Threshold: 20000, EncodeVersion: "alac-1", Load: l.load})
	if len(p.Failures) != 1 || p.Failures[0].RelPath != "a" || p.Failures[0].Err == nil {
		t.Errorf("failures = %+v", p.Failures)
	}
	if len(p.Deletes) != 0 || len(p.Items) != 0 || len(p.Touched) != 0 {
		t.Errorf("plan = %+v", p)
	}
	if p.TrackCountAfter != 1 {
		t.Errorf("TrackCountAfter = %d", p.TrackCountAfter)
	}
}

func TestBuildLoadFailureOnNewFileNotCounted(t *testing.T) {
	e := entry("a", 10, t0)
	l := &loader{tracks: map[string]*library.Track{}}
	p := Build(Input{Entries: []library.Entry{e}, State: newState(map[string]*state.Track{}), Free: 1 << 40, Threshold: 20000, EncodeVersion: "alac-1", Load: l.load})
	if len(p.Failures) != 1 || p.TrackCountAfter != 0 {
		t.Errorf("failures=%v count=%d", p.Failures, p.TrackCountAfter)
	}
}

func TestBuildTrackCountWarning(t *testing.T) {
	es := []library.Entry{entry("a", 1, t0), entry("b", 1, t0), entry("c", 1, t0)}
	l := &loader{tracks: map[string]*library.Track{}}
	for i, e := range es {
		l.tracks[e.RelPath] = track(e, "t"+e.RelPath, "a"+e.RelPath, 1, i+1)
	}
	p := Build(Input{Entries: es, State: newState(map[string]*state.Track{}), Free: 1 << 40, Threshold: 2, EncodeVersion: "alac-1", Load: l.load})
	found := false
	for _, w := range p.Warnings {
		if strings.Contains(w, "safe limit of 2") && strings.HasPrefix(w, "3 tracks exceeds") {
			found = true
		}
	}
	if !found {
		t.Errorf("warnings = %v", p.Warnings)
	}
}

func TestEmpty(t *testing.T) {
	if !(&Plan{}).Empty() {
		t.Errorf("zero plan should be empty")
	}
}
