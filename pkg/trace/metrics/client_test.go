// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"
)

type testStatsClient struct {
	counts atomic.Int64
}

//nolint:revive // TODO(APM) Fix revive linter
func (ts *testStatsClient) Gauge(name string, value float64, tags []string, rate float64) error {
	ts.counts.Inc()
	return nil
}

//nolint:revive // TODO(APM) Fix revive linter
func (ts *testStatsClient) Count(name string, value int64, tags []string, rate float64) error {
	ts.counts.Inc()
	return nil
}

//nolint:revive // TODO(APM) Fix revive linter
func (ts *testStatsClient) Histogram(name string, value float64, tags []string, rate float64) error {
	ts.counts.Inc()
	return nil
}

//nolint:revive // TODO(APM) Fix revive linter
func (ts *testStatsClient) Timing(name string, value time.Duration, tags []string, rate float64) error {
	ts.counts.Inc()
	return nil
}

func (ts *testStatsClient) Flush() error {
	ts.counts.Inc()
	return nil
}

func TestForwarding(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		defer func(old StatsClient) { Client = old }(Client)
		Client = nil
		assert.NoError(t, Gauge("stat", 1, nil, 1))
		assert.NoError(t, Count("stat", 1, nil, 1))
		assert.NoError(t, Histogram("stat", 1, nil, 1))
		assert.NoError(t, Timing("stat", time.Second, nil, 1))
		assert.NoError(t, Flush())
	})

	t.Run("valid", func(t *testing.T) {
		defer func(old StatsClient) { Client = old }(Client)
		var testclient testStatsClient
		Client = &testclient
		assert.NoError(t, Gauge("stat", 1, nil, 1))
		assert.NoError(t, Count("stat", 1, nil, 1))
		assert.NoError(t, Histogram("stat", 1, nil, 1))
		assert.NoError(t, Timing("stat", time.Second, nil, 1))
		assert.NoError(t, Flush())
		assert.Equal(t, testclient.counts.Load(), int64(5))
	})
}
