// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package segments

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func TestMilestone7SegmentRowsRoundTripSemantically(t *testing.T) {
	rows := []Row{
		{
			Type:           metrics.GaugeType,
			Name:           "system.cpu.user",
			Tags:           []string{"env:prod", "region:us-east-1"},
			Resources:      []metrics.Resource{{Type: "host", Name: "i-123"}},
			SourceTypeName: "System",
			Origin:         "origin-a",
			Unit:           "percent",
			Timestamp:      100,
			Value:          1.5,
		},
		{
			Type:           metrics.CountType,
			Name:           "custom.counter",
			Tags:           []string{"env:prod"},
			Resources:      []metrics.Resource{{Type: "host", Name: "i-123"}, {Type: "container", Name: "abc"}},
			SourceTypeName: "DogStatsD",
			Origin:         "origin-b",
			Unit:           "request",
			Timestamp:      101,
			Value:          2,
			NoIndex:        true,
		},
	}

	builder := NewBuilder(Options{MaxRows: 10})
	for _, row := range rows {
		require.NoError(t, builder.Add(row))
	}
	segment := builder.Seal()

	assert.Equal(t, rows, segment.Rows())
}

func TestMilestone7SegmentInternsPayloadAlignedDictionaries(t *testing.T) {
	builder := NewBuilder(Options{MaxRows: 10})
	for i := 0; i < 3; i++ {
		require.NoError(t, builder.Add(Row{
			Type:           metrics.GaugeType,
			Name:           "shared.metric",
			Tags:           []string{"env:prod", "team:agent"},
			Resources:      []metrics.Resource{{Type: "host", Name: "i-123"}},
			SourceTypeName: "DogStatsD",
			Origin:         "origin-a",
			Unit:           "count",
			Timestamp:      int64(100 + i),
			Value:          float64(i),
		}))
	}

	segment := builder.Seal()
	stats := segment.Stats()
	assert.Equal(t, 3, stats.Rows)
	assert.Equal(t, 1, stats.Names)
	assert.Equal(t, 2, stats.TagStrings)
	assert.Equal(t, 1, stats.Tagsets)
	assert.Equal(t, 2, stats.ResourceStrings)
	assert.Equal(t, 1, stats.ResourceSets)
	assert.Equal(t, 1, stats.SourceTypes)
	assert.Equal(t, 1, stats.Origins)
	assert.Equal(t, 1, stats.Units)

	assert.Equal(t, segment.NameRef(0), segment.NameRef(1))
	assert.Equal(t, segment.TagsetRef(0), segment.TagsetRef(1))
	assert.Equal(t, segment.OriginRef(0), segment.OriginRef(1))
}

func TestMilestone7SegmentUsesPayloadLocalDictionaryRefs(t *testing.T) {
	first := buildSingleRowSegment(t, "first.metric", "origin-first")
	second := buildSingleRowSegment(t, "second.metric", "origin-second")

	assert.Equal(t, uint32(0), first.NameRef(0))
	assert.Equal(t, uint32(0), first.OriginRef(0))
	assert.Equal(t, []string{"first.metric"}, first.NameDictionary)
	assert.Equal(t, []string{"origin-first"}, first.OriginDictionary)

	assert.Equal(t, uint32(0), second.NameRef(0))
	assert.Equal(t, uint32(0), second.OriginRef(0))
	assert.Equal(t, []string{"second.metric"}, second.NameDictionary)
	assert.Equal(t, []string{"origin-second"}, second.OriginDictionary)
}

func TestMilestone7SegmentEnforcesRowBudget(t *testing.T) {
	builder := NewBuilder(Options{MaxRows: 1})
	require.NoError(t, builder.Add(Row{Name: "first"}))
	err := builder.Add(Row{Name: "second"})
	assert.True(t, errors.Is(err, ErrSegmentFull))

	segment := builder.Seal()
	assert.Equal(t, 1, segment.Stats().Rows)
	assert.Equal(t, []Row{{Name: "first", Tags: []string{}, Resources: []metrics.Resource{}}}, segment.Rows())
}

func BenchmarkMilestone7SegmentBuildSeal(b *testing.B) {
	rows := make([]Row, 1024)
	for i := range rows {
		rows[i] = Row{
			Type:           metrics.GaugeType,
			Name:           fmt.Sprintf("metric.%d", i%64),
			Tags:           []string{fmt.Sprintf("env:%d", i%4), fmt.Sprintf("team:%d", i%16)},
			Resources:      []metrics.Resource{{Type: "host", Name: fmt.Sprintf("host-%d", i%128)}},
			SourceTypeName: "DogStatsD",
			Origin:         fmt.Sprintf("origin-%d", i%32),
			Unit:           "count",
			Timestamp:      int64(100 + i),
			Value:          float64(i),
		}
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		builder := NewBuilder(Options{MaxRows: len(rows)})
		for _, row := range rows {
			_ = builder.Add(row)
		}
		_ = builder.Seal()
	}
}

func buildSingleRowSegment(t *testing.T, name string, origin string) Segment {
	builder := NewBuilder(Options{MaxRows: 1})
	require.NoError(t, builder.Add(Row{Name: name, Origin: origin}))
	return builder.Seal()
}
