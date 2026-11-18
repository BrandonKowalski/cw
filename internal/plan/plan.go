package plan

import (
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/BrandonKowalski/cw/internal/library"
	"github.com/BrandonKowalski/cw/internal/state"
	"github.com/BrandonKowalski/cw/internal/units"
)

type Kind int

const (
	Upload Kind = iota
	AudioUpdate
	TagUpdate
)

const minFree = 64 << 20

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
	return len(p.Items) == 0 && len(p.Deletes) == 0 && len(p.Unmanaged) == 0 && len(p.Touched) == 0 && len(p.Blockers) == 0
}

func EstimateBytes(size int64) int64 {
	return size * 11 / 10
}

func NeedsLoad(old *state.Track, e library.Entry, encodeVersion string) bool {
	return old == nil || old.SourceSize != e.Size || old.SourceMTime != e.ModTime.UnixNano() || old.EncodeVersion != encodeVersion
}

func (p *Plan) OnlyTouched() bool {
	return len(p.Touched) > 0 && len(p.Items) == 0 && len(p.Deletes) == 0 && len(p.Unmanaged) == 0 && len(p.Blockers) == 0 && len(p.Failures) == 0
}

func Build(in Input) *Plan {
	p := &Plan{Touched: map[string]library.Entry{}}
	tracks := map[string]*state.Track{}
	if in.State != nil && in.State.Tracks != nil {
		tracks = in.State.Tracks
	}

	entries := append([]library.Entry(nil), in.Entries...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].RelPath < entries[j].RelPath })

	onDevice := make(map[string]bool, len(in.DeviceFiles))
	for _, f := range in.DeviceFiles {
		onDevice[strings.ToLower(f)] = true
	}

	inLibrary := make(map[string]bool, len(entries))
	tagsByPath := map[string]library.Tags{}
	kept := 0

	for _, e := range entries {
		inLibrary[e.RelPath] = true
		old := tracks[e.RelPath]
		missing := false
		if old != nil {
			kept++
			tagsByPath[e.RelPath] = old.Tags
			missing = len(onDevice) > 0 && !onDevice[strings.ToLower(old.DevicePath)]
			if !missing && !NeedsLoad(old, e, in.EncodeVersion) {
				continue
			}
		}
		tr, err := in.Load(e)
		if err == nil && tr == nil {
			err = errors.New("no track returned")
		}
		if err != nil {
			p.Failures = append(p.Failures, Failure{RelPath: e.RelPath, Err: fmt.Errorf("load %s: %w", e.RelPath, err)})
			continue
		}
		tagsByPath[e.RelPath] = tr.Tags
		switch {
		case old == nil:
			p.Items = append(p.Items, Item{Kind: Upload, Track: tr})
			p.BytesIn += EstimateBytes(e.Size)
			kept++
		case missing:
			p.Items = append(p.Items, Item{Kind: AudioUpdate, Track: tr})
			p.BytesIn += EstimateBytes(e.Size)
		case old.AudioFP != tr.AudioFP || old.EncodeVersion != in.EncodeVersion:
			p.Items = append(p.Items, Item{Kind: AudioUpdate, Track: tr})
			p.BytesIn += EstimateBytes(e.Size)
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
		if len(p.Deletes)*2 > len(tracks) {
			p.Warnings = append(p.Warnings, fmt.Sprintf("about to delete %d of %d tracks on the iPod", len(p.Deletes), len(tracks)))
		}
	}
	p.TrackCountAfter = kept

	referenced := make(map[string]bool, len(tracks))
	for _, t := range tracks {
		referenced[strings.ToLower(t.DevicePath)] = true
	}
	for _, f := range in.DeviceFiles {
		if referenced[strings.ToLower(f)] || strings.HasPrefix(path.Base(f), ".") {
			continue
		}
		p.Unmanaged = append(p.Unmanaged, f)
	}
	sort.Strings(p.Unmanaged)

	p.FreeAfter = in.Free - p.BytesIn + p.BytesFreed
	if p.BytesIn > 0 && in.Free-p.BytesIn < minFree {
		need := p.BytesIn + minFree
		p.Blockers = append(p.Blockers, fmt.Sprintf("not enough space on iPod: need %s, have %s", units.Size(need), units.Size(in.Free)))
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
