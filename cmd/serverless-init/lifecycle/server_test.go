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
	"os"
	"os/signal"
	"strings"
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
	// port 0 — handler-level tests don't bind. Tests that need a childHandle
	// or heartbeat assign srv.childHandle / srv.heartbeat after construction.
	srv := NewServer(0, metric, trace, logs, emitter, drainer, metrics.MetricSourceAWSMicroVMEnhanced, 2*time.Second, nil, nil)
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

func (f *fakeChildHandle) IsAlive() bool { return f.alive.Load() }

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
// WriteTimeout on the underlying http.Server. WriteTimeout is flushTimeout plus
// writeTimeoutHeadroom, so heartbeat.Stop() (called before flushAll on /suspend
// and /terminate) and response-write jitter can't cause a false-negative
// timeout after the flush itself has already completed.
func TestNewServerConfiguresHTTPTimeouts(t *testing.T) {
	flushTimeout := 5 * time.Second
	srv := NewServer(0, &mockFlusher{}, &mockFlusher{}, &mockLogsAgent{}, &mockMetricEmitter{}, &mockSampleDrainer{}, metrics.MetricSourceAWSMicroVMEnhanced, flushTimeout, nil, nil)
	assert.Equal(t, 30*time.Second, srv.httpServer.ReadTimeout)
	assert.Equal(t, flushTimeout+writeTimeoutHeadroom, srv.httpServer.WriteTimeout)
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

// --- dispatchHook unit tests ---
//
// These tests exercise dispatchHook directly, independent of which handler
// calls it, to pin the contract of the extracted shared path.

// TestDispatchHook_NoForwarder_WithFlushFalse emits a metric and returns 200
// without flushing (the run/resume path).
func TestDispatchHook_NoForwarder_WithFlushFalse_EmitsMetricReturns200NoFlush(t *testing.T) {
	srv, metric, trace, logs, emitter, drainer := newTestServer()
	rec := httptest.NewRecorder()
	srv.dispatchHook("test.metric", noFlush, rec)
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
	srv.dispatchHook("test.metric", flushParallel, rec)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, emitter.getEmitted(), "test.metric")
	assert.Equal(t, int32(1), metric.count.Load(), "flushParallel must flush metric agent")
	assert.Equal(t, int32(1), trace.count.Load())
	assert.Equal(t, int32(1), logs.count.Load())
	assert.Equal(t, int32(1), drainer.count.Load())
}

// TestFlushAllDrainTimeoutDoesNotBlock verifies that flushAll returns within the flush
// timeout even when the sample drainer never completes. This guards against the
// lifecycle server stalling — and blocking the MicroVM platform's suspend/terminate
// handshake — when the metric aggregator worker is deadlocked or slow.
func TestFlushAllDrainTimeoutDoesNotBlock(t *testing.T) {
	srv := NewServer(0, &mockFlusher{}, &mockFlusher{}, &mockLogsAgent{}, &mockMetricEmitter{}, &neverDrainer{}, metrics.MetricSourceAWSMicroVMEnhanced, 50*time.Millisecond, nil, nil)

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
