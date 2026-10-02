// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package eventlog

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestConcurrentLoggingAndCloseRetainsOneFinalEvent(t *testing.T) {
	var output bytes.Buffer
	l := New(&output)
	var workers sync.WaitGroup
	start := make(chan struct{})
	for range 16 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			for range 20 {
				l.Log("metrics", "submitted cpu (1 series)")
			}
			l.Close("replay", "complete")
		}()
	}
	close(start)
	workers.Wait()
	l.Log("metrics", "after close")
	l.Close("replay", "second final event")
	got := output.String()
	if strings.Count(got, "[replay] complete\n") != 1 || !strings.HasSuffix(got, "[replay] complete\n") || strings.Contains(got, "after close") || strings.Contains(got, "second final event") {
		t.Fatal("concurrent shutdown lost or duplicated the final event", got)
	}
	for _, line := range strings.Split(strings.TrimSpace(got), "\n") {
		timestamp, _, ok := strings.Cut(line, " ")
		if _, err := time.Parse(time.RFC3339, timestamp); err != nil || !ok {
			t.Fatalf("event lacks a timestamp with timezone: %s", line)
		}
	}
}

type blockedWriter struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	output  bytes.Buffer
}

func (w *blockedWriter) Write(data []byte) (int, error) {
	w.once.Do(func() { close(w.entered) })
	<-w.release
	return w.output.Write(data)
}

func TestCloseReturnsWhileWriterBlockedAndEventuallyDrains(t *testing.T) {
	w := &blockedWriter{entered: make(chan struct{}), release: make(chan struct{})}
	l := New(w)
	l.Log("metrics", "submitted cpu")
	<-w.entered
	l.Log("software", "submitted inventory")
	closed := make(chan struct{})
	go func() {
		l.Close("replay", "complete")
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		close(w.release)
		t.Fatal("blocked terminal prevented command exit")
	}
	close(w.release)
	select {
	case <-l.done:
	case <-time.After(5 * time.Second):
		t.Fatal("writer did not exit after terminal recovered")
	}
	got := w.output.String()
	if strings.Count(got, "\n") != 3 || !strings.Contains(got, "[metrics] submitted cpu\n") || !strings.Contains(got, "[software] submitted inventory\n") || !strings.HasSuffix(got, "[replay] complete\n") {
		t.Fatal("shutdown failed to drain accepted events", got)
	}
}

func TestNilWriterDisablesOutput(t *testing.T) {
	l := New(nil)
	if l != nil {
		t.Fatal("nil writer did not disable logging")
	}
	l.Log("metrics", "submitted cpu")
	l.Close("replay", "complete")
}
