// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lookbackimpl

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ns(sec int64) int64 { return sec * int64(time.Second) }

func resolve(key uint64) (string, []string, bool) {
	switch key {
	case 1:
		return "m1", []string{"env:prod"}, true
	case 2:
		return "m2", []string{"env:staging"}, true
	}
	return "", nil, false
}

func TestAggregateRecords1sInterval(t *testing.T) {
	recs := []record{
		{contextKey: 1, tsNs: ns(100) + 1, value: 1.0},
		{contextKey: 1, tsNs: ns(100) + 500_000_000, value: 2.0},
		{contextKey: 1, tsNs: ns(101), value: 3.0},
		{contextKey: 1, tsNs: ns(102), value: 4.0},
	}
	ks := map[uint64]struct{}{1: {}}
	buckets := aggregateRecords(recs, ks, ns(100), ns(103), int64(time.Second), resolve)
	require.Len(t, buckets, 3)

	b0 := buckets[0]
	assert.Equal(t, ns(100), b0.Ts)
	assert.Equal(t, int64(2), b0.Count)
	assert.InDelta(t, 3.0, b0.Sum, 1e-9)
	assert.Equal(t, 1.0, b0.Min)
	assert.Equal(t, 2.0, b0.Max)

	assert.Equal(t, ns(101), buckets[1].Ts)
	assert.Equal(t, ns(102), buckets[2].Ts)
}

func TestAggregateRecords5sInterval(t *testing.T) {
	recs := []record{
		{contextKey: 1, tsNs: ns(100), value: 1.0},
		{contextKey: 1, tsNs: ns(101), value: 2.0},
		{contextKey: 1, tsNs: ns(102), value: 3.0},
		{contextKey: 1, tsNs: ns(103), value: 4.0},
		{contextKey: 1, tsNs: ns(104), value: 5.0},
	}
	ks := map[uint64]struct{}{1: {}}
	buckets := aggregateRecords(recs, ks, ns(100), ns(105), 5*int64(time.Second), resolve)
	// All 5 records fall in the same 5s bucket starting at ns(100).
	require.Len(t, buckets, 1)
	assert.Equal(t, ns(100), buckets[0].Ts)
	assert.Equal(t, int64(5), buckets[0].Count)
	assert.InDelta(t, 15.0, buckets[0].Sum, 1e-9)
}

func TestAggregateRecordsRangeBoundary(t *testing.T) {
	recs := []record{
		{contextKey: 1, tsNs: ns(99), value: 99.0},  // before start — excluded
		{contextKey: 1, tsNs: ns(100), value: 1.0},
		{contextKey: 1, tsNs: ns(102), value: 2.0},
		{contextKey: 1, tsNs: ns(103), value: 103.0}, // at stop — excluded (half-open)
	}
	ks := map[uint64]struct{}{1: {}}
	buckets := aggregateRecords(recs, ks, ns(100), ns(103), int64(time.Second), resolve)
	require.Len(t, buckets, 2)
	assert.Equal(t, ns(100), buckets[0].Ts)
	assert.Equal(t, ns(102), buckets[1].Ts)
}

func TestAggregateRecordsMultipleKeys(t *testing.T) {
	recs := []record{
		{contextKey: 1, tsNs: ns(100), value: 1.0},
		{contextKey: 2, tsNs: ns(100), value: 2.0},
	}
	ks := map[uint64]struct{}{1: {}, 2: {}}
	buckets := aggregateRecords(recs, ks, ns(100), ns(101), int64(time.Second), resolve)
	require.Len(t, buckets, 2)
	names := []string{buckets[0].Name, buckets[1].Name}
	assert.Contains(t, names, "m1")
	assert.Contains(t, names, "m2")
}

func TestAggregateRecordsEmpty(t *testing.T) {
	buckets := aggregateRecords(nil, map[uint64]struct{}{1: {}}, ns(100), ns(200), int64(time.Second), resolve)
	assert.Nil(t, buckets)
}

func TestAggregateRecordsContextKey0Path(t *testing.T) {
	synKey := syntheticKey("synthetic", sortedTagsCopy([]string{"env:test"}))
	entries := map[uint64]contextEntry{synKey: {name: "synthetic", tags: []string{"env:test"}}}
	resolveEntries := func(k uint64) (string, []string, bool) {
		e, ok := entries[k]
		return e.name, e.tags, ok
	}

	recs := []record{
		{contextKey: synKey, tsNs: ns(100), value: 7.0},
		{contextKey: synKey, tsNs: ns(100) + 1, value: 3.0},
	}
	ks := map[uint64]struct{}{synKey: {}}
	buckets := aggregateRecords(recs, ks, ns(100), ns(101), int64(time.Second), resolveEntries)
	require.Len(t, buckets, 1)
	assert.Equal(t, "synthetic", buckets[0].Name)
	assert.Equal(t, int64(2), buckets[0].Count)
	assert.InDelta(t, 10.0, buckets[0].Sum, 1e-9)
}

func TestAggregateRecordsDefaultInterval(t *testing.T) {
	recs := []record{{contextKey: 1, tsNs: ns(5), value: 1.0}}
	ks := map[uint64]struct{}{1: {}}
	// intervalNs = 0 should default to 1s.
	buckets := aggregateRecords(recs, ks, ns(5), ns(6), 0, resolve)
	require.Len(t, buckets, 1)
	assert.Equal(t, ns(5), buckets[0].Ts)
}

func TestGetEntryAsResolver(t *testing.T) {
	entries := map[uint64]contextEntry{99: {name: "foo", tags: []string{"a:b"}}}
	resolveEntries := func(k uint64) (string, []string, bool) {
		e, ok := entries[k]
		return e.name, e.tags, ok
	}

	buckets := aggregateRecords(
		[]record{{contextKey: 99, tsNs: ns(1), value: 42}},
		map[uint64]struct{}{99: {}},
		ns(1), ns(2), int64(time.Second),
		resolveEntries,
	)
	require.Len(t, buckets, 1)
	assert.Equal(t, "foo", buckets[0].Name)
	assert.InDelta(t, 42.0, buckets[0].Sum, 1e-9)
}
