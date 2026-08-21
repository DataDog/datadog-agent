// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agentimpl

import (
	"context"
	"testing"

	"google.golang.org/grpc"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

func TestRemoteQueryExecuteResponseFromJSONMapsStructuredRows(t *testing.T) {
	resp, err := remoteQueryExecuteResponseFromJSON(`{"status":"SUCCEEDED","columns":[{"name":"value","type":"integer"}],"rows":[{"value":1}],"stats":{"elapsed_ms":2},"truncated":true}`)

	require.NoError(t, err)
	assert.Equal(t, "SUCCEEDED", resp.GetStatus())
	require.Len(t, resp.GetColumns(), 1)
	assert.Equal(t, "value", resp.GetColumns()[0].AsMap()["name"])
	require.Len(t, resp.GetRows(), 1)
	assert.Equal(t, float64(1), resp.GetRows()[0].AsMap()["value"])
	assert.True(t, resp.GetTruncated())
	assert.Equal(t, float64(2), resp.GetStats().AsMap()["elapsed_ms"])
}

func TestRemoteQueryExecuteRejectsUnaryInlineMode(t *testing.T) {
	resp, err := (&serverSecure{}).RemoteQueryExecute(context.Background(), &pb.RemoteQueryExecuteRequest{})

	require.NoError(t, err)
	assert.Equal(t, "invalid_request", resp.GetStatus())
	require.NotNil(t, resp.GetError())
	assert.Equal(t, "invalid_request", resp.GetError().GetCode())
	assert.Contains(t, resp.GetError().GetMessage(), "RemoteQueryExecuteStream")
}

func TestRemoteQueryExecuteStreamReturnsSanitizedUnavailableWhenServiceMissing(t *testing.T) {
	stream := &captureRemoteQueryExecuteStreamServer{}
	err := (&serverSecure{}).RemoteQueryExecuteStream(&pb.RemoteQueryExecuteRequest{}, stream)

	require.NoError(t, err)
	require.Len(t, stream.chunks, 2)
	assert.Equal(t, "executor_unavailable", stream.chunks[0].GetEvent().GetError().GetCode())
	assert.True(t, stream.chunks[1].GetFinal())
}

func TestRemoteQueryExecuteRequestFromProtoPreservesCopyStream(t *testing.T) {
	req, err := remoteQueryExecuteRequestFromProto(&pb.RemoteQueryExecuteRequest{
		Integration: "postgres",
		Operation:   "copy_stream",
		Format:      "csv",
		Target:      &pb.RemoteQueryTarget{Host: "LOCALHOST.", Port: 5432, Dbname: "postgres"},
		Query:       "SELECT city, country FROM cities ORDER BY city",
		CopyLimits:  &pb.RemoteQueryExecuteCopyLimits{ChunkBytes: 32, MaxBytes: 1024, MaxRowBytes: 1024, TimeoutMs: 1000},
	})

	require.NoError(t, err)
	assert.Equal(t, "postgres", req.Integration)
	assert.Equal(t, "copy_stream", req.Operation)
	assert.Equal(t, "csv", req.Format)
	require.NotNil(t, req.CopyLimits)
	assert.Equal(t, 32, req.CopyLimits.ChunkBytes)
	assert.Nil(t, req.Limits)
}

func TestRemoteQueryExecuteRequestFromProtoPreservesDatabaseInstanceTarget(t *testing.T) {
	req, err := remoteQueryExecuteRequestFromProto(&pb.RemoteQueryExecuteRequest{
		Integration: "postgres",
		Operation:   "copy_stream",
		Format:      "csv",
		Target:      &pb.RemoteQueryTarget{DatabaseInstance: "rq-proof-a1-db1"},
		Query:       "SELECT city, country FROM cities ORDER BY city",
	})

	require.NoError(t, err)
	assert.Equal(t, "rq-proof-a1-db1", req.Target.DatabaseInstance)
	assert.Empty(t, req.Target.Host)
	assert.Zero(t, req.Target.Port)
	assert.Empty(t, req.Target.DBName)
}

func TestRemoteQueryIPCStreamCoalescerFlushesDataAtFourMiB(t *testing.T) {
	stream := &captureRemoteQueryExecuteStreamServer{}
	coalescer := newRemoteQueryIPCStreamCoalescer(stream)
	const (
		threeMiB = 3 << 20 // 3 MiB.
		twoMiB   = 2 << 20 // 2 MiB.
		fiveMiB  = 5 << 20 // 5 MiB.
	)

	require.NoError(t, coalescer.Send(check.RemoteQueryStreamEvent{Type: "metadata", MetadataJSON: `{"operation":"copy_stream","format":"csv"}`}))
	require.NoError(t, coalescer.Send(check.RemoteQueryStreamEvent{Type: "data", MetadataJSON: `{"sequence":1,"offset":0,"bytes":3145728}`, Payload: make([]byte, threeMiB)}))
	assert.Len(t, stream.chunks, 1, "data below 4MiB should be coalesced before secure IPC send")
	require.NoError(t, coalescer.Send(check.RemoteQueryStreamEvent{Type: "data", MetadataJSON: `{"sequence":2,"offset":3145728,"bytes":2097152}`, Payload: make([]byte, twoMiB)}))
	require.Len(t, stream.chunks, 2, "crossing 4MiB should flush one coalesced data event")
	firstData := stream.chunks[1].GetEvent().GetData()
	require.NotNil(t, firstData)
	assert.Equal(t, uint64(0), firstData.GetOffset())
	assert.Equal(t, uint64(remoteQuerySecureIPCDataFlushBytes), firstData.GetBytes())
	assert.Len(t, firstData.GetPayload(), remoteQuerySecureIPCDataFlushBytes)

	require.NoError(t, coalescer.Flush())
	require.Len(t, stream.chunks, 3)
	secondData := stream.chunks[2].GetEvent().GetData()
	require.NotNil(t, secondData)
	assert.Equal(t, uint64(remoteQuerySecureIPCDataFlushBytes), secondData.GetOffset())
	assert.Equal(t, uint64(fiveMiB-remoteQuerySecureIPCDataFlushBytes), secondData.GetBytes())
	assert.Len(t, secondData.GetPayload(), fiveMiB-remoteQuerySecureIPCDataFlushBytes)
}

func TestRemoteQueryStreamEventFromCheckEventPreservesBinaryPayload(t *testing.T) {
	event, err := remoteQueryStreamEventFromCheckEvent(check.RemoteQueryStreamEvent{
		Type:         "data",
		MetadataJSON: `{"sequence":7,"offset":11,"bytes":3}`,
		Payload:      []byte{0x00, 0xff, 0x80},
	})

	require.NoError(t, err)
	assert.Equal(t, uint64(7), event.GetSequence())
	require.NotNil(t, event.GetData())
	assert.Equal(t, []byte{0x00, 0xff, 0x80}, event.GetData().GetPayload())
	assert.Equal(t, uint64(11), event.GetData().GetOffset())
	assert.Equal(t, uint64(3), event.GetData().GetBytes())
}

func TestRemoteQueryStreamEventFromCheckEventPreservesNestedErrorMetadata(t *testing.T) {
	event, err := remoteQueryStreamEventFromCheckEvent(check.RemoteQueryStreamEvent{
		Type:         "error",
		MetadataJSON: `{"status":"FAILED","error":{"code":"invalid_request","message":"query is not allowlisted","retryable":false}}`,
	})

	require.NoError(t, err)
	assert.Equal(t, uint64(0), event.GetSequence())
	require.NotNil(t, event.GetError())
	assert.Equal(t, "invalid_request", event.GetError().GetCode())
	assert.Equal(t, "query is not allowlisted", event.GetError().GetMessage())
	assert.False(t, event.GetError().GetRetryable())
	assert.Equal(t, map[string]string{"status": "FAILED"}, event.GetError().GetAttributes())
}

type captureRemoteQueryExecuteStreamServer struct {
	grpc.ServerStream
	chunks []*pb.RemoteQueryExecuteChunk
}

func (s *captureRemoteQueryExecuteStreamServer) Send(chunk *pb.RemoteQueryExecuteChunk) error {
	s.chunks = append(s.chunks, chunk)
	return nil
}

func TestRemoteQueryExecuteRequestFromProtoPreservesResultDelivery(t *testing.T) {
	req, err := remoteQueryExecuteRequestFromProto(&pb.RemoteQueryExecuteRequest{
		Integration:    "postgres",
		Operation:      "copy_stream",
		Format:         "csv",
		Target:         &pb.RemoteQueryTarget{Host: "localhost", Port: 5432, Dbname: "postgres"},
		Query:          "SELECT city, country FROM cities ORDER BY city",
		CopyLimits:     &pb.RemoteQueryExecuteCopyLimits{ChunkBytes: 8, MaxBytes: 24, MaxRowBytes: 32, TimeoutMs: 1000},
		ResultDelivery: &pb.RemoteQueryResultDelivery{Mode: "POC_PUBLIC_MULTIPART_UPLOAD", UploadId: "upload-01k", BaseUrl: "https://dd.datad0g.com/api/intake/its-agent-intake", Token: "scoped-upload-token", PartBytes: 8, MaxBytes: 24, Format: "csv", Compression: "none"},
	})

	require.NoError(t, err)
	require.NotNil(t, req.ResultDelivery)
	assert.Equal(t, "POC_PUBLIC_MULTIPART_UPLOAD", req.ResultDelivery.Mode)
	assert.Equal(t, "upload-01k", req.ResultDelivery.UploadID)
	assert.Equal(t, "https://dd.datad0g.com/api/intake/its-agent-intake", req.ResultDelivery.BaseURL)
	assert.Equal(t, "scoped-upload-token", req.ResultDelivery.Token)
	assert.Equal(t, 8, req.ResultDelivery.PartBytes)
	assert.Equal(t, 24, req.ResultDelivery.MaxBytes)
}

func TestRemoteQueryExecuteRequestFromProtoPreservesResultDeliveryBaseURL(t *testing.T) {
	req, err := remoteQueryExecuteRequestFromProto(&pb.RemoteQueryExecuteRequest{
		Integration:    "postgres",
		Operation:      "copy_stream",
		Format:         "csv",
		Target:         &pb.RemoteQueryTarget{Host: "localhost", Port: 5432, Dbname: "postgres"},
		Query:          "SELECT city, country FROM cities ORDER BY city",
		CopyLimits:     &pb.RemoteQueryExecuteCopyLimits{ChunkBytes: 8, MaxBytes: 24, MaxRowBytes: 32, TimeoutMs: 1000},
		ResultDelivery: &pb.RemoteQueryResultDelivery{Mode: "POC_PUBLIC_MULTIPART_UPLOAD", UploadId: "upload-01k", BaseUrl: "https://dd.datad0g.com/api/unstable/its-agent-intake", Token: "scoped-upload-token", PartBytes: 8, MaxBytes: 24, Format: "csv", Compression: "none"},
	})

	require.NoError(t, err)
	require.NotNil(t, req.ResultDelivery)
	// The Agent forwards baseUrl and token to the integration, which performs the HTTP upload itself.
	// It does not allowlist or reject the base URL; the intake mints and owns the URL.
	assert.Equal(t, "https://dd.datad0g.com/api/unstable/its-agent-intake", req.ResultDelivery.BaseURL)
	assert.Equal(t, "scoped-upload-token", req.ResultDelivery.Token)
	assert.Equal(t, "upload-01k", req.ResultDelivery.UploadID)
}

func TestRemoteQueryStreamEventFromCheckEventSurfacesUploadReceipt(t *testing.T) {
	event, err := remoteQueryStreamEventFromCheckEvent(check.RemoteQueryStreamEvent{
		Type:         "final",
		MetadataJSON: `{"status":"SUCCEEDED","bytes_emitted":18,"chunks_emitted":3,"upload_receipt":{"mode":"POC_PUBLIC_MULTIPART_UPLOAD","uploadId":"upload-01k","bucketName":"rq-bucket","objectPath":"manifests/upload-01k.json","totalBytes":18,"totalRows":2,"partCount":3,"sha256":"abc123"}}`,
	})

	require.NoError(t, err)
	require.NotNil(t, event.GetFinal())
	receipt := event.GetFinal().GetUploadReceipt()
	require.NotNil(t, receipt)
	assert.Equal(t, "POC_PUBLIC_MULTIPART_UPLOAD", receipt.GetMode())
	assert.Equal(t, "upload-01k", receipt.GetUploadId())
	assert.Equal(t, "rq-bucket", receipt.GetBucketName())
	assert.Equal(t, "manifests/upload-01k.json", receipt.GetObjectPath())
	assert.Equal(t, int64(18), receipt.GetTotalBytes())
	assert.Equal(t, int64(2), receipt.GetTotalRows())
	assert.Equal(t, int32(3), receipt.GetPartCount())
	assert.Equal(t, "abc123", receipt.GetSha256())
}
