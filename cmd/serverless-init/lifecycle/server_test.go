// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lifecycle

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
)

// mockFlusher counts how many times Flush was called and records when each
// Flush call returned. The completedAt pointer is nil until the first Flush.
type mockFlusher struct {
	count       atomic.Int32
	completedAt atomic.Pointer[time.Time]
}

func (m *mockFlusher) Flush() {
	m.count.Add(1)
	now := time.Now()
	m.completedAt.Store(&now)
}

// mockLogsAgent counts how many times Flush was called and records when each
// call returned.
type mockLogsAgent struct {
	count       atomic.Int32
	completedAt atomic.Pointer[time.Time]
}

func (m *mockLogsAgent) Flush(_ context.Context) {
	m.count.Add(1)
	now := time.Now()
	m.completedAt.Store(&now)
}

// mockSampleDrainer counts how many times WaitForPendingSamples was called.
type mockSampleDrainer struct{ count atomic.Int32 }

func (m *mockSampleDrainer) WaitForPendingSamples() { m.count.Add(1) }

// neverDrainer blocks in WaitForPendingSamples forever, simulating a stuck aggregator worker.
// Used to exercise the drain-timeout path in flushAll.
type neverDrainer struct{}

func (n *neverDrainer) WaitForPendingSamples() { select {} }

func newTestServer() (*Server, *mockFlusher, *mockFlusher, *mockLogsAgent, *mockMetricEmitter, *mockSampleDrainer) {
	metric := &mockFlusher{}
	trace := &mockFlusher{}
	logs := &mockLogsAgent{}
	emitter := &mockMetricEmitter{}
	drainer := &mockSampleDrainer{}
	// port 0 — handler-level tests don't bind. Tests that need a childHandle,
	// forwarder, or heartbeat assign srv.childHandle / srv.fwd / srv.heartbeat
	// after construction.
	srv := NewServer(0, metric, trace, logs, emitter, drainer, metrics.MetricSourceAWSMicroVMEnhanced, 2*time.Second, nil, nil, nil)
	return srv, metric, trace, logs, emitter, drainer
}

// /ready with a nil ChildHandle is a wiring bug. The handler logs WARN and
// returns 503. Production setup() always constructs a non-nil handle (real
// *Child in init mode, NoopChildHandle in sidecar mode); only legacy unit
// tests can hit this path.
func TestHandleReady_NilChildHandle_Returns503(t *testing.T) {
	srv, _, _, _, _, _ := newTestServer()
	req := httptest.NewRequest(http.MethodPost, pathReady, nil)
	rec := httptest.NewRecorder()
	srv.handleReady(rec, req)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestHandleValidateEmitsMetricAndReturns200(t *testing.T) {
	srv, _, _, _, emitter, _ := newTestServer()
	rec := httptest.NewRecorder()
	srv.handleValidate(rec, httptest.NewRequest(http.MethodPost, pathValidate, nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, emitter.getEmitted(), validateMetricName)
}

func TestHandleRunEmitsMetricAndReturns200(t *testing.T) {
	srv, _, _, _, emitter, _ := newTestServer()
	req := httptest.NewRequest(http.MethodPost, pathRun, nil)
	rec := httptest.NewRecorder()
	srv.handleRun(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, emitter.getEmitted(), runMetricName)
}

func TestHandleRunParsesInstanceID(t *testing.T) {
	srv, _, _, _, _, _ := newTestServer()

	body := strings.NewReader(`{"microvmId":"vm-abc123"}`)
	req := httptest.NewRequest(http.MethodPost, pathRun, body)
	rec := httptest.NewRecorder()
	srv.handleRun(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	id := srv.instanceID.Load()
	assert.Equal(t, "vm-abc123", id, "instance ID must be stored on the server for lifecycle metric tags")
}

// TestHandleRunWithForwarderParsesInstanceID verifies that when a forwarder is
// configured, /run still decodes the MicroVM instance ID from the request body
// before delegating to handleWithForwarder. Without the decode-then-restore fix, the
// forwarder path consumed r.Body first, so instanceID was never stored and all
// subsequent lifecycle metrics lost the instance_id tag.
func TestHandleRunWithForwarderParsesInstanceID(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	srv, _, _, _, _, _ := newTestServer()
	srv.fwd = &Forwarder{
		target:               upstream.URL,
		client:               &http.Client{},
		forwardTimeout:       2 * time.Second,
		maxResponseBodyBytes: defaultMaxResponseBodyBytes,
	}

	body := strings.NewReader(`{"microvmId":"vm-fwd123"}`)
	srv.handleRun(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, pathRun, body))

	id := srv.instanceID.Load()
	assert.Equal(t, "vm-fwd123", id, "instance ID must be stored even when forwarder is configured")
}

func TestHandleRunEmptyBodyDoesNotSetInstanceID(t *testing.T) {
	srv, _, _, _, _, _ := newTestServer()

	req := httptest.NewRequest(http.MethodPost, pathRun, nil)
	rec := httptest.NewRecorder()
	srv.handleRun(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	id := srv.instanceID.Load()
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

// /terminate (no forwarder) flushes telemetry, emits the metric, and returns
// 200. After the user-app-owns-response amendment it does NOT synthesize
// SIGTERM: the platform owns process termination via OS signals delivered
// independently. This test pins both the flush and the no-SIGTERM behavior.
func TestHandleTerminate_NoForwarder_FlushesAndEmitsMetric_NoSigterm(t *testing.T) {
	srv, metric, trace, logs, emitter, drainer := newTestServer()

	// Register a SIGTERM watcher BEFORE invoking the handler so we can detect
	// any synthetic signal that fires before or after the response.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	req := httptest.NewRequest(http.MethodPost, pathTerminate, nil)
	rec := httptest.NewRecorder()
	srv.handleTerminate(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, int32(1), metric.count.Load(), "metric agent must be flushed")
	assert.Equal(t, int32(1), trace.count.Load())
	assert.Equal(t, int32(1), logs.count.Load())
	assert.Contains(t, emitter.getEmitted(), terminateMetricName)
	assert.Equal(t, int32(1), drainer.count.Load(), "pending samples must be drained before flush")

	// No SIGTERM should reach the test process within a reasonable window.
	// 100ms is generous: today's removed code runed the syscall in a
	// fire-and-forget goroutine that fires immediately after WriteHeader.
	select {
	case sig := <-sigCh:
		t.Fatalf("/terminate must not synthesize SIGTERM after the user-app-owns-response amendment; got %v", sig)
	case <-time.After(100 * time.Millisecond):
		// Pass — no synthetic signal observed.
	}
}

// TestEmittedMetricsCarryCurrentTimestamp verifies the lifecycle handlers pass
// a current Unix-seconds timestamp to AddEnhancedMetric rather than the `0`
// sentinel that defers timestamp assignment to the metric agent. The window
// check also guards against unit regressions (e.g. ms vs. s).
func TestEmittedMetricsCarryCurrentTimestamp(t *testing.T) {
	srv, _, _, _, emitter, _ := newTestServer()

	before := float64(time.Now().UnixNano()) / float64(time.Second)
	rec := httptest.NewRecorder()
	srv.handleRun(rec, httptest.NewRequest(http.MethodPost, pathRun, nil))
	after := float64(time.Now().UnixNano()) / float64(time.Second)

	emitted := emitter.getEmittedMetrics()
	require.Len(t, emitted, 1)
	ts := emitted[0].timestamp
	assert.Greater(t, ts, 0.0, "timestamp must not be the 0 sentinel")
	assert.GreaterOrEqual(t, ts, before, "timestamp must be at or after pre-call time")
	assert.LessOrEqual(t, ts, after, "timestamp must be at or before post-call time")
}

// TestEmittedMetricsCarryCurrentTimestamp_ForwarderPath verifies the same
// timestamp guarantee for the handleWithForwarder code path (forwarder enabled).
// handleWithForwarder runs metric emission in a goroutine but joins before the
// handler returns, so the before/after window technique still applies.
func TestEmittedMetricsCarryCurrentTimestamp_ForwarderPath(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	srv, _, _, _, emitter, _ := newTestServer()
	srv.fwd = &Forwarder{
		target:               upstream.URL,
		client:               &http.Client{},
		forwardTimeout:       2 * time.Second,
		maxResponseBodyBytes: defaultMaxResponseBodyBytes,
	}

	before := float64(time.Now().UnixNano()) / float64(time.Second)
	srv.handleRun(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, pathRun, nil))
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
	// /ready needs an alive child handle to return 200; the other hooks
	// ignore childHandle when no forwarder is configured.
	h := newFakeChildHandle()
	h.alive.Store(true)
	srv.childHandle = h
	routes := []string{pathReady, pathValidate, pathRun, pathSuspend, pathResume, pathTerminate}
	handler := srv.handler()
	for _, route := range routes {
		req := httptest.NewRequest(http.MethodPost, route, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, "route %s should return 200", route)
	}
}

// TestRoutes_NonPost_Returns405 verifies that the mux rejects non-POST requests
// with 405 Method Not Allowed. All lifecycle hooks are POST-only; the platform
// never sends GET/PUT/DELETE to these paths.
func TestRoutes_NonPost_Returns405(t *testing.T) {
	srv, _, _, _, _, _ := newTestServer()
	handler := srv.handler()
	routes := []string{pathReady, pathValidate, pathRun, pathSuspend, pathResume, pathTerminate}
	for _, route := range routes {
		req := httptest.NewRequest(http.MethodGet, route, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusMethodNotAllowed, rec.Code, "route %s must reject GET with 405", route)
	}
}

// fakeChildHandle drives /ready behavior in tests.
type fakeChildHandle struct {
	alive atomic.Bool
}

func newFakeChildHandle() *fakeChildHandle { return &fakeChildHandle{} }
func (f *fakeChildHandle) IsAlive() bool   { return f.alive.Load() }

func TestHandleReady_NoForwarder_ChildAlive_Returns200(t *testing.T) {
	srv, _, _, _, _, _ := newTestServer()
	h := newFakeChildHandle()
	h.alive.Store(true)
	srv.childHandle = h
	rec := httptest.NewRecorder()
	srv.handleReady(rec, httptest.NewRequest(http.MethodPost, pathReady, nil))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandleReady_NoForwarder_ChildNotAlive_Returns503(t *testing.T) {
	srv, _, _, _, _, _ := newTestServer()
	srv.childHandle = newFakeChildHandle() // alive=false (default)
	rec := httptest.NewRecorder()
	srv.handleReady(rec, httptest.NewRequest(http.MethodPost, pathReady, nil))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestHandleReady_WithForwarder_PassesThrough(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-ready")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"ready":false,"reason":"warming"}`))
	}))
	defer upstream.Close()

	srv, _, _, _, _, _ := newTestServer()
	srv.fwd = &Forwarder{
		target:               upstream.URL,
		client:               &http.Client{},
		readyTimeout:         200 * time.Millisecond,
		maxResponseBodyBytes: defaultMaxResponseBodyBytes,
	}
	rec := httptest.NewRecorder()
	srv.handleReady(rec, httptest.NewRequest(http.MethodPost, pathReady, nil))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	// /ready must mirror the user app's Content-Type AND body, not just the
	// status code. The platform may surface the user-app reason string in
	// readiness diagnostics; dropping body/Content-Type would silently
	// hide that.
	assert.Equal(t, "application/x-ready", rec.Header().Get("Content-Type"))
	assert.Equal(t, `{"ready":false,"reason":"warming"}`, rec.Body.String())
}

// /run with a forwarder configured mirrors the user-app's status code,
// body, and Content-Type, and emits the run metric. Replaces the prior
// fire-and-forget contract.
func TestHandleRun_WithForwarder_MirrorsUserAppResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-run")
		w.WriteHeader(207)
		_, _ = w.Write([]byte(`{"warmed":true}`))
	}))
	defer upstream.Close()

	srv, metric, trace, logs, emitter, _ := newTestServer()
	srv.fwd = &Forwarder{
		target:               upstream.URL,
		client:               &http.Client{},
		forwardTimeout:       2 * time.Second,
		maxResponseBodyBytes: defaultMaxResponseBodyBytes,
	}

	rec := httptest.NewRecorder()
	srv.handleRun(rec, httptest.NewRequest(http.MethodPost, pathRun, nil))

	assert.Equal(t, 207, rec.Code, "must mirror user-app status, not hardcoded 200")
	assert.Equal(t, "application/x-run", rec.Header().Get("Content-Type"))
	assert.Equal(t, `{"warmed":true}`, rec.Body.String())
	assert.Contains(t, emitter.getEmitted(), runMetricName)
	// /run does NOT flush — these are no-op for run/resume.
	assert.Equal(t, int32(0), metric.count.Load(), "run must not flush")
	assert.Equal(t, int32(0), trace.count.Load())
	assert.Equal(t, int32(0), logs.count.Load())
}

// /resume with a forwarder configured mirrors the user-app's response and
// does NOT flush.
func TestHandleResume_WithForwarder_MirrorsUserAppResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(418)
	}))
	defer upstream.Close()

	srv, metric, trace, logs, emitter, _ := newTestServer()
	srv.fwd = &Forwarder{
		target:               upstream.URL,
		client:               &http.Client{},
		forwardTimeout:       2 * time.Second,
		maxResponseBodyBytes: defaultMaxResponseBodyBytes,
	}

	rec := httptest.NewRecorder()
	srv.handleResume(rec, httptest.NewRequest(http.MethodPost, pathResume, nil))

	assert.Equal(t, 418, rec.Code)
	assert.Contains(t, emitter.getEmitted(), resumeMetricName)
	assert.Equal(t, int32(0), metric.count.Load(), "resume must not flush")
	assert.Equal(t, int32(0), trace.count.Load())
	assert.Equal(t, int32(0), logs.count.Load())
}

// /terminate with a forwarder configured mirrors the user-app's response,
// emits the terminate metric, AND flushes telemetry. After the amendment,
// /terminate also no longer synthesizes a SIGTERM (separate test).
func TestHandleTerminate_WithForwarder_MirrorsUserAppResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(503)
	}))
	defer upstream.Close()

	srv, metric, trace, logs, emitter, _ := newTestServer()
	srv.fwd = &Forwarder{
		target:               upstream.URL,
		client:               &http.Client{},
		forwardTimeout:       2 * time.Second,
		maxResponseBodyBytes: defaultMaxResponseBodyBytes,
	}

	rec := httptest.NewRecorder()
	srv.handleTerminate(rec, httptest.NewRequest(http.MethodPost, pathTerminate, nil))

	assert.Equal(t, 503, rec.Code, "must mirror user-app status, not hardcoded 200")
	assert.Contains(t, emitter.getEmitted(), terminateMetricName)
	assert.Equal(t, int32(1), metric.count.Load(), "terminate must flush")
	assert.Equal(t, int32(1), trace.count.Load())
	assert.Equal(t, int32(1), logs.count.Load())
}

// When a forwarder is configured, /suspend must mirror the user app's
// status code, body, and Content-Type back to the platform — not the
// agent's previous hardcoded 200. This is the core behavior change of
// the user-app-owns-response amendment.
func TestHandleSuspend_WithForwarder_MirrorsUserAppResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-suspend")
		w.WriteHeader(207) // distinctive status to rule out any agent default
		_, _ = w.Write([]byte(`{"drained":true}`))
	}))
	defer upstream.Close()

	srv, metric, trace, logs, emitter, _ := newTestServer()
	srv.fwd = &Forwarder{
		target:               upstream.URL,
		client:               &http.Client{},
		forwardTimeout:       2 * time.Second,
		maxResponseBodyBytes: defaultMaxResponseBodyBytes,
	}

	rec := httptest.NewRecorder()
	srv.handleSuspend(rec, httptest.NewRequest(http.MethodPost, pathSuspend, nil))

	assert.Equal(t, 207, rec.Code, "must mirror user-app status, not hardcoded 200")
	assert.Equal(t, "application/x-suspend", rec.Header().Get("Content-Type"))
	assert.Equal(t, `{"drained":true}`, rec.Body.String())

	// Side-effect path still runs alongside the pass-through.
	assert.Equal(t, int32(1), metric.count.Load(), "metric flush must still run")
	assert.Equal(t, int32(1), trace.count.Load(), "trace flush must still run")
	assert.Equal(t, int32(1), logs.count.Load(), "logs flush must still run")
	assert.Contains(t, emitter.getEmitted(), suspendMetricName, "metric must still be emitted")
}

// Parallelism pin: with a slow user-app forward, flush mocks must complete
// BEFORE the forward returns. A sequential "forward then flush" implementation
// would flip this ordering. (A "flush then forward" implementation would also
// pass this assertion, but that variant is benign: it's still bounded and
// telemetry still flushes — just slower. The assertion catches the
// correctness-violating ordering.)
func TestHandleSuspend_WithForwarder_FlushCompletesBeforeForwardReturns(t *testing.T) {
	forwardCompletedAt := atomic.Pointer[time.Time]{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(150 * time.Millisecond) // give flush mocks ample time to land first
		now := time.Now()
		forwardCompletedAt.Store(&now)
		w.WriteHeader(200)
	}))
	defer upstream.Close()

	srv, metric, trace, logs, _, _ := newTestServer()
	srv.fwd = &Forwarder{
		target:               upstream.URL,
		client:               &http.Client{},
		forwardTimeout:       2 * time.Second,
		maxResponseBodyBytes: defaultMaxResponseBodyBytes,
	}

	rec := httptest.NewRecorder()
	srv.handleSuspend(rec, httptest.NewRequest(http.MethodPost, pathSuspend, nil))

	require.NotNil(t, forwardCompletedAt.Load(), "forward must have completed")
	fwd := *forwardCompletedAt.Load()
	for _, name := range []struct {
		label string
		ts    *time.Time
	}{
		{"metric", metric.completedAt.Load()},
		{"trace", trace.completedAt.Load()},
		{"logs", logs.completedAt.Load()},
	} {
		require.NotNilf(t, name.ts, "%s flush must have completed", name.label)
		assert.Truef(t, name.ts.Before(fwd), "%s flush_completed_ts must precede forward_completed_ts (parallel execution)", name.label)
	}
}

// Dial-error pin: when the user app is not listening, /suspend returns 503
// (mirrored from the Forwarder's error stub) AND all three flushers were
// invoked anyway. This guards against an implementation that conditioned
// the side-effect path on the forward succeeding.
func TestHandleSuspend_WithForwarder_DialErrorStillRunsFlush(t *testing.T) {
	srv, metric, trace, logs, _, _ := newTestServer()
	srv.fwd = &Forwarder{
		target:               "http://127.0.0.1:1", // unbound port → dial error
		client:               &http.Client{},
		forwardTimeout:       200 * time.Millisecond,
		maxResponseBodyBytes: defaultMaxResponseBodyBytes,
	}

	rec := httptest.NewRecorder()
	srv.handleSuspend(rec, httptest.NewRequest(http.MethodPost, pathSuspend, nil))

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code, "must mirror the Forwarder's 503 error stub")
	assert.Equal(t, int32(1), metric.count.Load(), "metric flush must run even on forward dial error")
	assert.Equal(t, int32(1), trace.count.Load(), "trace flush must run even on forward dial error")
	assert.Equal(t, int32(1), logs.count.Load(), "logs flush must run even on forward dial error")
}

func TestHandleSuspend_WithForwarder_WaitsForForwardBeforeResponse(t *testing.T) {
	forwardEntered := make(chan struct{})
	releaseForward := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(forwardEntered)
		<-releaseForward
		w.WriteHeader(200)
	}))
	defer upstream.Close()

	srv, metric, trace, logs, _, _ := newTestServer()
	srv.fwd = &Forwarder{target: upstream.URL, client: &http.Client{}, forwardTimeout: 5 * time.Second}

	handlerReturned := make(chan struct{})
	rec := httptest.NewRecorder()
	go func() {
		srv.handleSuspend(rec, httptest.NewRequest(http.MethodPost, pathSuspend, nil))
		close(handlerReturned)
	}()

	<-forwardEntered
	// Forward is mid-flight; handleSuspend must not have returned yet.
	select {
	case <-handlerReturned:
		t.Fatal("handleSuspend returned before forward completed")
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseForward)
	<-handlerReturned

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, int32(1), metric.count.Load())
	assert.Equal(t, int32(1), trace.count.Load())
	assert.Equal(t, int32(1), logs.count.Load())
}

// flushAll uses waitForFlushes to bound how long it waits for the metric,
// trace, and logs flushers. When a flusher exceeds flushTimeout, the handler
// MUST return promptly rather than blocking the platform's lifecycle window.
// This test pins the early-return path: a 50ms flushTimeout against a 1s
// flusher must not extend the handler's wall-clock past flushTimeout + ε.
//
// Without the timeout branch, /suspend and /terminate would block until the
// slowest flusher returned, potentially exceeding the platform's
// /terminate 60s deadline and getting the VM destroyed mid-flush.
func TestHandleSuspend_NoForwarder_FlushTimeout_ReturnsPromptly(t *testing.T) {
	srv, _, _, logs, _, _ := newTestServer()
	srv.flushTimeout = 50 * time.Millisecond

	// Replace the logs flusher with one that ignores the context and
	// blocks for far longer than flushTimeout. This is the realistic
	// shape of a slow downstream — fail-soft on timeout, don't block.
	slow := &slowLogsFlusher{block: 1 * time.Second}
	srv.logsFlusher = slow
	_ = logs // unused but documented in the test signature

	start := time.Now()
	rec := httptest.NewRecorder()
	srv.handleSuspend(rec, httptest.NewRequest(http.MethodPost, pathSuspend, nil))
	elapsed := time.Since(start)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Less(t, elapsed, 500*time.Millisecond,
		"handler must return promptly when flushTimeout fires (≪ slow flusher's 1s)")
}

type slowLogsFlusher struct {
	block time.Duration
}

func (s *slowLogsFlusher) Flush(_ context.Context) {
	time.Sleep(s.block)
}

// When a forwarder is configured and the flush goroutine exceeds flushTimeout,
// handleSuspend must still return promptly. The flush runs concurrently with
// PassThrough inside handleWithForwarder; flushAll's waitForFlushes honours the
// timeout and closes sideDone so the handler is not blocked on the slow flusher.
func TestHandleSuspend_WithForwarder_FlushTimeout_ReturnsPromptly(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	srv, _, _, _, _, _ := newTestServer()
	srv.flushTimeout = 50 * time.Millisecond
	srv.logsFlusher = &slowLogsFlusher{block: 1 * time.Second}
	srv.fwd = &Forwarder{
		target:               upstream.URL,
		client:               &http.Client{},
		forwardTimeout:       2 * time.Second,
		maxResponseBodyBytes: defaultMaxResponseBodyBytes,
	}

	start := time.Now()
	rec := httptest.NewRecorder()
	srv.handleSuspend(rec, httptest.NewRequest(http.MethodPost, pathSuspend, nil))
	elapsed := time.Since(start)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Less(t, elapsed, 500*time.Millisecond,
		"handleSuspend with forwarder must return promptly when flush times out (≪ slow flusher's 1s)")
}

// TestHandleSuspend_WithForwarder_BodyBufferedBeforeFlush verifies that the
// response body from the user app is fully mirrored even when the parallel
// flush runs past the point where the forwardTimeout context would have
// expired. Without buffering, the forwardTimeout context deadline fires
// while flushAll is still running, cancels the underlying TCP connection,
// and mirrorResponse reads an empty or partial body. With buffering the body
// is read immediately after PassThrough returns — while the context is
// still alive — so mirrorResponse reads from an in-memory buffer that is
// context-independent.
func TestHandleSuspend_WithForwarder_BodyBufferedBeforeFlush(t *testing.T) {
	// Larger than the transport's read-ahead buffer so the client cannot fully
	// receive the body in the single read that happens while parsing headers —
	// finishing the read requires an additional network read against the
	// connection, which is what the forwardTimeout context cancellation breaks
	// when the read is delayed until after the flush.
	wantBody := strings.Repeat("A", 256*1024)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, wantBody)
	}))
	defer upstream.Close()

	srv, _, _, _, _, _ := newTestServer()
	// flushTimeout is longer than forwardTimeout so the parallel flush runs
	// past the point where the forwardTimeout context would have expired.
	srv.flushTimeout = 200 * time.Millisecond
	srv.logsFlusher = &slowLogsFlusher{block: 150 * time.Millisecond}
	srv.fwd = &Forwarder{
		target: upstream.URL,
		client: &http.Client{},
		// forwardTimeout is short: expires during the flush, which means the
		// cancelOnCloseReader's context deadline fires before mirrorResponse
		// reads the body — unless the body was already buffered.
		forwardTimeout:       100 * time.Millisecond,
		maxResponseBodyBytes: defaultMaxResponseBodyBytes,
	}

	rec := httptest.NewRecorder()
	srv.handleSuspend(rec, httptest.NewRequest(http.MethodPost, pathSuspend, nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, wantBody, rec.Body.String(),
		"response body must be fully mirrored even after parallel flush runs past forwardTimeout")
}

// Same as TestHandleSuspend_WithForwarder_FlushTimeout_ReturnsPromptly but for
// /terminate, which also calls handleWithForwarder with flushSequential.
func TestHandleTerminate_WithForwarder_FlushTimeout_ReturnsPromptly(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	srv, _, _, _, _, _ := newTestServer()
	srv.flushTimeout = 50 * time.Millisecond
	srv.logsFlusher = &slowLogsFlusher{block: 1 * time.Second}
	srv.fwd = &Forwarder{
		target:               upstream.URL,
		client:               &http.Client{},
		forwardTimeout:       2 * time.Second,
		maxResponseBodyBytes: defaultMaxResponseBodyBytes,
	}

	start := time.Now()
	rec := httptest.NewRecorder()
	srv.handleTerminate(rec, httptest.NewRequest(http.MethodPost, pathTerminate, nil))
	elapsed := time.Since(start)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Less(t, elapsed, 500*time.Millisecond,
		"handleTerminate with forwarder must return promptly when flush times out (≪ slow flusher's 1s)")
}

// TestHandleTerminate_WithForwarder_BodyBufferedBeforeFlush verifies that the
// response body from the user app is fully mirrored even when the sequential
// flush runs after PassThrough returns and the forwardTimeout context expires
// during the flush. Without buffering, the forwardTimeout context deadline
// fires mid-flush, cancels the underlying TCP connection, and mirrorResponse
// reads an empty or partial body. With buffering the body is read immediately
// after PassThrough returns — while the context is still alive — so
// mirrorResponse reads from an in-memory buffer that is context-independent.
func TestHandleTerminate_WithForwarder_BodyBufferedBeforeFlush(t *testing.T) {
	const wantBody = "terminate-response-body"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, wantBody)
	}))
	defer upstream.Close()

	srv, _, _, _, _, _ := newTestServer()
	// flushTimeout is longer than forwardTimeout so the sequential flush runs
	// past the point where the forwardTimeout context would have expired.
	srv.flushTimeout = 200 * time.Millisecond
	srv.logsFlusher = &slowLogsFlusher{block: 150 * time.Millisecond}
	srv.fwd = &Forwarder{
		target: upstream.URL,
		client: &http.Client{},
		// forwardTimeout is short: expires during the flush, which means the
		// cancelOnCloseReader's context deadline fires before mirrorResponse
		// reads the body — unless the body was already buffered.
		forwardTimeout:       100 * time.Millisecond,
		maxResponseBodyBytes: defaultMaxResponseBodyBytes,
	}

	rec := httptest.NewRecorder()
	srv.handleTerminate(rec, httptest.NewRequest(http.MethodPost, pathTerminate, nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, wantBody, rec.Body.String(),
		"response body must be fully mirrored even after sequential flush runs past forwardTimeout")
}

// /terminate with a forwarder waits for the user-app forward to complete
// before responding. Without this wait, /terminate's parallel
// flush-then-mirror would race against the platform's destruction of the VM
// at WriteHeader time.
func TestHandleTerminate_WithForwarder_WaitsForSlowForward(t *testing.T) {
	forwardEntered := make(chan struct{})
	releaseForward := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(forwardEntered)
		<-releaseForward
		w.WriteHeader(200)
	}))
	defer upstream.Close()

	srv, _, _, _, _, _ := newTestServer()
	srv.fwd = &Forwarder{
		target:               upstream.URL,
		client:               &http.Client{},
		forwardTimeout:       5 * time.Second,
		maxResponseBodyBytes: defaultMaxResponseBodyBytes,
	}

	handlerReturned := make(chan struct{})
	rec := httptest.NewRecorder()
	go func() {
		srv.handleTerminate(rec, httptest.NewRequest(http.MethodPost, pathTerminate, nil))
		close(handlerReturned)
	}()

	<-forwardEntered
	// Forward is mid-flight inside the upstream handler. handleTerminate MUST
	// NOT have returned yet — that would mean it skipped the forward wait.
	select {
	case <-handlerReturned:
		t.Fatal("handleTerminate returned before forward completed")
	case <-time.After(50 * time.Millisecond):
		// Good — handler is correctly blocked on the forward.
	}
	close(releaseForward)
	<-handlerReturned

	assert.Equal(t, http.StatusOK, rec.Code, "must mirror user-app's 200")
}

// Stop on a nil *Server is safe so callers can defer Stop unconditionally —
// the contract main.go's defer chain depends on for non-MicroVM modes.
func TestStopOnNilServerReturnsNil(t *testing.T) {
	var srv *Server
	require.NoError(t, srv.Stop(context.Background()))
}

// Stop on a constructed-but-never-Started server is also safe — http.Server.Shutdown
// is documented to return immediately when there are no listeners.
func TestStopWithoutStartReturnsNil(t *testing.T) {
	srv, _, _, _, _, _ := newTestServer()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	require.NoError(t, srv.Stop(ctx))
}

// TestServeAndStopGracefulShutdown exercises the real HTTP server lifecycle.
// It binds a listener on a random free port, serves until Stop is called,
// and verifies that Serve returns http.ErrServerClosed — the contract that
// lets main.go's defer chain run cleanly when shutdown is triggered externally.
func TestServeAndStopGracefulShutdown(t *testing.T) {
	srv, _, _, _, _, _ := newTestServer()
	// /ready needs an alive child handle to return 200; without one the
	// handler returns 503, but Serve+Stop semantics are independent of the
	// route's reply, so injecting an alive handle keeps the smoke check meaningful.
	h := newFakeChildHandle()
	h.alive.Store(true)
	srv.childHandle = h

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	serverDone := make(chan struct{})
	go func() {
		srv.Serve(listener)
		close(serverDone)
	}()

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
// WriteTimeout on the underlying http.Server.
//
// Without a forwarder, WriteTimeout is flushTimeout+writeTimeoutHeadroom (flush
// budget + heartbeat.Stop() + write headroom).
//
// With a forwarder, WriteTimeout is max(flushTimeout, forwardTimeout)+writeTimeoutHeadroom.
// The forwardTimeout (default 30s) exceeds the flush budget (default 5s), so
// the forwarder-path WriteTimeout must be sized to forwardTimeout+writeTimeoutHeadroom,
// not flushTimeout+writeTimeoutHeadroom. Otherwise the HTTP server closes the
// platform-facing connection before the handler can write the mirrored response
// for any user app that responds after the flush-only deadline.
func TestNewServerConfiguresHTTPTimeouts(t *testing.T) {
	flushTimeout := 5 * time.Second
	srv := NewServer(0, &mockFlusher{}, &mockFlusher{}, &mockLogsAgent{}, &mockMetricEmitter{}, &mockSampleDrainer{}, metrics.MetricSourceAWSMicroVMEnhanced, flushTimeout, nil, nil, nil)
	assert.Equal(t, 30*time.Second, srv.httpServer.ReadTimeout)
	assert.Equal(t, flushTimeout+writeTimeoutHeadroom, srv.httpServer.WriteTimeout)
}

// TestNewServerWithForwarderWriteTimeoutCoversForwardBudget verifies that when a
// Forwarder is configured, WriteTimeout accounts for the /terminate sequential
// flush path whose wall-clock is forwardTimeout+flushTimeout. This prevents the
// HTTP server from closing the platform-facing connection before the handler
// finishes forwarding and flushing.
func TestNewServerWithForwarderWriteTimeoutCoversForwardBudget(t *testing.T) {
	flushTimeout := 5 * time.Second
	fwd := &Forwarder{
		forwardTimeout:       30 * time.Second,
		client:               &http.Client{},
		maxResponseBodyBytes: defaultMaxResponseBodyBytes,
	}
	srv := NewServer(0, &mockFlusher{}, &mockFlusher{}, &mockLogsAgent{}, &mockMetricEmitter{}, &mockSampleDrainer{}, metrics.MetricSourceAWSMicroVMEnhanced, flushTimeout, nil, fwd, nil)
	assert.Equal(t, fwd.forwardTimeout+flushTimeout+writeTimeoutHeadroom, srv.httpServer.WriteTimeout,
		"WriteTimeout must cover forwardTimeout+flushTimeout (terminate sequential-flush path)")
}

// TestInstanceIDTagAppearsInMetricsAfterRun verifies that once /run stores a
// MicroVM instance ID, subsequent lifecycle metrics include lambda_microvm_id:<id> as
// an extra tag. This is the primary tagging path for identifying individual MicroVM
// instances in lifecycle metrics.
func TestInstanceIDTagAppearsInMetricsAfterRun(t *testing.T) {
	srv, _, _, _, emitter, _ := newTestServer()

	body := strings.NewReader(`{"microvmId":"vm-abc123"}`)
	srv.handleRun(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, pathRun, body))

	// Suspend after run — the suspend metric must carry the instance_id tag.
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
	assert.Contains(t, found.extraTags, lambdaMicroVMID+"vm-abc123")
}

// errorReader is a helper io.Reader that always returns the provided error.
// Used to simulate a body read failure in mirrorResponse tests.
type errorReader struct{ err error }

func (e *errorReader) Read(_ []byte) (int, error) { return 0, e.err }

// TestMirrorResponse_CopiesStatusContentTypeAndBody verifies the happy path:
// status code, Content-Type, and body are all forwarded to the platform.
func TestMirrorResponse_CopiesStatusContentTypeAndBody(t *testing.T) {
	resp := &http.Response{
		StatusCode: 207,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
	}
	rec := httptest.NewRecorder()
	mirrorResponse(rec, resp)
	assert.Equal(t, 207, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Equal(t, `{"ok":true}`, rec.Body.String())
}

// TestMirrorResponse_NoContentType_HeaderOmitted verifies that an absent
// Content-Type in the upstream response is not forwarded (no sniff-trigger).
func TestMirrorResponse_NoContentType_HeaderOmitted(t *testing.T) {
	resp := &http.Response{
		StatusCode: 200,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader("")),
	}
	rec := httptest.NewRecorder()
	mirrorResponse(rec, resp)
	assert.Equal(t, 200, rec.Code)
	assert.Empty(t, rec.Header().Get("Content-Type"))
}

// TestMirrorResponse_BodyReadError_DoesNotPanic verifies that a mid-body read
// failure (e.g. user app drops connection) is logged and does not panic. The
// status code is already committed by WriteHeader, so recovery is impossible;
// the test confirms the handler survives gracefully.
func TestMirrorResponse_BodyReadError_DoesNotPanic(t *testing.T) {
	resp := &http.Response{
		StatusCode: 200,
		Header:     http.Header{},
		Body:       io.NopCloser(&errorReader{err: errors.New("simulated read failure")}),
	}
	rec := httptest.NewRecorder()
	assert.NotPanics(t, func() { mirrorResponse(rec, resp) })
	assert.Equal(t, 200, rec.Code, "status already committed before body copy")
}

// --- dispatchHook unit tests ---
//
// These tests exercise dispatchHook directly, independent of which handler
// calls it, to pin the contract of the extracted shared path.

// TestDispatchHook_NoForwarder_WithFlushFalse emits a metric and returns 200
// without flushing (the run/resume path).
func TestDispatchHook_NoForwarder_WithFlushFalse_EmitsMetricReturns200NoFlush(t *testing.T) {
	srv, metric, trace, logs, emitter, drainer := newTestServer()
	rec := httptest.NewRecorder()
	srv.dispatchHook("test.metric", "/test", noFlush, rec, httptest.NewRequest(http.MethodPost, "/test", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, emitter.getEmitted(), "test.metric")
	assert.Equal(t, int32(0), metric.count.Load(), "noFlush must not flush")
	assert.Equal(t, int32(0), trace.count.Load())
	assert.Equal(t, int32(0), logs.count.Load())
	assert.Equal(t, int32(0), drainer.count.Load())
}

// TestDispatchHook_NoForwarder_WithFlushTrue emits a metric, flushes
// telemetry, and returns 200 (the suspend/terminate path).
func TestDispatchHook_NoForwarder_WithFlushTrue_EmitsMetricAndFlushes(t *testing.T) {
	srv, metric, trace, logs, emitter, drainer := newTestServer()
	rec := httptest.NewRecorder()
	srv.dispatchHook("test.metric", "/test", flushParallel, rec, httptest.NewRequest(http.MethodPost, "/test", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, emitter.getEmitted(), "test.metric")
	assert.Equal(t, int32(1), metric.count.Load(), "flushParallel must flush metric agent")
	assert.Equal(t, int32(1), trace.count.Load())
	assert.Equal(t, int32(1), logs.count.Load())
	assert.Equal(t, int32(1), drainer.count.Load())
}

// TestDispatchHook_WithForwarder_DelegatesToHandleParallel verifies that when
// a forwarder is configured, dispatchHook mirrors the user app's response and
// emits the metric (via handleWithForwarder). The no-forwarder 200 path must NOT
// fire.
func TestDispatchHook_WithForwarder_MirrorsUserAppResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(418)
	}))
	defer upstream.Close()

	srv, _, _, _, emitter, _ := newTestServer()
	srv.fwd = &Forwarder{
		target:               upstream.URL,
		client:               &http.Client{},
		forwardTimeout:       2 * time.Second,
		maxResponseBodyBytes: defaultMaxResponseBodyBytes,
	}

	rec := httptest.NewRecorder()
	srv.dispatchHook("test.metric", pathResume, noFlush, rec, httptest.NewRequest(http.MethodPost, pathResume, nil))
	assert.Equal(t, 418, rec.Code, "must mirror user-app status, not hardcoded 200")
	assert.Contains(t, emitter.getEmitted(), "test.metric")
}

// TestFlushAllDrainTimeoutDoesNotBlock verifies that flushAll returns within the flush
// timeout even when the sample drainer never completes. This guards against the
// lifecycle server stalling — and blocking the MicroVM platform's suspend/terminate
// handshake — when the metric aggregator worker is deadlocked or slow.
func TestFlushAllDrainTimeoutDoesNotBlock(t *testing.T) {
	srv := NewServer(0, &mockFlusher{}, &mockFlusher{}, &mockLogsAgent{}, &mockMetricEmitter{}, &neverDrainer{}, metrics.MetricSourceAWSMicroVMEnhanced, 50*time.Millisecond, nil, nil, nil)

	ctx, cancel := context.WithTimeout(context.Background(), srv.flushTimeout)
	defer cancel()
	start := time.Now()
	srv.flushAll(ctx)
	assert.Less(t, time.Since(start), 500*time.Millisecond, "flushAll must return within flushTimeout even when drainer blocks")
}

// withFakeHeartbeat installs a real *Heartbeat with a long interval that
// will never tick during the test, lets us observe Start/Stop side effects
// via running goroutine count, and returns a teardown that ensures cleanup.
// This indirection avoids re-testing heartbeat internals while still
// exercising server.go's wiring with a production *Heartbeat type.
func withFakeHeartbeat(t *testing.T, srv *Server) (started func() bool, teardown func()) {
	t.Helper()
	emitter := &mockMetricEmitter{}
	hb := NewHeartbeat(time.Hour /* never ticks during the test */, emitter, metrics.MetricSourceAWSMicroVMEnhanced, nil)
	srv.heartbeat = hb
	started = func() bool {
		hb.mu.Lock()
		defer hb.mu.Unlock()
		return hb.cancel != nil
	}
	teardown = func() { hb.Stop() }
	return started, teardown
}

func TestHandleRun_StartsHeartbeat(t *testing.T) {
	srv, _, _, _, _, _ := newTestServer()
	started, teardown := withFakeHeartbeat(t, srv)
	defer teardown()

	rec := httptest.NewRecorder()
	srv.handleRun(rec, httptest.NewRequest(http.MethodPost, pathRun, nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, started(), "/run must start the heartbeat")
}

// /run must extract the MicroVM ID from the JSON body and apply it to the
// heartbeat before Start so the very first emission carries the correct
// microvm_id. The test calls handleRun then inspects the tags that the
// heartbeat would emit on its next tick.
func TestHandleRun_AppliesMicroVMIDFromBody(t *testing.T) {
	srv, _, _, _, _, _ := newTestServer()
	_, teardown := withFakeHeartbeat(t, srv)
	defer teardown()

	body := strings.NewReader(`{"microvmId":"vm-from-body"}`)
	srv.handleRun(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, pathRun, body))

	assert.Contains(t, srv.heartbeat.tagsForEmit(), "microvm_id:vm-from-body")
	id := srv.instanceID.Load()
	assert.Equal(t, "vm-from-body", id)
}

// When the platform body does not include microvmId, the heartbeat keeps the
// "unknown" placeholder rather than crashing or emitting an empty value.
func TestHandleRun_MissingBodyIDUsesUnknown(t *testing.T) {
	srv, _, _, _, _, _ := newTestServer()
	_, teardown := withFakeHeartbeat(t, srv)
	defer teardown()

	srv.handleRun(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, pathRun, nil))

	assert.Contains(t, srv.heartbeat.tagsForEmit(), "microvm_id:unknown")
}

// traced_invocations is emitted by the Heartbeat on each tick, not directly by
// handleRun. This test verifies the separation of concerns: the server's own
// emitter must never receive traced_invocations. Billing tag correctness is
// covered in heartbeat_test.go.
func TestHandleRun_ServerDoesNotDirectlyEmitTracedInvocations(t *testing.T) {
	srv, _, _, _, emitter, _ := newTestServer()
	_, teardown := withFakeHeartbeat(t, srv)
	defer teardown()

	body := strings.NewReader(`{"microvmId":"vm-abc123"}`)
	srv.handleRun(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, pathRun, body))

	for _, m := range emitter.getEmittedMetrics() {
		assert.NotEqual(t, activeInstancesMetricName, m.name,
			"server must not directly emit active_instances; that is the heartbeat's responsibility")
	}
}

func TestHandleSuspend_StopsHeartbeat(t *testing.T) {
	srv, _, _, _, _, _ := newTestServer()
	started, teardown := withFakeHeartbeat(t, srv)
	defer teardown()
	srv.heartbeat.Start() // simulate post-run state

	rec := httptest.NewRecorder()
	srv.handleSuspend(rec, httptest.NewRequest(http.MethodPost, pathSuspend, nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.False(t, started(), "/suspend must stop the heartbeat")
}

func TestHandleResume_RestartsHeartbeat(t *testing.T) {
	srv, _, _, _, _, _ := newTestServer()
	started, teardown := withFakeHeartbeat(t, srv)
	defer teardown()
	// Simulate suspend (heartbeat stopped) before resume.
	srv.heartbeat.Start()
	srv.heartbeat.Stop()
	require.False(t, started(), "precondition: heartbeat must be stopped before resume")

	rec := httptest.NewRecorder()
	srv.handleResume(rec, httptest.NewRequest(http.MethodPost, pathResume, nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, started(), "/resume must restart the heartbeat after suspend")
}

func TestHandleTerminate_StopsHeartbeat(t *testing.T) {
	srv, _, _, _, _, _ := newTestServer()
	started, teardown := withFakeHeartbeat(t, srv)
	defer teardown()
	srv.heartbeat.Start()

	rec := httptest.NewRecorder()
	srv.handleTerminate(rec, httptest.NewRequest(http.MethodPost, pathTerminate, nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.False(t, started(), "/terminate must stop the heartbeat")
}

// Server.Stop is a defense-in-depth path for shutdowns that don't first
// route through /suspend or /terminate — for example, the platform SIGKILLs
// the agent or main.go's defer chain fires for an unrelated reason.
func TestServerStop_AlsoStopsHeartbeat(t *testing.T) {
	srv, _, _, _, _, _ := newTestServer()
	started, teardown := withFakeHeartbeat(t, srv)
	defer teardown()
	srv.heartbeat.Start()
	require.True(t, started())

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	require.NoError(t, srv.Stop(ctx))

	assert.False(t, started(), "Server.Stop must also stop the heartbeat")
}

// ---------------------------------------------------------------------------
// Log tag setter tests
// ---------------------------------------------------------------------------

// mockLogsTagSetter records every SetLogsTags call for assertions.
type mockLogsTagSetter struct {
	mu    sync.Mutex
	calls [][]string
}

func (m *mockLogsTagSetter) SetLogsTags(tags []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, slices.Clone(tags))
}

func (m *mockLogsTagSetter) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

func (m *mockLogsTagSetter) lastCall() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.calls) == 0 {
		return nil
	}
	return slices.Clone(m.calls[len(m.calls)-1])
}

func (m *mockLogsTagSetter) allCalls() [][]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([][]string, len(m.calls))
	for i, c := range m.calls {
		out[i] = slices.Clone(c)
	}
	return out
}

// TestLogsTagSetterFunc_CallsWrappedFunction verifies that LogsTagSetterFunc
// delegates to the underlying function when SetLogsTags is called.
func TestLogsTagSetterFunc_CallsWrappedFunction(t *testing.T) {
	var received []string
	fn := LogsTagSetterFunc(func(tags []string) { received = tags })
	fn.SetLogsTags([]string{"env:prod", "region:us-east-1"})
	assert.Equal(t, []string{"env:prod", "region:us-east-1"}, received)
}

// TestSetLogsTagSetter_WiresFields verifies that SetLogsTagSetter stores both
// the setter and the base-tag snapshot on the server.
func TestSetLogsTagSetter_WiresFields(t *testing.T) {
	srv, _, _, _, _, _ := newTestServer()
	setter := &mockLogsTagSetter{}
	baseTags := []string{"region:us-east-1"}
	srv.SetLogsTagSetter(setter, baseTags)
	assert.Equal(t, setter, srv.logsTagSetter)
	assert.Equal(t, baseTags, srv.baseTags)
}

// TestHandleRun_UpdatesLogTagsWithMicroVMID is the primary feature test:
// /run with a microvmId body calls SetLogsTags with baseTags + lambdaMicroVMID + id.
func TestHandleRun_UpdatesLogTagsWithMicroVMID(t *testing.T) {
	srv, _, _, _, _, _ := newTestServer()
	setter := &mockLogsTagSetter{}
	srv.SetLogsTagSetter(setter, []string{"env:prod", "region:us-east-1"})

	body := strings.NewReader(`{"microvmId":"vm-abc123"}`)
	srv.handleRun(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, pathRun, body))

	require.Equal(t, 1, setter.callCount(), "SetLogsTags must be called exactly once on /run")
	assert.Equal(t, []string{"env:prod", "region:us-east-1", lambdaMicroVMID + "vm-abc123"}, setter.lastCall())
}

// TestHandleRun_NoMicroVmID_DoesNotUpdateLogTags verifies that when the platform
// sends /run with no microvmId, SetLogsTags is not called — the tag pipeline
// should not be updated with an unknown value.
func TestHandleRun_NoMicroVmID_DoesNotUpdateLogTags(t *testing.T) {
	srv, _, _, _, _, _ := newTestServer()
	setter := &mockLogsTagSetter{}
	srv.SetLogsTagSetter(setter, []string{"env:prod"})

	srv.handleRun(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, pathRun, nil))

	assert.Equal(t, 0, setter.callCount(), "SetLogsTags must not be called when microvmId is absent")
}

// TestHandleRun_NilLogsTagSetter_DoesNotPanic verifies nil-safety: a server
// constructed without SetLogsTagSetter must not panic when /run fires.
func TestHandleRun_NilLogsTagSetter_DoesNotPanic(t *testing.T) {
	srv, _, _, _, _, _ := newTestServer()
	// logsTagSetter is nil by default

	body := strings.NewReader(`{"microvmId":"vm-abc123"}`)
	assert.NotPanics(t, func() {
		srv.handleRun(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, pathRun, body))
	})
}

// TestHandleRun_BaseTagsNotMutated verifies the safe-append contract: each
// call to handleRun produces an independent slice and does not modify the
// baseTags stored on the server. This guards against the naive
// append(s.baseTags, ...) pattern which can corrupt baseTags when the slice
// has spare capacity.
func TestHandleRun_BaseTagsNotMutated(t *testing.T) {
	srv, _, _, _, _, _ := newTestServer()
	setter := &mockLogsTagSetter{}
	baseTags := []string{"env:prod", "service:foo"}
	originalBase := slices.Clone(baseTags)
	srv.SetLogsTagSetter(setter, baseTags)

	body1 := strings.NewReader(`{"microvmId":"vm-first"}`)
	srv.handleRun(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, pathRun, body1))
	assert.Equal(t, originalBase, baseTags, "handleRun must not mutate the baseTags slice")

	// Simulate a second /run (e.g. resumed from snapshot with a new ID) to
	// confirm each call produces an independent result.
	body2 := strings.NewReader(`{"microvmId":"vm-second"}`)
	srv.handleRun(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, pathRun, body2))

	calls := setter.allCalls()
	require.Len(t, calls, 2)
	assert.Equal(t, []string{"env:prod", "service:foo", lambdaMicroVMID + "vm-first"}, calls[0])
	assert.Equal(t, []string{"env:prod", "service:foo", lambdaMicroVMID + "vm-second"}, calls[1])
}

// TestHandleRun_EmptyBaseTags_AppendsMicroVMIDOnly verifies that when the
// server is started with no base tags, the resulting tag slice contains only
// the microvm_id tag (not an empty leading element).
func TestHandleRun_EmptyBaseTags_AppendsMicroVMIDOnly(t *testing.T) {
	srv, _, _, _, _, _ := newTestServer()
	setter := &mockLogsTagSetter{}
	srv.SetLogsTagSetter(setter, nil)

	body := strings.NewReader(`{"microvmId":"vm-solo"}`)
	srv.handleRun(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, pathRun, body))

	require.Equal(t, 1, setter.callCount())
	assert.Equal(t, []string{lambdaMicroVMID + "vm-solo"}, setter.lastCall())
}

// TestHandleRun_WithForwarder_UpdatesLogTags verifies that the log tag update
// fires even when a user-app forwarder is configured. handleRun parses the
// body and calls the setter before delegating to handleWithForwarder.
func TestHandleRun_WithForwarder_UpdatesLogTags(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	srv, _, _, _, _, _ := newTestServer()
	srv.fwd = &Forwarder{
		target:               upstream.URL,
		client:               &http.Client{},
		forwardTimeout:       2 * time.Second,
		maxResponseBodyBytes: defaultMaxResponseBodyBytes,
	}
	setter := &mockLogsTagSetter{}
	srv.SetLogsTagSetter(setter, []string{"env:staging"})

	body := strings.NewReader(`{"microvmId":"vm-fwd456"}`)
	srv.handleRun(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, pathRun, body))

	require.Equal(t, 1, setter.callCount(), "SetLogsTags must be called even when forwarder is configured")
	assert.Equal(t, []string{"env:staging", lambdaMicroVMID + "vm-fwd456"}, setter.lastCall())
}

// ---------------------------------------------------------------------------
// Trace tag setter tests
// ---------------------------------------------------------------------------

// mockTraceTagSetter records every SetTraceTags call for assertions.
type mockTraceTagSetter struct {
	mu    sync.Mutex
	calls []map[string]string
}

func (m *mockTraceTagSetter) SetTraceTags(tags map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make(map[string]string, len(tags))
	for k, v := range tags {
		cp[k] = v
	}
	m.calls = append(m.calls, cp)
}

func (m *mockTraceTagSetter) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

func (m *mockTraceTagSetter) lastCall() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.calls) == 0 {
		return nil
	}
	return m.calls[len(m.calls)-1]
}

// TestTraceTagSetterFunc_CallsWrappedFunction verifies that TraceTagSetterFunc
// delegates to the underlying function when SetTraceTags is called.
func TestTraceTagSetterFunc_CallsWrappedFunction(t *testing.T) {
	var received map[string]string
	fn := TraceTagSetterFunc(func(tags map[string]string) { received = tags })
	fn.SetTraceTags(map[string]string{"env": "prod"})
	assert.Equal(t, map[string]string{"env": "prod"}, received)
}

// TestSetTraceTagSetter_WiresFields verifies that SetTraceTagSetter stores both
// the setter and the base trace tag snapshot on the server.
func TestSetTraceTagSetter_WiresFields(t *testing.T) {
	srv, _, _, _, _, _ := newTestServer()
	setter := &mockTraceTagSetter{}
	base := map[string]string{"env": "prod", "region": "us-east-1"}
	srv.SetTraceTagSetter(setter, base)
	assert.Equal(t, setter, srv.traceTagSetter)
	assert.Equal(t, base, srv.baseTraceTags)
}

// TestHandleRun_UpdatesTraceTagsWithMicroVMID is the primary feature test:
// /run with a microvmId body calls SetTraceTags with baseTraceTags + lambda_microvm_id.
func TestHandleRun_UpdatesTraceTagsWithMicroVMID(t *testing.T) {
	srv, _, _, _, _, _ := newTestServer()
	setter := &mockTraceTagSetter{}
	srv.SetTraceTagSetter(setter, map[string]string{"env": "prod", "region": "us-east-1"})

	body := strings.NewReader(`{"microvmId":"vm-abc123"}`)
	srv.handleRun(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, pathRun, body))

	require.Equal(t, 1, setter.callCount(), "SetTraceTags must be called exactly once on /run")
	got := setter.lastCall()
	assert.Equal(t, "vm-abc123", got["lambda_microvm_id"])
	assert.Equal(t, "prod", got["env"])
	assert.Equal(t, "us-east-1", got["region"])
}

// TestHandleRun_NoMicroVmID_DoesNotUpdateTraceTags verifies that when the
// platform sends /run with no microvmId, SetTraceTags is not called.
func TestHandleRun_NoMicroVmID_DoesNotUpdateTraceTags(t *testing.T) {
	srv, _, _, _, _, _ := newTestServer()
	setter := &mockTraceTagSetter{}
	srv.SetTraceTagSetter(setter, map[string]string{"env": "prod"})

	srv.handleRun(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, pathRun, nil))

	assert.Equal(t, 0, setter.callCount(), "SetTraceTags must not be called when microvmId is absent")
}

// TestHandleRun_NilTraceTagSetter_DoesNotPanic verifies nil-safety: a server
// constructed without SetTraceTagSetter must not panic when /run fires.
func TestHandleRun_NilTraceTagSetter_DoesNotPanic(t *testing.T) {
	srv, _, _, _, _, _ := newTestServer()

	body := strings.NewReader(`{"microvmId":"vm-abc123"}`)
	assert.NotPanics(t, func() {
		srv.handleRun(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, pathRun, body))
	})
}

// TestHandleRun_NilBaseTraceTags_DoesNotPanic verifies that a non-nil
// traceTagSetter wired with a nil baseTraceTags map (the default when no
// trace tags are configured, e.g. MakeTraceAgentTags passes through a nil
// input unchanged) does not panic on /run. maps.Clone(nil) returns nil, so
// writing the microvm_id tag into it would otherwise panic with "assignment
// to entry in nil map".
func TestHandleRun_NilBaseTraceTags_DoesNotPanic(t *testing.T) {
	srv, _, _, _, _, _ := newTestServer()
	setter := &mockTraceTagSetter{}
	srv.SetTraceTagSetter(setter, nil)

	body := strings.NewReader(`{"microvmId":"vm-abc123"}`)
	assert.NotPanics(t, func() {
		srv.handleRun(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, pathRun, body))
	})
	assert.Equal(t, map[string]string{"lambda_microvm_id": "vm-abc123"}, setter.lastCall())
}

// TestHandleRun_BaseTraceTagsNotMutated verifies the safe-copy contract: each
// /run call produces an independent map and does not modify baseTraceTags.
func TestHandleRun_BaseTraceTagsNotMutated(t *testing.T) {
	srv, _, _, _, _, _ := newTestServer()
	setter := &mockTraceTagSetter{}
	base := map[string]string{"env": "prod"}
	srv.SetTraceTagSetter(setter, base)

	body1 := strings.NewReader(`{"microvmId":"vm-first"}`)
	srv.handleRun(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, pathRun, body1))

	assert.Equal(t, map[string]string{"env": "prod"}, base, "handleRun must not mutate baseTraceTags")
	assert.Equal(t, "vm-first", setter.lastCall()["lambda_microvm_id"])

	// A second /run (e.g. resume from snapshot with new ID) must produce an
	// independent result without leaking the first ID into baseTraceTags.
	body2 := strings.NewReader(`{"microvmId":"vm-second"}`)
	srv.handleRun(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, pathRun, body2))

	assert.Equal(t, map[string]string{"env": "prod"}, base, "baseTraceTags must still be unmodified after second /run")
	assert.Equal(t, "vm-second", setter.lastCall()["lambda_microvm_id"])
}

// TestHandleRun_WithForwarder_UpdatesTraceTags verifies that the trace tag
// update fires even when a user-app forwarder is configured.
func TestHandleRun_WithForwarder_UpdatesTraceTags(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	srv, _, _, _, _, _ := newTestServer()
	srv.fwd = &Forwarder{
		target:               upstream.URL,
		client:               &http.Client{},
		forwardTimeout:       2 * time.Second,
		maxResponseBodyBytes: defaultMaxResponseBodyBytes,
	}
	setter := &mockTraceTagSetter{}
	srv.SetTraceTagSetter(setter, map[string]string{"env": "staging"})

	body := strings.NewReader(`{"microvmId":"vm-fwd789"}`)
	srv.handleRun(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, pathRun, body))

	require.Equal(t, 1, setter.callCount(), "SetTraceTags must be called even when forwarder is configured")
	assert.Equal(t, "vm-fwd789", setter.lastCall()["lambda_microvm_id"])
}
