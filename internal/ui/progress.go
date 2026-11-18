package ui

import (
	"fmt"
	"io"
	"sync"

	"github.com/BrandonKowalski/cw/internal/apply"
)

type progress struct {
	mu     sync.Mutex
	w      io.Writer
	tty    bool
	name   string
	total  int
	n      int
	active bool
}

func NewProgress(w io.Writer, tty bool) apply.Progress {
	return &progress{w: w, tty: tty}
}

func (p *progress) Phase(name string, total int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commit()
	p.name, p.total, p.n = name, total, 0
	if p.tty {
		p.active = true
		p.draw("")
		return
	}
	fmt.Fprintf(p.w, "%s (%d)\n", name, total)
}

func (p *progress) Step(label string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.n++
	if p.tty && p.active {
		p.draw(label)
	}
}

func (p *progress) Fail(label string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.tty && p.active {
		fmt.Fprint(p.w, "\r\x1b[2K")
	}
	fmt.Fprintf(p.w, "failed: %s: %v\n", label, err)
	if p.tty && p.active {
		p.draw("")
	}
}

func (p *progress) Done() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commit()
}

func (p *progress) draw(label string) {
	line := fmt.Sprintf("%s  %d/%d", p.name, p.n, p.total)
	if label != "" {
		line += "  " + label
	}
	fmt.Fprintf(p.w, "\r\x1b[2K%s", line)
}

func (p *progress) commit() {
	if p.tty && p.active {
		fmt.Fprintln(p.w)
	}
	p.active = false
}
