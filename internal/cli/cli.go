package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"

	"github.com/BrandonKowalski/cw/internal/apply"
	"github.com/BrandonKowalski/cw/internal/config"
	"github.com/BrandonKowalski/cw/internal/device"
	"github.com/BrandonKowalski/cw/internal/library"
	"github.com/BrandonKowalski/cw/internal/plan"
	"github.com/BrandonKowalski/cw/internal/state"
	"github.com/BrandonKowalski/cw/internal/transcode"
	"github.com/BrandonKowalski/cw/internal/ui"
)

const usage = "usage: cw <sync|diff|info> [flags]\n"

const noLibrary = "no library set: pass --library or set library in ~/.config/cw/config.toml"

type options struct {
	device, library string
	verbose, dryRun bool
	yes             bool
	jobs            int
	jobsSet         bool
}

func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "sync", "diff":
		return runPlan(args[0], args[1:], stdin, stdout, stderr)
	case "info":
		return runInfo(args[1:], stdout, stderr)
	case "help":
		fmt.Fprint(stdout, usage)
		return 0
	}
	fmt.Fprint(stderr, usage)
	return 2
}

func newFlags(name string, o *options, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("cw "+name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&o.device, "device", "", "iPod mount point")
	fs.StringVar(&o.library, "library", "", "FLAC library directory")
	if name == "info" {
		return fs
	}
	fs.BoolVar(&o.verbose, "verbose", false, "list individual files")
	fs.IntVar(&o.jobs, "jobs", 0, "parallel jobs")
	fs.IntVar(&o.jobs, "j", 0, "parallel jobs")
	if name == "sync" {
		fs.BoolVar(&o.dryRun, "dry-run", false, "show the plan without applying it")
		fs.BoolVar(&o.yes, "y", false, "apply without prompting")
	}
	return fs
}

func parse(name string, args []string, stderr io.Writer) (options, int, bool) {
	var o options
	fs := newFlags(name, &o, stderr)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return o, 0, false
		}
		return o, 2, false
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "unexpected argument: %s\n%s", fs.Arg(0), usage)
		return o, 2, false
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "jobs" || f.Name == "j" {
			o.jobsSet = true
		}
	})
	return o, 0, true
}

func resolveLibrary(o options, cfg config.Config) (string, error) {
	lib := o.library
	if lib == "" {
		lib = cfg.Library
	}
	if lib == "" {
		return "", errors.New(noLibrary)
	}
	return lib, nil
}

func resolveDevice(o options) (*device.Device, error) {
	if o.device != "" {
		return device.Open(o.device)
	}
	mount, err := device.Find("/Volumes")
	if err != nil {
		return nil, err
	}
	return device.Open(mount)
}

func loadConfig() (config.Config, error) {
	p, err := config.DefaultPath()
	if err != nil {
		return config.Config{}, fmt.Errorf("config path: %w", err)
	}
	cfg, err := config.Load(p)
	if err != nil {
		return config.Config{}, fmt.Errorf("load config: %w", err)
	}
	return cfg, nil
}

func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(int(f.Fd()))
}

func fail(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "error: %v\n", err)
	return 1
}

type loaded struct {
	t   *library.Track
	err error
}

func prefetch(lib string, entries []library.Entry, st *state.State, jobs int) func(library.Entry) (*library.Track, error) {
	var need []library.Entry
	for _, e := range entries {
		if plan.NeedsLoad(st.Tracks[e.RelPath], e, transcode.Version) {
			need = append(need, e)
		}
	}
	results := make(map[string]loaded, len(need))
	var mu sync.Mutex
	ch := make(chan library.Entry)
	var wg sync.WaitGroup
	for i := 0; i < jobs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for e := range ch {
				t, err := library.Load(lib, e)
				mu.Lock()
				results[e.RelPath] = loaded{t, err}
				mu.Unlock()
			}
		}()
	}
	for _, e := range need {
		ch <- e
	}
	close(ch)
	wg.Wait()
	return func(e library.Entry) (*library.Track, error) {
		if r, ok := results[e.RelPath]; ok {
			return r.t, r.err
		}
		return library.Load(lib, e)
	}
}

func dirTotals(entries []library.Entry) map[string]int {
	m := map[string]int{}
	for _, e := range entries {
		m[path.Dir(e.RelPath)]++
	}
	return m
}

func runPlan(name string, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	o, code, ok := parse(name, args, stderr)
	if !ok {
		return code
	}
	cfg, err := loadConfig()
	if err != nil {
		return fail(stderr, err)
	}
	jobs := cfg.Jobs
	if o.jobsSet {
		jobs = o.jobs
	}
	if jobs < 1 {
		fmt.Fprintln(stderr, "--jobs must be at least 1")
		return 2
	}
	lib, err := resolveLibrary(o, cfg)
	if err != nil {
		return fail(stderr, err)
	}
	d, err := resolveDevice(o)
	if err != nil {
		return fail(stderr, err)
	}
	syncing := name == "sync"
	if syncing {
		if err := transcode.CheckFFmpeg(); err != nil {
			return fail(stderr, err)
		}
	}
	entries, err := library.Scan(lib)
	if err != nil {
		return fail(stderr, fmt.Errorf("scan library: %w", err))
	}
	st, err := state.Load(d.Mount)
	if err != nil {
		return fail(stderr, err)
	}
	if st.DBPending {
		if syncing && !o.dryRun {
			recovered, err := apply.Recover(d, st)
			if err != nil {
				return fail(stderr, fmt.Errorf("recover: %w", err))
			}
			if recovered {
				fmt.Fprintln(stdout, "Recovered an interrupted sync: rebuilt the iPod database.")
			}
		} else {
			fmt.Fprintln(stdout, "Warning: the last sync was interrupted; run cw sync to repair it.")
		}
	}
	files, err := d.MusicFiles()
	if err != nil {
		return fail(stderr, err)
	}
	free, err := d.Free()
	if err != nil {
		return fail(stderr, err)
	}
	p := plan.Build(plan.Input{
		Entries:       entries,
		State:         st,
		DeviceFiles:   files,
		Free:          free,
		Threshold:     cfg.TrackWarnThreshold,
		EncodeVersion: transcode.Version,
		Load:          prefetch(lib, entries, st, jobs),
	})
	quiet := syncing && !o.dryRun && p.OnlyTouched()
	if !quiet {
		ui.RenderPlanFor(stdout, ui.PlanInfo{Mount: d.Mount, Library: lib, Threshold: cfg.TrackWarnThreshold, DirTotals: dirTotals(entries)}, p, o.verbose)
	}
	if !syncing || o.dryRun {
		return 0
	}
	if quiet {
		code := applyPlan(d, st, p, lib, jobs, io.Discard, stderr, true)
		if code == 0 {
			fmt.Fprintln(stdout, "Already in sync.")
		}
		return code
	}
	if len(p.Blockers) > 0 {
		return 1
	}
	if p.Empty() {
		if len(p.Failures) > 0 {
			return 1
		}
		return 0
	}
	if !o.yes {
		fmt.Fprint(stdout, "\nApply? [y/N] ")
		if !confirm(stdin) {
			fmt.Fprintln(stdout, "Nothing changed.")
			return 0
		}
		fmt.Fprintln(stdout)
	}
	return applyPlan(d, st, p, lib, jobs, stdout, stderr, false)
}

func confirm(stdin io.Reader) bool {
	if stdin == nil {
		return false
	}
	line, _ := bufio.NewReader(stdin).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	}
	return false
}

func applyPlan(d *device.Device, st *state.State, p *plan.Plan, lib string, jobs int, stdout, stderr io.Writer, quiet bool) int {
	tmp, err := os.MkdirTemp("", "cw-")
	if err != nil {
		return fail(stderr, fmt.Errorf("create temp dir: %w", err))
	}
	defer os.RemoveAll(tmp)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	rep, err := apply.Apply(ctx, d, st, p, lib, apply.Options{Jobs: jobs, TempDir: tmp}, ui.NewProgress(stdout, isTTY(stdout)))
	if quiet {
		if err != nil {
			return fail(stderr, err)
		}
		return 0
	}
	fmt.Fprintln(stdout)
	if errors.Is(err, context.Canceled) {
		ui.RenderInterrupted(stdout, rep)
		return 1
	}
	ui.RenderReport(stdout, rep)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if len(rep.Failures) > 0 {
		return 1
	}
	return 0
}

func runInfo(args []string, stdout, stderr io.Writer) int {
	o, code, ok := parse("info", args, stderr)
	if !ok {
		return code
	}
	cfg, err := loadConfig()
	if err != nil {
		return fail(stderr, err)
	}
	d, err := resolveDevice(o)
	if err != nil {
		return fail(stderr, err)
	}
	st, err := state.Load(d.Mount)
	if err != nil {
		return fail(stderr, err)
	}
	free, err := d.Free()
	if err != nil {
		return fail(stderr, err)
	}
	var sfs unix.Statfs_t
	if err := unix.Statfs(d.Mount, &sfs); err != nil {
		return fail(stderr, fmt.Errorf("statfs %s: %w", d.Mount, err))
	}
	info := ui.Info{
		Mount:    d.Mount,
		Model:    d.ModelString(),
		Capacity: int64(sfs.Blocks) * int64(sfs.Bsize),
		Free:     free,
		Tracks:   len(st.Tracks),
	}
	if st.LastSync > 0 {
		info.LastSync = time.Unix(st.LastSync, 0)
	}
	if lib, err := resolveLibrary(o, cfg); err == nil {
		info.Library = lib
	}
	ui.RenderInfo(stdout, info)
	return 0
}
