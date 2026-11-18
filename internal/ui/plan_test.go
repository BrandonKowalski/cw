package ui_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/BrandonKowalski/cw/internal/apply"
	"github.com/BrandonKowalski/cw/internal/library"
	"github.com/BrandonKowalski/cw/internal/plan"
	"github.com/BrandonKowalski/cw/internal/ui"
)

func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	m.Run()
}

func upload(rel string) plan.Item {
	return plan.Item{Kind: plan.Upload, Track: &library.Track{
		Entry: library.Entry{RelPath: rel, Size: 1000},
		Tags:  library.Tags{AlbumArtist: "Artist", Album: "Album"},
	}}
}

func threeUploads() *plan.Plan {
	return &plan.Plan{
		Items:           []plan.Item{upload("Artist/Album/01.flac"), upload("Artist/Album/02.flac"), upload("Artist/Album/03.flac")},
		TrackCountAfter: 3,
		BytesIn:         3300,
		FreeAfter:       5000000,
	}
}

func TestRenderPlanSummaryAndCollapse(t *testing.T) {
	var b bytes.Buffer
	ui.RenderPlanFor(&b, ui.PlanInfo{Mount: "/Volumes/IPOD", Library: "/Music", Threshold: 20000}, threeUploads(), false)
	out := b.String()
	for _, want := range []string{
		"Plan for /Volumes/IPOD from /Music",
		"  upload            3 tracks   ~3.3 kB",
		"  update audio      0 tracks",
		"  tracks after  3 / 20,000",
		"  free after    5.0 MB",
		"Upload\n  Artist / Album (3)\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "01.flac") {
		t.Fatal(out)
	}
	b.Reset()
	ui.RenderPlanFor(&b, ui.PlanInfo{}, threeUploads(), true)
	for _, f := range []string{"01.flac", "02.flac", "03.flac"} {
		if !strings.Contains(b.String(), "Artist/Album/"+f) {
			t.Fatalf("missing %s in:\n%s", f, b.String())
		}
	}
}

func TestRenderPlanPartialAlbumListsPaths(t *testing.T) {
	var b bytes.Buffer
	ui.RenderPlanFor(&b, ui.PlanInfo{DirTotals: map[string]int{"Artist/Album": 12}}, threeUploads(), false)
	if !strings.Contains(b.String(), "Artist/Album/01.flac") || strings.Contains(b.String(), "Artist / Album (3)") {
		t.Fatal(b.String())
	}
}

func TestRenderPlanBlockersFirst(t *testing.T) {
	p := threeUploads()
	p.Blockers = []string{"refusing to wipe"}
	p.Failures = []plan.Failure{{RelPath: "x.flac", Err: errors.New("bad header")}}
	p.Warnings = []string{"possible duplicate: a"}
	var b bytes.Buffer
	ui.RenderPlanFor(&b, ui.PlanInfo{}, p, false)
	lines := strings.Split(strings.TrimSpace(b.String()), "\n")
	if lines[0] != "Cannot sync" || !strings.Contains(lines[1], "refusing to wipe") {
		t.Fatal(b.String())
	}
	for _, want := range []string{"Unreadable\n  x.flac: bad header", "Warnings\n  possible duplicate: a"} {
		if !strings.Contains(b.String(), want) {
			t.Fatalf("missing %q in:\n%s", want, b.String())
		}
	}
}

func TestRenderPlanEmpty(t *testing.T) {
	var b bytes.Buffer
	ui.RenderPlanFor(&b, ui.PlanInfo{}, &plan.Plan{}, false)
	if b.String() != "Already in sync.\n" {
		t.Fatalf("%q", b.String())
	}
}

func TestRenderPlanUnmanagedOnlyVerbose(t *testing.T) {
	p := &plan.Plan{Unmanaged: []string{"iPod_Control/Music/F00/AAAA.m4a"}}
	var b bytes.Buffer
	ui.RenderPlanFor(&b, ui.PlanInfo{}, p, false)
	if strings.Contains(b.String(), "AAAA.m4a") || !strings.Contains(b.String(), "not managed       1 file") {
		t.Fatal(b.String())
	}
	b.Reset()
	ui.RenderPlanFor(&b, ui.PlanInfo{}, p, true)
	if !strings.Contains(b.String(), "AAAA.m4a") {
		t.Fatal(b.String())
	}
}

func TestProgressPlain(t *testing.T) {
	var b bytes.Buffer
	p := ui.NewProgress(&b, false)
	p.Phase("transcoding", 2)
	p.Step("a")
	p.Fail("b", errors.New("boom"))
	p.Done()
	if b.String() != "transcoding (2)\nfailed: b: boom\n" {
		t.Fatalf("%q", b.String())
	}
}

func TestProgressTTY(t *testing.T) {
	var b bytes.Buffer
	p := ui.NewProgress(&b, true)
	p.Phase("transcoding", 2658)
	p.Step("Hot Mulligan / you'll be fine / 03 Something.flac")
	p.Done()
	p.Phase("updating", 1)
	want := "\r\x1b[2Ktranscoding  0/2658" +
		"\r\x1b[2Ktranscoding  1/2658  Hot Mulligan / you'll be fine / 03 Something.flac\n" +
		"\r\x1b[2Kupdating  0/1"
	if b.String() != want {
		t.Fatalf("%q", b.String())
	}
}

func TestRenderReport(t *testing.T) {
	var b bytes.Buffer
	ui.RenderReport(&b, apply.Report{
		Uploaded: 2658, Unmanaged: 412, Elapsed: 14*time.Minute + 2*time.Second,
		Warnings: []string{"no cover art for a"},
		Failures: []plan.Failure{{RelPath: "a.flac", Err: errors.New("boom")}},
	})
	want := "Done in 14m02s. 2658 uploaded, 0 updated, 0 deleted, 412 unmanaged removed.\n\nWarnings\n  no cover art for a\n\nFailed\n  a.flac: boom\n"
	if b.String() != want {
		t.Fatalf("%q", b.String())
	}
}

func TestRenderInfo(t *testing.T) {
	var b bytes.Buffer
	ui.RenderInfo(&b, ui.Info{Mount: "/Volumes/IPOD", Capacity: 80e9, Free: 20e9, Tracks: 1234})
	for _, want := range []string{"/Volumes/IPOD", "80.0 GB", "20.0 GB", "1,234", "never"} {
		if !strings.Contains(b.String(), want) {
			t.Fatalf("missing %q in:\n%s", want, b.String())
		}
	}
}

func TestRenderPlanSingular(t *testing.T) {
	p := &plan.Plan{Items: []plan.Item{upload("A/B/01.flac")}, Unmanaged: []string{"x"}}
	var b bytes.Buffer
	ui.RenderPlanFor(&b, ui.PlanInfo{}, p, false)
	if !strings.Contains(b.String(), "1 track ") && !strings.Contains(b.String(), "1 track\n") || !strings.Contains(b.String(), "1 file") || strings.Contains(b.String(), "1 files") {
		t.Fatal(b.String())
	}
}

func TestRenderInterrupted(t *testing.T) {
	var b bytes.Buffer
	ui.RenderInterrupted(&b, apply.Report{Uploaded: 3, Elapsed: 5 * time.Second})
	if b.String() != "Interrupted after 5s. 3 uploaded, 0 updated, 0 deleted, 0 unmanaged removed.\n" {
		t.Fatal(b.String())
	}
}
