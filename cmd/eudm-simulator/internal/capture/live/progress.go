// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package live

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// Events carry only fixed component names, owned summaries, and timestamps.
// No payload or mutable evidence is retained by the terminal writer.
type captureEvent struct {
	at        time.Time
	component string
	message   string
}

type captureProgress struct {
	mu                sync.Mutex
	duration          time.Duration
	phaseName, detail string
	events            chan captureEvent
	done              chan struct{}
	closed            bool
	omitted           uint64
	final             captureEvent
}

func newCaptureProgress(w io.Writer, duration time.Duration) *captureProgress {
	if w == nil {
		return nil
	}
	p := &captureProgress{duration: duration, events: make(chan captureEvent, 64), done: make(chan struct{})}
	p.phase("initializing local authentication", "")
	go func() {
		defer close(p.done)
		for event := range p.events {
			if err := writeCaptureEvent(w, event); err != nil {
				return
			}
		}
		p.mu.Lock()
		final, omitted := p.final, p.omitted
		p.mu.Unlock()
		if omitted != 0 {
			if err := writeCaptureEvent(w, captureEvent{at: final.at, component: "capture", message: fmt.Sprintf("omitted %d log messages because output could not keep up; captured data is unaffected", omitted)}); err != nil {
				return
			}
		}
		_ = writeCaptureEvent(w, final)
	}()
	return p
}

func (p *captureProgress) event(component, message string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.enqueueLocked(component, message)
}

// Terminal output must never delay capture, heartbeats, or producer cleanup.
func (p *captureProgress) enqueueLocked(component, message string) {
	if p.closed {
		return
	}
	select {
	case p.events <- captureEvent{at: time.Now(), component: component, message: message}:
	default:
		p.omitted++
	}
}

func (p *captureProgress) phase(phase, detail string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.phaseName == phase && p.detail == detail {
		return
	}
	p.phaseName, p.detail = phase, detail
	if detail != "" {
		phase += "; " + detail
	}
	p.enqueueLocked("capture", phase)
}

func (p *captureProgress) armed(origin time.Time) {
	if p != nil {
		p.event("capture", fmt.Sprintf("recording started; duration=%s; ends=%s", p.duration, origin.Add(p.duration).Local().Format(time.RFC3339)))
	}
}

func (p *captureProgress) finish(directory string, err error) {
	if p == nil {
		return
	}
	message := "complete; bundle=" + directory
	if err != nil {
		message = "failed; no new bundle completed"
	}
	p.mu.Lock()
	p.final = captureEvent{at: time.Now(), component: "capture", message: message}
	p.closed = true
	close(p.events)
	p.mu.Unlock()
	// Drain queued events in order, but don't let a blocked writer hold up exit.
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-p.done:
	case <-timer.C:
	}
}

func writeCaptureEvent(w io.Writer, event captureEvent) error {
	_, err := fmt.Fprintf(w, "%s [%s] %s\n", event.at.Format(time.RFC3339), event.component, event.message)
	return err
}
