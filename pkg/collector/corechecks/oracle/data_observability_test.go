// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/config"
)

func TestDataObservabilityIntervalSchedules(t *testing.T) {
	queries := newDataObservabilityScheduledQueries([]config.DataObservabilityQuery{
		{MonitorID: 0, IntervalSeconds: 30},
		{MonitorID: 0, IntervalSeconds: 60},
	})
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	due := getDueDataObservabilityQueries(queries, now)
	require.Len(t, due, 2)
	assert.Equal(t, now, due[0].scheduledTime)
	assert.Equal(t, now, due[1].scheduledTime)
	due[0].complete(now.Add(5 * time.Second))
	due[1].complete(now.Add(10 * time.Second))

	due = getDueDataObservabilityQueries(queries, now.Add(30*time.Second))
	assert.Empty(t, due)
	due = getDueDataObservabilityQueries(queries, now.Add(35*time.Second))
	require.Len(t, due, 1)
	assert.Same(t, queries[0], due[0].scheduledQuery)
	assert.Equal(t, now.Add(35*time.Second), due[0].scheduledTime)
	due[0].complete(now.Add(40 * time.Second))

	due = getDueDataObservabilityQueries(queries, now.Add(70*time.Second))
	require.Len(t, due, 2)
	for _, query := range due {
		assert.Equal(t, now.Add(70*time.Second), query.scheduledTime)
	}
}

func TestDataObservabilityCronStartup(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name  string
		delay time.Duration
		want  bool
	}{
		{"on boundary", 0, true},
		{"within lookback", 299 * time.Second, true},
		{"at lookback minus offset", 300*time.Second - time.Millisecond, true},
		{"outside lookback with offset", 300 * time.Second, false},
		{"outside lookback", 301 * time.Second, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			queries := newDataObservabilityScheduledQueries([]config.DataObservabilityQuery{{Schedule: "0 * * * *", IntervalSeconds: 1}})
			due := getDueDataObservabilityQueries(queries, now.Add(tt.delay))
			if tt.want {
				require.Len(t, due, 1)
				assert.Equal(t, now, due[0].scheduledTime)
				due[0].complete(now.Add(tt.delay))
			} else {
				assert.Empty(t, due)
			}
			due = getDueDataObservabilityQueries(queries, now.Add(tt.delay+time.Second))
			assert.Empty(t, due)
		})
	}
}

func TestDataObservabilityCronCollapsesMissedTicks(t *testing.T) {
	queries := newDataObservabilityScheduledQueries([]config.DataObservabilityQuery{{Schedule: "* * * * *"}})
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	due := getDueDataObservabilityQueries(queries, now)
	require.Len(t, due, 1)
	due[0].complete(now)
	due = getDueDataObservabilityQueries(queries, now.Add(5*time.Minute+20*time.Second))
	require.Len(t, due, 1)
	assert.Equal(t, now.Add(5*time.Minute), due[0].scheduledTime)
	due = getDueDataObservabilityQueries(queries, now.Add(5*time.Minute+30*time.Second))
	assert.Empty(t, due)
}

func TestDataObservabilityCronUsesUTC(t *testing.T) {
	queries := newDataObservabilityScheduledQueries([]config.DataObservabilityQuery{{Schedule: "0 9 * * *"}})
	now := time.Date(2026, 1, 1, 4, 0, 0, 0, time.FixedZone("EST", -5*60*60))
	due := getDueDataObservabilityQueries(queries, now)
	require.Len(t, due, 1)
	assert.Equal(t, time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC), due[0].scheduledTime)
	assert.Empty(t, getDueDataObservabilityQueries(queries, now.Add(time.Second)))
}

func TestDataObservabilityCronNoNextTick(t *testing.T) {
	for _, tt := range []struct {
		schedule string
		wantDue  int
	}{
		{"0 0 31 2 *", 0},
		{"0 0 29 2 *", 1},
	} {
		t.Run(tt.schedule, func(t *testing.T) {
			queries := newDataObservabilityScheduledQueries([]config.DataObservabilityQuery{{Schedule: tt.schedule}})
			now := time.Date(2096, 2, 29, 0, 0, 10, 0, time.UTC)
			due := getDueDataObservabilityQueries(queries, now)
			require.Len(t, due, tt.wantDue)
			assert.True(t, queries[0].nextTick.IsZero())
			assert.Empty(t, getDueDataObservabilityQueries(queries, now.Add(time.Second)))
		})
	}
}

func TestDataObservabilityRetries(t *testing.T) {
	queries := newDataObservabilityScheduledQueries([]config.DataObservabilityQuery{
		{MonitorID: 42, Schedule: "* * * * *"},
		{MonitorID: 42, Schedule: "0 * * * *"},
		{MonitorID: 42, IntervalSeconds: 60},
	})
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	due := getDueDataObservabilityQueries(queries, now)
	require.Len(t, due, 3)
	for _, query := range due {
		query.retry()
	}
	due = getDueDataObservabilityQueries(queries, now.Add(10*time.Second))
	require.Len(t, due, 3)
	assert.Equal(t, now, due[0].scheduledTime)
	assert.Equal(t, now, due[1].scheduledTime)
	assert.Equal(t, now.Add(10*time.Second), due[2].scheduledTime)
	for _, query := range due {
		query.retry()
	}
	due = getDueDataObservabilityQueries(queries, now.Add(time.Minute))
	require.Len(t, due, 3)
	assert.Equal(t, now.Add(time.Minute), due[0].scheduledTime)
	assert.Equal(t, now, due[1].scheduledTime)
	assert.Equal(t, now.Add(time.Minute), due[2].scheduledTime)
	for _, query := range due {
		query.complete(now.Add(time.Minute))
	}
	due = getDueDataObservabilityQueries(queries, now.Add(70*time.Second))
	assert.Empty(t, due)
}

func TestDataObservabilityCompletedQueriesDoNotRetryWithFailedBatch(t *testing.T) {
	queries := newDataObservabilityScheduledQueries([]config.DataObservabilityQuery{
		{Schedule: "0 * * * *"},
		{Schedule: "0 * * * *"},
		{IntervalSeconds: 60},
	})
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	due := getDueDataObservabilityQueries(queries, now)
	require.Len(t, due, 3)
	due[0].complete(now)
	for _, query := range due[1:] {
		query.retry()
	}
	due = getDueDataObservabilityQueries(queries, now.Add(10*time.Second))
	require.Len(t, due, 2)
	assert.Same(t, queries[1], due[0].scheduledQuery)
	assert.Same(t, queries[2], due[1].scheduledQuery)
}
