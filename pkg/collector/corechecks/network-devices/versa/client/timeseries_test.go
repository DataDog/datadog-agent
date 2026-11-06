// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupByFieldCount(t *testing.T) {
	tests := []struct {
		query    string
		expected int
		hasError bool
	}{
		{query: "slam(localsite,remotesite,localaccckt,remoteaccckt,fc)", expected: 5},
		{query: "pathcos(localsitename,remotesitename)", expected: 2},
		{query: "cos(site,accckt)", expected: 2},
		{query: "linkusage(site)", expected: 1},
		{query: "noargs()", expected: 0},
		{query: "malformed", hasError: true},
		{query: ")backwards(", hasError: true},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			count, err := groupByFieldCount(tt.query)
			if tt.hasError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, count)
		})
	}
}

func TestFlattenTimeseries(t *testing.T) {
	metrics := []string{"delay", "fwdLossRatio"}
	series := []AnalyticsTimeseries{
		{
			Name:   "branch-1,hub-1,INET-1,INET-1,fc_nc",
			Metric: "delay",
			Data: [][]interface{}{
				{float64(1700000900000), float64(101)},
				{float64(1700000000000), float64(99)},
			},
		},
		{
			Name:   "branch-1,hub-1,INET-1,INET-1,fc_nc",
			Metric: "fwdLossRatio",
			Data: [][]interface{}{
				{float64(1700000000000), float64(0)},
				{float64(1700000900000), float64(0.5)},
			},
		},
	}

	rows, dropped := flattenTimeseries(series, metrics, 5)
	require.Equal(t, 0, dropped)
	require.Len(t, rows, 2)

	// Buckets come back oldest first regardless of the order the series listed them in,
	// and each row is laid out exactly like an aaData row.
	assert.Equal(t, int64(1700000000000), rows[0].timestamp)
	assert.Equal(t, []interface{}{
		"branch-1,hub-1,INET-1,INET-1,fc_nc",
		"branch-1", "hub-1", "INET-1", "INET-1", "fc_nc",
		float64(99), float64(0),
	}, rows[0].row)

	assert.Equal(t, int64(1700000900000), rows[1].timestamp)
	assert.Equal(t, []interface{}{
		"branch-1,hub-1,INET-1,INET-1,fc_nc",
		"branch-1", "hub-1", "INET-1", "INET-1", "fc_nc",
		float64(101), float64(0.5),
	}, rows[1].row)
}

func TestFlattenTimeseriesDropsIncompleteBuckets(t *testing.T) {
	metrics := []string{"delay", "fwdLossRatio"}
	series := []AnalyticsTimeseries{
		{
			Name:   "branch-1,hub-1",
			Metric: "delay",
			Data: [][]interface{}{
				{float64(1700000000000), float64(99)},
				{float64(1700000900000), float64(101)},
			},
		},
		{
			Name:   "branch-1,hub-1",
			Metric: "fwdLossRatio",
			Data: [][]interface{}{
				{float64(1700000000000), float64(0)},
				// Versa reports null for a bucket the entity did not report in. Emitting a
				// row for it would shift the remaining columns, so the bucket is dropped.
				{float64(1700000900000), nil},
			},
		},
	}

	rows, dropped := flattenTimeseries(series, metrics, 2)
	require.Equal(t, 1, dropped)
	require.Len(t, rows, 1)
	assert.Equal(t, int64(1700000000000), rows[0].timestamp)
}

func TestFlattenTimeseriesIgnoresUnrequestedMetrics(t *testing.T) {
	series := []AnalyticsTimeseries{
		{
			Name:   "branch-1,hub-1",
			Metric: "delay",
			Data:   [][]interface{}{{float64(1700000000000), float64(99)}},
		},
		{
			Name:   "branch-1,hub-1",
			Metric: "somethingElse",
			Data:   [][]interface{}{{float64(1700000000000), float64(7)}},
		},
	}

	rows, dropped := flattenTimeseries(series, []string{"delay"}, 2)
	require.Equal(t, 0, dropped)
	require.Len(t, rows, 1)
	assert.Equal(t, []interface{}{"branch-1,hub-1", "branch-1", "hub-1", float64(99)}, rows[0].row)
}

func TestFlattenTimeseriesDropsUnsplittableNames(t *testing.T) {
	series := []AnalyticsTimeseries{
		{
			Name:   "branch-1,hub-1",
			Metric: "delay",
			Data: [][]interface{}{
				{float64(1700000000000), float64(99)},
				{float64(1700000900000), float64(98)},
			},
		},
	}

	// The query grouped by three fields but the drill key only splits into two, so there
	// is no way to tell which value belongs in which column.
	rows, dropped := flattenTimeseries(series, []string{"delay"}, 3)
	assert.Empty(t, rows)
	assert.Equal(t, 2, dropped)
}
