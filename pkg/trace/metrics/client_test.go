package metrics

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type testStatsClient struct {
	counts int64
}

func (ts *testStatsClient) Gauge(name string, value float64, tags []string, rate float64) error {
	atomic.AddInt64(&ts.counts, 1)
	return nil
}

func (ts *testStatsClient) Count(name string, value int64, tags []string, rate float64) error {
	atomic.AddInt64(&ts.counts, 1)
	return nil
}

func (ts *testStatsClient) Histogram(name string, value float64, tags []string, rate float64) error {
	atomic.AddInt64(&ts.counts, 1)
	return nil
}

func (ts *testStatsClient) Timing(name string, value time.Duration, tags []string, rate float64) error {
	atomic.AddInt64(&ts.counts, 1)
	return nil
}

func (ts *testStatsClient) Flush() error {
	atomic.AddInt64(&ts.counts, 1)
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
		assert.Equal(t, atomic.LoadInt64(&testclient.counts), int64(5))
	})
}
