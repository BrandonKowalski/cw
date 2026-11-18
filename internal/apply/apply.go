package apply

import (
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"

	"github.com/BrandonKowalski/cw/internal/artwork"
	"github.com/BrandonKowalski/cw/internal/device"
	"github.com/BrandonKowalski/cw/internal/fsutil"
	"github.com/BrandonKowalski/cw/internal/plan"
	"github.com/BrandonKowalski/cw/internal/state"
	"github.com/BrandonKowalski/cw/internal/transcode"
)

type Progress interface {
	Phase(name string, total int)
	Step(label string)
	Fail(label string, err error)
	Done()
}

type Options struct {
	Jobs    int
	TempDir string
	Now     func() time.Time
}

type Report struct {
	Uploaded, AudioUpdated, TagUpdated, Deleted, Unmanaged int
	Failures                                               []plan.Failure
	Warnings                                               []string
	Elapsed                                                time.Duration
}

type outcome struct {
	item    plan.Item
	path    string
	res     transcode.Result
	artID   string
	artSize uint32
	artWarn error
	err     error
}

type applier struct {
	d       *device.Device
	lib     string
	tmpDir  string
	pathMu  sync.Mutex
	artMu   sync.Mutex
	artJobs map[string]*artJob
}

type artJob struct {
	once sync.Once
	err  error
}

func Apply(ctx context.Context, d *device.Device, st *state.State, p *plan.Plan, lib string, o Options, prog Progress) (rep Report, err error) {
	now := time.Now
	if o.Now != nil {
		now = o.Now
	}
	start := now()
	wrote := false
	defer func() {
		if wrote {
			unix.Sync()
		}
		rep.Elapsed = now().Sub(start)
	}()
	if len(p.Blockers) > 0 {
		return rep, fmt.Errorf("sync blocked: %s", strings.Join(p.Blockers, "; "))
	}
	jobs := max(o.Jobs, 1)
	tmpDir := o.TempDir
	if tmpDir == "" {
		tmpDir = os.TempDir()
	}
	a := &applier{d: d, lib: lib, tmpDir: tmpDir, artJobs: map[string]*artJob{}}

	var work, tagItems []plan.Item
	for _, it := range p.Items {
		if it.Kind == plan.TagUpdate {
			tagItems = append(tagItems, it)
		} else {
			work = append(work, it)
		}
	}

	prog.Phase("transcoding", len(work))
	wrote = true
	done := a.transcodeAll(ctx, work, jobs, prog, &rep)
	prog.Done()
	if err := ctx.Err(); err != nil {
		a.discard(done)
		return rep, err
	}

	prog.Phase("updating", len(done)+len(tagItems)+len(p.Deletes))
	used := make(map[uint64]bool, len(st.Tracks))
	for _, t := range st.Tracks {
		used[t.DBID] = true
	}
	for _, oc := range done {
		tr := oc.item.Track
		rel := tr.Entry.RelPath
		if oc.artWarn != nil {
			rep.Warnings = append(rep.Warnings, artWarning(rel, oc.artWarn))
		}
		nt := &state.Track{
			DevicePath:    oc.path,
			SourceSize:    tr.Entry.Size,
			SourceMTime:   tr.Entry.ModTime.UnixNano(),
			TagFP:         tr.TagFP,
			AudioFP:       tr.AudioFP,
			EncodeVersion: transcode.Version,
			Tags:          tr.Tags,
			DurationMS:    oc.res.DurationMS,
			SampleRate:    oc.res.SampleRate,
			BitrateKbps:   oc.res.BitrateKbps,
			FileSize:      oc.res.Size,
			ArtID:         oc.artID,
			ArtSize:       oc.artSize,
		}
		if old := st.Tracks[rel]; oc.item.Kind == plan.AudioUpdate && old != nil {
			nt.DBID = old.DBID
			nt.DateAdded = old.DateAdded
			rep.AudioUpdated++
		} else {
			nt.DBID = newDBID(used)
			nt.DateAdded = now().Unix()
			rep.Uploaded++
		}
		used[nt.DBID] = true
		st.Tracks[rel] = nt
		prog.Step(rel)
	}
	for _, it := range tagItems {
		tr := it.Track
		rel := tr.Entry.RelPath
		old := st.Tracks[rel]
		if old == nil {
			err := fmt.Errorf("update %s: not in state", rel)
			rep.Failures = append(rep.Failures, plan.Failure{RelPath: rel, Err: err})
			prog.Fail(rel, err)
			continue
		}
		id, size, artErr := a.ensureArt(tr.Cover)
		if artErr != nil {
			rep.Warnings = append(rep.Warnings, artWarning(rel, artErr))
		}
		old.Tags = tr.Tags
		old.TagFP = tr.TagFP
		old.ArtID = id
		old.ArtSize = size
		old.SourceSize = tr.Entry.Size
		old.SourceMTime = tr.Entry.ModTime.UnixNano()
		rep.TagUpdated++
		prog.Step(rel)
	}
	for rel, e := range p.Touched {
		if t := st.Tracks[rel]; t != nil {
			t.SourceSize = e.Size
			t.SourceMTime = e.ModTime.UnixNano()
		}
	}
	for _, del := range p.Deletes {
		if _, ok := st.Tracks[del.RelPath]; ok {
			delete(st.Tracks, del.RelPath)
			rep.Deleted++
		}
		prog.Step(del.RelPath)
	}
	rep.Failures = append(rep.Failures, p.Failures...)
	prog.Done()

	st.LastSync = now().Unix()
	st.DBPending = true
	if err := state.Save(d.Mount, st); err != nil {
		return rep, err
	}

	prog.Phase("writing database", 1)
	missing, err := WriteDatabases(d, st)
	if err != nil {
		return rep, err
	}
	for _, rel := range missing {
		rep.Warnings = append(rep.Warnings, "cover art cache missing for "+rel)
	}
	st.DBPending = false
	if err := state.Save(d.Mount, st); err != nil {
		return rep, err
	}
	pc := filepath.Join(d.Mount, "iPod_Control", "iTunes", "Play Counts")
	if err := os.Remove(pc); err != nil && !os.IsNotExist(err) {
		return rep, fmt.Errorf("remove Play Counts: %w", err)
	}
	prog.Step("iTunesDB")
	prog.Done()

	return rep, a.cleanup(st, p, prog, &rep)
}

func Recover(d *device.Device, st *state.State) (bool, error) {
	if !st.DBPending {
		return false, nil
	}
	if _, err := WriteDatabases(d, st); err != nil {
		return false, err
	}
	st.DBPending = false
	if err := state.Save(d.Mount, st); err != nil {
		return false, err
	}
	unix.Sync()
	return true, nil
}

func artWarning(rel string, err error) string {
	return fmt.Sprintf("no cover art for %s: %v", rel, err)
}

func (a *applier) transcodeAll(ctx context.Context, work []plan.Item, jobs int, prog Progress, rep *Report) []outcome {
	in := make(chan plan.Item)
	out := make(chan outcome)
	var wg sync.WaitGroup
	for range min(jobs, max(len(work), 1)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for it := range in {
				out <- a.transcodeOne(ctx, it)
			}
		}()
	}
	go func() {
		defer close(in)
		for _, it := range work {
			select {
			case <-ctx.Done():
				return
			case in <- it:
			}
		}
	}()
	go func() {
		wg.Wait()
		close(out)
	}()
	var done []outcome
	for oc := range out {
		rel := oc.item.Track.Entry.RelPath
		if oc.err != nil {
			if ctx.Err() == nil {
				rep.Failures = append(rep.Failures, plan.Failure{RelPath: rel, Err: oc.err})
				prog.Fail(rel, oc.err)
			}
			continue
		}
		done = append(done, oc)
		prog.Step(rel)
	}
	sort.Slice(done, func(i, j int) bool {
		return done[i].item.Track.Entry.RelPath < done[j].item.Track.Entry.RelPath
	})
	return done
}

func (a *applier) transcodeOne(ctx context.Context, it plan.Item) outcome {
	oc := outcome{item: it}
	tr := it.Track
	if err := ctx.Err(); err != nil {
		oc.err = err
		return oc
	}
	oc.artID, oc.artSize, oc.artWarn = a.ensureArt(tr.Cover)
	f, err := os.CreateTemp(a.tmpDir, "cw-*.m4a")
	if err != nil {
		oc.err = fmt.Errorf("create temp file: %w", err)
		return oc
	}
	tmp := f.Name()
	f.Close()
	defer os.Remove(tmp)
	src := filepath.Join(a.lib, filepath.FromSlash(tr.Entry.RelPath))
	if err := transcode.Run(ctx, src, tmp, tr.Stream); err != nil {
		oc.err = fmt.Errorf("%s: %w", tr.Entry.RelPath, err)
		return oc
	}
	if oc.res, err = transcode.Inspect(tmp, tr.Stream); err != nil {
		oc.err = fmt.Errorf("%s: %w", tr.Entry.RelPath, err)
		return oc
	}
	if oc.path, err = a.place(tmp); err != nil {
		oc.err = fmt.Errorf("%s: %w", tr.Entry.RelPath, err)
	}
	return oc
}

func (a *applier) place(tmp string) (string, error) {
	src, err := os.Open(tmp)
	if err != nil {
		return "", fmt.Errorf("open transcoded file: %w", err)
	}
	defer src.Close()
	a.pathMu.Lock()
	rel, err := a.d.NewTrackPath(".m4a")
	if err != nil {
		a.pathMu.Unlock()
		return "", err
	}
	dst := filepath.Join(a.d.Mount, filepath.FromSlash(rel))
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	a.pathMu.Unlock()
	if err != nil {
		return "", fmt.Errorf("create %s: %w", rel, err)
	}
	_, err = io.Copy(f, src)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(dst)
		return "", fmt.Errorf("copy to %s: %w", rel, err)
	}
	return rel, nil
}

func (a *applier) discard(done []outcome) {
	for _, oc := range done {
		os.Remove(filepath.Join(a.d.Mount, filepath.FromSlash(oc.path)))
	}
}

func artDir(mount string) string {
	return filepath.Join(mount, "iPod_Control", "cw", "art")
}

func artPath(mount, id string, f artwork.Format) string {
	return filepath.Join(artDir(mount), fmt.Sprintf("%s-%d.rgb", id, f.ID))
}

func (a *applier) ensureArt(cover []byte) (string, uint32, error) {
	if len(cover) == 0 {
		return "", 0, nil
	}
	id := artwork.ID(cover)
	a.artMu.Lock()
	j := a.artJobs[id]
	if j == nil {
		j = &artJob{}
		a.artJobs[id] = j
	}
	a.artMu.Unlock()
	j.once.Do(func() { j.err = a.renderArt(id, cover) })
	if j.err != nil {
		return "", 0, j.err
	}
	return id, uint32(len(cover)), nil
}

func (a *applier) renderArt(id string, cover []byte) error {
	if _, err := loadThumbs(a.d.Mount, id); err == nil {
		return nil
	}
	for _, f := range artwork.Formats {
		b, err := artwork.Render(cover, f)
		if err != nil {
			return err
		}
		if err := fsutil.WriteAtomic(artPath(a.d.Mount, id, f), b); err != nil {
			return fmt.Errorf("cache cover art: %w", err)
		}
	}
	return nil
}

func (a *applier) cleanup(st *state.State, p *plan.Plan, prog Progress, rep *Report) error {
	files, err := a.d.MusicFiles()
	if err != nil {
		return err
	}
	referenced := make(map[string]bool, len(st.Tracks))
	arts := map[string]bool{}
	for _, t := range st.Tracks {
		referenced[t.DevicePath] = true
		if t.ArtID != "" {
			arts[t.ArtID] = true
		}
	}
	unmanaged := make(map[string]bool, len(p.Unmanaged))
	for _, u := range p.Unmanaged {
		unmanaged[u] = true
	}
	var remove []string
	for _, f := range files {
		if !referenced[f] {
			remove = append(remove, filepath.Join(a.d.Mount, filepath.FromSlash(f)))
			if unmanaged[f] {
				rep.Unmanaged++
			}
		}
	}
	entries, err := os.ReadDir(artDir(a.d.Mount))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("list art cache: %w", err)
	}
	for _, e := range entries {
		id, _, _ := strings.Cut(e.Name(), "-")
		if !arts[id] || strings.HasSuffix(e.Name(), ".tmp") {
			remove = append(remove, filepath.Join(artDir(a.d.Mount), e.Name()))
		}
	}
	prog.Phase("cleaning up", len(remove))
	for _, path := range remove {
		rel, _ := filepath.Rel(a.d.Mount, path)
		rel = filepath.ToSlash(rel)
		if err := os.RemoveAll(path); err != nil {
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("could not remove %s: %v", rel, err))
			prog.Fail(rel, err)
			continue
		}
		prog.Step(rel)
	}
	prog.Done()
	return nil
}

func newDBID(used map[uint64]bool) uint64 {
	for {
		id := rand.Uint64()
		if id != 0 && !used[id] {
			return id
		}
	}
}
