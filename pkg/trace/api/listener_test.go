package api

import (
	"net"
	"sync"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"
	"github.com/stretchr/testify/assert"
)

type mockNetError struct{ timeout bool }

func (e mockNetError) Error() string   { return "mock-net-error" }
func (e mockNetError) Timeout() bool   { return e.timeout }
func (e mockNetError) Temporary() bool { return !e.timeout }

type mockListener struct {
	mu  sync.RWMutex // guards below fields
	err net.Error    // returned error, if non-nil
}

var _ net.Listener = (*mockListener)(nil)

func (ml *mockListener) Accept() (net.Conn, error) {
	ml.mu.RLock()
	defer ml.mu.RUnlock()
	return nil, ml.err
}

func (ml *mockListener) SetSuccess() {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	ml.err = nil
}

func (ml *mockListener) SetError() {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	ml.err = mockNetError{timeout: false}
}

func (ml *mockListener) SetTimeoutError() {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	ml.err = mockNetError{timeout: true}
}

func (ml *mockListener) Close() error { return nil }

func (ml *mockListener) Addr() net.Addr { return nil }

func TestMockListener(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		var ln mockListener
		_, err := ln.Accept()
		assert.NoError(t, err)
	})

	t.Run("error", func(t *testing.T) {
		assert := assert.New(t)
		var ln mockListener
		ln.SetError()
		_, err := ln.Accept()
		nerr, ok := err.(net.Error)
		assert.True(ok)
		assert.True(nerr.Temporary())
		assert.False(nerr.Timeout())
	})

	t.Run("timeout", func(t *testing.T) {
		assert := assert.New(t)
		var ln mockListener
		ln.SetTimeoutError()
		_, err := ln.Accept()
		nerr, ok := err.(net.Error)
		assert.True(ok)
		assert.False(nerr.Temporary())
		assert.True(nerr.Timeout())
	})

	t.Run("toggle", func(t *testing.T) {
		assert := assert.New(t)
		var ln mockListener

		ln.SetTimeoutError()
		_, err := ln.Accept()
		nerr, ok := err.(net.Error)
		assert.True(ok)
		assert.False(nerr.Temporary())
		assert.True(nerr.Timeout())

		ln.SetSuccess()
		_, err = ln.Accept()
		assert.NoError(err)

		ln.SetError()
		_, err = ln.Accept()
		nerr, ok = err.(net.Error)
		assert.True(ok)
		assert.True(nerr.Temporary())
		assert.False(nerr.Timeout())

		ln.SetSuccess()
		_, err = ln.Accept()
		assert.NoError(err)
	})
}

func TestMeasuredListener(t *testing.T) {
	assert := assert.New(t)
	stats := &testutil.TestStatsClient{}
	defer func(old metrics.StatsClient) { metrics.Client = old }(metrics.Client)
	metrics.Client = stats

	var mockln mockListener
	ln := NewMeasuredListener(&mockln, "test-metric").(*measuredListener)
	mockln.SetSuccess()
	ln.Accept()
	ln.Accept()
	ln.Accept()
	ln.flushMetrics()
	call, ok := stats.GetCountSummaries()["datadog.trace_agent.receiver.test-metric"]
	assert.True(ok)
	assert.Len(call.Calls, 1)
	assert.EqualValues(call.Calls[0].Tags, []string{"status:accepted"})
	assert.EqualValues(call.Calls[0].Value, 3)

	stats.Reset()
	mockln.SetError()
	ln.Accept()
	ln.Accept()
	ln.flushMetrics()
	call, ok = stats.GetCountSummaries()["datadog.trace_agent.receiver.test-metric"]
	assert.True(ok)
	assert.Len(call.Calls, 1)
	assert.EqualValues(call.Calls[0].Tags, []string{"status:errored"})
	assert.EqualValues(call.Calls[0].Value, 2)

	stats.Reset()
	mockln.SetTimeoutError()
	ln.Accept()
	ln.flushMetrics()
	call, ok = stats.GetCountSummaries()["datadog.trace_agent.receiver.test-metric"]
	assert.True(ok)
	assert.Len(call.Calls, 1)
	assert.EqualValues(call.Calls[0].Tags, []string{"status:timedout"})
	assert.EqualValues(call.Calls[0].Value, 1)
}
