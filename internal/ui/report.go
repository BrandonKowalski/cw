package ui

import (
	"fmt"
	"io"
	"time"

	"github.com/BrandonKowalski/cw/internal/apply"
)

type Info struct {
	Mount, Model   string
	Capacity, Free int64
	Tracks         int
	LastSync       time.Time
	Library        string
}

func RenderReport(w io.Writer, r apply.Report) {
	fmt.Fprintf(w, "Done in %s. %d uploaded, %d updated, %d deleted, %d unmanaged removed.\n",
		FormatDuration(r.Elapsed), r.Uploaded, r.AudioUpdated+r.TagUpdated, r.Deleted, r.Unmanaged)
	if len(r.Warnings) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, yellow("Warnings"))
		for _, m := range r.Warnings {
			fmt.Fprintf(w, "  %s\n", m)
		}
	}
	if len(r.Failures) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, red("Failed"))
		for _, f := range r.Failures {
			fmt.Fprintf(w, "  %s: %v\n", f.RelPath, f.Err)
		}
	}
}

func RenderInfo(w io.Writer, i Info) {
	row := func(k, v string) {
		fmt.Fprintf(w, "  %-10s%s\n", k, v)
	}
	fmt.Fprintln(w, heading("iPod"))
	row("mount", i.Mount)
	if i.Model != "" {
		row("model", i.Model)
	}
	row("capacity", FormatSize(i.Capacity))
	row("free", FormatSize(i.Free))
	row("tracks", FormatCount(i.Tracks))
	last := "never"
	if !i.LastSync.IsZero() {
		last = i.LastSync.Format("2006-01-02 15:04")
	}
	row("last sync", last)
	if i.Library != "" {
		row("library", i.Library)
	}
}

func RenderInterrupted(w io.Writer, r apply.Report) {
	fmt.Fprintf(w, "Interrupted after %s. %d uploaded, %d updated, %d deleted, %d unmanaged removed.\n",
		FormatDuration(r.Elapsed), r.Uploaded, r.AudioUpdated+r.TagUpdated, r.Deleted, r.Unmanaged)
}
