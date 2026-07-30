// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttelemetryimpl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.uber.org/atomic"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	logconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log/errortracking"
)

// captureClient records every Do(req) call's request and body, and returns
// configurable responses. The existing clientMock in agenttelemetry_test.go
// only stores the LAST body; we need every body to assert single-POST
// semantics for batches.
type captureClient struct {
	mu sync.Mutex

	requests []*http.Request
	bodies   [][]byte

	statusCode int
	err        error
}

func (c *captureClient) Do(req *http.Request) (*http.Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.requests = append(c.requests, req)
	if req.Body != nil {
		body, _ := io.ReadAll(req.Body)
		c.bodies = append(c.bodies, body)
	} else {
		c.bodies = append(c.bodies, nil)
	}

	if c.err != nil {
		return nil, c.err
	}
	status := c.statusCode
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Body:       io.NopCloser(strings.NewReader("")),
		Header:     make(http.Header),
	}, nil
}

func (c *captureClient) snapshot() ([]*http.Request, [][]byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	reqs := append([]*http.Request{}, c.requests...)
	bodies := make([][]byte, 0, len(c.bodies))
	for _, b := range c.bodies {
		if b == nil {
			bodies = append(bodies, nil)
			continue
		}
		cp := make([]byte, len(b))
		copy(cp, b)
		bodies = append(bodies, cp)
	}
	return reqs, bodies
}

// newTestSender constructs a senderImpl wired to a single in-memory
// endpoint, bypassing the heavyweight newSenderImpl path that would
// require a full config.Component and BuildHTTPEndpointsWithConfig call.
// The fields populated here are exactly the ones sendLogsBatch and
// sendPayload read.
func newTestSender(t *testing.T, cl client) *senderImpl {
	t.Helper()
	main := logconfig.NewEndpoint("test-api-key", "", "instrumentation-telemetry-intake.datad0g.com", 0, "", true)
	return &senderImpl{
		logComp: logmock.New(t),
		client:  cl,
		endpoints: &logconfig.Endpoints{
			Endpoints: []logconfig.Endpoint{main},
		},
		agentVersion: "test-7.79.0-devel",
		payloadTemplate: Payload{
			APIVersion: "v2",
			DebugFlag:  false,
			Host:       HostPayload{Hostname: "x"},
		},
	}
}

// TestSendPayloadBody_StatusCodes locks the contract of the extracted
// shared transport helper (review comment C3 on PR #49946). Both
// flushSession and sendLogsBatch route per-endpoint POSTs through
// sendPayload, which in turn calls this helper; the helper
// must return the raw HTTP status code regardless of value so the
// uniform Debug-log policy in sendPayload can apply
// (non-2xx is observability noise, only transport errors propagate).
func TestSendPayloadBody_StatusCodes(t *testing.T) {
	cases := []struct {
		name   string
		status int
	}{
		{"2xx success", http.StatusOK},
		{"4xx terminal", http.StatusBadRequest},
		{"5xx retryable", http.StatusInternalServerError},
		{"request timeout", http.StatusRequestTimeout},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cl := &captureClient{statusCode: tc.status}
			s := newTestSender(t, cl)
			status, err := s.sendPayloadBody(context.Background(),
				[]byte(`{"k":"v"}`), "logs", "test-key",
				"https://example.invalid/api/v2/apmtelemetry", false)
			require.NoError(t, err)
			assert.Equal(t, tc.status, status)

			reqs, bodies := cl.snapshot()
			require.Len(t, reqs, 1)
			assert.Equal(t, "logs", reqs[0].Header.Get("DD-Telemetry-request-type"))
			assert.Equal(t, "test-key", reqs[0].Header.Get("DD-Api-Key"))
			assert.Equal(t, "application/json", reqs[0].Header.Get("Content-Type"))
			assert.Equal(t, `{"k":"v"}`, string(bodies[0]))
		})
	}
}

func TestSendPayloadBody_NetworkError(t *testing.T) {
	cl := &captureClient{err: errors.New("simulated network down")}
	s := newTestSender(t, cl)
	status, err := s.sendPayloadBody(context.Background(),
		[]byte("{}"), "logs", "key", "https://example.invalid/path", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulated network down")
	assert.Equal(t, 0, status, "network failure must surface as status=0")
}

// =============================================================================
// atel-level errortracking tests (buffered channel + flush + enrichErrorLog)
// =============================================================================

// TestErrorLogToLog_WireShape locks the wire payload shape: stack-derived
// data (StackTrace, ErrorKind), git source integration Tags, Level,
// TracerTime, and defaults are populated. Message, TraceID, SpanID stay
// empty — the handler does not capture user-controlled inputs.
func TestErrorLogToLog_WireShape(t *testing.T) {
	in := errortracking.ErrorLog{
		Time: time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC),
	}
	in.PCsLen = runtime.Callers(1, in.PCs[:])
	require.GreaterOrEqual(t, in.PCsLen, 1)
	in.PC = in.PCs[0]

	out := enrichErrorLog(in)

	// Stack-derived + defaults: present on the wire.
	assert.Equal(t, LogLevelError, out.Level)
	assert.Equal(t, in.Time.Unix(), out.TracerTime)
	assert.Equal(t, 1, out.Count)
	assert.False(t, out.IsCrash)
	assert.NotEmpty(t, out.StackTrace, "captured PCs must produce a stack_trace")
	assert.Empty(t, out.ErrorKind,
		"ErrorKind must be empty when no error attribute is present in the log record")

	// Git source integration tags.
	assert.Contains(t, out.Tags, "git.repository_url:https://github.com/DataDog/datadog-agent",
		"Tags must carry git.repository_url for Source Code Integration")

	// Origin tag: identifies which agent binary emitted the log, since this
	// pipeline is shared across core agent, cluster-agent, process-agent, etc.
	assert.Contains(t, out.Tags, "agent.flavor:"+flavor.GetFlavor(),
		"Tags must carry agent.flavor so COAT can attribute errors to the emitting binary")

	// Wire schema fields that are intentionally not populated.
	assert.Empty(t, out.Message, "Message must NOT be on the wire")
	assert.Empty(t, out.TraceID, "TraceID must NOT be on the wire")
	assert.Empty(t, out.SpanID, "SpanID must NOT be on the wire")
}

func TestErrorLogToLog_NoPC_EmptyStackTrace(t *testing.T) {
	in := errortracking.ErrorLog{
		Time: time.Now(),
		PC:   0,
	}
	out := enrichErrorLog(in)
	assert.Empty(t, out.StackTrace, "no captured PCs must produce empty stack_trace")
	assert.Empty(t, out.ErrorKind, "no captured PCs must produce empty error_kind")
}

// TestErrorLogToLog_MultiFramePCs locks the multi-frame stack format.
// Each frame produces two lines in Go's standard panic/debug.Stack shape:
//
//	github.com/DataDog/datadog-agent/.../Func
//		/path/to/file.go:42 +0x1a4
//
// The Error Tracking parser requires this format for frame extraction.
func TestErrorLogToLog_MultiFramePCs(t *testing.T) {
	in := errortracking.ErrorLog{
		Time: time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC),
	}
	in.PCsLen = runtime.Callers(1, in.PCs[:])
	require.GreaterOrEqual(t, in.PCsLen, 2,
		"this test must capture at least 2 frames (test func + testing harness)")

	out := enrichErrorLog(in)

	require.NotEmpty(t, out.StackTrace, "multi-frame PCs must produce non-empty stack_trace")
	lines := strings.Split(out.StackTrace, "\n")
	// Each frame produces 2 lines: function name + "\tfile:line +0xaddr".
	// Between frames a blank separator '\n' is emitted, so total lines >= 2*frames.
	require.GreaterOrEqual(t, len(lines), 4,
		"at least 2 frames → at least 4 lines in stack_trace")

	// Odd-indexed lines (0, 2, ...) are function names — no leading tab.
	// Even-indexed lines (1, 3, ...) are file:line — leading tab + ":<digits> +0x".
	// Source files may be .go or .s (assembly); both are valid.
	for i, line := range lines {
		if i%2 == 0 {
			assert.NotEmpty(t, line, "function line (index %d) must not be empty", i)
			assert.NotContains(t, line, "\t", "function line must not have a leading tab, got %q", line)
		} else {
			assert.True(t, strings.HasPrefix(line, "\t"),
				"file:line line (index %d) must start with tab, got %q", i, line)
			assert.Contains(t, line, "+0x",
				"file:line line must contain entry offset (+0x), got %q", line)
		}
	}
	// First line is the fully-qualified function name from this test.
	assert.Contains(t, lines[0], "TestErrorLogToLog_MultiFramePCs")
	// Second line is the file:line for this test file.
	assert.Contains(t, lines[1], "errortracking_sender_test.go")

	frames := runtime.CallersFrames(in.PCs[:in.PCsLen])
	firstFrame, _ := frames.Next()
	expectedOffset := uintptr(0)
	if firstFrame.PC >= firstFrame.Entry {
		expectedOffset = firstFrame.PC - firstFrame.Entry
	}
	gotOffset, err := parseStackOffset(lines[1])
	require.NoError(t, err)
	assert.Equal(t, expectedOffset, gotOffset, "stack suffix must be PC-Entry offset")
	assert.Less(t, gotOffset, uintptr(math.MaxInt32), "stack suffix must be a small in-function offset, not a raw code address")
}

func parseStackOffset(line string) (uintptr, error) {
	idx := strings.LastIndex(line, "+0x")
	if idx == -1 {
		return 0, fmt.Errorf("missing +0x suffix in %q", line)
	}
	v, err := strconv.ParseUint(line[idx+3:], 16, 64)
	if err != nil {
		return 0, err
	}
	return uintptr(v), nil
}

// errorLog is a convenience for the atel-level tests below.
func errorLog(_ string) errortracking.ErrorLog {
	return errortracking.ErrorLog{
		Time: time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC),
	}
}

// newTestAtelMinimal builds a minimal *atel just enough for the
// per-record entry-point tests: enabled=true, sender wired, cancelCtx
// initialised, errLogsCh present. Tests drive flushErrortracking
// directly rather than scheduling it via the runner.
func newTestAtelMinimal(t *testing.T, sndr sender, bufSize int) *atel {
	t.Helper()
	a := &atel{
		enabled:              true,
		errortrackingEnabled: true,
		logComp:              logmock.New(t),
		sender:               sndr,
		errLogsCh:            make(chan errortracking.ErrorLog, bufSize),
		errLogsDropped:       atomic.NewUint64(0),
		errLogsFlushInterval: 60 * time.Second,
		shutdownDrainTimeout: 5 * time.Second,
	}
	a.cancelCtx, a.cancel = context.WithCancel(context.Background())
	return a
}

// TestSubmitErrorLog_DisabledNoOp: a disabled atel must accept calls
// and drop them silently (no panic, no enqueue, no drop counter bump —
// because we never reached the enqueue path).
func TestSubmitErrorLog_DisabledNoOp(t *testing.T) {
	a := &atel{enabled: false, errLogsDropped: atomic.NewUint64(0)}
	a.SubmitErrorLog(errorLog("dropped"))
	assert.Equal(t, uint64(0), a.errLogsDropped.Load())
}

// TestSubmitErrorLog_AcceptsRecord_PCZero: a record carrying PC=0
// (the common case for caller-PC-less origins, e.g. synthetic test
// inputs) must be enqueued normally and reach the sender on flush.
// Regression coverage for the positive-path enqueue contract previously
// folded into the now-removed recursion-guard tests. Drives via the
// public SubmitErrorLog + fake-sender observation rather than
// reading a.errLogsCh directly (pducolin's "skip internals from
// testing" comment on PR #50607).
func TestSubmitErrorLog_AcceptsRecord_PCZero(t *testing.T) {
	sm := &senderMock{}
	a := newTestAtelMinimal(t, sm, 8)
	defer a.cancel()

	a.SubmitErrorLog(errorLog("from-external"))

	a.flushErrortracking(context.Background())

	assert.Len(t, sm.capturedLogs(), 1,
		"PC=0 record must be enqueued and flushed to the sender")
}

// TestSubmitErrorLog_FeatureDisabled_NoChannel: when
// agent_telemetry.errortracking.enabled is false (default), or when
// gov/FIPS exclusion blocks the parent agent_telemetry feature,
// SubmitErrorLog must be a no-op and the underlying channel must not
// be allocated. This is the gating contract: deployments that don't opt
// in pay zero overhead (no buffer, no idle flush goroutine).
func TestSubmitErrorLog_FeatureDisabled_NoChannel(t *testing.T) {
	// Simulate the "createAtel saw errortracking gate=false" shape:
	// enabled (agenttelemetry) is true, errortrackingEnabled is false,
	// errLogsCh is left as the zero value (nil).
	a := &atel{
		enabled:              true,
		errortrackingEnabled: false,
		logComp:              logmock.New(t),
		errLogsDropped:       atomic.NewUint64(0),
	}

	assert.Nil(t, a.errLogsCh, "feature-disabled atel must not allocate the channel")

	// Calling SubmitErrorLog must be a no-op: the errLogsCh==nil
	// guard short-circuits before the select, so no panic and no drop
	// counter bump (we never reached the enqueue path).
	assert.NotPanics(t, func() {
		a.SubmitErrorLog(errorLog("ignored"))
	})
	assert.Equal(t, uint64(0), a.errLogsDropped.Load(),
		"feature-disabled drops are not overflow drops; counter must stay at 0")
}

// TestSubmitErrorLog_NonBlocking_DropsOnOverflow: when the bounded
// channel is full, SubmitErrorLog MUST drop silently (NOT block) and
// increment the drop counter. The hot path is the slog handler — it
// cannot block on a slow or stuck backend.
func TestSubmitErrorLog_NonBlocking_DropsOnOverflow(t *testing.T) {
	a := newTestAtelMinimal(t, &senderMock{}, 2)
	defer a.cancel()

	a.SubmitErrorLog(errorLog("one"))
	a.SubmitErrorLog(errorLog("two"))
	// Channel full (cap=2). The next two must be dropped silently.
	a.SubmitErrorLog(errorLog("drop-1"))
	a.SubmitErrorLog(errorLog("drop-2"))

	assert.Equal(t, 2, len(a.errLogsCh), "buffer should still hold exactly cap records")
	assert.Equal(t, uint64(2), a.errLogsDropped.Load(),
		"both overflow submits MUST be counted as drops")
}

// TestFlushErrortracking_DrainsWholeBufferInOneCall: flushErrortracking
// drains every record currently buffered and dispatches them as ONE
// HTTP call. The pre-batchSize-removal behaviour split a large drain
// across multiple sendLogsBatch invocations; after T3 the contract
// is "the entire flush is a single typed-logs POST" regardless of
// in-channel count.
func TestFlushErrortracking_DrainsWholeBufferInOneCall(t *testing.T) {
	sm := &senderMock{}
	const total = 250
	a := newTestAtelMinimal(t, sm, total)
	defer a.cancel()

	for i := 0; i < total; i++ {
		a.SubmitErrorLog(errorLog(fmt.Sprintf("r-%d", i)))
	}

	a.flushErrortracking(context.Background())

	got := sm.capturedLogs()
	require.Len(t, got, total, "every enqueued record must be dispatched in one drain pass")
	require.Equal(t, 1, sm.sendLogsCalls(),
		"drain must dispatch the entire buffer in exactly one sendLogsBatch call; multiple calls would indicate a regression to per-batch dispatch")
	for _, log := range got {
		// Per-record identification by Message was removed when Message
		// stopped shipping — it is always empty on the wire.
		assert.Empty(t, log.Message)
		assert.Equal(t, LogLevelError, log.Level)
	}
}

// TestFlushErrortracking_DrainIsBoundedToSnapshot: a flush must dispatch
// only what was queued at start. Retries until a refill is confirmed
// mid-drain, so a slow CI schedule can't pass without exercising it.
func TestFlushErrortracking_DrainIsBoundedToSnapshot(t *testing.T) {
	sm := &senderMock{}
	const bufSize = 10
	a := newTestAtelMinimal(t, sm, bufSize)
	defer a.cancel()

	var refills atomic.Int64
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
				a.SubmitErrorLog(errorLog("hot"))
				refills.Add(1)
			}
		}
	})
	t.Cleanup(func() {
		close(stop)
		wg.Wait()
	})

	deadline := time.Now().Add(10 * time.Second)
	for {
		require.False(t, time.Now().After(deadline),
			"producer never landed a refill while a flush was draining, across repeated attempts")

		for len(a.errLogsCh) < bufSize {
			runtime.Gosched()
		}

		before := refills.Load()
		logsBefore := len(sm.capturedLogs())
		callsBefore := sm.sendLogsCalls()
		done := make(chan struct{})
		go func() {
			defer close(done)
			a.flushErrortracking(context.Background())
		}()

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("flush did not return while a producer kept refilling the channel; the drain is not bounded to a fixed snapshot")
		}

		require.Equal(t, bufSize, len(sm.capturedLogs())-logsBefore,
			"flush must dispatch exactly the snapshot count (channel capacity, %d), not chase concurrent refills", bufSize)
		assert.Equal(t, callsBefore+1, sm.sendLogsCalls(),
			"snapshot-bounded flush must dispatch in exactly one sendLogsBatch call")

		if refills.Load() > before {
			return // this attempt proved the producer actually raced the drain
		}
	}
}

// TestFlushErrortracking_LateArrivalsWaitForNextTick: records enqueued
// after a flush has already returned must not be lost — they are picked
// up by the following flush rather than requiring the batch that already
// shipped to somehow retroactively include them.
func TestFlushErrortracking_LateArrivalsWaitForNextTick(t *testing.T) {
	sm := &senderMock{}
	const bufSize = 10
	a := newTestAtelMinimal(t, sm, bufSize)
	defer a.cancel()

	const preFilled = 5
	for i := range preFilled {
		a.SubmitErrorLog(errorLog(fmt.Sprintf("pre-%d", i)))
	}
	a.flushErrortracking(context.Background())

	const late = 3
	for i := range late {
		a.SubmitErrorLog(errorLog(fmt.Sprintf("late-%d", i)))
	}
	a.flushErrortracking(context.Background())

	assert.Len(t, sm.capturedLogs(), preFilled+late,
		"items enqueued after the first flush must be picked up by the next one")
	assert.Equal(t, 2, sm.sendLogsCalls(),
		"the late arrivals must be dispatched in their own batch")
}

// TestFlushErrortracking_FinalDrain: records enqueued shortly before
// shutdown must be picked up by the final flushErrortracking call in
// atel.stop. The atel.stop ordering is:
//
//  1. submitter/bouncer slots cleared (producers stop)
//  2. a.cancel() + runner.stop() (in-flight tick completes/cancels)
//  3. flushErrortracking with a fresh background-derived ctx
//
// This test exercises step 3 directly: records are buffered and the
// final drain dispatches them as one batch.
func TestFlushErrortracking_FinalDrain(t *testing.T) {
	sm := &senderMock{}
	a := newTestAtelMinimal(t, sm, 16)
	defer a.cancel()

	a.SubmitErrorLog(errorLog("pending-1"))
	a.SubmitErrorLog(errorLog("pending-2"))
	a.SubmitErrorLog(errorLog("pending-3"))

	shutdownCtx, cancelDrain := context.WithTimeout(context.Background(), a.shutdownDrainTimeout)
	defer cancelDrain()
	a.flushErrortracking(shutdownCtx)

	got := sm.capturedLogs()
	require.Len(t, got, 3, "all pending records must be flushed on stop")
	// Per-record identification by Message was removed when Message
	// stopped shipping. Count-and-level is the strongest assertion
	// available; the order check is implicit in the channel FIFO.
	for _, log := range got {
		assert.Empty(t, log.Message)
		assert.Equal(t, LogLevelError, log.Level)
	}
}

// ctxObservingSender records the cancellation state of the context it
// receives, so a test can assert the shutdown drain was given a live
// context (not the already-canceled lifecycle context).
type ctxObservingSender struct {
	senderMock
	mu            sync.Mutex
	ctxCanceledOn []bool
	ctxDeadlineOn []bool
}

func (s *ctxObservingSender) sendLogsBatch(ctx context.Context, logs []Log) error {
	s.mu.Lock()
	canceled := false
	select {
	case <-ctx.Done():
		canceled = true
	default:
	}
	_, hasDeadline := ctx.Deadline()
	s.ctxCanceledOn = append(s.ctxCanceledOn, canceled)
	s.ctxDeadlineOn = append(s.ctxDeadlineOn, hasDeadline)
	s.mu.Unlock()
	return s.senderMock.sendLogsBatch(ctx, logs)
}

// TestFlushErrortracking_ShutdownCtxIsLive: the shutdown-path drain
// must dispatch with a fresh background-derived context that has a
// timeout — not the already-canceled a.cancelCtx — so HTTP POSTs in
// the final drain are not pre-canceled. Regression for the "shutdown
// drain sends with an already-canceled context" review thread.
//
// Models atel.stop's ordering: cancel lifecycle context, then drive
// the final flush with a derived ctx that has a bounded timeout.
func TestFlushErrortracking_ShutdownCtxIsLive(t *testing.T) {
	sm := &ctxObservingSender{}
	a := newTestAtelMinimal(t, sm, 16)

	a.SubmitErrorLog(errorLog("pre-stop"))

	// Simulate the lifecycle cancel; the shutdown-drain ctx is built
	// from background, NOT from a.cancelCtx.
	a.cancel()

	shutdownCtx, cancelDrain := context.WithTimeout(context.Background(), a.shutdownDrainTimeout)
	defer cancelDrain()
	a.flushErrortracking(shutdownCtx)

	sm.mu.Lock()
	defer sm.mu.Unlock()
	require.Len(t, sm.ctxCanceledOn, 1, "shutdown drain must dispatch exactly one batch")
	assert.False(t, sm.ctxCanceledOn[0],
		"shutdown-drain ctx must NOT be canceled at dispatch time")
	assert.True(t, sm.ctxDeadlineOn[0],
		"shutdown-drain ctx must carry the bounded timeout deadline")
}

// BenchmarkSymbolizeStack measures the sender-side cost of walking the
// captured PCs and producing the multi-line "file:line\tfunc"
// stack_trace string the dd-go intake schema expects. Symbolization is
// deferred from log-time to flush-time on purpose — runtime.CallersFrames
// performs symbol-table lookups — so this is the only place that cost is
// paid. The bench captures a real on-stack PC list so the symbol-table
// entries are valid and the work is representative.
func BenchmarkSymbolizeStack(b *testing.B) {
	var pcs [errortracking.MaxStackFrames]uintptr
	n := runtime.Callers(1, pcs[:])
	in := errortracking.ErrorLog{
		Time:   time.Now(),
		PC:     pcs[0],
		PCs:    pcs,
		PCsLen: n,
	}
	var sink string
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink = symbolizeStackFrames(in)
	}
	// Defeat dead-store elimination so the compiler cannot prove sink is
	// unused and elide the call.
	runtime.KeepAlive(sink)
}
