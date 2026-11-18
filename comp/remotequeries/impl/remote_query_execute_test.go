// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remotequeriesimpl

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

func singleMatchCollector(instance string) fakeCollector {
	return fakeCollector{checks: []check.Check{
		fakeWrappedCheck{Check: &fakeStreamRunnerCheck{
			fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: instance}},
			events:          []check.RemoteQueryStreamEvent{{Type: "final", MetadataJSON: `{"status":"SUCCEEDED"}`}},
		}},
	}}
}

func executeTestRequest(t *testing.T) RemoteQueryExecuteRequest {
	t.Helper()
	req, err := NewRemoteQueryExecuteRequest("postgres",
		RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"},
		"SELECT * FROM arbitrary_table", false, pagedTestDelivery())
	require.NoError(t, err)
	return req
}

// executeAndWait executes the request and waits for the started detached run to end.
func executeAndWait(service *RemoteQueryExecuteService, req RemoteQueryExecuteRequest) RemoteQueryExecuteResult {
	result := service.ExecuteStream(context.Background(), req)
	service.runs.Wait()
	return result
}

// blockingRunnerCheck blocks its execute run until release is closed and records
// whether emit kept working and whether the run ended.
type blockingRunnerCheck struct {
	*fakeStreamRunnerCheck
	started  chan struct{}
	release  chan struct{}
	finished chan struct{}
	emitErr  error
}

func newBlockingRunnerCheck(instance string) *blockingRunnerCheck {
	b := &blockingRunnerCheck{
		fakeStreamRunnerCheck: &fakeStreamRunnerCheck{
			fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: instance}},
		},
		started:  make(chan struct{}),
		release:  make(chan struct{}),
		finished: make(chan struct{}),
	}
	b.run = func(emit func(check.RemoteQueryStreamEvent) error) error {
		defer close(b.finished)
		close(b.started)
		<-b.release
		b.emitErr = emit(check.RemoteQueryStreamEvent{Type: "final", MetadataJSON: `{"status":"SUCCEEDED"}`})
		return nil
	}
	return b
}

// The request is answered as soon as the run starts; the run keeps going after the
// answer and after the request context ends, and its emit callback never fails.
func TestExecuteStreamAnswersBeforeRunEnds(t *testing.T) {
	runner := newBlockingRunnerCheck("host: localhost\nport: 5432\ndbname: postgres\n")
	service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{runner}}, true, remoteQueryTestConfig(t))
	ctx, cancel := context.WithCancel(context.Background())

	result := service.ExecuteStream(ctx, executeTestRequest(t))
	cancel()

	require.Nil(t, result.Error)
	assert.Equal(t, http.StatusOK, result.HTTPStatus)
	assert.Equal(t, "SUCCEEDED", result.Status)
	select {
	case <-runner.started:
	case <-time.After(5 * time.Second):
		t.Fatal("the run did not start")
	}
	select {
	case <-runner.finished:
		t.Fatal("the run ended before it was released")
	default:
	}

	close(runner.release)
	select {
	case <-runner.finished:
	case <-time.After(5 * time.Second):
		t.Fatal("the run did not continue after the request context ended")
	}
	service.runs.Wait()
	assert.NoError(t, runner.emitErr)
}

func TestExecuteStreamCancelledRequestDoesNotStartRun(t *testing.T) {
	runner := newBlockingRunnerCheck("host: localhost\nport: 5432\ndbname: postgres\n")
	service := NewRemoteQueryExecuteService(fakeCollector{checks: []check.Check{runner}}, true, remoteQueryTestConfig(t))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := service.ExecuteStream(ctx, executeTestRequest(t))
	service.runs.Wait()

	require.NotNil(t, result.Error)
	assert.Equal(t, http.StatusRequestTimeout, result.HTTPStatus)
	assert.Equal(t, statusExecutorUnavailable, result.Error.Code)
	assert.Zero(t, runner.executeCalls)
}

// Errors before the hand-off keep their codes and never start a run.
func TestExecuteStreamErrorsBeforeHandOffKeepCodes(t *testing.T) {
	const instance = "host: localhost\nport: 5432\ndbname: postgres\n"
	runnerService := func(t *testing.T, enabled bool, collector RemoteQueryCollector) *RemoteQueryExecuteService {
		return NewRemoteQueryExecuteService(collector, enabled, remoteQueryTestConfig(t))
	}

	t.Run("disabled", func(t *testing.T) {
		runner := newBlockingRunnerCheck(instance)
		result := executeAndWait(runnerService(t, false, fakeCollector{checks: []check.Check{runner}}), executeTestRequest(t))

		require.NotNil(t, result.Error)
		assert.Equal(t, http.StatusServiceUnavailable, result.HTTPStatus)
		assert.Equal(t, statusBridgeDisabled, result.Error.Code)
		assert.Zero(t, runner.executeCalls)
	})

	t.Run("missing delivery", func(t *testing.T) {
		runner := newBlockingRunnerCheck(instance)
		req := executeTestRequest(t)
		req.ResultDelivery = nil
		result := executeAndWait(runnerService(t, true, fakeCollector{checks: []check.Check{runner}}), req)

		require.NotNil(t, result.Error)
		assert.Equal(t, http.StatusBadRequest, result.HTTPStatus)
		assert.Equal(t, statusInvalidRequest, result.Error.Code)
		assert.Zero(t, runner.executeCalls)
	})

	t.Run("check without stream support", func(t *testing.T) {
		collector := fakeCollector{checks: []check.Check{
			fakeWrappedCheck{Check: &fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: instance}}},
		}}
		result := executeAndWait(runnerService(t, true, collector), executeTestRequest(t))

		require.NotNil(t, result.Error)
		assert.Equal(t, statusExecutorUnavailable, result.Error.Code)
	})
}

func TestRemoteQueryRunOutcomeObservesTerminalEvents(t *testing.T) {
	for _, tc := range []struct {
		name     string
		events   []check.RemoteQueryStreamEvent
		status   string
		wantCode string
	}{
		{name: "no events"},
		{name: "metadata only", events: []check.RemoteQueryStreamEvent{{Type: "metadata", MetadataJSON: `{"status":"STARTED"}`}}},
		{name: "final", events: []check.RemoteQueryStreamEvent{{Type: "final", MetadataJSON: `{"status":"SUCCEEDED"}`}}, status: "SUCCEEDED"},
		{name: "nested error code", events: []check.RemoteQueryStreamEvent{{Type: "error", MetadataJSON: `{"status":"FAILED","error":{"code":"cancelled","message":"m"}}`}}, status: "FAILED", wantCode: "cancelled"},
		{name: "top-level error code without status", events: []check.RemoteQueryStreamEvent{{Type: "error", MetadataJSON: `{"code":"boom"}`}}, status: "FAILED", wantCode: "boom"},
		{name: "malformed metadata", events: []check.RemoteQueryStreamEvent{{Type: "final", MetadataJSON: `{`}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var outcome remoteQueryRunOutcome
			for _, event := range tc.events {
				outcome.observe(event)
			}
			assert.Equal(t, tc.status, outcome.status)
			assert.Equal(t, tc.wantCode, outcome.code)
		})
	}
}

// TestExecuteStreamAnswersPlainMatchOutcomes proves execute resolves the target
// fresh with the plain zero/one/many outcomes of the
// sweep — there is no resolve-time binding: a unique match executes even if the
// check identity changed since any earlier resolve, a no-match sweep answers
// target_not_found, and multiple matched verdicts answer ambiguous_target before
// the execute dispatch, so no Python execution call and no SQL ever happens.
func TestExecuteStreamAnswersPlainMatchOutcomes(t *testing.T) {
	t.Run("unique match executes", func(t *testing.T) {
		collector := singleMatchCollector("host: localhost\nport: 5432\ndbname: postgres\npassword: secret-value\n")
		runner := collector.checks[0].(fakeWrappedCheck).Check.(*fakeStreamRunnerCheck)
		service := NewRemoteQueryExecuteService(collector, true, remoteQueryTestConfig(t))

		result := executeAndWait(service, executeTestRequest(t))

		require.Nil(t, result.Error)
		assert.Equal(t, http.StatusOK, result.HTTPStatus)
		assert.Equal(t, 1, runner.executeCalls)
		// The execute-time sweep asked the check once before the execute dispatch.
		assert.Equal(t, 1, runner.resolveCalls)
	})

	t.Run("zero matched verdicts answers target_not_found", func(t *testing.T) {
		collector := singleMatchCollector("host: localhost\nport: 5432\ndbname: postgres\n")
		runner := collector.checks[0].(fakeWrappedCheck).Check.(*fakeStreamRunnerCheck)
		runner.resolveEvents = resolveTargetNotFoundEvents()
		service := NewRemoteQueryExecuteService(collector, true, remoteQueryTestConfig(t))

		result := executeAndWait(service, executeTestRequest(t))

		require.NotNil(t, result.Error)
		assert.Equal(t, http.StatusNotFound, result.HTTPStatus)
		assert.Equal(t, statusTargetNotFound, result.Error.Code)
		assert.Zero(t, runner.executeCalls)
	})

	t.Run("multiple matched verdicts answers ambiguous_target", func(t *testing.T) {
		collector := fakeCollector{checks: []check.Check{
			fakeWrappedCheck{Check: &fakeStreamRunnerCheck{fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-one\n"}}}},
			fakeWrappedCheck{Check: &fakeStreamRunnerCheck{fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "kube", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-two\n"}}}},
		}}
		service := NewRemoteQueryExecuteService(collector, true, remoteQueryTestConfig(t))

		result := executeAndWait(service, executeTestRequest(t))

		require.NotNil(t, result.Error)
		assert.Equal(t, http.StatusConflict, result.HTTPStatus)
		assert.Equal(t, statusAmbiguous, result.Error.Code)
		for _, chk := range collector.checks {
			runner := chk.(fakeWrappedCheck).Check.(*fakeStreamRunnerCheck)
			assert.Zero(t, runner.executeCalls)
		}
	})
}

// TestNewRemoteQueryTraceContextValidatesOptionalMetadata proves the optional
// trace context is accepted only in its supported shape: non-zero trace and
// parent IDs (64-bit unsigned, exercising the largest representable value) and
// one of the positive keep sampling priorities (1 AutoKeep, 2 UserKeep) — the
// exact domain the integration's strict carrier validation accepts. Anything
// else, including the tracer's drop priorities, is invalid metadata and
// normalizes to nil — dropped, never fatal.
func TestNewRemoteQueryTraceContextValidatesOptionalMetadata(t *testing.T) {
	const maxUint64 = ^uint64(0)
	tests := []struct {
		name             string
		traceID          uint64
		spanID           uint64
		samplingPriority int
		valid            bool
	}{
		{name: "user keep", traceID: 1234567890123456789, spanID: 9876543210987654321, samplingPriority: 2, valid: true},
		{name: "auto keep", traceID: 1, spanID: 2, samplingPriority: 1, valid: true},
		{name: "largest 64-bit values", traceID: maxUint64, spanID: maxUint64, samplingPriority: 2, valid: true},
		{name: "zero trace id", traceID: 0, spanID: 2, samplingPriority: 2, valid: false},
		{name: "zero parent id", traceID: 1, spanID: 0, samplingPriority: 2, valid: false},
		{name: "priority above keep domain", traceID: 1, spanID: 2, samplingPriority: 3, valid: false},
		{name: "priority below keep domain", traceID: 1, spanID: 2, samplingPriority: -2, valid: false},
		{name: "auto drop priority", traceID: 1, spanID: 2, samplingPriority: 0, valid: false},
		{name: "user drop priority", traceID: 1, spanID: 2, samplingPriority: -1, valid: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			traceContext := NewRemoteQueryTraceContext(tt.traceID, tt.spanID, tt.samplingPriority)
			if !tt.valid {
				assert.Nil(t, traceContext)
				return
			}
			require.NotNil(t, traceContext)
			assert.Equal(t, tt.traceID, traceContext.TraceID)
			assert.Equal(t, tt.spanID, traceContext.SpanID)
			assert.Equal(t, tt.samplingPriority, traceContext.SamplingPriority)
		})
	}
}

// traceContextExecuteRequest builds a valid execute request carrying the optional
// trace context, the way the AgentSecure gRPC mapping assembles one.
func traceContextExecuteRequest(t *testing.T, traceContext *RemoteQueryTraceContext) remoteQueryExecuteRequest {
	t.Helper()
	req, err := NewRemoteQueryExecuteRequest("postgres",
		RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"},
		"SELECT 1 AS value", false, pagedTestDelivery())
	require.NoError(t, err)
	req.TraceContext = traceContext
	return req.internal()
}

// TestMarshalExecuteRequestEmitsTraceContextContractShape pins the exact
// integration-request JSON contract: the optional top-level traceContext object
// carries the trace ID and parent ID as unsigned decimal strings — so the exact
// 64-bit value survives without numeric precision or format drift — and the
// sampling priority as an integer.
func TestMarshalExecuteRequestEmitsTraceContextContractShape(t *testing.T) {
	requestJSON, err := marshalExecuteRequest(traceContextExecuteRequest(t,
		NewRemoteQueryTraceContext(1234567890123456789, 9876543210987654321, 2)))
	require.NoError(t, err)

	assert.Contains(t, requestJSON,
		`"traceContext":{"traceId":"1234567890123456789","parentId":"9876543210987654321","samplingPriority":2}`)

	// The object is the exact wire contract, not a loose superset.
	var wire struct {
		TraceContext struct {
			TraceID          string `json:"traceId"`
			ParentID         string `json:"parentId"`
			SamplingPriority int    `json:"samplingPriority"`
		} `json:"traceContext"`
	}
	require.NoError(t, json.Unmarshal([]byte(requestJSON), &wire))
	assert.Equal(t, "1234567890123456789", wire.TraceContext.TraceID)
	assert.Equal(t, "9876543210987654321", wire.TraceContext.ParentID)
	assert.Equal(t, 2, wire.TraceContext.SamplingPriority)
}

// TestMarshalExecuteRequestOmitsAbsentTraceContext proves absence stays absent:
// a request without the optional metadata emits no traceContext key, so an old
// integration sees byte-identical request JSON.
func TestMarshalExecuteRequestOmitsAbsentTraceContext(t *testing.T) {
	requestJSON, err := marshalExecuteRequest(traceContextExecuteRequest(t, nil))
	require.NoError(t, err)
	assert.NotContains(t, requestJSON, "traceContext")
}

// TestMarshalExecuteRequestDropsInvalidTraceContext proves invalid metadata is
// dropped without failing the run: the request still marshals, and the
// integration request omits the key exactly as if no context had been attached.
// Drop priorities (0, -1) are dropped the same way, so the integration never
// receives a carrier its strict keep-priority validation would reject.
func TestMarshalExecuteRequestDropsInvalidTraceContext(t *testing.T) {
	for _, tc := range []struct {
		name         string
		traceContext *RemoteQueryTraceContext
	}{
		{name: "zero trace id", traceContext: &RemoteQueryTraceContext{TraceID: 0, SpanID: 2, SamplingPriority: 2}},
		{name: "zero parent id", traceContext: &RemoteQueryTraceContext{TraceID: 1, SpanID: 0, SamplingPriority: 2}},
		{name: "priority above keep domain", traceContext: &RemoteQueryTraceContext{TraceID: 1, SpanID: 2, SamplingPriority: 3}},
		{name: "priority below keep domain", traceContext: &RemoteQueryTraceContext{TraceID: 1, SpanID: 2, SamplingPriority: -2}},
		{name: "auto drop priority", traceContext: &RemoteQueryTraceContext{TraceID: 1, SpanID: 2, SamplingPriority: 0}},
		{name: "user drop priority", traceContext: &RemoteQueryTraceContext{TraceID: 1, SpanID: 2, SamplingPriority: -1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requestJSON, err := marshalExecuteRequest(traceContextExecuteRequest(t, tc.traceContext))
			require.NoError(t, err)
			assert.NotContains(t, requestJSON, "traceContext")
		})
	}
}

// TestRemoteQueryExecuteRequestTraceContextRoundTrip proves the optional metadata
// survives the typed-request conversions unchanged: a validated context crossing
// internal() and remoteQueryExecuteRequestFromInternal keeps its exact values.
func TestRemoteQueryExecuteRequestTraceContextRoundTrip(t *testing.T) {
	traceContext := NewRemoteQueryTraceContext(1234567890123456789, 9876543210987654321, 1)

	internal := traceContextExecuteRequest(t, traceContext)
	require.NotNil(t, internal.TraceContext)
	public := remoteQueryExecuteRequestFromInternal(internal)
	require.NotNil(t, public.TraceContext)
	assert.Equal(t, traceContext, public.TraceContext)

	roundTripped := public.internal()
	require.NotNil(t, roundTripped.TraceContext)
	assert.Equal(t, traceContext, roundTripped.TraceContext)

	// An absent context stays absent through the same conversions.
	absent := traceContextExecuteRequest(t, nil)
	assert.Nil(t, remoteQueryExecuteRequestFromInternal(absent).TraceContext)
	assert.Nil(t, remoteQueryExecuteRequestFromInternal(absent).internal().TraceContext)
}
