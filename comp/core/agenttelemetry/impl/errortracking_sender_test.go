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
	"log/slog"
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

// TestSlogLevelToLogLevel_ErrorAndAbove: the only contract the function
// honors is "Level >= Error -> LogLevelError". The pkg/util/log
// errortracking handler filters everything lower before dispatch, so
// the mapping never sees Warn/Info/Debug in practice (review comment F5
// on PR #50607).
func TestSlogLevelToLogLevel_ErrorAndAbove(t *testing.T) {
	cases := []struct {
		in   slog.Level
		want LogLevel
	}{
		{slog.LevelError, LogLevelError},
		{slog.LevelError + 4, LogLevelError},
	}
	for _, c := range cases {
		got := slogLevelToLogLevel(c.in)
		assert.Equalf(t, c.want, got, "level %v -> %s", c.in, c.want)
	}
}

// TestSlogLevelToLogLevel_BelowErrorPanics: lower levels are a contract
// violation -- they cannot reach the wire mapping because the handler
// filters them. Panic loudly so a future regression is caught in tests
// instead of producing an invalid wire payload (review comment F5).
func TestSlogLevelToLogLevel_BelowErrorPanics(t *testing.T) {
	for _, lvl := range []slog.Level{slog.LevelWarn, slog.LevelInfo, slog.LevelDebug, slog.LevelDebug - 4} {
		lvl := lvl
		t.Run(lvl.String(), func(t *testing.T) {
			require.Panics(t, func() { slogLevelToLogLevel(lvl) })
		})
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
// v3 tests (atel-owned buffered channel + flush + recursion guard, errorLogToLog)
// =============================================================================

// TestErrorLogToLog_FieldMapping exercises the DTO -> wire conversion:
// reserved trace_id / span_id extraction, alphabetically-ordered CSV
// tags, file:line stack_trace from PC, and the v3 defaults
// (count=1, is_crash=false, uppercase level).
func TestErrorLogToLog_FieldMapping(t *testing.T) {
	var pcs [1]uintptr
	n := runtime.Callers(1, pcs[:])
	require.Equal(t, 1, n)
	pc := pcs[0]

	in := errortracking.ErrorLog{
		Time:    time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC),
		Level:   slog.LevelError,
		Message: "boom",
		PC:      pc,
		Attrs: []slog.Attr{
			slog.String("c", "3"),
			slog.String("a", "1"),
			slog.String("trace_id", "abc-trace"),
			slog.String("b", "2"),
			slog.String("span_id", "abc-span"),
		},
	}
	out := errorLogToLog(in)

	assert.Equal(t, "boom", out.Message)
	assert.Equal(t, LogLevelError, out.Level)
	assert.Equal(t, in.Time.Unix(), out.TracerTime)
	assert.Equal(t, 1, out.Count)
	assert.False(t, out.IsCrash)
	assert.Equal(t, "abc-trace", out.TraceID)
	assert.Equal(t, "abc-span", out.SpanID)
	assert.Equal(t, "a:1,b:2,c:3", out.Tags,
		"reserved attrs trace_id/span_id MUST be extracted; remaining attrs alphabetical CSV")
	assert.NotEmpty(t, out.StackTrace, "non-zero PC must produce file:line stack_trace")
}

func TestErrorLogToLog_NoPC_EmptyStackTrace(t *testing.T) {
	in := errortracking.ErrorLog{
		Time:    time.Now(),
		Level:   slog.LevelError,
		Message: "no pc",
		PC:      0,
	}
	out := errorLogToLog(in)
	assert.Equal(t, "", out.StackTrace, "PC=0 must produce empty stack_trace")
}

// errorLog is a convenience for the atel-level tests below.
func errorLog(msg string, attrs ...slog.Attr) errortracking.ErrorLog {
	return errortracking.ErrorLog{
		Time:    time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC),
		Level:   slog.LevelError,
		Message: msg,
		Attrs:   attrs,
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
		enabled:   true,
		logComp:   logmock.New(t),
		sender:    sndr,
		errLogsCh: make(chan errortracking.ErrorLog, bufSize),
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
// inputs) must be enqueued normally. Regression coverage for the
// positive-path enqueue contract previously folded into the now-removed
// recursion-guard tests.
func TestSubmitErrorRecord_AcceptsRecord_PCZero(t *testing.T) {
	a := newTestAtelMinimal(t, &senderMock{}, 8)
	defer a.cancel()

	a.SubmitErrorRecord(errorLog("from-external"))
	assert.Equal(t, 1, len(a.errLogsCh))
	assert.Equal(t, uint64(0), a.errLogsDropped.Load())
}

// TestSubmitErrorRecord_FeatureDisabled_NoChannel: when
// errortracking.enabled is false (default), SubmitErrorRecord must be a
// no-op and the underlying channel must not be allocated. This is the
// F2 gating contract: deployments that don't opt in pay zero overhead
// (no ~80KB buffer, no idle flush goroutine).
func TestSubmitErrorRecord_FeatureDisabled_NoChannel(t *testing.T) {
	// Simulate the "createAtel saw errortracking.enabled=false" shape:
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
// must drain the channel in batches of errLogsBatchSize and dispatch
// each via sender.sendLogsTypedBatch.
func TestDrainAndSend_BatchesAndDispatches(t *testing.T) {
	sm := &senderMock{}
	a := newTestAtelMinimal(t, sm, errLogsBatchSize*3)
	defer a.cancel()

	// Enqueue 2.5 batches' worth of records.
	total := errLogsBatchSize*2 + errLogsBatchSize/2
	for i := 0; i < total; i++ {
		a.SubmitErrorRecord(errorLog(fmt.Sprintf("r-%d", i)))
	}

	batch := make([]errortracking.ErrorLog, 0, errLogsBatchSize)
	a.drainAndSend(&batch)

	got := sm.capturedLogs()
	require.Len(t, got, total, "every enqueued record must be dispatched in one drain pass")
	for i, log := range got {
		assert.Equal(t, fmt.Sprintf("r-%d", i), log.Message)
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
	assert.Equal(t, "pending-1", got[0].Message)
	assert.Equal(t, "pending-2", got[1].Message)
	assert.Equal(t, "pending-3", got[2].Message)
}
