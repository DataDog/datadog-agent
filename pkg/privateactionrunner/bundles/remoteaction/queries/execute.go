// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_queries

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// BridgeClient is the narrow AgentSecure gRPC client surface required by this action.
type BridgeClient interface {
	RemoteQueryExecute(ctx context.Context, in *pb.RemoteQueryExecuteRequest, opts ...grpc.CallOption) (*pb.RemoteQueryExecuteResponse, error)
	RemoteQueryExecuteStream(ctx context.Context, in *pb.RemoteQueryExecuteRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[pb.RemoteQueryExecuteChunk], error)
}

// BridgeClientFactory returns an authenticated AgentSecure client over the local Agent IPC channel.
type BridgeClientFactory func() (BridgeClient, error)

type ExecuteAction struct {
	newBridgeClient BridgeClientFactory
}

func NewExecuteAction(newBridgeClient BridgeClientFactory) *ExecuteAction {
	return &ExecuteAction{newBridgeClient: newBridgeClient}
}

type ExecuteInputs struct {
	Integration string            `json:"integration"`
	Operation   string            `json:"operation,omitempty"`
	Target      TargetInputs      `json:"target"`
	Query       string            `json:"query"`
	Format      string            `json:"format,omitempty"`
	Limits      *LimitsInputs     `json:"limits,omitempty"`
	CopyLimits  *CopyLimitsInputs `json:"copyLimits,omitempty"`
}

type TargetInputs struct {
	Host                string `json:"host"`
	Port                int    `json:"port"`
	DBName              string `json:"dbname"`
	DatabaseInstance    string `json:"database_instance"`
	hostSet             bool
	portSet             bool
	dbnameSet           bool
	databaseInstanceSet bool
}

func (t *TargetInputs) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	var wire struct {
		Host             string  `json:"host"`
		Port             *int    `json:"port"`
		DBName           string  `json:"dbname"`
		DatabaseInstance *string `json:"database_instance"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return err
	}

	*t = TargetInputs{}
	t.Host = wire.Host
	_, t.hostSet = raw["host"]
	if wire.Port != nil {
		t.Port = *wire.Port
	}
	_, t.portSet = raw["port"]
	t.DBName = wire.DBName
	_, t.dbnameSet = raw["dbname"]
	if wire.DatabaseInstance != nil {
		t.DatabaseInstance = *wire.DatabaseInstance
	}
	_, t.databaseInstanceSet = raw["database_instance"]
	return nil
}

type LimitsInputs struct {
	MaxRows   int `json:"maxRows"`
	MaxBytes  int `json:"maxBytes"`
	TimeoutMs int `json:"timeoutMs"`
}

type CopyLimitsInputs struct {
	ChunkBytes  int `json:"chunkBytes"`
	MaxBytes    int `json:"maxBytes"`
	MaxRowBytes int `json:"maxRowBytes"`
	TimeoutMs   int `json:"timeoutMs"`
}

func validateTargetInputs(target TargetInputs) error {
	databaseInstance := target.DatabaseInstance
	hasHost := strings.TrimSpace(target.Host) != ""
	hasDBName := target.DBName != ""
	hasTupleSelectorField := target.hostSet || target.portSet || target.dbnameSet
	if target.databaseInstanceSet {
		if databaseInstance == "" {
			return errors.New("target.database_instance is required")
		}
		if strings.TrimSpace(databaseInstance) != databaseInstance {
			return errors.New("target.database_instance must not contain surrounding whitespace")
		}
		if hasTupleSelectorField {
			return errors.New("target must specify exactly one selector mode")
		}
		return nil
	}
	if !hasHost || !target.portSet || !hasDBName {
		return errors.New("target must specify host, port, and dbname")
	}
	if target.Port < 1 || target.Port > 65535 {
		return errors.New("target.port is out of range")
	}
	return nil
}

func (a *ExecuteAction) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	actionStart := time.Now()
	inputs, err := types.ExtractInputs[ExecuteInputs](task)
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(
			errors.New("invalid remote query action inputs"),
			"invalid remote query action inputs",
		)
	}

	if err := validateTargetInputs(inputs.Target); err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(
			errors.New("invalid remote query action inputs"),
			"invalid remote query action inputs",
		)
	}

	if a == nil || a.newBridgeClient == nil {
		return nil, util.DefaultActionError(errors.New("remote query action requires an Agent IPC client"))
	}
	client, err := a.newBridgeClient()
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(err, "remote query action could not create an Agent IPC client")
	}
	if client == nil {
		return nil, util.DefaultActionError(errors.New("remote query action requires an AgentSecure client"))
	}

	rpcStart := time.Now()
	stream, err := client.RemoteQueryExecuteStream(ctx, remoteQueryExecuteRequestFromInputs(inputs))
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(err, "remote query AgentSecure streaming RPC failed")
	}
	rpcCreatedAt := time.Now()
	output, err := remoteQueryExecuteOutputFromStream(stream, inputs.Format)
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(err, "remote query AgentSecure streaming RPC response was invalid")
	}
	if timing, ok := output["stream_timing"].(map[string]interface{}); ok {
		now := time.Now()
		timing["action_total_ms"] = durationMillis(now.Sub(actionStart))
		timing["rpc_create_ms"] = durationMillis(rpcCreatedAt.Sub(rpcStart))
		timing["rpc_receive_and_assemble_ms"] = durationMillis(now.Sub(rpcCreatedAt))
	}
	return output, nil
}

func remoteQueryExecuteRequestFromInputs(inputs ExecuteInputs) *pb.RemoteQueryExecuteRequest {
	req := &pb.RemoteQueryExecuteRequest{
		Integration: inputs.Integration,
		Operation:   inputs.Operation,
		Format:      inputs.Format,
		Target: &pb.RemoteQueryTarget{
			Host:             inputs.Target.Host,
			Port:             int32(inputs.Target.Port),
			Dbname:           inputs.Target.DBName,
			DatabaseInstance: inputs.Target.DatabaseInstance,
		},
		Query: inputs.Query,
	}
	if inputs.Limits != nil {
		req.Limits = &pb.RemoteQueryExecuteLimits{
			MaxRows:   int32(inputs.Limits.MaxRows),
			MaxBytes:  int32(inputs.Limits.MaxBytes),
			TimeoutMs: int32(inputs.Limits.TimeoutMs),
		}
	}
	if inputs.CopyLimits != nil {
		req.CopyLimits = &pb.RemoteQueryExecuteCopyLimits{
			ChunkBytes:  int32(inputs.CopyLimits.ChunkBytes),
			MaxBytes:    int32(inputs.CopyLimits.MaxBytes),
			MaxRowBytes: int32(inputs.CopyLimits.MaxRowBytes),
			TimeoutMs:   int32(inputs.CopyLimits.TimeoutMs),
		}
	}
	return req
}

func remoteQueryExecuteOutputFromStream(stream grpc.ServerStreamingClient[pb.RemoteQueryExecuteChunk], requestedFormat string) (map[string]interface{}, error) {
	if stream == nil {
		return nil, errors.New("remote query response stream missing")
	}

	typedStreamEvents := make([]map[string]interface{}, 0)
	streamStart := time.Now()
	var firstChunkAt time.Time
	var firstDataAt time.Time
	var finalChunkAt time.Time
	payloadBytes := 0
	dataChunksReceived := 0
	expectedChunkIndex := int32(0)
	seenFinal := false
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if chunk == nil {
			return nil, errors.New("remote query response stream returned nil chunk")
		}
		if firstChunkAt.IsZero() {
			firstChunkAt = time.Now()
		}
		if chunk.GetChunkIndex() != expectedChunkIndex {
			return nil, errors.New("remote query response stream chunk index mismatch")
		}
		if seenFinal {
			return nil, errors.New("remote query response stream sent chunk after final")
		}
		if event := chunk.GetEvent(); event != nil {
			streamEvent, err := remoteQueryStreamEventFromProto(event)
			if err != nil {
				return nil, err
			}
			if streamEvent["type"] == "data" {
				if firstDataAt.IsZero() {
					firstDataAt = time.Now()
				}
				if payload, ok := streamEvent["payload"].([]byte); ok {
					payloadBytes += len(payload)
				}
				dataChunksReceived++
			}
			typedStreamEvents = append(typedStreamEvents, streamEvent)
		} else if !chunk.GetFinal() {
			return nil, errors.New("remote query response stream chunk missing typed event")
		}
		seenFinal = chunk.GetFinal()
		if seenFinal {
			finalChunkAt = time.Now()
		}
		expectedChunkIndex++
	}
	if !seenFinal {
		return nil, errors.New("remote query response stream missing final chunk")
	}
	if len(typedStreamEvents) == 0 {
		return nil, errors.New("remote query response stream missing typed events")
	}
	output, err := remoteQueryExecuteOutputFromTypedEvents(typedStreamEvents, requestedFormat)
	if err != nil {
		return nil, err
	}
	if _, isError := output["error"]; !isError {
		output["stream_summary"] = map[string]interface{}{
			"payload_bytes":   payloadBytes,
			"chunks_received": dataChunksReceived,
		}
		if payloadBytes > 0 {
			output["stream_timing"] = remoteQueryStreamTiming(streamStart, firstChunkAt, firstDataAt, finalChunkAt, payloadBytes, dataChunksReceived)
		}
	}
	return output, nil
}

func remoteQueryStreamTiming(streamStart time.Time, firstChunkAt time.Time, firstDataAt time.Time, finalChunkAt time.Time, payloadBytes int, chunksReceived int) map[string]interface{} {
	streamEnd := time.Now()
	dataDuration := finalChunkAt.Sub(firstDataAt)
	if firstDataAt.IsZero() || finalChunkAt.IsZero() {
		dataDuration = 0
	}
	return map[string]interface{}{
		"payload_bytes":                payloadBytes,
		"chunks_received":              chunksReceived,
		"first_chunk_latency_ms":       durationMillis(firstChunkAt.Sub(streamStart)),
		"first_data_latency_ms":        durationMillis(firstDataAt.Sub(streamStart)),
		"data_to_final_ms":             durationMillis(dataDuration),
		"stream_loop_total_ms":         durationMillis(streamEnd.Sub(streamStart)),
		"data_to_final_mib_per_second": mibPerSecond(payloadBytes, dataDuration),
		"stream_loop_mib_per_second":   mibPerSecond(payloadBytes, streamEnd.Sub(streamStart)),
	}
}

func durationMillis(duration time.Duration) float64 {
	if duration <= 0 {
		return 0
	}
	return duration.Seconds() * 1000
}

func mibPerSecond(bytes int, duration time.Duration) float64 {
	if bytes <= 0 || duration <= 0 {
		return 0
	}
	return (float64(bytes) / 1024 / 1024) / duration.Seconds()
}

func remoteQueryStreamEventFromProto(event *pb.RemoteQueryExecuteStreamEvent) (map[string]interface{}, error) {
	out := map[string]interface{}{"sequence": event.GetSequence()}
	switch e := event.GetEvent().(type) {
	case *pb.RemoteQueryExecuteStreamEvent_Metadata:
		out["type"] = "metadata"
		out["operation"] = e.Metadata.GetOperation()
		out["integration"] = e.Metadata.GetIntegration()
		out["format"] = e.Metadata.GetFormat()
		if len(e.Metadata.GetAttributes()) > 0 {
			out["attributes"] = e.Metadata.GetAttributes()
		}
	case *pb.RemoteQueryExecuteStreamEvent_Data:
		out["type"] = "data"
		payload := append([]byte(nil), e.Data.GetPayload()...)
		out["payload"] = payload
		out["offset"] = e.Data.GetOffset()
		out["bytes"] = e.Data.GetBytes()
	case *pb.RemoteQueryExecuteStreamEvent_Final:
		out["type"] = "final"
		out["status"] = e.Final.GetStatus()
		out["bytes_emitted"] = e.Final.GetBytesEmitted()
		out["chunks_emitted"] = e.Final.GetChunksEmitted()
		if len(e.Final.GetAttributes()) > 0 {
			out["attributes"] = e.Final.GetAttributes()
		}
	case *pb.RemoteQueryExecuteStreamEvent_Error:
		out["type"] = "error"
		out["code"] = e.Error.GetCode()
		out["message"] = e.Error.GetMessage()
		out["retryable"] = e.Error.GetRetryable()
		if len(e.Error.GetAttributes()) > 0 {
			out["attributes"] = e.Error.GetAttributes()
		}
	default:
		return nil, errors.New("remote query stream response contained unknown event")
	}
	return out, nil
}

func remoteQueryExecuteOutputFromTypedEvents(events []map[string]interface{}, requestedFormat string) (map[string]interface{}, error) {
	var finalEvent map[string]interface{}
	var errorEvent map[string]interface{}
	var data bytes.Buffer
	resultFormat := strings.TrimSpace(requestedFormat)
	for _, event := range events {
		switch event["type"] {
		case "metadata":
			if format, ok := event["format"].(string); ok && strings.TrimSpace(format) != "" {
				resultFormat = format
			}
		case "data":
			if payload, ok := event["payload"].([]byte); ok {
				_, _ = data.Write(payload)
			}
		case "final":
			finalEvent = event
		case "error":
			errorEvent = event
		}
	}
	if finalEvent == nil {
		if errorEvent != nil {
			code, _ := errorEvent["code"].(string)
			message, _ := errorEvent["message"].(string)
			return map[string]interface{}{
				"status": code,
				"error":  map[string]interface{}{"code": code, "message": message},
			}, nil
		}
		return nil, errors.New("remote query stream response missing final event")
	}
	status, _ := finalEvent["status"].(string)
	if status == "" {
		return nil, errors.New("remote query stream final event missing status")
	}
	dataBytes := data.Bytes()
	output := map[string]interface{}{
		"status": status,
		"bytes":  len(dataBytes),
	}
	if resultFormat != "" {
		output["format"] = resultFormat
	}
	if strings.EqualFold(resultFormat, "binary") || !utf8.Valid(dataBytes) {
		if resultFormat == "" {
			output["format"] = "binary"
		}
		output["encoding"] = "base64"
		output["data_base64"] = base64.StdEncoding.EncodeToString(dataBytes)
		return output, nil
	}
	output["encoding"] = "utf8"
	output["data"] = string(dataBytes)
	return output, nil
}
