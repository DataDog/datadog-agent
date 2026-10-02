// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package eventlog writes bounded, timestamped CLI events independently of
// telemetry processing.
package eventlog

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// Events retain only owned summaries and timestamps, never mutable telemetry.
type event struct {
	at        time.Time
	component string
	message   string
}

// Logger writes CLI events without allowing terminal output to delay telemetry
// processing. It is safe for concurrent use; a nil Logger disables output.
type Logger struct {
	mu      sync.Mutex
	events  chan event
	done    chan struct{}
	closed  bool
	omitted uint64
	final   event
}

// New starts a logger for w, or returns nil when output is disabled.
func New(w io.Writer) *Logger {
	if w == nil {
		return nil
	}
	l := &Logger{events: make(chan event, 64), done: make(chan struct{})}
	go l.write(w)
	return l
}

func (l *Logger) write(w io.Writer) {
	defer close(l.done)
	for e := range l.events {
		if err := writeEvent(w, e); err != nil {
			return
		}
	}
	l.mu.Lock()
	final, omitted := l.final, l.omitted
	l.mu.Unlock()
	if omitted != 0 {
		if err := writeEvent(w, event{
			at: final.at, component: final.component,
			message: fmt.Sprintf("omitted %d log messages because output could not keep up; telemetry processing is unaffected", omitted),
		}); err != nil {
			return
		}
	}
	_ = writeEvent(w, final)
}

// Log queues an event without waiting for terminal output. When its bounded
// queue is full, the logger counts omissions and reports them during Close.
func (l *Logger) Log(component, message string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return
	}
	select {
	case <-l.done:
		// A failed writer must not accumulate more events.
		return
	default:
	}
	select {
	case l.events <- event{at: time.Now(), component: component, message: message}:
	default:
		l.omitted++
	}
}

// Close drains accepted events in order, followed by the final event. It waits
// at most one second so a blocked writer cannot hold up command exit. Repeated
// calls keep the first final event, and subsequent Log calls have no effect.
func (l *Logger) Close(component, message string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	if !l.closed {
		l.final = event{at: time.Now(), component: component, message: message}
		l.closed = true
		close(l.events)
	}
	l.mu.Unlock()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-l.done:
	case <-timer.C:
	}
}

func writeEvent(w io.Writer, e event) error {
	_, err := fmt.Fprintf(w, "%s [%s] %s\n", e.at.Local().Format(time.RFC3339), e.component, e.message)
	return err
}
