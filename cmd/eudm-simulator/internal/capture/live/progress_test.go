// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package live

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	tc "github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

type progressLines chan string

func (w progressLines) Write(data []byte) (int, error) {
	w <- string(data)
	return len(data), nil
}

func progressLine(t *testing.T, lines progressLines) string {
	t.Helper()
	select {
	case line := <-lines:
		return line
	case <-time.After(5 * time.Second):
		t.Fatal("capture event did not arrive")
		return ""
	}
}

func assertEventTimestamp(t *testing.T, line string) {
	t.Helper()
	timestamp, _, ok := strings.Cut(line, " ")
	if _, err := time.Parse(time.RFC3339, timestamp); err != nil || !ok {
		t.Fatalf("event lacks a timestamp with timezone: %s", line)
	}
}

func TestCaptureLogsEventsImmediatelyAndDrainsBeforeCompletion(t *testing.T) {
	lines := make(progressLines, 16)
	p := newCaptureProgress(lines, 35*time.Minute)
	initial := progressLine(t, lines)
	assertEventTimestamp(t, initial)
	if !strings.Contains(initial, "[capture] initializing local authentication") {
		t.Fatal("missing immediate startup event", initial)
	}
	p.phase("waiting for producers", "processes=unavailable")
	p.phase("waiting for producers", "processes=unavailable")
	origin := time.Now()
	p.armed(origin)
	p.event("metrics", "captured battery, cpu (3 series)")
	// No explicit wait for the writer: finish must drain accepted events first.
	p.phase("finalizing bundle", "")
	p.finish("/capture/example", nil)
	expected := []string{
		"[capture] waiting for producers; processes=unavailable",
		"[capture] recording started; duration=35m0s; ends=" + origin.Add(35*time.Minute).Local().Format(time.RFC3339),
		"[metrics] captured battery, cpu (3 series)",
		"[capture] finalizing bundle",
		"[capture] complete; bundle=/capture/example",
	}
	for _, want := range expected {
		line := progressLine(t, lines)
		assertEventTimestamp(t, line)
		if !strings.Contains(line, want) || strings.Contains(line, "remaining=") || strings.Contains(line, "cycles:") {
			t.Fatalf("expected event %q, got %s", want, line)
		}
	}
	if len(lines) != 0 {
		t.Fatal("duplicate phases or summary output")
	}
}

type blockedProgressWriter struct {
	once    sync.Once
	entered chan struct{}
	release chan struct{}
	output  bytes.Buffer
}

func (w *blockedProgressWriter) Write(data []byte) (int, error) {
	w.once.Do(func() { close(w.entered) })
	<-w.release
	return w.output.Write(data)
}

func TestBlockedCaptureOutputCannotBlockEvents(t *testing.T) {
	w := &blockedProgressWriter{entered: make(chan struct{}), release: make(chan struct{})}
	p := newCaptureProgress(w, time.Minute)
	<-w.entered
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 100 {
			p.event("processes", "captured 20 processes (2 chunks)")
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		close(w.release)
		t.Fatal("blocked terminal stopped capture bookkeeping")
	}
	close(w.release)
	p.finish("/capture/example", nil)
	if got := w.output.String(); !strings.Contains(got, "omitted 36 log messages") || !strings.Contains(got, "captured data is unaffected") || !strings.HasSuffix(got, "[capture] complete; bundle=/capture/example\n") {
		t.Fatal("missing log overflow notice or final completion", got)
	}
}

func TestFailedCaptureLogsDoNotPrintErrorContentsOrSuccess(t *testing.T) {
	var output bytes.Buffer
	p := newCaptureProgress(&output, time.Minute)
	p.finish("/capture/example", errors.New("private-payload-sentinel"))
	if got := output.String(); !strings.Contains(got, "[capture] failed; no new bundle completed") || strings.Contains(got, "private-payload-sentinel") || strings.Contains(got, "bundle=/capture/example") {
		t.Fatal("failure leaked error contents or claimed success", got)
	}
}

func TestEvidenceLogsOnlyCompletePersistedObservations(t *testing.T) {
	f := newEvidenceFixture(t, "windows", false, true, true)
	var output bytes.Buffer
	p := newCaptureProgress(&output, f.session.Duration)
	f.e.progress = p
	metrics := f.record(t, tc.Metrics, time.Second)
	cpu, battery := metrics.Payload.Series[0], metrics.Payload.Series[0]
	battery.Name = "system.battery.current_charge_pct"
	metrics.Payload.Series = []tc.Series{cpu, battery, cpu}
	f.accept(context.Background(), t, metrics)
	for _, stream := range []tc.Stream{tc.Processes, tc.Connections, tc.Metadata, tc.AgentInventory, tc.HostInventory, tc.HostSystemInfo, tc.Software} {
		f.accept(context.Background(), t, f.record(t, stream, time.Second))
	}
	// Empty and out-of-window observations are not reported as captured.
	empty := f.record(t, tc.Metrics, time.Second)
	empty.Payload.Series = nil
	f.accept(context.Background(), t, empty)
	f.accept(context.Background(), t, f.record(t, tc.Processes, -time.Second))
	// A partial group must produce no successful-capture event.
	partial := f.record(t, tc.Processes, 2*time.Second)
	partial.Payload.Chunks = partial.Payload.Chunks[:1]
	if err := f.e.Accept(context.Background(), partial); err == nil {
		t.Fatal("accepted a partial group")
	}
	p.finish("", errors.New("partial group"))
	got := output.String()
	for _, want := range []string{
		"[metrics] captured battery, cpu (3 series)",
		"[processes] captured 1 processes (2 chunks)",
		"[connections] captured 1 connections (2 chunks)",
		"[host_metadata] captured host metadata",
		"[agent_inventory] captured snapshot",
		"[host_inventory] captured snapshot",
		"[host_system_info] captured snapshot",
		"[software] captured inventory (2 applications)",
	} {
		if strings.Count(got, want) != 1 {
			t.Fatalf("expected exactly one event %q, got %s", want, got)
		}
	}
	if strings.Count(got, "] captured ") != 8 || strings.Contains(got, evidenceHost) || strings.Contains(got, f.session.ID) {
		t.Fatal("invalid or duplicate events, or telemetry identity escaped into output", got)
	}
	for _, line := range strings.Split(strings.TrimSpace(got), "\n") {
		assertEventTimestamp(t, line)
	}
}

func TestCoordinatorLogsOneRecordingStart(t *testing.T) {
	clients, _, sink := newCoordinatorFixture(t, "macos")
	var output bytes.Buffer
	p := newCaptureProgress(&output, testCaptureDuration)
	options := testOptions
	options.progress = p
	err := run(context.Background(), "macos", clients, sink, testCaptureDuration, options)
	p.finish("/capture/example", err)
	if err != nil {
		t.Fatal(err)
	}
	if got := output.String(); strings.Count(got, "recording started") != 1 || strings.Contains(got, "Capture 0s/") || strings.Contains(got, "remaining=") {
		t.Fatal("duplicated recording start or old summary output", got)
	}
}
