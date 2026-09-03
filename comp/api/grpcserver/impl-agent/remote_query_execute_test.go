// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agentimpl

import (
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

	// A request without the backend-injected delivery fails closed as a terminal
	// error event; the final chunk still terminates the stream.
	require.NoError(t, err)
	require.Len(t, stream.chunks, 2)
	assert.Equal(t, int32(0), stream.chunks[0].GetChunkIndex())
	assert.Equal(t, remotequeriesimpl.RemoteQueryStatusInvalidRequest, stream.chunks[0].GetEvent().GetError().GetCode())
	assert.Equal(t, "result_delivery is required", stream.chunks[0].GetEvent().GetError().GetMessage())
	assert.Equal(t, int32(1), stream.chunks[1].GetChunkIndex())
	assert.True(t, stream.chunks[1].GetFinal())
}

func remoteQueriesServiceForTest(t *testing.T) *remotequeriesimpl.RemoteQueryExecuteService {
	t.Helper()
	return remotequeriesimpl.NewRemoteQueryExecuteService(nil, true, true, nil)
}

func validRemoteQueryResultDeliveryProto() *pb.RemoteQueryResultDelivery {
	return &pb.RemoteQueryResultDelivery{
		RunId:           "run-proof",
		TaskId:          "task-proof",
		ArtifactVersion: int32(remotequeriesimpl.RemoteQueryArtifactVersion),
		UploadId:        "upload-proof",
		BaseUrl:         "https://dd.datad0g.com/api/unstable/its-agent-intake",
		Token:           "scoped-upload-token",
		PartBytes:       64 << 20,
		Limits: &pb.RemoteQueryUploadLimits{
			MaxFileBytes:   128 << 20,
			MaxResultBytes: 10 << 30,
			MaxRowBytes:    16 << 20,
			MaxColumns:     1024,
			MaxSchemaBytes: 1 << 20,
			MaxPages:       128,
			TimeoutMs:      30000,
		},
	}
}

func TestRemoteQueryExecuteRequestFromProtoPreservesPagedJSONContract(t *testing.T) {
	req, err := remoteQueryExecuteRequestFromProto(&pb.RemoteQueryExecuteRequest{
		Integration:    "postgres",
		Target:         &pb.RemoteQueryTarget{Host: "LOCALHOST.", Port: 5432, Dbname: "postgres"},
		Query:          "SELECT city, country FROM cities ORDER BY city",
		IncludeSchema:  true,
		ResultDelivery: validRemoteQueryResultDeliveryProto(),
	})

	require.NoError(t, err)
	assert.Equal(t, "postgres", req.Integration)
	assert.Equal(t, "localhost", req.Target.Host)
	assert.Equal(t, 5432, req.Target.Port)
	assert.Equal(t, "postgres", req.Target.DBName)
	assert.Equal(t, "SELECT city, country FROM cities ORDER BY city", req.Query)
	assert.True(t, req.IncludeSchema)
	require.NotNil(t, req.ResultDelivery)
	assert.Equal(t, "run-proof", req.ResultDelivery.RunID)
	assert.Equal(t, "task-proof", req.ResultDelivery.TaskID)
	assert.Equal(t, remotequeriesimpl.RemoteQueryArtifactVersion, req.ResultDelivery.ArtifactVersion)
	assert.Equal(t, "upload-proof", req.ResultDelivery.UploadID)
	assert.Equal(t, 64<<20, req.ResultDelivery.PartBytes)
	require.NotNil(t, req.ResultDelivery.Limits)
	assert.Equal(t, &remotequeriesimpl.RemoteQueryUploadLimits{
		MaxFileBytes:   128 << 20,
		MaxResultBytes: 10 << 30,
		MaxRowBytes:    16 << 20,
		MaxColumns:     1024,
		MaxSchemaBytes: 1 << 20,
		MaxPages:       128,
		TimeoutMs:      30000,
	}, req.ResultDelivery.Limits)
}

func TestRemoteQueryExecuteRequestFromProtoPreservesDatabaseInstanceTarget(t *testing.T) {
	req, err := remoteQueryExecuteRequestFromProto(&pb.RemoteQueryExecuteRequest{
		Integration:    "postgres",
		Target:         &pb.RemoteQueryTarget{DatabaseInstance: "rq-proof-a1-db1"},
		Query:          "SELECT city, country FROM cities ORDER BY city",
		ResultDelivery: validRemoteQueryResultDeliveryProto(),
	})

	require.NoError(t, err)
	assert.Equal(t, "rq-proof-a1-db1", req.Target.DatabaseInstance)
	assert.Empty(t, req.Target.Host)
	assert.Zero(t, req.Target.Port)
	assert.Empty(t, req.Target.DBName)
}

// TestRemoteQueryExecuteRequestFromProtoPreservesResultDeliverySecrets proves the
// AgentSecure proto boundary forwards baseUrl and token to the integration opaquely:
// the intake mints and owns the URL, the token is scoped to the upload session, and the
// Agent performs no allowlisting and no logging.
func TestRemoteQueryExecuteRequestFromProtoPreservesResultDeliverySecrets(t *testing.T) {
	req, err := remoteQueryExecuteRequestFromProto(&pb.RemoteQueryExecuteRequest{
		Integration:    "postgres",
		Target:         &pb.RemoteQueryTarget{Host: "localhost", Port: 5432, Dbname: "postgres"},
		Query:          "SELECT 1 AS value",
		ResultDelivery: validRemoteQueryResultDeliveryProto(),
	})

	require.NoError(t, err)
	require.NotNil(t, req.ResultDelivery)
	assert.Equal(t, "https://dd.datad0g.com/api/unstable/its-agent-intake", req.ResultDelivery.BaseURL)
	assert.Equal(t, "scoped-upload-token", req.ResultDelivery.Token)
	assert.Equal(t, "upload-proof", req.ResultDelivery.UploadID)
}

// TestRemoteQueryExecuteRequestFromProtoPreserves10GiBInt64Fidelity proves the 10 GiB
// result cap survives the AgentSecure proto boundary and the typed request without loss.
// The limit fields are int64 so the backend-owned 10 GiB cap is representable without
// overflow, and a value one byte above the cap fails closed rather than truncating.
func TestRemoteQueryExecuteRequestFromProtoPreserves10GiBInt64Fidelity(t *testing.T) {
	const tenGiB = int64(10) << 30
	const tenGiBPlusOne = tenGiB + 1

	req, err := remoteQueryExecuteRequestFromProto(&pb.RemoteQueryExecuteRequest{
		Integration:    "postgres",
		Target:         &pb.RemoteQueryTarget{Host: "localhost", Port: 5432, Dbname: "postgres"},
		Query:          "SELECT city, country FROM cities ORDER BY city",
		ResultDelivery: validRemoteQueryResultDeliveryProto(),
	})
	require.NoError(t, err)
	require.NotNil(t, req.ResultDelivery.Limits)
	assert.Equal(t, int(tenGiB), req.ResultDelivery.Limits.MaxResultBytes)

	overflowProto := validRemoteQueryResultDeliveryProto()
	overflowProto.Limits.MaxResultBytes = tenGiBPlusOne
	_, err = remoteQueryExecuteRequestFromProto(&pb.RemoteQueryExecuteRequest{
		Integration:    "postgres",
		Target:         &pb.RemoteQueryTarget{Host: "localhost", Port: 5432, Dbname: "postgres"},
		Query:          "SELECT city, country FROM cities ORDER BY city",
		ResultDelivery: overflowProto,
	})
	require.Error(t, err)
	assert.EqualError(t, err, "result_delivery.limits.maxResultBytes must not exceed 10737418240")
}

func TestRemoteQueryStreamEventFromCheckEventMapsMetadata(t *testing.T) {
	event, err := remoteQueryStreamEventFromCheckEvent(check.RemoteQueryStreamEvent{
		Type:         "metadata",
		MetadataJSON: `{"status":"STARTED","operation":"produce_json_pages","includeSchema":true,"resultDelivery":{"runId":"run-proof","uploadId":"upload-proof","limits":{"maxPages":128}}}`,
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
		MetadataJSON: `{"status":"SUCCEEDED","upload_receipt":{"uploadId":"upload-proof","pageCount":3,"totalRows":123456,"totalBytes":987654},"stats":{"rowsEmitted":123456,"pagesEmitted":3,"partsEmitted":12,"bytesEmitted":987654,"elapsedMs":4321}}`,
	}, "postgres")

	require.NoError(t, err)
	require.NotNil(t, event.GetFinal())
	assert.Equal(t, "SUCCEEDED", event.GetFinal().GetStatus())
	receipt := event.GetFinal().GetUploadReceipt()
	require.NotNil(t, receipt)
	assert.Equal(t, "upload-proof", receipt.GetUploadId())
	assert.Equal(t, int64(3), receipt.GetPageCount())
	assert.Equal(t, int64(123456), receipt.GetTotalRows())
	assert.Equal(t, int64(987654), receipt.GetTotalBytes())

	// The run progress stats surface as compact string attributes, never as bulk
	// result bytes, and the receipt fields stay exactly the four contract fields.
	attrs := event.GetFinal().GetAttributes()
	assert.Equal(t, "123456", attrs["stats.rowsEmitted"])
	assert.Equal(t, "3", attrs["stats.pagesEmitted"])
	assert.Equal(t, "12", attrs["stats.partsEmitted"])
	assert.Equal(t, "987654", attrs["stats.bytesEmitted"])
	assert.Equal(t, "4321", attrs["stats.elapsedMs"])
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
	require.NotNil(t, final.GetUploadReceipt())
	assert.Equal(t, "upload-proof", final.GetUploadReceipt().GetUploadId())
	assert.Equal(t, int64(18), final.GetUploadReceipt().GetTotalBytes())

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
