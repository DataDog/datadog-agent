// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package engine

import (
	"context"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/bundle"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
)

func TestCapturedOffsetsChunksAndPartialFinalCycle(t *testing.T) {
	b := &bundle.Loaded{Manifest: bundle.Manifest{
		Samples: []bundle.SampleRef{
			{Stream: schema.Processes, Offset: 13 * time.Second, File: "second"},
			{Stream: schema.Processes, Offset: 2 * time.Second, File: "first-a"},
			{Stream: schema.Processes, Offset: 2 * time.Second, File: "first-b"},
		},
		Cadences: map[schema.Stream]time.Duration{schema.Processes: 10 * time.Second},
	}}
	timeline, err := makeTimeline(b, schema.Processes, 34*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	// The native 2s/13s offsets repeat after 21s, without emitting a cycle
	// exactly at the scenario end. Chunks at one offset stay one collection.
	want := []time.Duration{2 * time.Second, 13 * time.Second, 23 * time.Second}
	if timeline.count != int64(len(want)) {
		t.Fatalf("count %d, want %d", timeline.count, len(want))
	}
	for i, expected := range want {
		cycle, offset := timeline.at(int64(i))
		if offset != expected {
			t.Fatalf("offset %s, want %s", offset, expected)
		}
		if i%2 == 0 && len(cycle.refs) != 2 {
			t.Fatal("split process chunks")
		}
	}
	for _, check := range []struct {
		offset  time.Duration
		ordinal int64
	}{{0, 0}, {12 * time.Second, 0}, {13 * time.Second, 1}, {22 * time.Second, 1}, {23 * time.Second, 2}} {
		_, ordinal := timeline.nearest(check.offset)
		if ordinal != check.ordinal {
			t.Fatalf("nearest(%s)=%d, want %d", check.offset, ordinal, check.ordinal)
		}
	}
	if _, err := makeTimeline(b, schema.Processes, time.Second); err == nil {
		t.Fatal("accepted scenario ending before first process cycle")
	}
}

func TestWallClockCancellationDoesNotWaitForFutureStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (WallClock{}).WaitUntil(ctx, time.Now().Add(time.Hour)); err != context.Canceled {
		t.Fatalf("got %v", err)
	}
}
