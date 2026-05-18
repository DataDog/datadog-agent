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
	"net/http"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	logconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
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
// The fields populated here are exactly the ones sendLogsTypedBatch and
// sendSerializedPayload read.
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
// flushSession and sendLogsTypedBatch route per-endpoint POSTs through
// sendSerializedPayload, which in turn calls this helper; the helper
// must return the raw HTTP status code regardless of value so the
// uniform Debug-log policy in sendSerializedPayload can apply
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
// atel-level errortracking tests (buffered channel + flush + errorLogToLog)
// =============================================================================

// TestErrorLogToLog_WireShape locks the wire payload shape: only
// stack-derived data (StackTrace) + Level + TracerTime + defaults are
// populated. The schema fields Message, Tags, TraceID, SpanID stay on
// Log so the dd-go intake sees the canonical shape, but they emit
// empty — the handler does not capture user-controlled inputs.
func TestErrorLogToLog_WireShape(t *testing.T) {
	in := errortracking.ErrorLog{
		Time: time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC),
	}
	in.PCsLen = runtime.Callers(1, in.PCs[:])
	require.GreaterOrEqual(t, in.PCsLen, 1)
	in.PC = in.PCs[0]

	out := errorLogToLog(in)

	// Stack-derived + defaults: present on the wire.
	assert.Equal(t, LogLevelError, out.Level)
	assert.Equal(t, in.Time.Unix(), out.TracerTime)
	assert.Equal(t, 1, out.Count)
	assert.False(t, out.IsCrash)
	assert.NotEmpty(t, out.StackTrace, "captured PCs must produce file:line stack_trace")

	// Wire schema fields that are intentionally not populated.
	assert.Empty(t, out.Message, "Message must NOT be on the wire")
	assert.Empty(t, out.Tags, "Tags must NOT be on the wire")
	assert.Empty(t, out.TraceID, "TraceID must NOT be on the wire")
	assert.Empty(t, out.SpanID, "SpanID must NOT be on the wire")
}

func TestErrorLogToLog_NoPC_EmptyStackTrace(t *testing.T) {
	in := errortracking.ErrorLog{
		Time: time.Now(),
		PC:   0,
	}
	out := errorLogToLog(in)
	assert.Empty(t, out.StackTrace, "no captured PCs must produce empty stack_trace")
}

// TestErrorLogToLog_MultiFramePCs locks the multi-frame stack format
// (PR #50607 C7 / iglendd's "stack offset + frame limit" + pducolin's
// "add function's name" threads): file:line\tfunc per line, newline
// separated, function-named, bounded by the PCs array size.
func TestErrorLogToLog_MultiFramePCs(t *testing.T) {
	in := errortracking.ErrorLog{
		Time: time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC),
	}
	in.PCsLen = runtime.Callers(1, in.PCs[:])
	require.GreaterOrEqual(t, in.PCsLen, 2,
		"this test must capture at least 2 frames (test func + testing harness)")

	out := errorLogToLog(in)

	require.NotEmpty(t, out.StackTrace, "multi-frame PCs must produce non-empty stack_trace")
	lines := strings.Split(out.StackTrace, "\n")
	assert.GreaterOrEqual(t, len(lines), 2,
		"multi-frame stack_trace must be newline-separated, one frame per line")
	for _, line := range lines {
		parts := strings.SplitN(line, "\t", 2)
		require.Len(t, parts, 2, "each frame must be file:line\\tfunc, got %q", line)
		assert.Contains(t, parts[0], ":", "frame file part must be file:line, got %q", parts[0])
		assert.NotEmpty(t, parts[1], "frame func part must be non-empty, got %q", line)
	}
	// First frame should be in this test file (we captured at depth 1).
	assert.Contains(t, lines[0], "errortracking_sender_test.go")
}

// errorLog is a convenience for the atel-level tests below.
func errorLog(_ string) errortracking.ErrorLog {
	return errortracking.ErrorLog{
		Time: time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC),
	}
}

// newTestAtelMinimal builds a minimal *atel just enough for the
// per-record entry-point tests: enabled=true, sender wired, cancelCtx
// initialised, errLogsCh present. Does NOT spawn the flush goroutine;
// tests that need that lifecycle call atel.runErrorLogsFlush themselves
// (see TestFlushJob_DrainsOnStop).
func newTestAtelMinimal(t *testing.T, sndr sender, bufSize int) *atel {
	t.Helper()
	a := &atel{
		enabled:              true,
		logComp:              logmock.New(t),
		sender:               sndr,
		errLogsCh:            make(chan errortracking.ErrorLog, bufSize),
		errLogsBatchSize:     defaultErrLogsBatchSize,
		errLogsFlushInterval: defaultErrLogsFlushInterval,
		shutdownDrainTimeout: 5 * time.Second,
	}
	a.cancelCtx, a.cancel = context.WithCancel(context.Background())
	return a
}

// TestSubmitErrorRecord_DisabledNoOp: a disabled atel must accept calls
// and drop them silently (no panic, no enqueue, no drop counter bump —
// because we never reached the enqueue path).
func TestSubmitErrorRecord_DisabledNoOp(t *testing.T) {
	a := &atel{enabled: false}
	a.SubmitErrorRecord(errorLog("dropped"))
	assert.Equal(t, uint64(0), a.errLogsDropped.Load())
}

// TestSubmitErrorRecord_AcceptsRecord_PCZero: a record carrying PC=0
// (the common case for caller-PC-less origins, e.g. synthetic test
// inputs) must be enqueued normally and reach the sender on flush.
// Regression coverage for the positive-path enqueue contract previously
// folded into the now-removed recursion-guard tests. Drives via the
// public SubmitErrorRecord + fake-sender observation rather than
// reading a.errLogsCh directly (pducolin's "skip internals from
// testing" comment on PR #50607).
func TestSubmitErrorRecord_AcceptsRecord_PCZero(t *testing.T) {
	sm := &senderMock{}
	a := newTestAtelMinimal(t, sm, 8)
	defer a.cancel()

	a.SubmitErrorRecord(errorLog("from-external"))

	batch := make([]errortracking.ErrorLog, 0, a.errLogsBatchSize)
	a.drainAndSend(context.Background(), &batch)

	assert.Len(t, sm.capturedLogs(), 1,
		"PC=0 record must be enqueued and flushed to the sender")
}

// TestSubmitErrorRecord_FeatureDisabled_NoChannel: when
// agent_telemetry.errortracking.enabled is false (default), or when
// gov/FIPS exclusion blocks the parent agent_telemetry feature,
// SubmitErrorRecord must be a no-op and the underlying channel must not
// be allocated. This is the gating contract: deployments that don't opt
// in pay zero overhead (no buffer, no idle flush goroutine).
func TestSubmitErrorRecord_FeatureDisabled_NoChannel(t *testing.T) {
	// Simulate the "createAtel saw errortracking gate=false" shape:
	// enabled (agenttelemetry) is true, errortrackingEnabled is false,
	// errLogsCh is left as the zero value (nil).
	a := &atel{
		enabled:              true,
		errortrackingEnabled: false,
		logComp:              logmock.New(t),
	}

	assert.Nil(t, a.errLogsCh, "feature-disabled atel must not allocate the channel")

	// Calling SubmitErrorRecord must be a no-op: the errLogsCh==nil
	// guard short-circuits before the select, so no panic and no drop
	// counter bump (we never reached the enqueue path).
	assert.NotPanics(t, func() {
		a.SubmitErrorRecord(errorLog("ignored"))
	})
	assert.Equal(t, uint64(0), a.errLogsDropped.Load(),
		"feature-disabled drops are not overflow drops; counter must stay at 0")
}

// TestSubmitErrorRecord_NonBlocking_DropsOnOverflow: when the bounded
// channel is full, SubmitErrorRecord MUST drop silently (NOT block) and
// increment the drop counter. The hot path is the slog handler — it
// cannot block on a slow or stuck backend.
func TestSubmitErrorRecord_NonBlocking_DropsOnOverflow(t *testing.T) {
	a := newTestAtelMinimal(t, &senderMock{}, 2)
	defer a.cancel()

	a.SubmitErrorRecord(errorLog("one"))
	a.SubmitErrorRecord(errorLog("two"))
	// Channel full (cap=2). The next two must be dropped silently.
	a.SubmitErrorRecord(errorLog("drop-1"))
	a.SubmitErrorRecord(errorLog("drop-2"))

	assert.Equal(t, 2, len(a.errLogsCh), "buffer should still hold exactly cap records")
	assert.Equal(t, uint64(2), a.errLogsDropped.Load(),
		"both overflow submits MUST be counted as drops")
}

// TestDrainAndSend_BatchesAndDispatches: drainAndSend, called once,
// must drain the channel in batches of defaultErrLogsBatchSize and dispatch
// each via sender.sendLogsTypedBatch.
func TestDrainAndSend_BatchesAndDispatches(t *testing.T) {
	sm := &senderMock{}
	a := newTestAtelMinimal(t, sm, defaultErrLogsBatchSize*3)
	defer a.cancel()

	// Enqueue 2.5 batches' worth of records.
	total := defaultErrLogsBatchSize*2 + defaultErrLogsBatchSize/2
	for i := 0; i < total; i++ {
		a.SubmitErrorRecord(errorLog(fmt.Sprintf("r-%d", i)))
	}

	batch := make([]errortracking.ErrorLog, 0, defaultErrLogsBatchSize)
	a.drainAndSend(context.Background(), &batch)

	got := sm.capturedLogs()
	require.Len(t, got, total, "every enqueued record must be dispatched in one drain pass")
	for _, log := range got {
		// Per-record identification by Message was removed with the PR
		// #50607 PII pivot — Message is always empty on the wire.
		assert.Empty(t, log.Message)
		assert.Equal(t, LogLevelError, log.Level)
	}
}

// TestFlushJob_DrainsOnStop: the flush goroutine, on cancelCtx.Done,
// performs one final drain so records buffered just before stop are
// still dispatched. Test by spawning runErrorLogsFlush, enqueueing,
// cancelling, and waiting on errLogsFlushWG.
func TestFlushJob_DrainsOnStop(t *testing.T) {
	sm := &senderMock{}
	a := newTestAtelMinimal(t, sm, 16)

	a.errLogsFlushWG.Add(1)
	go a.runErrorLogsFlush()

	a.SubmitErrorRecord(errorLog("pending-1"))
	a.SubmitErrorRecord(errorLog("pending-2"))
	a.SubmitErrorRecord(errorLog("pending-3"))

	a.cancel()
	a.errLogsFlushWG.Wait()

	got := sm.capturedLogs()
	require.Len(t, got, 3, "all pending records must be flushed on stop")
	// Per-record identification by Message was removed with the PR
	// #50607 PII pivot. Count-and-level is the strongest assertion
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

func (s *ctxObservingSender) sendLogsTypedBatch(ctx context.Context, logs []Log) error {
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
	return s.senderMock.sendLogsTypedBatch(ctx, logs)
}

// TestFlushJob_ShutdownDrainUsesFreshContext: the cancel-branch drain
// must dispatch with a fresh background-derived context that has a
// timeout — not the already-canceled a.cancelCtx — so HTTP POSTs in the
// final drain are not pre-canceled. Regression for louis-cqrl's
// "shutdown drain sends with an already-canceled context" thread on
// PR #50607.
func TestFlushJob_ShutdownDrainUsesFreshContext(t *testing.T) {
	sm := &ctxObservingSender{}
	a := newTestAtelMinimal(t, sm, 16)

	a.errLogsFlushWG.Add(1)
	go a.runErrorLogsFlush()

	a.SubmitErrorRecord(errorLog("pre-stop"))

	a.cancel()
	a.errLogsFlushWG.Wait()

	sm.mu.Lock()
	defer sm.mu.Unlock()
	require.Len(t, sm.ctxCanceledOn, 1, "shutdown drain must dispatch exactly one batch")
	assert.False(t, sm.ctxCanceledOn[0],
		"shutdown-drain ctx must NOT be canceled at dispatch time")
	assert.True(t, sm.ctxDeadlineOn[0],
		"shutdown-drain ctx must carry the bounded timeout deadline")
}
