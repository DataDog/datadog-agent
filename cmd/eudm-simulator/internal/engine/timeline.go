// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package engine

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"time"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/bundle"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

type cycle struct {
	producerID string
	cycleID    uint64
	offset     time.Duration
	refs       []bundle.SampleRef
}
type timeline struct {
	cycles []cycle
	period time.Duration
	count  int64
	family string
}

func makeTimeline(b *bundle.Loaded, stream schema.Stream, duration time.Duration) (*timeline, error) {
	if b == nil || duration <= 0 {
		return nil, errors.New("invalid captured timeline or replay duration")
	}
	type cycleKey struct {
		producerID string
		cycleID    uint64
	}
	type sequenceKey struct {
		producerID string
		sequence   uint64
	}
	groups := make(map[cycleKey]int)
	sequences := make(map[sequenceKey]cycleKey)
	t := &timeline{}
	for _, ref := range b.Manifest.Samples {
		if ref.Stream != stream {
			continue
		}
		if ref.ProducerID == "" || ref.CycleID == 0 || ref.Sequence == 0 || ref.Offset < 0 ||
			ref.ChunkCount <= 0 || ref.ChunkIndex < 0 || ref.ChunkIndex >= ref.ChunkCount {
			return nil, errors.New("invalid captured cycle identity, offset, or chunk")
		}
		key := cycleKey{ref.ProducerID, ref.CycleID}
		sequence := sequenceKey{ref.ProducerID, ref.Sequence}
		if previous, ok := sequences[sequence]; ok && previous != key {
			return nil, errors.New("captured sequence belongs to multiple cycles")
		}
		sequences[sequence] = key
		index, exists := groups[key]
		if !exists {
			index = len(t.cycles)
			groups[key] = index
			t.cycles = append(t.cycles, cycle{producerID: ref.ProducerID, cycleID: ref.CycleID, offset: ref.Offset})
		} else {
			first := t.cycles[index].refs[0]
			if ref.Offset != first.Offset || ref.Sequence != first.Sequence || ref.ChunkCount != first.ChunkCount {
				return nil, errors.New("inconsistent captured cycle group")
			}
		}
		t.cycles[index].refs = append(t.cycles[index].refs, ref)
	}
	for i := range t.cycles {
		refs := t.cycles[i].refs
		if len(refs) != refs[0].ChunkCount {
			return nil, errors.New("incomplete captured cycle group")
		}
		sort.Slice(refs, func(i, j int) bool { return refs[i].ChunkIndex < refs[j].ChunkIndex })
		for index, ref := range refs {
			if ref.ChunkIndex != index {
				return nil, errors.New("duplicate or missing captured cycle chunk")
			}
		}
	}
	sort.Slice(t.cycles, func(i, j int) bool {
		first, second := t.cycles[i], t.cycles[j]
		if first.offset != second.offset {
			return first.offset < second.offset
		}
		if first.producerID != second.producerID {
			return first.producerID < second.producerID
		}
		return first.cycleID < second.cycleID
	})
	if len(t.cycles) == 0 {
		return nil, fmt.Errorf("bundle lacks %s cycles", stream)
	}
	if err := t.finish(b.Manifest.Cadences[stream], duration, string(stream)); err != nil {
		return nil, err
	}
	return t, nil
}

// makeTimelines keeps each metric check family's observed gaps and repeat
// cadence independent of other checks sharing the serializer flush.
func makeTimelines(b *bundle.Loaded, stream schema.Stream, duration time.Duration) ([]*timeline, error) {
	all, err := makeTimeline(b, stream, duration)
	if err != nil {
		return nil, err
	}
	if stream != schema.Metrics {
		return []*timeline{all}, nil
	}
	families := map[string]*timeline{}
	for _, c := range all.cycles {
		refs := map[string][]bundle.SampleRef{}
		for _, ref := range c.refs {
			sample, err := decodeCapturedSample(b, ref)
			if err != nil {
				return nil, err
			}
			present := map[string]bool{}
			for _, serie := range sample.Metrics {
				family := telemetrycapture.MetricFamily(serie.Name)
				if family == "" || b.Manifest.MetricCadences[family] <= 0 {
					return nil, errors.New("captured metric lacks a supported family cadence")
				}
				present[family] = true
			}
			for family := range present {
				refs[family] = append(refs[family], ref)
			}
		}
		for family, selected := range refs {
			if families[family] == nil {
				families[family] = &timeline{family: family}
			}
			filtered := c
			filtered.refs = selected
			families[family].cycles = append(families[family].cycles, filtered)
		}
	}
	if len(families) != len(b.Manifest.MetricCadences) {
		return nil, errors.New("metric family cadence has no observed collection")
	}
	result := make([]*timeline, 0, len(families))
	for family, t := range families {
		if err := t.finish(b.Manifest.MetricCadences[family], duration, "metrics/"+family); err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	slices.SortFunc(result, func(a, b *timeline) int {
		if a.family < b.family {
			return -1
		}
		if a.family > b.family {
			return 1
		}
		return 0
	})
	return result, nil
}

func (t *timeline) finish(cadence, duration time.Duration, name string) error {
	span := t.cycles[len(t.cycles)-1].offset - t.cycles[0].offset
	if cadence <= 0 || span > time.Duration(math.MaxInt64)-cadence {
		return fmt.Errorf("invalid %s capture cadence", name)
	}
	t.period = span + cadence
	for _, c := range t.cycles {
		if c.offset < duration {
			count := 1 + int64((duration-1-c.offset)/t.period)
			if t.count > math.MaxInt64-count {
				return errors.New("captured timeline cycle count overflows")
			}
			t.count += count
		}
	}
	if t.count == 0 {
		return fmt.Errorf("scenario ends before the first captured %s cycle; extend its duration", name)
	}
	return nil
}

func (t *timeline) at(ordinal int64) (cycle, time.Duration) {
	n := int64(len(t.cycles))
	c := t.cycles[ordinal%n]
	return c, c.offset + time.Duration(ordinal/n)*t.period
}

// nearest locates the process baseline and stable cycle key shared with a host
// metric collected at the same point on the phase clock. Before the first
// process collection the first captured cycle supplies the baseline. At equal
// offsets, the last cycle in the deterministic timeline order is selected.
func (t *timeline) nearest(offset time.Duration) (cycle, int64) {
	if offset < t.cycles[0].offset {
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
