package plan

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/BrandonKowalski/cw/internal/library"
	"github.com/BrandonKowalski/cw/internal/state"
)

type Kind int

const (
	Upload Kind = iota
	AudioUpdate
	TagUpdate
)

const minFree = 64 << 20

const cwPrefix = "iPod_Control/cw/"

type Item struct {
	Kind  Kind
	Track *library.Track
}

type Delete struct {
	RelPath string
	Track   *state.Track
}

type Failure struct {
	RelPath string
	Err     error
}

type Plan struct {
	Items           []Item
	Deletes         []Delete
	Unmanaged       []string
	Touched         map[string]library.Entry
	Failures        []Failure
	Warnings        []string
	Blockers        []string
	TrackCountAfter int
	BytesIn         int64
	BytesFreed      int64
	FreeAfter       int64
}

type Input struct {
	Entries       []library.Entry
	State         *state.State
	DeviceFiles   []string
	Free          int64
	Threshold     int
	EncodeVersion string
	Load          func(library.Entry) (*library.Track, error)
}

func (p *Plan) Empty() bool {
	return len(p.Items) == 0 && len(p.Deletes) == 0 && len(p.Unmanaged) == 0 && len(p.Touched) == 0
}

func Build(in Input) *Plan {
	p := &Plan{Touched: map[string]library.Entry{}}
	tracks := map[string]*state.Track{}
	if in.State != nil && in.State.Tracks != nil {
		tracks = in.State.Tracks
	}

	entries := append([]library.Entry(nil), in.Entries...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].RelPath < entries[j].RelPath })

	inLibrary := make(map[string]bool, len(entries))
	tagsByPath := map[string]library.Tags{}
	kept := 0

	for _, e := range entries {
		inLibrary[e.RelPath] = true
		old := tracks[e.RelPath]
		if old != nil {
			kept++
			tagsByPath[e.RelPath] = old.Tags
			if old.SourceSize == e.Size && old.SourceMTime == e.ModTime.UnixNano() && old.EncodeVersion == in.EncodeVersion {
				continue
			}
		}
		tr, err := in.Load(e)
		if err != nil {
			p.Failures = append(p.Failures, Failure{RelPath: e.RelPath, Err: fmt.Errorf("load %s: %w", e.RelPath, err)})
			continue
		}
		tagsByPath[e.RelPath] = tr.Tags
		switch {
		case old == nil:
			p.Items = append(p.Items, Item{Kind: Upload, Track: tr})
			p.BytesIn += e.Size * 11 / 10
			kept++
		case old.AudioFP != tr.AudioFP || old.EncodeVersion != in.EncodeVersion:
			p.Items = append(p.Items, Item{Kind: AudioUpdate, Track: tr})
			p.BytesIn += e.Size * 11 / 10
			p.BytesFreed += old.FileSize
		case old.TagFP != tr.TagFP:
			p.Items = append(p.Items, Item{Kind: TagUpdate, Track: tr})
		default:
			p.Touched[e.RelPath] = e
		}
	}

	if len(entries) == 0 && len(tracks) > 0 {
		p.Blockers = append(p.Blockers, fmt.Sprintf("library is empty but the iPod has %d tracks; refusing to wipe it", len(tracks)))
		kept = len(tracks)
	} else {
		keys := make([]string, 0, len(tracks))
		for k := range tracks {
			if !inLibrary[k] {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			p.Deletes = append(p.Deletes, Delete{RelPath: k, Track: tracks[k]})
			p.BytesFreed += tracks[k].FileSize
		}
	}
	p.TrackCountAfter = kept

	referenced := make(map[string]bool, len(tracks))
	for _, t := range tracks {
		referenced[t.DevicePath] = true
	}
	for _, f := range in.DeviceFiles {
		if referenced[f] || strings.HasPrefix(f, cwPrefix) || strings.HasPrefix(path.Base(f), ".") {
			continue
		}
		p.Unmanaged = append(p.Unmanaged, f)
	}
	sort.Strings(p.Unmanaged)

	p.FreeAfter = in.Free - p.BytesIn + p.BytesFreed
	if p.FreeAfter < minFree {
		need := p.BytesIn - p.BytesFreed + minFree
		p.Blockers = append(p.Blockers, fmt.Sprintf("not enough space on iPod: need %s, have %s", formatSize(need), formatSize(in.Free)))
	}

	p.Warnings = append(p.Warnings, duplicateWarnings(tagsByPath)...)
	if p.TrackCountAfter > in.Threshold {
		p.Warnings = append(p.Warnings, fmt.Sprintf("%d tracks exceeds the 5th gen safe limit of %d; the iPod may be slow or unstable", p.TrackCountAfter, in.Threshold))
	}
	return p
}

func duplicateWarnings(tagsByPath map[string]library.Tags) []string {
	type key struct {
		dir         string
		disc, track int
	}
	groups := map[key][]string{}
	for rel, tg := range tagsByPath {
		if tg.TrackNumber <= 0 {
			continue
		}
		k := key{path.Dir(rel), tg.DiscNumber, tg.TrackNumber}
		groups[k] = append(groups[k], path.Base(rel))
	}
	var out []string
	for k, files := range groups {
		if len(files) < 2 {
			continue
		}
		sort.Strings(files)
		out = append(out, fmt.Sprintf("possible duplicate: %s: %s", k.dir, strings.Join(files, ", ")))
	}
	sort.Strings(out)
	return out
}

func formatSize(n int64) string {
	if n < 0 {
		return "-" + formatSize(-n)
	}
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	v := float64(n)
	for _, s := range []string{"kB", "MB", "GB", "TB"} {
		v /= unit
		if v < unit || s == "TB" {
			return fmt.Sprintf("%.1f %s", v, s)
		}
	}
	return ""
}
