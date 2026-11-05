// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agentimpl

import (
	"strings"
	"testing"

	"google.golang.org/grpc"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	remotequeriesimpl "github.com/DataDog/datadog-agent/comp/remotequeries/impl"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func TestRemoteQueryExecuteStreamReturnsSanitizedUnavailableWhenServiceMissing(t *testing.T) {
	stream := &captureRemoteQueryExecuteStreamServer{}
	err := (&serverSecure{}).RemoteQueryExecuteStream(&pb.RemoteQueryExecuteRequest{}, stream)

	require.NoError(t, err)
	require.Len(t, stream.chunks, 2)
	assert.Equal(t, "executor_unavailable", stream.chunks[0].GetEvent().GetError().GetCode())
	assert.True(t, stream.chunks[1].GetFinal())
}

func TestRemoteQueryExecuteStreamRejectsInvalidRequestWithTerminalErrorEvent(t *testing.T) {
	stream := &captureRemoteQueryExecuteStreamServer{}
	err := (&serverSecure{remoteQueries: remoteQueriesServiceForTest(t)}).RemoteQueryExecuteStream(
		&pb.RemoteQueryExecuteRequest{
			Integration: "postgres",
			Target:      &pb.RemoteQueryTarget{Host: "localhost", Port: 5432, Dbname: "postgres"},
			Query:       "SELECT 1 AS value",
		}, stream)

	// A request without the backend-injected session_info fails closed as a terminal
	// error event; the final chunk still terminates the stream.
	require.NoError(t, err)
	require.Len(t, stream.chunks, 2)
	assert.Equal(t, int32(0), stream.chunks[0].GetChunkIndex())
	assert.Equal(t, remotequeriesimpl.RemoteQueryStatusInvalidRequest, stream.chunks[0].GetEvent().GetError().GetCode())
	assert.Equal(t, "session_info is required", stream.chunks[0].GetEvent().GetError().GetMessage())
	assert.Equal(t, int32(1), stream.chunks[1].GetChunkIndex())
	assert.True(t, stream.chunks[1].GetFinal())
}

func remoteQueriesServiceForTest(t *testing.T) *remotequeriesimpl.RemoteQueryExecuteService {
	t.Helper()
	return remotequeriesimpl.NewRemoteQueryExecuteService(nil, true, nil)
}

// validRemoteQuerySessionInfoJSON is an arbitrary session_info object: the AgentSecure
// boundary carries it as its exact bytes and never interprets any field of it, so the
// fixture mixes nested objects, arrays, nulls and a future field.
const validRemoteQuerySessionInfoJSON = `{"runId":"run-proof","taskId":"task-proof","uploadId":"upload-proof","baseUrl":"https://dd.datad0g.com/api/unstable/its-agent-intake","limits":{"maxFileBytes":134217728,"maxResultBytes":107374182400,"maxRowBytes":16777216,"maxColumns":1024,"maxSchemaBytes":1048576,"maxPages":128,"timeoutMs":30000},"future":{"nested":[1,null,{"deep":[true]}]},"nullField":null}`

func validRemoteQuerySessionInfoJSONBytes() []byte {
	return []byte(validRemoteQuerySessionInfoJSON)
}

func TestRemoteQueryExecuteRequestFromProtoPreservesPagedJSONContract(t *testing.T) {
	req, err := remoteQueryExecuteRequestFromProto(&pb.RemoteQueryExecuteRequest{
		Integration:     "postgres",
		Target:          &pb.RemoteQueryTarget{Host: "LOCALHOST.", Port: 5432, Dbname: "postgres"},
		Query:           "SELECT city, country FROM cities ORDER BY city",
		IncludeSchema:   true,
		SessionInfoJson: validRemoteQuerySessionInfoJSONBytes(),
	})

	require.NoError(t, err)
	assert.Equal(t, "postgres", req.Integration)
	assert.Equal(t, "localhost", req.Target.Host)
	assert.Equal(t, 5432, req.Target.Port)
	assert.Equal(t, "postgres", req.Target.DBName)
	assert.Equal(t, "SELECT city, country FROM cities ORDER BY city", req.Query)
	assert.True(t, req.IncludeSchema)
	// session_info crosses the boundary as its exact bytes, including nested future
	// fields, arrays and nulls the Agent knows nothing about.
	assert.Equal(t, validRemoteQuerySessionInfoJSON, string(req.SessionInfo))
}

func TestRemoteQueryExecuteRequestFromProtoPreservesDatabaseInstanceTarget(t *testing.T) {
	req, err := remoteQueryExecuteRequestFromProto(&pb.RemoteQueryExecuteRequest{
		Integration:     "postgres",
		Target:          &pb.RemoteQueryTarget{DatabaseInstance: "rq-proof-a1-db1"},
		Query:           "SELECT city, country FROM cities ORDER BY city",
		SessionInfoJson: validRemoteQuerySessionInfoJSONBytes(),
	})

	require.NoError(t, err)
	assert.Equal(t, "rq-proof-a1-db1", req.Target.DatabaseInstance)
	assert.Empty(t, req.Target.Host)
	assert.Zero(t, req.Target.Port)
	assert.Empty(t, req.Target.DBName)
}

// TestRemoteQueryExecuteRequestFromProtoPreservesArbitrarySessionInfo proves the transport
// keeps any JSON object unchanged — large integers keep their exact text, an unknown
// future shape passes, and even semantically invalid values are the integration's to
// reject — while a non-object or oversized value fails closed.
func TestRemoteQueryExecuteRequestFromProtoPreservesArbitrarySessionInfo(t *testing.T) {
	passing := []string{
		`{}`,
		`{"cap":107374182400,"big":123456789012345678901234567890}`,
		`{"protocol":"future-v9","artifactVersion":7,"baseUrl":"http://169.254.169.254/","limits":"nope"}`,
		validRemoteQuerySessionInfoJSON,
	}
	for _, sessionInfo := range passing {
		req, err := remoteQueryExecuteRequestFromProto(&pb.RemoteQueryExecuteRequest{
			Integration:     "postgres",
			Target:          &pb.RemoteQueryTarget{Host: "localhost", Port: 5432, Dbname: "postgres"},
			Query:           "SELECT 1 AS value",
			SessionInfoJson: []byte(sessionInfo),
		})
		require.NoError(t, err)
		assert.Equal(t, sessionInfo, string(req.SessionInfo))
	}

	failing := map[string][]byte{
		"null":      []byte(`null`),
		"array":     []byte(`[]`),
		"string":    []byte(`"x"`),
		"truncated": []byte(`{"a":`),
		"oversized": []byte(`{"a":"` + strings.Repeat("x", remotequeriesimpl.RemoteQuerySessionInfoMaxBytes) + `"}`),
	}
	for name, sessionInfo := range failing {
		_, err := remoteQueryExecuteRequestFromProto(&pb.RemoteQueryExecuteRequest{
			Integration:     "postgres",
			Target:          &pb.RemoteQueryTarget{Host: "localhost", Port: 5432, Dbname: "postgres"},
			Query:           "SELECT 1 AS value",
			SessionInfoJson: sessionInfo,
		})
		require.Error(t, err, name)
	}
}

func TestRemoteQueryStreamEventFromCheckEventMapsMetadata(t *testing.T) {
	event, err := remoteQueryStreamEventFromCheckEvent(check.RemoteQueryStreamEvent{
		Type:         "metadata",
		MetadataJSON: `{"status":"STARTED","operation":"produce_json_pages","includeSchema":true,"session_info":{"runId":"run-proof","uploadId":"upload-proof","limits":{"maxPages":128}}}`,
	}, "postgres")

	require.NoError(t, err)
	require.NotNil(t, event.GetMetadata())
	assert.Equal(t, uint64(0), event.GetSequence())
	assert.Equal(t, "produce_json_pages", event.GetMetadata().GetOperation())
	assert.Equal(t, "postgres", event.GetMetadata().GetIntegration())
	assert.Equal(t, map[string]string{"status": "STARTED", "includeSchema": "true"}, event.GetMetadata().GetAttributes())
}

func TestRemoteQueryStreamEventFromCheckEventSurfacesCompactReceipt(t *testing.T) {
	event, err := remoteQueryStreamEventFromCheckEvent(check.RemoteQueryStreamEvent{
		Type:         "final",
		MetadataJSON: `{"status":"SUCCEEDED","upload_receipt":{"uploadId":"upload-proof","pageCount":3,"totalRows":123456,"totalBytes":987654},"stats":{"rowsEmitted":123456,"pagesEmitted":3,"bytesEmitted":987654,"elapsedMs":4321}}`,
	}, "postgres")

	require.NoError(t, err)
	require.NotNil(t, event.GetFinal())
	assert.Equal(t, "SUCCEEDED", event.GetFinal().GetStatus())
	// The receipt crosses as its exact JSON bytes: no field is copied, typed or rewritten.
	assert.JSONEq(t, `{"uploadId":"upload-proof","pageCount":3,"totalRows":123456,"totalBytes":987654}`, string(event.GetFinal().GetUploadReceiptJson()))

	// The run progress stats surface as compact string attributes, never as bulk
	// result bytes, and the receipt never flattens into attributes.
	attrs := event.GetFinal().GetAttributes()
	assert.Equal(t, "123456", attrs["stats.rowsEmitted"])
	assert.Equal(t, "3", attrs["stats.pagesEmitted"])
	assert.Equal(t, "987654", attrs["stats.bytesEmitted"])
	assert.Equal(t, "4321", attrs["stats.elapsedMs"])
}

// legacyExecutionDiagnosticsCases spans the mixed-version surface an older
// integration still emits as top-level event metadata: a valid contract-v1
// shaped object, JSON null, and the malformed shapes (non-object, unknown or
// negative fields, fractional values, oversized padding) the old Agent's
// validator used to drop. Every shape must now be ignored identically.
var legacyExecutionDiagnosticsCases = []struct {
	name            string
	diagnosticsJSON string
}{
	{name: "valid contract-shaped object", diagnosticsJSON: `{"contractVersion":1,"producer":{"totalMs":831900,"databaseFetchMs":184000,"pageCount":52,"rowCount":7485000,"uploadRetryCount":0}}`},
	{name: "json null", diagnosticsJSON: `null`},
	{name: "non-object value", diagnosticsJSON: `"not-an-object"`},
	{name: "unknown top-level field", diagnosticsJSON: `{"contractVersion":1,"extra":1}`},
	{name: "unknown producer field", diagnosticsJSON: `{"contractVersion":1,"producer":{"bogus":1}}`},
	{name: "negative duration", diagnosticsJSON: `{"contractVersion":1,"producer":{"totalMs":-5}}`},
	{name: "fractional float", diagnosticsJSON: `{"contractVersion":1,"producer":{"totalMs":1.5}}`},
	{name: "oversized padded object", diagnosticsJSON: `{"contractVersion":1,"producer":{"totalMs":831900}` + strings.Repeat(" ", 5000) + `}`},
}

// TestRemoteQueryStreamEventFromCheckEventIgnoresLegacyExecutionDiagnostics
// proves the mixed-version behavior after the typed execution-diagnostics
// transport was retired: an older integration still emits its top-level
// executionDiagnostics metadata — valid-shaped, null, malformed, or oversized —
// and the Agent simply ignores it. The receipt conversion and the sanitized
// error conversion are untouched, and the legacy object never leaks into the
// flattened generic progress/final/error attributes.
func TestRemoteQueryStreamEventFromCheckEventIgnoresLegacyExecutionDiagnostics(t *testing.T) {
	receiptJSON := `"upload_receipt":{"uploadId":"upload-proof","pageCount":3,"totalRows":123456,"totalBytes":987654}`
	for _, tt := range legacyExecutionDiagnosticsCases {
		t.Run(tt.name, func(t *testing.T) {
			finalEvent, err := remoteQueryStreamEventFromCheckEvent(check.RemoteQueryStreamEvent{
				Type:         "final",
				MetadataJSON: `{"status":"SUCCEEDED",` + receiptJSON + `,"stats":{"elapsedMs":4321},"executionDiagnostics":` + tt.diagnosticsJSON + `}`,
			}, "postgres")

			require.NoError(t, err)
			require.NotNil(t, finalEvent.GetFinal())
			require.NotEmpty(t, finalEvent.GetFinal().GetUploadReceiptJson(), "legacy diagnostics must never affect the receipt")
			assert.JSONEq(t, `{"uploadId":"upload-proof","pageCount":3,"totalRows":123456,"totalBytes":987654}`, string(finalEvent.GetFinal().GetUploadReceiptJson()))
			assert.Equal(t, "SUCCEEDED", finalEvent.GetFinal().GetStatus())
			attrs := finalEvent.GetFinal().GetAttributes()
			assert.NotContains(t, attrs, "executionDiagnostics", "legacy diagnostics must not leak into the flattened attributes")
			assert.Equal(t, "4321", attrs["stats.elapsedMs"], "ordinary stats keep flattening")

			errorEvent, err := remoteQueryStreamEventFromCheckEvent(check.RemoteQueryStreamEvent{
				Type:         "error",
				MetadataJSON: `{"status":"FAILED","error":{"code":"invalid_request","message":"query is not allowlisted","retryable":false},"stats":{"elapsedMs":7},"executionDiagnostics":` + tt.diagnosticsJSON + `}`,
			}, "postgres")

			require.NoError(t, err)
			require.NotNil(t, errorEvent.GetError())
			assert.Equal(t, "invalid_request", errorEvent.GetError().GetCode())
			assert.Equal(t, "query is not allowlisted", errorEvent.GetError().GetMessage())
			assert.False(t, errorEvent.GetError().GetRetryable())
			errorAttrs := errorEvent.GetError().GetAttributes()
			assert.NotContains(t, errorAttrs, "executionDiagnostics", "legacy diagnostics must not leak into the flattened attributes")
			assert.Equal(t, "7", errorAttrs["stats.elapsedMs"])
		})
	}
}

func TestRemoteQueryStreamEventFromCheckEventPreservesNestedErrorMetadata(t *testing.T) {
	event, err := remoteQueryStreamEventFromCheckEvent(check.RemoteQueryStreamEvent{
		Type:         "error",
		MetadataJSON: `{"status":"FAILED","error":{"code":"invalid_request","message":"query is not allowlisted","retryable":false},"stats":{"elapsedMs":7}}`,
	}, "postgres")

	require.NoError(t, err)
	assert.Equal(t, uint64(0), event.GetSequence())
	require.NotNil(t, event.GetError())
	assert.Equal(t, "invalid_request", event.GetError().GetCode())
	assert.Equal(t, "query is not allowlisted", event.GetError().GetMessage())
	assert.False(t, event.GetError().GetRetryable())
	assert.Equal(t, map[string]string{"status": "FAILED", "stats.elapsedMs": "7"}, event.GetError().GetAttributes())
}

// TestRemoteQueryStreamEventFromCheckEventRejectsDataEvents proves the inline result-byte
// path is gone: a legacy data event fails closed instead of crossing AgentSecure.
func TestRemoteQueryStreamEventFromCheckEventRejectsDataEvents(t *testing.T) {
	_, err := remoteQueryStreamEventFromCheckEvent(check.RemoteQueryStreamEvent{
		Type:         "data",
		MetadataJSON: `{"sequence":7,"offset":11,"bytes":3}`,
	}, "postgres")

	require.Error(t, err)
	assert.EqualError(t, err, "unknown remote query stream event type")
}

func TestRemoteQueryIPCStreamForwarderIndexesAndTimesChunks(t *testing.T) {
	stream := &captureRemoteQueryExecuteStreamServer{}
	forwarder := newRemoteQueryIPCStreamForwarder(stream, "postgres")

	require.NoError(t, forwarder.Send(check.RemoteQueryStreamEvent{Type: "metadata", MetadataJSON: `{"status":"STARTED","operation":"produce_json_pages"}`}))
	require.NoError(t, forwarder.Send(check.RemoteQueryStreamEvent{Type: "final", MetadataJSON: `{"status":"SUCCEEDED","upload_receipt":{"uploadId":"upload-proof","pageCount":1,"totalRows":2,"totalBytes":18}}`}))

	require.Len(t, stream.chunks, 2)
	assert.Equal(t, int32(0), stream.chunks[0].GetChunkIndex())
	assert.Equal(t, "STARTED", stream.chunks[0].GetEvent().GetMetadata().GetAttributes()["status"])
	assert.Equal(t, int32(1), stream.chunks[1].GetChunkIndex())
	final := stream.chunks[1].GetEvent().GetFinal()
	require.NotNil(t, final)
	assert.Equal(t, "SUCCEEDED", final.GetStatus())
	assert.JSONEq(t, `{"uploadId":"upload-proof","pageCount":1,"totalRows":2,"totalBytes":18}`, string(final.GetUploadReceiptJson()))

	// The agent-side timing attributes are compact progress metadata on the final event.
	assert.Equal(t, "2", final.GetAttributes()["agent_ipc_send_calls"])
	assert.Contains(t, final.GetAttributes(), "agent_first_event_latency_ms")
	assert.Contains(t, final.GetAttributes(), "agent_total_stream_ms")
	assert.Contains(t, final.GetAttributes(), "agent_ipc_send_total_ms")
	assert.Contains(t, final.GetAttributes(), "agent_ipc_send_max_ms")
	assert.Equal(t, int32(2), forwarder.NextChunkIndex())
}

func TestRemoteQueryIPCStreamForwarderPropagatesSendErrors(t *testing.T) {
	stream := &captureRemoteQueryExecuteStreamServer{sendErr: assert.AnError}
	forwarder := newRemoteQueryIPCStreamForwarder(stream, "postgres")

	// A cancelled or broken IPC stream surfaces as a send error so cancellation and
	// terminal failures propagate.
	err := forwarder.Send(check.RemoteQueryStreamEvent{Type: "metadata", MetadataJSON: `{"status":"STARTED"}`})
	require.ErrorIs(t, err, assert.AnError)
}

type captureRemoteQueryExecuteStreamServer struct {
	grpc.ServerStream
	chunks  []*pb.RemoteQueryExecuteChunk
	sendErr error
}

func (s *captureRemoteQueryExecuteStreamServer) Send(chunk *pb.RemoteQueryExecuteChunk) error {
	if s.sendErr != nil {
		return s.sendErr
	}
	s.chunks = append(s.chunks, chunk)
	return nil
}

// TestRemoteQueryExecuteRequestFromProtoPreservesTraceContext proves the optional
// trace-continuation metadata crosses the AgentSecure proto boundary with its exact
// values: the active trace ID, the action.run span's own ID as the parent, and the
// propagated sampling priority — here a positive keep priority, the only domain the
// bridge forwards.
func TestRemoteQueryExecuteRequestFromProtoPreservesTraceContext(t *testing.T) {
	req, err := remoteQueryExecuteRequestFromProto(&pb.RemoteQueryExecuteRequest{
		Integration:     "postgres",
		Target:          &pb.RemoteQueryTarget{Host: "localhost", Port: 5432, Dbname: "postgres"},
		Query:           "SELECT 1 AS value",
		SessionInfoJson: validRemoteQuerySessionInfoJSONBytes(),
		TraceContext: &pb.RemoteQueryTraceContext{
			TraceId:          1234567890123456789,
			ParentId:         9876543210987654321,
			SamplingPriority: 2,
		},
	})

	require.NoError(t, err)
	require.NotNil(t, req.TraceContext)
	assert.Equal(t, uint64(1234567890123456789), req.TraceContext.TraceID)
	assert.Equal(t, uint64(9876543210987654321), req.TraceContext.SpanID)
	assert.Equal(t, 2, req.TraceContext.SamplingPriority)
}

// TestRemoteQueryExecuteRequestFromProtoDropsInvalidTraceContext proves invalid
// trace context is dropped fail-open, never fatal: a nil message, a zero trace or
// parent ID, or any non-keep sampling priority — including the tracer's drop
// priorities, which the integration's strict carrier validation would reject —
// leaves the typed request with no trace context, and the request itself stays
// valid so execution is unchanged.
func TestRemoteQueryExecuteRequestFromProtoDropsInvalidTraceContext(t *testing.T) {
	tests := []struct {
		name         string
		traceContext *pb.RemoteQueryTraceContext
	}{
		{name: "absent", traceContext: nil},
		{name: "zero trace id", traceContext: &pb.RemoteQueryTraceContext{TraceId: 0, ParentId: 2, SamplingPriority: 2}},
		{name: "zero parent id", traceContext: &pb.RemoteQueryTraceContext{TraceId: 1, ParentId: 0, SamplingPriority: 2}},
		{name: "priority above keep domain", traceContext: &pb.RemoteQueryTraceContext{TraceId: 1, ParentId: 2, SamplingPriority: 3}},
		{name: "priority below keep domain", traceContext: &pb.RemoteQueryTraceContext{TraceId: 1, ParentId: 2, SamplingPriority: -2}},
		{name: "auto drop priority", traceContext: &pb.RemoteQueryTraceContext{TraceId: 1, ParentId: 2, SamplingPriority: 0}},
		{name: "user drop priority", traceContext: &pb.RemoteQueryTraceContext{TraceId: 1, ParentId: 2, SamplingPriority: -1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := remoteQueryExecuteRequestFromProto(&pb.RemoteQueryExecuteRequest{
				Integration:     "postgres",
				Target:          &pb.RemoteQueryTarget{Host: "localhost", Port: 5432, Dbname: "postgres"},
				Query:           "SELECT 1 AS value",
				SessionInfoJson: validRemoteQuerySessionInfoJSONBytes(),
				TraceContext:    tt.traceContext,
			})

			require.NoError(t, err, "invalid trace context must never fail the request")
			assert.Nil(t, req.TraceContext)
		})
	}
}

// TestRemoteQueryStreamEventFromCheckEventTransportsArbitraryReceipt proves the final
// event carries any bounded JSON object receipt unchanged: the sessionless and
// session-backed shapes, nested future fields, arrays and nulls all cross as exact
// bytes, with no per-field copy that could drop a member.
func TestRemoteQueryStreamEventFromCheckEventTransportsArbitraryReceipt(t *testing.T) {
	receipts := []string{
		`{"objectIds":["0b9d2f4e-6a1c-4d3b-9f7e-1a2b3c4d5e6f",7,null],"pageCount":2,"totalRows":42,"totalBytes":4096}`,
		`{"uploadId":"upload-proof","pageCount":1,"totalRows":2,"totalBytes":18}`,
		`{"future":{"nested":[1,null,{"deep":[]}]},"big":123456789012345678901234567890,"n":null}`,
		`{}`,
	}
	for _, receipt := range receipts {
		event, err := remoteQueryStreamEventFromCheckEvent(check.RemoteQueryStreamEvent{
			Type:         "final",
			MetadataJSON: `{"status":"SUCCEEDED","upload_receipt":` + receipt + `,"stats":{"rowsEmitted":42}}`,
		}, "postgres")

		require.NoError(t, err)
		assert.Equal(t, receipt, string(event.GetFinal().GetUploadReceiptJson()))
		assert.NotContains(t, event.GetFinal().GetAttributes(), "upload_receipt")
	}
}

func TestRemoteQueryStreamEventFromCheckEventWithoutReceiptHasNoReceipt(t *testing.T) {
	for _, metadata := range []string{`{"status":"SUCCEEDED"}`, `{"status":"SUCCEEDED","upload_receipt":null}`} {
		event, err := remoteQueryStreamEventFromCheckEvent(check.RemoteQueryStreamEvent{Type: "final", MetadataJSON: metadata}, "postgres")
		require.NoError(t, err)
		assert.Empty(t, event.GetFinal().GetUploadReceiptJson())
	}
}

// TestRemoteQueryStreamEventFromCheckEventRejectsUnboundedReceipt proves a receipt that is
// not one JSON object within the transport bound fails the event instead of being
// truncated or reshaped.
func TestRemoteQueryStreamEventFromCheckEventRejectsUnboundedReceipt(t *testing.T) {
	tests := map[string]string{
		"array":     `[1,2]`,
		"string":    `"x"`,
		"number":    `1`,
		"oversized": `{"a":"` + strings.Repeat("x", remotequeriesimpl.RemoteQueryUploadReceiptMaxBytes) + `"}`,
	}
	for name, receipt := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := remoteQueryStreamEventFromCheckEvent(check.RemoteQueryStreamEvent{
				Type:         "final",
				MetadataJSON: `{"status":"SUCCEEDED","upload_receipt":` + receipt + `}`,
			}, "postgres")
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "xxxx")
		})
	}
}
