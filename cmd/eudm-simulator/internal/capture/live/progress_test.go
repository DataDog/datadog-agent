// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package live

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
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
		t.Fatal("capture progress did not arrive")
		return ""
	}
}

func TestCaptureProgressWhileWaitingRecordingAndFinalizing(t *testing.T) {
	lines, ticks := make(progressLines, 16), make(chan time.Time)
	p := newCaptureProgress(lines, 35*time.Minute, ticks)
	if line := progressLine(t, lines); !strings.Contains(line, "initializing local authentication") || !strings.Contains(line, "recording=35m0s") {
		t.Fatal("missing immediate startup progress", line)
	}
	p.phase("waiting for producers", "processes=unavailable")
	progressLine(t, lines)
	at := time.Now()
	ticks <- at
	if line := progressLine(t, lines); !strings.Contains(line, "processes=unavailable") || strings.Contains(line, "remaining=") {
		t.Fatal("readiness wait falsely claimed recording", line)
	}
	p.armed(at)
	p.samples("metrics=3 processes=2 software=0", "software: 0/1 cycles, effective cadence 10m0s", 3<<19)
	p.phase("recording", "")
	progressLine(t, lines)
	ticks <- at.Add(time.Minute)
	line := progressLine(t, lines)
	for _, want := range []string{"1m0s/35m0s", "remaining=34m0s", "recording", "metrics=3 processes=2 software=0", "sample data=1.5 MiB", "waiting for: software: 0/1"} {
		if !strings.Contains(line, want) {
			t.Fatalf("missing %q in %s", want, line)
		}
	}
	p.samples("metrics=4 processes=3 software=1", "", 2<<20)
	ticks <- at.Add(2 * time.Minute)
	if line := progressLine(t, lines); !strings.Contains(line, "coverage=complete") || !strings.Contains(line, "remaining=33m0s") || !strings.Contains(line, "recording") {
		t.Fatal("coverage must not imply recording has stopped", line)
	}
	p.phase("finalizing bundle", "")
	progressLine(t, lines)
	ticks <- at.Add(36 * time.Minute)
	if line := progressLine(t, lines); !strings.Contains(line, "35m0s/35m0s | remaining=0s | finalizing bundle") {
		t.Fatal("finalization progress lost or recording overcounted", line)
	}
	p.finish("/capture/example", nil)
	if line := progressLine(t, lines); !strings.Contains(line, "complete") || !strings.Contains(line, "bundle=/capture/example") {
		t.Fatal("missing successful completion and output path", line)
	}
}

type blockedProgressWriter struct {
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (w *blockedProgressWriter) Write(data []byte) (int, error) {
	w.once.Do(func() { close(w.entered) })
	<-w.release
	return len(data), nil
}

func TestBlockedCaptureOutputCannotBlockSnapshotUpdates(t *testing.T) {
	w := &blockedProgressWriter{entered: make(chan struct{}), release: make(chan struct{})}
	p := newCaptureProgress(w, time.Minute, make(chan time.Time))
	<-w.entered
	defer func() { close(w.release); p.finish("", errors.New("cancelled")) }()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 100 {
			p.phase("recording", "")
			p.phase("finalizing bundle", "")
			p.samples("processes=10", "", 1000)
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("blocked terminal stopped capture bookkeeping")
	}
	if got := p.snapshot(); got.bytes != 1000 || got.cycles != "processes=10" {
		t.Fatal("blocked output lost the latest snapshot")
	}
}

func TestFailedCaptureProgressDoesNotPrintErrorContentsOrSuccess(t *testing.T) {
	var output bytes.Buffer
	p := newCaptureProgress(&output, time.Minute, make(chan time.Time))
	p.finish("/capture/example", errors.New("private-payload-sentinel"))
	if got := output.String(); !strings.Contains(got, "failed") || !strings.Contains(got, "bundle incomplete") || strings.Contains(got, "private-payload-sentinel") || strings.Contains(got, "bundle=/capture/example") {
		t.Fatal("failed progress leaked error contents or claimed success", got)
	}
}

func TestEvidenceProgressCountsCompleteGroupsAndPersistedBytes(t *testing.T) {
	f := newEvidenceFixture(t, "windows")
	p := &captureProgress{}
	f.e.progress = p
	f.e.publishProgress()
	if s := p.snapshot(); !strings.Contains(s.cycles, "processes=0") || !strings.Contains(s.missing, "metrics/cpu: 0/2") || s.bytes != 0 {
		t.Fatal("initial progress lacks required streams and family coverage")
	}
	for _, stream := range []tc.Stream{tc.Metrics, tc.Processes, tc.Connections} {
		f.accept(context.Background(), t, f.record(t, stream, time.Second))
	}
	var size int64
	files, err := os.ReadDir(f.e.directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		info, err := os.Stat(filepath.Join(f.e.directory, file.Name()))
		if err != nil {
			t.Fatal(err)
		}
		size += info.Size()
	}
	s := p.snapshot()
	if !strings.Contains(s.cycles, "processes=1") || !strings.Contains(s.cycles, "connections=1") || s.bytes != size || size == 0 || len(files) < 4 {
		t.Fatal("progress counted chunks as cycles or lost persisted bytes")
	}
	if strings.Contains(s.cycles+s.missing, evidenceHost) || strings.Contains(s.cycles+s.missing, f.session.ID) {
		t.Fatal("telemetry or session identity escaped into progress")
	}
}
