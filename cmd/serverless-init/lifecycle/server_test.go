// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lifecycle

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockFlusher counts how many times Flush was called.
type mockFlusher struct{ count atomic.Int32 }

func (m *mockFlusher) Flush() { m.count.Add(1) }

// mockLogsAgent counts how many times Flush was called.
type mockLogsAgent struct{ count atomic.Int32 }

func (m *mockLogsAgent) Flush(_ context.Context) { m.count.Add(1) }

// emittedMetric records one AddEnhancedMetric call.
type emittedMetric struct {
	name      string
	timestamp float64
	extraTags []string
}

// mockSampleDrainer counts how many times WaitForPendingSamples was called.
type mockSampleDrainer struct{ count atomic.Int32 }

func (m *mockSampleDrainer) WaitForPendingSamples() { m.count.Add(1) }

// neverDrainer blocks in WaitForPendingSamples forever, simulating a stuck aggregator worker.
// Used to exercise the drain-timeout path in flushAll.
type neverDrainer struct{}

func (n *neverDrainer) WaitForPendingSamples() { select {} }

// mockMetricEmitter records metric calls. Protected by mu for goroutine safety.
type mockMetricEmitter struct {
	mu      sync.Mutex
	metrics []emittedMetric
}

func (m *mockMetricEmitter) AddEnhancedMetric(name string, _ float64, _ metrics.MetricSource, ts float64, tags ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metrics = append(m.metrics, emittedMetric{name: name, timestamp: ts, extraTags: tags})
}

func (m *mockMetricEmitter) getEmitted() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	names := make([]string, len(m.metrics))
	for i, em := range m.metrics {
		names[i] = em.name
	}
	return names
}

func (m *mockMetricEmitter) getEmittedMetrics() []emittedMetric {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]emittedMetric, len(m.metrics))
	copy(result, m.metrics)
	return result
}

func newTestServer() (*Server, *mockFlusher, *mockFlusher, *mockLogsAgent, *mockMetricEmitter, *mockSampleDrainer) {
	metric := &mockFlusher{}
	trace := &mockFlusher{}
	logs := &mockLogsAgent{}
	emitter := &mockMetricEmitter{}
	drainer := &mockSampleDrainer{}
	// port 0 — handler-level tests don't bind, but Stop()/Serve() tests use a
	// custom listener so the server's configured port is irrelevant.
	srv := NewServer(0, metric, trace, logs, emitter, drainer, metrics.MetricSourceAWSMicroVMEnhanced, 2*time.Second)
	return srv, metric, trace, logs, emitter, drainer
}

func TestHandleReadyReturns200(t *testing.T) {
	srv, _, _, _, _, _ := newTestServer()
	req := httptest.NewRequest(http.MethodPost, pathReady, nil)
	rec := httptest.NewRecorder()
	srv.handleReady(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleLaunchEmitsMetricAndReturns200(t *testing.T) {
	srv, _, _, _, emitter, _ := newTestServer()
	req := httptest.NewRequest(http.MethodPost, pathLaunch, nil)
	rec := httptest.NewRecorder()
	srv.handleLaunch(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, emitter.getEmitted(), launchMetricName)
}

func TestHandleLaunchParsesInstanceID(t *testing.T) {
	srv, _, _, _, _, _ := newTestServer()

	body := strings.NewReader(`{"microVmId":"vm-abc123"}`)
	req := httptest.NewRequest(http.MethodPost, pathLaunch, body)
	rec := httptest.NewRecorder()
	srv.handleLaunch(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	id, _ := srv.instanceID.Load().(string)
	assert.Equal(t, "vm-abc123", id, "instance ID must be stored on the server for lifecycle metric tags")
}

func TestHandleLaunchEmptyBodyDoesNotSetInstanceID(t *testing.T) {
	srv, _, _, _, _, _ := newTestServer()

	req := httptest.NewRequest(http.MethodPost, pathLaunch, nil)
	rec := httptest.NewRecorder()
	srv.handleLaunch(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	id, _ := srv.instanceID.Load().(string)
	assert.Empty(t, id, "empty body must not set instance ID")
}

func TestHandleSuspendFlushesBeforeResponding(t *testing.T) {
	srv, metric, trace, logs, emitter, drainer := newTestServer()
	req := httptest.NewRequest(http.MethodPost, pathSuspend, nil)
	rec := httptest.NewRecorder()
	srv.handleSuspend(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, int32(1), metric.count.Load(), "metric agent must be flushed")
	assert.Equal(t, int32(1), trace.count.Load(), "trace agent must be flushed")
	assert.Equal(t, int32(1), logs.count.Load(), "logs agent must be flushed")
	assert.Contains(t, emitter.getEmitted(), suspendMetricName)
	assert.Equal(t, int32(1), drainer.count.Load(), "pending samples must be drained before flush")
}

func TestHandleResumeReturns200WithoutFlush(t *testing.T) {
	srv, metric, trace, logs, emitter, drainer := newTestServer()
	req := httptest.NewRequest(http.MethodPost, pathResume, nil)
	rec := httptest.NewRecorder()
	srv.handleResume(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, int32(0), metric.count.Load(), "must not flush on resume")
	assert.Equal(t, int32(0), trace.count.Load())
	assert.Equal(t, int32(0), logs.count.Load())
	assert.Equal(t, int32(0), drainer.count.Load(), "must not drain on resume")
	assert.Contains(t, emitter.getEmitted(), resumeMetricName)
}

func TestHandleTerminateFlushesAndEmitsMetric(t *testing.T) {
	srv, metric, trace, logs, emitter, drainer := newTestServer()

	req := httptest.NewRequest(http.MethodPost, pathTerminate, nil)
	rec := httptest.NewRecorder()
	srv.handleTerminate(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, int32(1), metric.count.Load(), "metric agent must be flushed")
	assert.Equal(t, int32(1), trace.count.Load())
	assert.Equal(t, int32(1), logs.count.Load())
	assert.Contains(t, emitter.getEmitted(), terminateMetricName)
	assert.Equal(t, int32(1), drainer.count.Load(), "pending samples must be drained before flush")
}

// TestEmittedMetricsCarryCurrentTimestamp verifies the lifecycle handlers pass
// a current Unix-seconds timestamp to AddEnhancedMetric rather than the `0`
// sentinel that defers timestamp assignment to the metric agent. The window
// check also guards against unit regressions (e.g. ms vs. s).
func TestEmittedMetricsCarryCurrentTimestamp(t *testing.T) {
	srv, _, _, _, emitter, _ := newTestServer()

	before := float64(time.Now().UnixNano()) / float64(time.Second)
	rec := httptest.NewRecorder()
	srv.handleLaunch(rec, httptest.NewRequest(http.MethodPost, pathLaunch, nil))
	after := float64(time.Now().UnixNano()) / float64(time.Second)

	emitted := emitter.getEmittedMetrics()
	require.Len(t, emitted, 1)
	ts := emitted[0].timestamp
	assert.Greater(t, ts, 0.0, "timestamp must not be the 0 sentinel")
	assert.GreaterOrEqual(t, ts, before, "timestamp must be at or after pre-call time")
	assert.LessOrEqual(t, ts, after, "timestamp must be at or before post-call time")
}

func TestRoutes(t *testing.T) {
	srv, _, _, _, _, _ := newTestServer()
	routes := []string{pathReady, pathLaunch, pathSuspend, pathResume, pathTerminate}
	handler := srv.handler()
	for _, route := range routes {
		req := httptest.NewRequest(http.MethodPost, route, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, "route %s should return 200", route)
	}
}

func TestStopOnNilServerReturnsNil(t *testing.T) {
	var srv *Server
	require.NoError(t, srv.Stop(context.Background()))
}

func TestStopWithoutStartReturnsNil(t *testing.T) {
	srv, _, _, _, _, _ := newTestServer()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	require.NoError(t, srv.Stop(ctx))
}

// TestServeAndStopGracefulShutdown exercises the real HTTP server lifecycle.
// It binds a listener on a random port, serves until Stop is called, and
// verifies that Serve returns http.ErrServerClosed — the contract that lets
// main.go's defer chain run cleanly when shutdown is triggered externally.
func TestServeAndStopGracefulShutdown(t *testing.T) {
	srv, _, _, _, _, _ := newTestServer()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	serverDone := make(chan struct{})
	go func() {
		srv.Serve(listener)
		close(serverDone)
	}()

	// Confirm the server is accepting requests before we ask it to stop.
	resp, err := http.Post("http://"+listener.Addr().String()+pathReady, "", nil)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, srv.Stop(ctx))

	select {
	case <-serverDone:
		// Serve returned after Stop — correct.
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after Stop")
	}
}

// TestNewServerConfiguresHTTPTimeouts verifies that NewServer sets ReadTimeout and
// WriteTimeout on the underlying http.Server. WriteTimeout is flushTimeout + 10s
// because /suspend and /terminate flush all telemetry before responding 200.
func TestNewServerConfiguresHTTPTimeouts(t *testing.T) {
	flushTimeout := 5 * time.Second
	srv := NewServer(0, &mockFlusher{}, &mockFlusher{}, &mockLogsAgent{}, &mockMetricEmitter{}, &mockSampleDrainer{}, metrics.MetricSourceAWSMicroVMEnhanced, flushTimeout)
	assert.Equal(t, 30*time.Second, srv.httpServer.ReadTimeout)
	assert.Equal(t, flushTimeout+10*time.Second, srv.httpServer.WriteTimeout)
}

// TestInstanceIDTagAppearsInMetricsAfterLaunch verifies that once /launch stores a
// MicroVM instance ID, subsequent lifecycle metrics include instance_id:<id> as an
// extra tag. This is the primary tagging path for identifying individual MicroVM
// instances in lifecycle metrics.
func TestInstanceIDTagAppearsInMetricsAfterLaunch(t *testing.T) {
	srv, _, _, _, emitter, _ := newTestServer()

	body := strings.NewReader(`{"microVmId":"vm-abc123"}`)
	srv.handleLaunch(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, pathLaunch, body))

	// Suspend after launch — the suspend metric must carry the instance_id tag.
	srv.handleSuspend(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, pathSuspend, nil))

	emitted := emitter.getEmittedMetrics()
	var found *emittedMetric
	for i := range emitted {
		if emitted[i].name == suspendMetricName {
			found = &emitted[i]
			break
		}
	}
	require.NotNil(t, found, "suspend metric must be emitted")
	assert.Contains(t, found.extraTags, instanceIDTagPrefix+"vm-abc123")
}

// TestFlushAllDrainTimeoutDoesNotBlock verifies that flushAll returns within the flush
// timeout even when the sample drainer never completes. This guards against the
// lifecycle server stalling — and blocking the MicroVM platform's suspend/terminate
// handshake — when the metric aggregator worker is deadlocked or slow.
func TestFlushAllDrainTimeoutDoesNotBlock(t *testing.T) {
	srv := NewServer(0, &mockFlusher{}, &mockFlusher{}, &mockLogsAgent{}, &mockMetricEmitter{}, &neverDrainer{}, metrics.MetricSourceAWSMicroVMEnhanced, 50*time.Millisecond)

	start := time.Now()
	srv.flushAll()
	assert.Less(t, time.Since(start), 500*time.Millisecond, "flushAll must return within flushTimeout even when drainer blocks")
}

// TestFlushAllNilLogsFlusherDoesNotPanic verifies that flushAll skips a nil logs
// flusher — which happens when the logs agent fails to start and SetupLogAgent
// returns nil — instead of panicking, while still flushing the metric and trace
// agents. This guards the MicroVM suspend/terminate handshake against a crash.
func TestFlushAllNilLogsFlusherDoesNotPanic(t *testing.T) {
	metric := &mockFlusher{}
	trace := &mockFlusher{}
	srv := NewServer(0, metric, trace, nil, &mockMetricEmitter{}, &mockSampleDrainer{}, metrics.MetricSourceAWSMicroVMEnhanced, 2*time.Second)

	assert.NotPanics(t, func() { srv.flushAll() })

	assert.Equal(t, int32(1), metric.count.Load(), "metric flusher should still run when logs flusher is nil")
	assert.Equal(t, int32(1), trace.count.Load(), "trace flusher should still run when logs flusher is nil")
}
