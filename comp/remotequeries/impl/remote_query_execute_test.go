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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

// resolveFingerprintFor runs the resolve service against the given collector and
// target and returns the issued fingerprint, proving resolve and execute agree
// through the same integration-owned sweep and the same fingerprint computation.
func resolveFingerprintFor(t *testing.T, collector fakeCollector, target RemoteQueryExecuteTarget) string {
	t.Helper()
	service := NewRemoteQueryResolveService(collector, true)
	result := service.Resolve(RemoteQueryResolveRequest{Integration: "postgres", Target: target})
	require.Nil(t, result.Error)
	require.Equal(t, statusMatched, result.Status)
	require.NotEmpty(t, result.MatchFingerprint)
	return result.MatchFingerprint
}

func singleMatchCollector(instance string) fakeCollector {
	return fakeCollector{checks: []check.Check{
		fakeWrappedCheck{Check: &fakeStreamRunnerCheck{
			fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: instance}},
			events:          []check.RemoteQueryStreamEvent{{Type: "final", MetadataJSON: `{"status":"SUCCEEDED","upload_receipt":{"uploadId":"upload-proof","pageCount":1,"totalRows":1,"totalBytes":9}}`}},
		}},
	}}
}

func staleFingerprintExecuteRequest(t *testing.T) RemoteQueryExecuteRequest {
	t.Helper()
	req, err := NewRemoteQueryExecuteRequest("postgres",
		RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"},
		remoteQueryFixtureTableProofQuery, false, pagedTestDelivery())
	require.NoError(t, err)
	return req
}

// TestExecuteStreamRevalidatesMatchFingerprint proves the match-before-execute
// contract: an execute request carrying the resolve-time fingerprint re-runs one
// complete integration-owned sweep under the execution admission and proceeds
// only when the same unique match with the same integration-reported identity
// still holds. Any change — zero verdicts, multiple verdicts, or a different
// identity — fails with target_resolution_stale before the execute dispatch, so
// no Python execution call and no SQL ever happens.
func TestExecuteStreamRevalidatesMatchFingerprint(t *testing.T) {
	t.Run("matching fingerprint proceeds", func(t *testing.T) {
		collector := singleMatchCollector("host: localhost\nport: 5432\ndbname: postgres\npassword: secret-value\n")
		fingerprint := resolveFingerprintFor(t, collector, RemoteQueryExecuteTarget{Host: "LOCALHOST.", Port: 5432, DBName: "postgres"})
		runner := collector.checks[0].(fakeWrappedCheck).Check.(*fakeStreamRunnerCheck)
		service := NewRemoteQueryExecuteService(collector, true, false, nil)

		req := staleFingerprintExecuteRequest(t)
		req.MatchFingerprint = fingerprint
		result := service.ExecuteStream(context.Background(), req, func(check.RemoteQueryStreamEvent) error { return nil })

		require.Nil(t, result.Error)
		assert.Equal(t, 1, runner.executeCalls)
		// The execute-time revalidation sweep asked the check once more before the
		// execute dispatch.
		assert.Equal(t, 2, runner.resolveCalls)
	})

	t.Run("different fingerprint fails before the execute dispatch", func(t *testing.T) {
		collector := singleMatchCollector("host: localhost\nport: 5432\ndbname: postgres\npassword: secret-value\n")
		otherCollector := singleMatchCollector("host: otherhost\nport: 5432\ndbname: postgres\npassword: other-secret\n")
		otherFingerprint := resolveFingerprintFor(t, otherCollector, RemoteQueryExecuteTarget{Host: "otherhost", Port: 5432, DBName: "postgres"})
		runner := collector.checks[0].(fakeWrappedCheck).Check.(*fakeStreamRunnerCheck)
		service := NewRemoteQueryExecuteService(collector, true, false, nil)

		req := staleFingerprintExecuteRequest(t)
		req.MatchFingerprint = otherFingerprint
		result := service.ExecuteStream(context.Background(), req, func(check.RemoteQueryStreamEvent) error { return nil })

		require.NotNil(t, result.Error)
		assert.Equal(t, http.StatusConflict, result.HTTPStatus)
		assert.Equal(t, statusTargetResolutionStale, result.Error.Code)
		assert.Equal(t, "target resolution changed since the selected match", result.Error.Message)
		assert.Zero(t, runner.executeCalls)
		assert.NotContains(t, result.Error.Message, "otherhost")
	})

	t.Run("zero matched verdicts fails stale, not not-found", func(t *testing.T) {
		// Resolve against a matching sweep, then execute against one whose check no
		// longer admits the target (the eligible set changed).
		resolveCollector := singleMatchCollector("host: localhost\nport: 5432\ndbname: postgres\n")
		fingerprint := resolveFingerprintFor(t, resolveCollector, RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"})
		executeCollector := singleMatchCollector("host: localhost\nport: 5432\ndbname: postgres\n")
		executeRunner := executeCollector.checks[0].(fakeWrappedCheck).Check.(*fakeStreamRunnerCheck)
		executeRunner.resolveEvents = resolveTargetNotFoundEvents()
		service := NewRemoteQueryExecuteService(executeCollector, true, false, nil)

		req := staleFingerprintExecuteRequest(t)
		req.MatchFingerprint = fingerprint
		result := service.ExecuteStream(context.Background(), req, func(check.RemoteQueryStreamEvent) error { return nil })

		require.NotNil(t, result.Error)
		assert.Equal(t, http.StatusConflict, result.HTTPStatus)
		assert.Equal(t, statusTargetResolutionStale, result.Error.Code)
		assert.Zero(t, executeRunner.executeCalls)
	})

	t.Run("multiple matched verdicts fails stale, not ambiguous", func(t *testing.T) {
		resolveCollector := singleMatchCollector("host: localhost\nport: 5432\ndbname: postgres\n")
		fingerprint := resolveFingerprintFor(t, resolveCollector, RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"})
		executeCollector := fakeCollector{checks: []check.Check{
			fakeWrappedCheck{Check: &fakeStreamRunnerCheck{fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-one\n"}}}},
			fakeWrappedCheck{Check: &fakeStreamRunnerCheck{fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "kube", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-two\n"}}}},
		}}
		service := NewRemoteQueryExecuteService(executeCollector, true, false, nil)

		req := staleFingerprintExecuteRequest(t)
		req.MatchFingerprint = fingerprint
		result := service.ExecuteStream(context.Background(), req, func(check.RemoteQueryStreamEvent) error { return nil })

		require.NotNil(t, result.Error)
		assert.Equal(t, http.StatusConflict, result.HTTPStatus)
		assert.Equal(t, statusTargetResolutionStale, result.Error.Code)
		for _, chk := range executeCollector.checks {
			runner := chk.(fakeWrappedCheck).Check.(*fakeStreamRunnerCheck)
			assert.Zero(t, runner.executeCalls)
		}
	})
}

// TestExecuteStreamWithoutFingerprintIsUnchanged proves the absent-fingerprint
// behavior keeps the plain zero/one/many outcomes of the sweep: a no-match sweep
// answers target_not_found, multiple matched verdicts answer ambiguous_target,
// and a plain unique verdict executes.
func TestExecuteStreamWithoutFingerprintIsUnchanged(t *testing.T) {
	t.Run("zero matched verdicts keeps target_not_found", func(t *testing.T) {
		collector := singleMatchCollector("host: localhost\nport: 5432\ndbname: postgres\n")
		runner := collector.checks[0].(fakeWrappedCheck).Check.(*fakeStreamRunnerCheck)
		runner.resolveEvents = resolveTargetNotFoundEvents()
		service := NewRemoteQueryExecuteService(collector, true, false, nil)

		result := service.ExecuteStream(context.Background(), staleFingerprintExecuteRequest(t), func(check.RemoteQueryStreamEvent) error { return nil })

		require.NotNil(t, result.Error)
		assert.Equal(t, http.StatusNotFound, result.HTTPStatus)
		assert.Equal(t, statusTargetNotFound, result.Error.Code)
		assert.Zero(t, runner.executeCalls)
	})

	t.Run("multiple matched verdicts keeps ambiguous_target", func(t *testing.T) {
		collector := fakeCollector{checks: []check.Check{
			fakeWrappedCheck{Check: &fakeStreamRunnerCheck{fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "file", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-one\n"}}}},
			fakeWrappedCheck{Check: &fakeStreamRunnerCheck{fakeRunnerCheck: fakeRunnerCheck{fakeCheck: fakeCheck{name: "postgres", loader: "python", provider: "kube", instance: "host: localhost\nport: 5432\ndbname: postgres\npassword: secret-two\n"}}}},
		}}
		service := NewRemoteQueryExecuteService(collector, true, false, nil)

		result := service.ExecuteStream(context.Background(), staleFingerprintExecuteRequest(t), func(check.RemoteQueryStreamEvent) error { return nil })

		require.NotNil(t, result.Error)
		assert.Equal(t, http.StatusConflict, result.HTTPStatus)
		assert.Equal(t, statusAmbiguous, result.Error.Code)
	})
}

// TestExecuteStreamStaleFailsBeforeMarshal proves the fingerprint revalidation gate
// runs before the integration request JSON is ever built: a stale fingerprint with
// a valid delivery and allowlisted query never reaches marshalExecuteRequest or
// the Python execute dispatch — only the side-effect-free resolve sweep ran.
func TestExecuteStreamStaleFailsBeforeMarshal(t *testing.T) {
	resolveCollector := singleMatchCollector("host: localhost\nport: 5432\ndbname: postgres\n")
	fingerprint := resolveFingerprintFor(t, resolveCollector, RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "postgres"})
	executeCollector := singleMatchCollector("host: localhost\nport: 5432\ndbname: completely-elsewhere\n")
	runner := executeCollector.checks[0].(fakeWrappedCheck).Check.(*fakeStreamRunnerCheck)
	runner.resolveEvents = resolveTargetNotFoundEvents()
	service := NewRemoteQueryExecuteService(executeCollector, true, true, nil)

	req, err := NewRemoteQueryExecuteRequest("postgres",
		RemoteQueryExecuteTarget{Host: "localhost", Port: 5432, DBName: "completely-elsewhere"},
		remoteQueryFixtureTableProofQuery, false, pagedTestDelivery())
	require.NoError(t, err)
	req.MatchFingerprint = fingerprint

	result := service.ExecuteStream(context.Background(), req, func(check.RemoteQueryStreamEvent) error { return nil })

	require.NotNil(t, result.Error)
	assert.Equal(t, statusTargetResolutionStale, result.Error.Code)
	assert.Zero(t, runner.executeCalls)
	assert.Empty(t, runner.streamSeen)
	assert.Equal(t, 1, runner.resolveCalls)
}

// TestNewRemoteQueryTraceContextValidatesOptionalMetadata proves the optional
// trace context is accepted only in its supported shape: non-zero trace and
// parent IDs (64-bit unsigned, exercising the largest representable value) and a
// sampling priority inside the Datadog tracer's priority domain. Anything else
// is invalid metadata and normalizes to nil — dropped, never fatal.
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
		{name: "auto drop", traceID: 1, spanID: 2, samplingPriority: 0, valid: true},
		{name: "user drop", traceID: 1, spanID: 2, samplingPriority: -1, valid: true},
		{name: "largest 64-bit values", traceID: maxUint64, spanID: maxUint64, samplingPriority: 2, valid: true},
		{name: "zero trace id", traceID: 0, spanID: 2, samplingPriority: 2, valid: false},
		{name: "zero parent id", traceID: 1, spanID: 0, samplingPriority: 2, valid: false},
		{name: "priority above domain", traceID: 1, spanID: 2, samplingPriority: 3, valid: false},
		{name: "priority below domain", traceID: 1, spanID: 2, samplingPriority: -2, valid: false},
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
		remoteQueryProofSeedQuery, false, pagedTestDelivery())
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
func TestMarshalExecuteRequestDropsInvalidTraceContext(t *testing.T) {
	for _, tc := range []struct {
		name         string
		traceContext *RemoteQueryTraceContext
	}{
		{name: "zero trace id", traceContext: &RemoteQueryTraceContext{TraceID: 0, SpanID: 2, SamplingPriority: 2}},
		{name: "zero parent id", traceContext: &RemoteQueryTraceContext{TraceID: 1, SpanID: 0, SamplingPriority: 2}},
		{name: "priority above domain", traceContext: &RemoteQueryTraceContext{TraceID: 1, SpanID: 2, SamplingPriority: 3}},
		{name: "priority below domain", traceContext: &RemoteQueryTraceContext{TraceID: 1, SpanID: 2, SamplingPriority: -2}},
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
