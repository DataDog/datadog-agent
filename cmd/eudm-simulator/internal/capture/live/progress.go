// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package live

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

const captureProgressInterval = 30 * time.Second

type captureSnapshot struct {
	origin   time.Time
	duration time.Duration
	phase    string
	detail   string
	cycles   string
	missing  string
	bytes    int64
}

// Only owned counts, fixed stream names, and control-plane status cross into
// the observer. It never reads mutable evidence or writes while holding a lock.
type captureProgress struct {
	mu      sync.Mutex
	current captureSnapshot
	changes chan captureSnapshot
	stop    chan struct{}
	done    chan struct{}
}

func newCaptureProgress(w io.Writer, duration time.Duration, ticks <-chan time.Time) *captureProgress {
	if w == nil {
		return nil
	}
	p := &captureProgress{current: captureSnapshot{duration: duration, phase: "initializing local authentication"},
		changes: make(chan captureSnapshot, 8), stop: make(chan struct{}), done: make(chan struct{})}
	initial := p.current
	go func() {
		defer close(p.done)
		if ticks == nil {
			ticker := time.NewTicker(captureProgressInterval)
			defer ticker.Stop()
			ticks = ticker.C
		}
		if err := writeCaptureProgress(w, initial, time.Now()); err != nil {
			return
		}
		for {
			var snapshot captureSnapshot
			var at time.Time
			final := false
			select {
			case <-p.stop:
				snapshot, final = p.snapshot(), true
			case snapshot = <-p.changes:
			case at = <-ticks:
				snapshot = p.snapshot()
			}
			if at.IsZero() {
				at = time.Now()
			}
			if err := writeCaptureProgress(w, snapshot, at); err != nil || final {
				return
			}
		}
	}()
	return p
}

func (p *captureProgress) snapshot() captureSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.current
}

// State changes request immediate output. A blocked terminal can retain at
// most eight old snapshots; periodic output always uses the latest snapshot.
func (p *captureProgress) phase(phase, detail string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	if p.current.phase == phase && p.current.detail == detail {
		p.mu.Unlock()
		return
	}
	p.current.phase, p.current.detail = phase, detail
	snapshot := p.current
	p.mu.Unlock()
	select {
	case p.changes <- snapshot:
	default:
	}
}

func (p *captureProgress) armed(origin time.Time) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.current.origin = origin
	p.mu.Unlock()
}

func (p *captureProgress) samples(cycles, missing string, bytes int64) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.current.cycles, p.current.missing, p.current.bytes = cycles, missing, bytes
	p.mu.Unlock()
}

func (p *captureProgress) finish(directory string, err error) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.current.phase, p.current.detail = "complete", "bundle="+directory
	if err != nil {
		p.current.phase, p.current.detail = "failed", "bundle incomplete"
	}
	p.mu.Unlock()
	close(p.stop)
	// A terminal writer is not cancellable. It must not hold up producer
	// cleanup or command exit indefinitely; output remains best effort.
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-p.done:
	case <-timer.C:
	}
}

func writeCaptureProgress(w io.Writer, s captureSnapshot, at time.Time) error {
	var line strings.Builder
	if s.origin.IsZero() {
		fmt.Fprintf(&line, "Capture | %s | recording=%s", s.phase, s.duration)
	} else {
		elapsed := min(s.duration, max(0, at.Sub(s.origin)))
		fmt.Fprintf(&line, "Capture %s/%s | remaining=%s | %s", elapsed.Truncate(time.Second), s.duration, (s.duration - elapsed).Round(time.Second), s.phase)
		fmt.Fprintf(&line, " | cycles: %s | sample data=%.1f MiB", s.cycles, float64(s.bytes)/(1<<20))
		if s.missing == "" {
			line.WriteString(" | coverage=complete")
		} else {
			fmt.Fprintf(&line, " | waiting for: %s", s.missing)
		}
	}
	if s.detail != "" {
		fmt.Fprintf(&line, " | %s", s.detail)
	}
	line.WriteByte('\n')
	_, err := io.WriteString(w, line.String())
	return err
}
