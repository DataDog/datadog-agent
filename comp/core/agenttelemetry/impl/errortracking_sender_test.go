// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agenttelemetryimpl

import (
	"context"
	"encoding/json"
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
// The fields populated here are exactly the ones sendLogsBatch reads.
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

func errorRecord(msg string, attrs ...slog.Attr) slog.Record {
	r := slog.NewRecord(time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC), slog.LevelError, msg, 0)
	if len(attrs) > 0 {
		r.AddAttrs(attrs...)
	}
	return r
}

// envelopeForTest is a decode-only shadow of Payload that intentionally
// does NOT trigger Payload's custom UnmarshalJSON method (which is defined
// in agenttelemetry_test.go and only knows the metric/event request types).
// We need to read the wire bytes our sender produced for "logs" — a third
// request type — so this shadow gives us field access without that
// validation.
type envelopeForTest struct {
	APIVersion  string          `json:"api_version"`
	RequestType string          `json:"request_type"`
	EventTime   int64           `json:"event_time"`
	DebugFlag   bool            `json:"debug"`
	Host        HostPayload     `json:"host"`
	Payload     json.RawMessage `json:"payload"`
}

// decodeLogsPayload decodes a request body into the apmtelemetry envelope
// and the inner LogsPayload.
func decodeLogsPayload(t *testing.T, body []byte) (envelopeForTest, LogsPayload) {
	t.Helper()
	var env envelopeForTest
	require.NoError(t, json.Unmarshal(body, &env))
	var lp LogsPayload
	require.NoError(t, json.Unmarshal(env.Payload, &lp))
	return env, lp
}

func TestSendErrorLogs_OneRecord(t *testing.T) {
	cl := &captureClient{}
	s := newTestSender(t, cl)

	require.NoError(t, s.sendLogsBatch(context.Background(), []slog.Record{
		errorRecord("single-record"),
	}))

	reqs, bodies := cl.snapshot()
	require.Len(t, reqs, 1)
	require.Len(t, bodies, 1)

	// Headers
	assert.Equal(t, "logs", reqs[0].Header.Get("DD-Telemetry-request-type"))
	assert.Equal(t, "v2", reqs[0].Header.Get("DD-Telemetry-api-version"))
	assert.Equal(t, "test-api-key", reqs[0].Header.Get("DD-Api-Key"))
	assert.Equal(t, "application/json", reqs[0].Header.Get("Content-Type"))
	assert.Equal(t, "agent", reqs[0].Header.Get("DD-Telemetry-Product"))

	env, lp := decodeLogsPayload(t, bodies[0])
	assert.Equal(t, "v2", env.APIVersion)
	assert.Equal(t, "logs", env.RequestType)
	assert.NotZero(t, env.EventTime)

	require.Len(t, lp.Logs, 1)
	assert.Equal(t, "single-record", lp.Logs[0].Message)
	assert.Equal(t, LogLevelError, lp.Logs[0].Level)
	assert.Equal(t, 1, lp.Logs[0].Count)
	assert.False(t, lp.Logs[0].IsCrash)
}

func TestSendErrorLogs_BatchOfThirty(t *testing.T) {
	cl := &captureClient{}
	s := newTestSender(t, cl)

	batch := make([]slog.Record, 30)
	for i := range batch {
		batch[i] = errorRecord(fmt.Sprintf("m-%d", i))
	}
	require.NoError(t, s.sendLogsBatch(context.Background(), batch))

	reqs, bodies := cl.snapshot()
	require.Len(t, reqs, 1, "batch must produce exactly one POST; chunking is the Pipeline's concern")

	_, lp := decodeLogsPayload(t, bodies[0])
	require.Len(t, lp.Logs, 30)
	for i, log := range lp.Logs {
		assert.Equal(t, fmt.Sprintf("m-%d", i), log.Message)
	}
}

func TestSendErrorLogs_LevelMapping(t *testing.T) {
	cases := []struct {
		in   slog.Level
		want LogLevel
	}{
		{slog.LevelError, LogLevelError},
		{slog.LevelError + 4, LogLevelError},
		{slog.LevelWarn, LogLevelWarn},
		{slog.LevelInfo, LogLevelInfo},
		{slog.LevelDebug, LogLevelDebug},
		{slog.LevelDebug - 4, LogLevelDebug},
	}
	for _, c := range cases {
		got := slogLevelToLogLevel(c.in)
		assert.Equalf(t, c.want, got, "level %v → %s", c.in, c.want)
	}
}

func TestSendErrorLogs_TraceIDExtraction(t *testing.T) {
	cl := &captureClient{}
	s := newTestSender(t, cl)

	rec := errorRecord("with-trace",
		slog.String("trace_id", "abc-123"),
		slog.String("span_id", "span-7"),
		slog.String("svc", "frontend"),
	)
	require.NoError(t, s.sendLogsBatch(context.Background(), []slog.Record{rec}))

	_, bodies := cl.snapshot()
	_, lp := decodeLogsPayload(t, bodies[0])

	assert.Equal(t, "abc-123", lp.Logs[0].TraceID)
	assert.Equal(t, "span-7", lp.Logs[0].SpanID)
	assert.NotContains(t, lp.Logs[0].Tags, "trace_id")
	assert.NotContains(t, lp.Logs[0].Tags, "span_id")
	assert.Contains(t, lp.Logs[0].Tags, "svc:frontend")
}

func TestSendErrorLogs_TagsCSVOrdering(t *testing.T) {
	cl := &captureClient{}
	s := newTestSender(t, cl)

	rec := errorRecord("ordered",
		slog.String("c", "3"),
		slog.String("a", "1"),
		slog.String("b", "2"),
	)
	require.NoError(t, s.sendLogsBatch(context.Background(), []slog.Record{rec}))

	_, bodies := cl.snapshot()
	_, lp := decodeLogsPayload(t, bodies[0])

	assert.Equal(t, "a:1,b:2,c:3", lp.Logs[0].Tags,
		"tags MUST be CSV-encoded in alphabetical key order for deterministic wire output")
}

func TestSendErrorLogs_StackTraceFromPC(t *testing.T) {
	cl := &captureClient{}
	s := newTestSender(t, cl)

	// Capture the current PC so we have a known caller location.
	var pcs [1]uintptr
	n := runtime.Callers(1, pcs[:])
	require.Equal(t, 1, n)
	pc := pcs[0]

	rWithPC := slog.NewRecord(time.Now().UTC(), slog.LevelError, "with-pc", pc)
	rNoPC := slog.NewRecord(time.Now().UTC(), slog.LevelError, "no-pc", 0)

	require.NoError(t, s.sendLogsBatch(context.Background(), []slog.Record{rWithPC, rNoPC}))

	_, bodies := cl.snapshot()
	_, lp := decodeLogsPayload(t, bodies[0])

	assert.NotEmpty(t, lp.Logs[0].StackTrace, "non-zero PC should resolve to file:line")
	assert.Contains(t, lp.Logs[0].StackTrace, ":")
	assert.Equal(t, "", lp.Logs[1].StackTrace, "PC=0 should produce empty stack_trace")
}

func TestSendErrorLogs_Empty_NoOp(t *testing.T) {
	cl := &captureClient{}
	s := newTestSender(t, cl)

	require.NoError(t, s.sendLogsBatch(context.Background(), nil))
	require.NoError(t, s.sendLogsBatch(context.Background(), []slog.Record{}))

	reqs, _ := cl.snapshot()
	assert.Empty(t, reqs, "empty batch must not fire any HTTP request")
}

func TestSendErrorLogs_5xxReturnsError(t *testing.T) {
	cl := &captureClient{statusCode: http.StatusInternalServerError}
	s := newTestSender(t, cl)

	err := s.sendLogsBatch(context.Background(), []slog.Record{errorRecord("boom")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestSendErrorLogs_4xxIsTerminal(t *testing.T) {
	cl := &captureClient{statusCode: http.StatusBadRequest}
	s := newTestSender(t, cl)

	err := s.sendLogsBatch(context.Background(), []slog.Record{errorRecord("boom")})
	assert.NoError(t, err, "4xx must be terminal (return nil) so the Pipeline does not retry a malformed payload")
}

func TestSendErrorLogs_NetworkError(t *testing.T) {
	cl := &captureClient{err: errors.New("simulated network failure")}
	s := newTestSender(t, cl)

	err := s.sendLogsBatch(context.Background(), []slog.Record{errorRecord("boom")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulated network failure")
}

// TestSendPayloadBody_StatusCodes locks the contract of the extracted
// shared transport helper (review comment C3 on PR #49946). Both
// flushSession and sendLogsBatch route per-endpoint POSTs through this
// helper; the helper must return the raw HTTP status code so each caller
// can apply its own policy (flushSession logs only; sendLogsBatch
// distinguishes retryable 5xx from terminal 4xx).
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

func TestSendErrorLogs_PayloadShape_ByteForByte(t *testing.T) {
	cl := &captureClient{}
	s := newTestSender(t, cl)

	fixed := slog.NewRecord(
		time.Date(2026, 4, 28, 14, 30, 0, 0, time.UTC),
		slog.LevelError,
		"golden-message",
		0,
	)
	fixed.AddAttrs(
		slog.String("svc", "auth"),
		slog.Int("code", 500),
	)

	require.NoError(t, s.sendLogsBatch(context.Background(), []slog.Record{fixed}))

	_, bodies := cl.snapshot()
	require.Len(t, bodies, 1)

	// Decode into a generic map so we can assert exact JSON keys, which is
	// what dd-go's processor dispatches on.
	var raw map[string]any
	require.NoError(t, json.Unmarshal(bodies[0], &raw))

	for _, key := range []string{"api_version", "request_type", "event_time", "host", "payload"} {
		assert.Containsf(t, raw, key, "envelope must contain top-level key %q", key)
	}
	assert.Equal(t, "logs", raw["request_type"])

	payload, ok := raw["payload"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, payload, "logs", `inner key MUST be "logs" (NOT "records")`)
	assert.NotContains(t, payload, "records")

	logs, ok := payload["logs"].([]any)
	require.True(t, ok)
	require.Len(t, logs, 1)
	log := logs[0].(map[string]any)
	for _, key := range []string{
		"message", "tags", "level", "stack_trace",
		"tracer_time", "count", "trace_id", "span_id", "is_crash",
	} {
		assert.Containsf(t, log, key, "Log payload must have key %q (dd-go schema)", key)
	}

	// Level MUST be UPPERCASE.
	assert.Equal(t, "ERROR", log["level"])
	// Tags CSV alphabetical (code:500 sorts before svc:auth).
	assert.Equal(t, "code:500,svc:auth", log["tags"])
	// Defaults for v1.
	assert.Equal(t, float64(1), log["count"])
	assert.Equal(t, false, log["is_crash"])
	assert.Equal(t, "", log["trace_id"])
	assert.Equal(t, "", log["span_id"])
}
