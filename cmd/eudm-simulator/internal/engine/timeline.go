// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package engine

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/bundle"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
)

type cycle struct {
	offset time.Duration
	refs   []bundle.SampleRef
}
type timeline struct {
	cycles []cycle
	period time.Duration
	count  int64
}

func makeTimeline(b *bundle.Loaded, stream schema.Stream, duration time.Duration) (*timeline, error) {
	var refs []bundle.SampleRef
	for _, ref := range b.Manifest.Samples {
		if ref.Stream == stream {
			refs = append(refs, ref)
		}
	}
	sort.SliceStable(refs, func(i, j int) bool { return refs[i].Offset < refs[j].Offset })
	t := &timeline{}
	for _, ref := range refs {
		if len(t.cycles) == 0 || t.cycles[len(t.cycles)-1].offset != ref.Offset {
			t.cycles = append(t.cycles, cycle{offset: ref.Offset})
		}
		t.cycles[len(t.cycles)-1].refs = append(t.cycles[len(t.cycles)-1].refs, ref)
	}
	if len(t.cycles) == 0 {
		return nil, fmt.Errorf("bundle lacks %s cycles", stream)
	}
	span := t.cycles[len(t.cycles)-1].offset - t.cycles[0].offset
	cadence := b.Manifest.Cadences[stream]
	if cadence <= 0 || span > time.Duration(math.MaxInt64)-cadence {
		return nil, fmt.Errorf("invalid %s capture cadence", stream)
	}
	t.period = span + cadence
	for _, c := range t.cycles {
		if c.offset < 0 {
			return nil, errors.New("invalid captured cycle offset")
		}
		if c.offset < duration {
			t.count += 1 + int64((duration-1-c.offset)/t.period)
		}
	}
	if t.count == 0 {
		return nil, fmt.Errorf("scenario ends before the first captured %s cycle; extend its duration", stream)
	}
	return t, nil
}

func (t *timeline) at(ordinal int64) (cycle, time.Duration) {
	n := int64(len(t.cycles))
	c := t.cycles[ordinal%n]
	return c, c.offset + time.Duration(ordinal/n)*t.period
}

// nearest locates the process baseline and stable cycle key shared with a host
// metric collected at the same point on the phase clock. Before the first
// process collection the first captured cycle supplies the baseline.
func (t *timeline) nearest(offset time.Duration) (cycle, int64) {
	if offset <= t.cycles[0].offset {
		return t.cycles[0], 0
	}
	repeat := int64((offset - t.cycles[0].offset) / t.period)
	relative := offset - time.Duration(repeat)*t.period
	i := sort.Search(len(t.cycles), func(i int) bool { return t.cycles[i].offset > relative }) - 1
	if i < 0 {
		i = 0
	}
	return t.cycles[i], repeat*int64(len(t.cycles)) + int64(i)
}

func phaseAt(s *schema.Scenario, offset time.Duration) (int, time.Duration) {
	for i, p := range s.Phases {
		if offset < p.Duration.Duration {
			return i, offset
		}
		offset -= p.Duration.Duration
	}
	return len(s.Phases) - 1, s.Phases[len(s.Phases)-1].Duration.Duration
}

func durationOf(s *schema.Scenario) time.Duration {
	var d time.Duration
	for _, p := range s.Phases {
		d += p.Duration.Duration
	}
	return d
}
