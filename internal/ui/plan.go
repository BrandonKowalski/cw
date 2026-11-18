package ui

import (
	"fmt"
	"io"
	"path"

	"github.com/BrandonKowalski/cw/internal/library"
	"github.com/BrandonKowalski/cw/internal/plan"
)

type PlanInfo struct {
	Mount     string
	Library   string
	Threshold int
	DirTotals map[string]int
}

type row struct {
	rel  string
	tags library.Tags
}

func RenderPlan(w io.Writer, p *plan.Plan, verbose bool) {
	RenderPlanFor(w, PlanInfo{}, p, verbose)
}

func RenderPlanFor(w io.Writer, info PlanInfo, p *plan.Plan, verbose bool) {
	if p.Empty() && len(p.Failures) == 0 {
		fmt.Fprintln(w, green("Already in sync."))
		return
	}
	if info.Mount != "" {
		fmt.Fprintf(w, "Plan for %s from %s\n\n", info.Mount, info.Library)
	}
	if len(p.Blockers) > 0 {
		fmt.Fprintln(w, red("Cannot sync"))
		for _, b := range p.Blockers {
			fmt.Fprintf(w, "  %s\n", red(b))
		}
		fmt.Fprintln(w)
	}
	if len(p.Failures) > 0 {
		fmt.Fprintln(w, red("Unreadable"))
		for _, f := range p.Failures {
			fmt.Fprintf(w, "  %s: %v\n", f.RelPath, f.Err)
		}
		fmt.Fprintln(w)
	}

	var uploads, audio, tags []row
	var uploadBytes, audioBytes int64
	for _, it := range p.Items {
		r := row{rel: it.Track.Entry.RelPath, tags: it.Track.Tags}
		switch it.Kind {
		case plan.Upload:
			uploads = append(uploads, r)
			uploadBytes += it.Track.Entry.Size * 11 / 10
		case plan.AudioUpdate:
			audio = append(audio, r)
			audioBytes += it.Track.Entry.Size * 11 / 10
		case plan.TagUpdate:
			tags = append(tags, r)
		}
	}
	var deletes []row
	for _, d := range p.Deletes {
		deletes = append(deletes, row{rel: d.RelPath, tags: d.Track.Tags})
	}

	summary(w, "upload", len(uploads), "tracks", sizeNote(uploadBytes, len(uploads)))
	summary(w, "update audio", len(audio), "tracks", sizeNote(audioBytes, len(audio)))
	summary(w, "update tags", len(tags), "tracks", "")
	summary(w, "delete", len(deletes), "tracks", "")
	summary(w, "not managed", len(p.Unmanaged), "files", "")
	fmt.Fprintln(w)
	after := FormatCount(p.TrackCountAfter)
	if info.Threshold > 0 {
		after += " / " + FormatCount(info.Threshold)
	}
	fmt.Fprintf(w, "  %-14s%s\n", "tracks after", after)
	fmt.Fprintf(w, "  %-14s%s\n", "free after", FormatSize(p.FreeAfter))

	if len(p.Warnings) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, yellow("Warnings"))
		for _, m := range p.Warnings {
			fmt.Fprintf(w, "  %s\n", m)
		}
	}

	section(w, "Upload", uploads, verbose, info.DirTotals, false)
	section(w, "Update audio", audio, verbose, info.DirTotals, false)
	section(w, "Update tags", tags, verbose, info.DirTotals, false)
	section(w, "Delete", deletes, verbose, info.DirTotals, true)
	if verbose && len(p.Unmanaged) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, heading("Not managed"))
		for _, f := range p.Unmanaged {
			fmt.Fprintf(w, "  %s\n", f)
		}
	}
}

func sizeNote(n int64, count int) string {
	if count == 0 {
		return ""
	}
	return "~" + FormatSize(n)
}

func summary(w io.Writer, label string, n int, unit, note string) {
	line := fmt.Sprintf("  %-14s%5s %s", label, FormatCount(n), unit)
	if note != "" {
		line += "   " + note
	}
	if n == 0 {
		line = dim(line)
	}
	fmt.Fprintln(w, line)
}

func albumLabel(t library.Tags) string {
	artist := t.AlbumArtist
	if artist == "" {
		artist = t.Artist
	}
	if artist == "" || t.Album == "" {
		return ""
	}
	return artist + " / " + t.Album
}

func section(w io.Writer, title string, rows []row, verbose bool, totals map[string]int, isDelete bool) {
	if len(rows) == 0 {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, heading(title))
	type group struct {
		dir  string
		rows []row
	}
	var groups []*group
	byDir := map[string]*group{}
	for _, r := range rows {
		dir := path.Dir(r.rel)
		g := byDir[dir]
		if g == nil {
			g = &group{dir: dir}
			byDir[dir] = g
			groups = append(groups, g)
		}
		g.rows = append(g.rows, r)
	}
	for _, g := range groups {
		label := albumLabel(g.rows[0].tags)
		total, known := totals[g.dir]
		whole := !known || total == len(g.rows)
		if isDelete {
			whole = !known || total == 0
		}
		if !verbose && label != "" && whole && sameAlbum(g.rows) {
			fmt.Fprintf(w, "  %s (%d)\n", label, len(g.rows))
			continue
		}
		for _, r := range g.rows {
			fmt.Fprintf(w, "  %s\n", r.rel)
		}
	}
}

func sameAlbum(rows []row) bool {
	first := albumLabel(rows[0].tags)
	for _, r := range rows[1:] {
		if albumLabel(r.tags) != first {
			return false
		}
	}
	return first != ""
}
