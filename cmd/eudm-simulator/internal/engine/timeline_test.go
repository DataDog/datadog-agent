// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package engine

import (
	"context"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/bundle"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
)

func TestCapturedOffsetsAndChunksAreEmittedOnlyOnce(t *testing.T) {
	b := &bundle.Loaded{Manifest: bundle.Manifest{
		Samples: []bundle.SampleRef{
			{Stream: schema.Processes, ProducerID: "process-a", CycleID: 2, Sequence: 2, ChunkCount: 1, Offset: 13 * time.Second, File: "second"},
			{Stream: schema.Processes, ProducerID: "process-a", CycleID: 1, Sequence: 1, ChunkCount: 2, ChunkIndex: 1, Offset: 2 * time.Second, File: "first-b"},
			{Stream: schema.Processes, ProducerID: "process-a", CycleID: 1, Sequence: 1, ChunkCount: 2, ChunkIndex: 0, Offset: 2 * time.Second, File: "first-a"},
		},
		Cadences: map[schema.Stream]time.Duration{schema.Processes: 10 * time.Second},
	}}
	timeline, err := makeTimeline(b, schema.Processes, 34*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	// An idle tail does not manufacture extra collections. Explicit group
	// identity keeps chunks together and preserves their original offsets.
	want := []time.Duration{2 * time.Second, 13 * time.Second}
	if timeline.count != int64(len(want)) {
		t.Fatalf("count %d, want %d", timeline.count, len(want))
	}
	for i, expected := range want {
		cycle, offset := timeline.at(int64(i))
		if offset != expected {
			t.Fatalf("offset %s, want %s", offset, expected)
		}
		if i == 0 && len(cycle.refs) != 2 {
			t.Fatal("split process chunks")
		}
		if i == 0 && (cycle.refs[0].File != "first-a" || cycle.refs[1].File != "first-b") {
			t.Fatal("did not restore declared chunk order")
		}
	}
	for _, check := range []struct {
		offset  time.Duration
		ordinal int64
	}{{0, 0}, {12 * time.Second, 0}, {13 * time.Second, 1}, {22 * time.Second, 1}, {time.Hour, 1}} {
		_, ordinal := timeline.nearest(check.offset)
		if ordinal != check.ordinal {
			t.Fatalf("nearest(%s)=%d, want %d", check.offset, ordinal, check.ordinal)
		}
	}
	if _, err := makeTimeline(b, schema.Processes, time.Second); err == nil {
		t.Fatal("accepted scenario ending before first process cycle")
	}
	for _, duration := range []time.Duration{3 * time.Second, 13 * time.Second} {
		cropped, err := makeTimeline(b, schema.Processes, duration)
		if err != nil || cropped.count != 1 {
			t.Fatalf("scenario boundary %s must exclude the final cycle: count=%v, error=%v", duration, cropped, err)
		}
	}
	inclusive, err := makeTimeline(b, schema.Processes, 13*time.Second+1)
	if err != nil || inclusive.count != 2 {
		t.Fatalf("cycle before scenario end was cropped: %v", err)
	}
}

func TestExplicitCycleIdentitiesRemainDistinctAtEqualOffsets(t *testing.T) {
	b := &bundle.Loaded{Manifest: bundle.Manifest{
		Samples: []bundle.SampleRef{
			{Stream: schema.Processes, ProducerID: "process-b", CycleID: 1, Sequence: 1, ChunkCount: 1, Offset: 2 * time.Second, File: "b-1"},
			{Stream: schema.Processes, ProducerID: "process-a", CycleID: 2, Sequence: 2, ChunkCount: 1, Offset: 2 * time.Second, File: "a-2"},
			{Stream: schema.Processes, ProducerID: "process-a", CycleID: 1, Sequence: 1, ChunkCount: 2, ChunkIndex: 1, Offset: 2 * time.Second, File: "a-1-last"},
			{Stream: schema.Processes, ProducerID: "process-a", CycleID: 1, Sequence: 1, ChunkCount: 2, Offset: 2 * time.Second, File: "a-1-first"},
		},
		Cadences: map[schema.Stream]time.Duration{schema.Processes: 10 * time.Second},
	}}
	timeline, err := makeTimeline(b, schema.Processes, 23*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(timeline.cycles) != 3 || timeline.count != 3 {
		t.Fatalf("distinct producer/cycle identities collapsed: cycles=%d count=%d", len(timeline.cycles), timeline.count)
	}
	if got := []string{timeline.cycles[0].refs[0].File, timeline.cycles[0].refs[1].File, timeline.cycles[1].refs[0].File, timeline.cycles[2].refs[0].File}; !reflect.DeepEqual(got, []string{"a-1-first", "a-1-last", "a-2", "b-1"}) {
		t.Fatalf("unexpected tie or chunk ordering: %v", got)
	}
	for ordinal := int64(0); ordinal < timeline.count; ordinal++ {
		c, offset := timeline.at(ordinal)
		if offset != 2*time.Second || c.cycleID != []uint64{1, 2, 1}[ordinal] {
			t.Fatalf("wrong cycle at ordinal %d: %+v at %s", ordinal, c, offset)
		}
	}
	for _, check := range []struct {
		offset  time.Duration
		ordinal int64
	}{{0, 0}, {2 * time.Second, 2}, {11 * time.Second, 2}, {time.Hour, 2}} {
		_, ordinal := timeline.nearest(check.offset)
		if ordinal != check.ordinal {
			t.Fatalf("nearest(%s)=%d, want %d", check.offset, ordinal, check.ordinal)
		}
	}
}

func TestTimelineRejectsInvalidCycleGroups(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*bundle.Manifest)
	}{
		{"missing producer", func(m *bundle.Manifest) { m.Samples[0].ProducerID = "" }},
		{"missing cycle", func(m *bundle.Manifest) { m.Samples[0].CycleID = 0 }},
		{"missing sequence", func(m *bundle.Manifest) { m.Samples[0].Sequence = 0 }},
		{"negative offset", func(m *bundle.Manifest) { m.Samples[0].Offset = -1 }},
		{"negative chunk", func(m *bundle.Manifest) { m.Samples[0].ChunkIndex = -1 }},
		{"zero chunks", func(m *bundle.Manifest) { m.Samples[0].ChunkCount = 0 }},
		{"out of range chunk", func(m *bundle.Manifest) { m.Samples[0].ChunkIndex = 2 }},
		{"mixed offsets", func(m *bundle.Manifest) { m.Samples[1].Offset++ }},
		{"mixed sequences", func(m *bundle.Manifest) { m.Samples[1].Sequence++ }},
		{"mixed chunk declarations", func(m *bundle.Manifest) { m.Samples[1].ChunkCount++ }},
		{"duplicate chunk", func(m *bundle.Manifest) { m.Samples[1].ChunkIndex = 0 }},
		{"missing chunk", func(m *bundle.Manifest) { m.Samples = m.Samples[:1] }},
		{"shared sequence", func(m *bundle.Manifest) {
			m.Samples[1].CycleID++
			for i := range m.Samples {
				m.Samples[i].ChunkCount, m.Samples[i].ChunkIndex = 1, 0
			}
		}},
		{"missing cadence", func(m *bundle.Manifest) { m.Cadences = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := bundle.Manifest{
				Samples: []bundle.SampleRef{
					{Stream: schema.Processes, ProducerID: "process-a", CycleID: 1, Sequence: 1, ChunkCount: 2, File: "first"},
					{Stream: schema.Processes, ProducerID: "process-a", CycleID: 1, Sequence: 1, ChunkCount: 2, ChunkIndex: 1, File: "second"},
				},
				Cadences: map[schema.Stream]time.Duration{schema.Processes: time.Second},
			}
			tc.mutate(&m)
			if _, err := makeTimeline(&bundle.Loaded{Manifest: m}, schema.Processes, time.Minute); err == nil {
				t.Fatal("accepted invalid captured cycle group")
			}
		})
	}
}

func TestTimelineRejectsInvalidDurationWithoutExtrapolating(t *testing.T) {
	b := &bundle.Loaded{Manifest: bundle.Manifest{
		Samples: []bundle.SampleRef{
			{Stream: schema.Processes, ProducerID: "process-a", CycleID: 1, Sequence: 1, ChunkCount: 1, File: "first"},
			{Stream: schema.Processes, ProducerID: "process-a", CycleID: 2, Sequence: 2, ChunkCount: 1, File: "second"},
		},
		Cadences: map[schema.Stream]time.Duration{schema.Processes: 1},
	}}
	for _, duration := range []time.Duration{0, -1} {
		if _, err := makeTimeline(b, schema.Processes, duration); err == nil {
			t.Fatalf("accepted invalid timeline duration %d", duration)
		}
	}
	b.Manifest.Samples[1].Offset = math.MaxInt64 - 1
	if timeline, err := makeTimeline(b, schema.Processes, math.MaxInt64); err != nil || timeline.count != 2 {
		t.Fatalf("long recording must retain exactly its real cycles: %v", err)
	}
	if _, err := makeTimeline(nil, schema.Processes, time.Minute); err == nil {
		t.Fatal("accepted missing timeline bundle")
	}
}

func TestWallClockCancellationDoesNotWaitForFutureStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (WallClock{}).WaitUntil(ctx, time.Now().Add(time.Hour)); err != context.Canceled {
		t.Fatalf("got %v", err)
	}
}
