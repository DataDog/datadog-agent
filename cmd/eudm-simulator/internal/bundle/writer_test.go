// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package bundle

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

type cancelWhenFileExistsContext struct {
	context.Context
	cancel context.CancelFunc
	path   string
}

func (c cancelWhenFileExistsContext) Err() error {
	if _, err := os.Stat(c.path); err == nil {
		c.cancel()
	}
	return c.Context.Err()
}

func writerFromFixture(t *testing.T) (*Writer, *Loaded) {
	t.Helper()
	_, source := fixture(t, "macos")
	w, err := NewWriter(filepath.Join(t.TempDir(), "capture"), source.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range source.Manifest.Samples {
		if err := w.Append(ref, json.RawMessage(source.Files[ref.File])); err != nil {
			t.Fatal(err)
		}
	}
	return w, source
}

func TestWriterCancellationRevokesCompletion(t *testing.T) {
	for _, boundary := range []string{"manifest.json", "COMPLETE"} {
		t.Run(boundary, func(t *testing.T) {
			w, source := writerFromFixture(t)
			directory := w.directory
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			// Cancel at a concrete persistence boundary, without sleeping or
			// depending on the speed of filesystem writes and final validation.
			cancellation := cancelWhenFileExistsContext{Context: ctx, cancel: cancel, path: filepath.Join(directory, boundary)}
			loaded, err := w.CompleteContext(cancellation, source.Manifest.Duration, source.Manifest.Profile, source.Manifest.Cadences)
			if !errors.Is(err, context.Canceled) || loaded != nil {
				t.Fatalf("canceled finalization returned success: %v", err)
			}
			if _, err := os.Stat(filepath.Join(directory, "manifest.json")); err != nil {
				t.Fatal("cancellation did not reach its persistence boundary")
			}
			if _, err := os.Stat(filepath.Join(directory, "COMPLETE")); !os.IsNotExist(err) {
				t.Fatal("canceled finalization left a valid completion marker")
			}
			if _, err := Load(directory); err == nil {
				t.Fatal("canceled finalization left a replayable bundle")
			}
		})
	}
}

func TestWriterPreservesExistingCompletionMarker(t *testing.T) {
	w, source := writerFromFixture(t)
	path := filepath.Join(w.directory, "COMPLETE")
	const existing = "pre-existing completion marker"
	if err := os.WriteFile(path, []byte(existing), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := w.CompleteContext(context.Background(), source.Manifest.Duration, source.Manifest.Profile, source.Manifest.Cadences)
	if !errors.Is(err, os.ErrExist) || loaded != nil {
		t.Fatalf("accepted a pre-existing completion marker: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != existing {
		t.Fatal("failed exclusive creation removed or changed another marker")
	}
}

type panicWhenFileExistsContext struct {
	context.Context
	path string
}

func (c panicWhenFileExistsContext) Err() error {
	if _, err := os.Stat(c.path); err == nil {
		panic("must not escape finalization")
	}
	return c.Context.Err()
}

func TestWriterPanicRevokesOwnedCompletion(t *testing.T) {
	w, source := writerFromFixture(t)
	ctx := panicWhenFileExistsContext{Context: context.Background(), path: filepath.Join(w.directory, "COMPLETE")}
	loaded, err := w.CompleteContext(ctx, source.Manifest.Duration, source.Manifest.Profile, source.Manifest.Cadences)
	if err == nil || err.Error() != "bundle finalization failed" || loaded != nil {
		t.Fatalf("finalization panic escaped or exposed its value: %v", err)
	}
	if _, err := os.Stat(ctx.path); !os.IsNotExist(err) {
		t.Fatal("failed finalization left its owned completion marker")
	}
}

func TestWriterRequiresExclusivePrivateOutput(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bundle")
	manifest := Manifest{CaptureTool: BuildIdentity{Version: "7.85.0", Commit: strings.Repeat("a", 40)}, SessionID: "writer-capture-session"}
	w, err := NewWriter(dir, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewWriter(dir, manifest); err == nil {
		t.Fatal("accepted an existing output directory")
	}
	info, err := os.Stat(dir)
	if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0700) {
		t.Fatal("capture directory is not private")
	}
	ref := SampleRef{Stream: schema.Metrics, ProducerID: "writer-core-agent", CycleID: 1, Sequence: 1, ChunkCount: 1}
	if err := w.Append(ref, fixtureSample(t, schema.Metrics, "macos")); err != nil {
		t.Fatal(err)
	}
	if len(w.manifest.Files) != 1 {
		t.Fatal("writer persisted more than the typed sample")
	}
	for name := range w.manifest.Files {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || (runtime.GOOS != "windows" && info.Mode().Perm() != 0600) {
			t.Fatal("capture file is not private")
		}
	}

}

func TestWriterCannotCompleteWithoutStoppedAcknowledgements(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bundle")
	w, err := NewWriter(dir, Manifest{CaptureTool: BuildIdentity{Version: "7.85.0", Commit: strings.Repeat("a", 40)}, SessionID: "writer-capture-session"})
	if err != nil {
		t.Fatal(err)
	}
	producer := Producer{Role: "core-agent", InstanceID: "writer-core-agent", Version: "7.85.0", Commit: strings.Repeat("b", 40), ProtocolVersion: telemetrycapture.ProtocolVersion,
		Streams: []schema.Stream{schema.Metrics}, StopOffset: time.Minute, FinalSequence: 1, AcknowledgedSequence: 1}
	if err := w.SetProducers([]Producer{producer}); err != nil {
		t.Fatal(err)
	}
	producer.Streams[0] = schema.Processes
	if w.manifest.Producers[0].Streams[0] != schema.Metrics {
		t.Fatal("writer borrowed the producer inventory")
	}
	_, err = w.Complete(time.Minute, schema.Profile{OS: "macos", Architecture: "arm64", Streams: []schema.Stream{schema.Metrics}}, map[schema.Stream]time.Duration{schema.Metrics: time.Second})
	if err == nil || !strings.Contains(err.Error(), "stopped") {
		t.Fatalf("missing stopped acknowledgement was accepted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "COMPLETE")); !os.IsNotExist(err) {
		t.Fatal("failed completion left a completion marker")
	}
	if err := w.SetProducers(nil); err == nil {
		t.Fatal("finalized writer accepted later provenance")
	}
}

func TestBundleRejectsSymlinksAndOversizedFiles(t *testing.T) {
	for _, kind := range []string{"symlink", "oversized"} {
		t.Run(kind, func(t *testing.T) {
			dir, loaded := fixture(t, "macos")
			path := filepath.Join(dir, loaded.Manifest.Samples[0].File)
			if kind == "symlink" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(loaded.Manifest.Samples[1].File, path); err != nil {
					if runtime.GOOS == "windows" {
						t.Skip("test runner cannot create Windows symbolic links")
					}
					t.Fatal(err)
				}
			} else if err := os.Truncate(path, maxFileBytes+1); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(dir); err == nil {
				t.Fatal("accepted an unbounded or indirect bundle file")
			}
		})
	}
}

func TestWriterAggregateByteLimitFailsBeforeWritingAndCannotComplete(t *testing.T) {
	sample := fixtureSample(t, schema.Metrics, "macos")
	data, err := json.Marshal(sample)
	if err != nil {
		t.Fatal(err)
	}
	limit := int64(len(data))
	w, err := newWriterWithLimit(filepath.Join(t.TempDir(), "capture"), Manifest{CaptureTool: BuildIdentity{Version: "7.85.0", Commit: strings.Repeat("a", 40)}, SessionID: "writer-budget-session"}, limit)
	if err != nil {
		t.Fatal(err)
	}
	ref := SampleRef{Stream: schema.Metrics, ProducerID: "writer-core-agent", CycleID: 1, Sequence: 1, ChunkCount: 1}
	if err := w.Append(ref, sample); err != nil {
		t.Fatalf("sample exactly at byte limit was rejected: %v", err)
	}
	ref.CycleID, ref.Sequence = 2, 2
	if err := w.Append(ref, sample); err == nil || !strings.Contains(err.Error(), "sample byte limit") {
		t.Fatalf("second sample exceeded aggregate budget without failing: %v", err)
	}
	if w.bytes != limit || len(w.manifest.Files) != 1 || len(w.manifest.Samples) != 1 {
		t.Fatal("rejected sample changed the accepted byte/file inventory")
	}
	if _, err := os.Stat(filepath.Join(w.directory, "sample-000001.json")); !os.IsNotExist(err) {
		t.Fatal("over-budget sample was written")
	}
	if _, err := w.Complete(time.Minute, schema.Profile{}, nil); err == nil {
		t.Fatal("writer could complete after exceeding its byte limit")
	}
	if _, err := os.Stat(filepath.Join(w.directory, "COMPLETE")); !os.IsNotExist(err) {
		t.Fatal("over-budget writer created a completion marker")
	}
}

func TestLoadAggregateByteLimit(t *testing.T) {
	dir, source := fixture(t, "macos")
	var total int64
	for _, data := range source.Files {
		total += int64(len(data))
	}
	if _, err := loadWithLimit(dir, total); err != nil {
		t.Fatalf("bundle exactly at aggregate limit was rejected: %v", err)
	}
	if _, err := loadWithLimit(dir, total-1); err == nil || !strings.Contains(err.Error(), "sample byte limit") {
		t.Fatalf("aggregate budget was not enforced across sample files: %v", err)
	}
	// A sparse file larger than the remaining budget must be rejected from its
	// size before attempting to read it or failing later on its checksum/JSON.
	path := filepath.Join(dir, source.Manifest.Samples[0].File)
	if err := os.Truncate(path, total+1); err != nil {
		t.Fatal(err)
	}
	if _, err := loadWithLimit(dir, total); err == nil || !strings.Contains(err.Error(), "sample byte limit") {
		t.Fatalf("oversized sample was not rejected before loading: %v", err)
	}
}
