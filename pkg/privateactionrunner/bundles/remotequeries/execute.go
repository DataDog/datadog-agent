// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remotequeries

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	Host   string `json:"host"`
	Port   int    `json:"port"`
	DBName string `json:"dbname"`
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

func (a *ExecuteAction) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	inputs, err := types.ExtractInputs[ExecuteInputs](task)
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(
			fmt.Errorf("invalid remote query action inputs"),
			"invalid remote query action inputs",
		)
	}

	if a == nil || a.newBridgeClient == nil {
		return nil, util.DefaultActionError(fmt.Errorf("remote query action requires an Agent IPC client"))
	}
	client, err := a.newBridgeClient()
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(err, "remote query action could not create an Agent IPC client")
	}
	if client == nil {
		return nil, util.DefaultActionError(fmt.Errorf("remote query action requires an AgentSecure client"))
	}

	stream, err := client.RemoteQueryExecuteStream(ctx, remoteQueryExecuteRequestFromInputs(inputs))
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(err, "remote query AgentSecure streaming RPC failed")
	}
	output, err := remoteQueryExecuteOutputFromStream(stream)
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(err, "remote query AgentSecure streaming RPC response was invalid")
	}
	return output, nil
}

func remoteQueryExecuteRequestFromInputs(inputs ExecuteInputs) *pb.RemoteQueryExecuteRequest {
	req := &pb.RemoteQueryExecuteRequest{
		Integration: inputs.Integration,
		Operation:   inputs.Operation,
		Format:      inputs.Format,
		Target: &pb.RemoteQueryTarget{
			Host:   inputs.Target.Host,
			Port:   int32(inputs.Target.Port),
			Dbname: inputs.Target.DBName,
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

func remoteQueryExecuteOutputFromStream(stream grpc.ServerStreamingClient[pb.RemoteQueryExecuteChunk]) (map[string]interface{}, error) {
	if stream == nil {
		return nil, fmt.Errorf("remote query response stream missing")
	}

	typedStreamEvents := make([]map[string]interface{}, 0)
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
			return nil, fmt.Errorf("remote query response stream returned nil chunk")
		}
		if chunk.GetChunkIndex() != expectedChunkIndex {
			return nil, fmt.Errorf("remote query response stream chunk index mismatch")
		}
		if seenFinal {
			return nil, fmt.Errorf("remote query response stream sent chunk after final")
		}
		if event := chunk.GetEvent(); event != nil {
			streamEvent, err := remoteQueryStreamEventFromProto(event)
			if err != nil {
				return nil, err
			}
			typedStreamEvents = append(typedStreamEvents, streamEvent)
		} else if !chunk.GetFinal() {
			return nil, fmt.Errorf("remote query response stream chunk missing typed event")
		}
		seenFinal = chunk.GetFinal()
		expectedChunkIndex++
	}
	if !seenFinal {
		return nil, fmt.Errorf("remote query response stream missing final chunk")
	}
	if len(typedStreamEvents) == 0 {
		return nil, fmt.Errorf("remote query response stream missing typed events")
	}
	return remoteQueryExecuteOutputFromTypedEvents(typedStreamEvents)
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
		if utf8.Valid(payload) {
			out["data"] = string(payload)
		}
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
		return nil, fmt.Errorf("remote query stream response contained unknown event")
	}
	return out, nil
}

func remoteQueryExecuteOutputFromTypedEvents(events []map[string]interface{}) (map[string]interface{}, error) {
	var finalEvent map[string]interface{}
	var errorEvent map[string]interface{}
	var data bytes.Buffer
	for _, event := range events {
		if event["type"] == "data" {
			if payload, ok := event["payload"].([]byte); ok {
				_, _ = data.Write(payload)
			}
		}
		if event["type"] == "final" {
			finalEvent = event
		}
		if event["type"] == "error" {
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
		return nil, fmt.Errorf("remote query stream response missing final event")
	}
	status, _ := finalEvent["status"].(string)
	if status == "" {
		return nil, fmt.Errorf("remote query stream final event missing status")
	}
	dataBytes := data.Bytes()
	output := map[string]interface{}{
		"status":     status,
		"events":     normalizeRemoteQueryOutput(events),
		"data_bytes": append([]byte(nil), dataBytes...),
	}
	if utf8.Valid(dataBytes) {
		output["data"] = string(dataBytes)
	}
	return output, nil
}

func remoteQueryExecuteOutputFromProto(resp *pb.RemoteQueryExecuteResponse) (map[string]interface{}, error) {
	if resp == nil || resp.GetStatus() == "" {
		return nil, fmt.Errorf("remote query response missing status")
	}

	output := map[string]interface{}{"status": resp.GetStatus()}
	if resp.GetError() != nil {
		output["error"] = map[string]interface{}{
			"code":    resp.GetError().GetCode(),
			"message": resp.GetError().GetMessage(),
		}
	}
	if len(resp.GetColumns()) > 0 {
		columns := make([]interface{}, 0, len(resp.GetColumns()))
		for _, column := range resp.GetColumns() {
			columns = append(columns, column.AsMap())
		}
		output["columns"] = columns
	}
	if len(resp.GetRows()) > 0 {
		rows := make([]interface{}, 0, len(resp.GetRows()))
		for _, row := range resp.GetRows() {
			rows = append(rows, row.AsMap())
		}
		output["rows"] = rows
	}
	if resp.GetTruncated() {
		output["truncated"] = true
	}
	if resp.GetStats() != nil {
		output["stats"] = resp.GetStats().AsMap()
	}
	return output, nil
}

func normalizeRemoteQueryOutput(value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(v))
		for key, item := range v {
			out[key] = normalizeRemoteQueryOutput(item)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(v))
		for i, item := range v {
			out[i] = normalizeRemoteQueryOutput(item)
		}
		return out
	case json.Number:
		if f, err := v.Float64(); err == nil {
			return f
		}
		return v.String()
	default:
		return v
	}
}
