// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_queries

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

// remoteQueryOperationProduceJSONPages is the one supported integration operation: the
// integration produces bounded JSON page files and uploads them directly to
// its-agent-intake. The AP input carries no operation field; the native request mapping
// emits it.
const remoteQueryOperationProduceJSONPages = "produce_json_pages"

// BridgeClient is the narrow AgentSecure gRPC client surface required by this action.
// Only the streaming RPC exists: bulk result bytes never traverse AgentSecure, so there
// is no unary inline-result call.
type BridgeClient interface {
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

// ExecuteInputs is the AP action input injected by the backend: the integration,
// target, query, the explicit includeSchema flag, and the backend-owned
// resultDelivery (authoritative run/task identity, artifact version, scoped upload
// instructions, and effective limits). The input carries no credentials: the org
// API/application keys are read by the integration from Agent config, and the upload
// token is scoped by the intake to the run session.
type ExecuteInputs struct {
	Integration    string                `json:"integration"`
	Target         TargetInputs          `json:"target"`
	Query          string                `json:"query"`
	IncludeSchema  bool                  `json:"includeSchema"`
	ResultDelivery *ResultDeliveryInputs `json:"resultDelivery"`
}

type ResultDeliveryInputs struct {
	RunID           string                `json:"runId"`
	TaskID          string                `json:"taskId"`
	ArtifactVersion int64                 `json:"artifactVersion"`
	UploadID        string                `json:"uploadId"`
	BaseURL         string                `json:"baseUrl"`
	Token           string                `json:"token"`
	PartBytes       int64                 `json:"partBytes"`
	Limits          *DeliveryLimitsInputs `json:"limits"`
}

type DeliveryLimitsInputs struct {
	MaxFileBytes   int64 `json:"maxFileBytes"`
	MaxResultBytes int64 `json:"maxResultBytes"`
	MaxRowBytes    int64 `json:"maxRowBytes"`
	MaxColumns     int64 `json:"maxColumns"`
	MaxSchemaBytes int64 `json:"maxSchemaBytes"`
	MaxPages       int64 `json:"maxPages"`
	TimeoutMs      int64 `json:"timeoutMs"`
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

// validateDeliveryInputs performs the structural presence checks the bundle can do
// without duplicating the Agent's authoritative value validation: a run cannot produce
// page files without the backend-injected upload handle and limits.
func validateDeliveryInputs(delivery *ResultDeliveryInputs) error {
	if delivery == nil {
		return errors.New("resultDelivery is required")
	}
	if delivery.Limits == nil {
		return errors.New("resultDelivery.limits is required")
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
	if err := validateDeliveryInputs(inputs.ResultDelivery); err != nil {
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
	output, err := remoteQueryExecuteOutputFromStream(stream, inputs.ResultDelivery.UploadID)
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(err, "remote query AgentSecure streaming RPC response was invalid")
	}
	if timing, ok := output["stream_timing"].(map[string]interface{}); ok {
		now := time.Now()
		timing["action_total_ms"] = durationMillis(now.Sub(actionStart))
		timing["rpc_create_ms"] = durationMillis(now.Sub(rpcStart))
	}
	return output, nil
}

// remoteQueryExecuteRequestFromInputs maps the AP action input to the credential-free
// AgentSecure request. The fixed operation is emitted by the Agent's native request
// mapping; the bundle carries the integration, target, query, the explicit includeSchema
// flag, and the backend-owned result delivery.
func remoteQueryExecuteRequestFromInputs(inputs ExecuteInputs) *pb.RemoteQueryExecuteRequest {
	req := &pb.RemoteQueryExecuteRequest{
		Integration: inputs.Integration,
		Target: &pb.RemoteQueryTarget{
			Host:             inputs.Target.Host,
			Port:             int32(inputs.Target.Port),
			Dbname:           inputs.Target.DBName,
			DatabaseInstance: inputs.Target.DatabaseInstance,
		},
		Query:         inputs.Query,
		IncludeSchema: inputs.IncludeSchema,
	}
	if delivery := inputs.ResultDelivery; delivery != nil {
		protoDelivery := &pb.RemoteQueryResultDelivery{
			RunId:           delivery.RunID,
			TaskId:          delivery.TaskID,
			ArtifactVersion: int32(delivery.ArtifactVersion),
			UploadId:        delivery.UploadID,
			BaseUrl:         delivery.BaseURL,
			Token:           delivery.Token,
			PartBytes:       delivery.PartBytes,
		}
		if limits := delivery.Limits; limits != nil {
			protoDelivery.Limits = &pb.RemoteQueryUploadLimits{
				MaxFileBytes:   limits.MaxFileBytes,
				MaxResultBytes: limits.MaxResultBytes,
				MaxRowBytes:    limits.MaxRowBytes,
				MaxColumns:     limits.MaxColumns,
				MaxSchemaBytes: limits.MaxSchemaBytes,
				MaxPages:       limits.MaxPages,
				TimeoutMs:      limits.TimeoutMs,
			}
		}
		req.ResultDelivery = protoDelivery
	}
	return req
}

// remoteQueryExecuteOutputFromStream consumes the AgentSecure stream and builds the
// receipt-only AP output. The stream carries progress metadata, the final compact run
// receipt, and errors; there is no result-byte path, so the output never contains bulk
// data. A successful final must carry the compact receipt, and its uploadId must match
// the injected upload session.
func remoteQueryExecuteOutputFromStream(stream grpc.ServerStreamingClient[pb.RemoteQueryExecuteChunk], requestedUploadID string) (map[string]interface{}, error) {
	if stream == nil {
		return nil, errors.New("remote query response stream missing")
	}

	attributes := map[string]interface{}{}
	var finalEvent *pb.RemoteQueryStreamFinal
	var errorEvent *pb.RemoteQueryStreamError
	streamStart := time.Now()
	var finalChunkAt time.Time
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
		if chunk.GetChunkIndex() != expectedChunkIndex {
			return nil, errors.New("remote query response stream chunk index mismatch")
		}
		if seenFinal {
			return nil, errors.New("remote query response stream sent chunk after final")
		}
		if event := chunk.GetEvent(); event != nil {
			switch e := event.GetEvent().(type) {
			case *pb.RemoteQueryExecuteStreamEvent_Metadata:
				mergeStringAttributes(attributes, e.Metadata.GetAttributes())
			case *pb.RemoteQueryExecuteStreamEvent_Final:
				finalEvent = e.Final
				mergeStringAttributes(attributes, e.Final.GetAttributes())
			case *pb.RemoteQueryExecuteStreamEvent_Error:
				errorEvent = e.Error
				mergeStringAttributes(attributes, e.Error.GetAttributes())
			default:
				return nil, errors.New("remote query response stream contained unknown event")
			}
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

	if finalEvent == nil {
		// Terminal-error propagation: an error event replaces the final event and the
		// run reports no receipt.
		if errorEvent != nil {
			out := remoteQueryErrorOutput(errorEvent)
			if len(attributes) > 0 {
				out["attributes"] = attributes
			}
			return out, nil
		}
		return nil, errors.New("remote query response stream missing final event")
	}
	if errorEvent != nil {
		return nil, errors.New("remote query response stream sent both final and error events")
	}
	if finalEvent.GetStatus() == "" {
		return nil, errors.New("remote query response stream final event missing status")
	}

	receipt := finalEvent.GetUploadReceipt()
	if receipt == nil {
		return nil, errors.New("remote query response stream final event missing upload receipt")
	}
	if receipt.GetUploadId() == "" {
		return nil, errors.New("remote query upload receipt missing uploadId")
	}
	if requestedUploadID != "" && receipt.GetUploadId() != requestedUploadID {
		return nil, errors.New("remote query upload receipt uploadId does not match the requested upload session")
	}

	output := map[string]interface{}{
		"status": finalEvent.GetStatus(),
		"uploadReceipt": map[string]interface{}{
			"uploadId":   receipt.GetUploadId(),
			"pageCount":  receipt.GetPageCount(),
			"totalRows":  receipt.GetTotalRows(),
			"totalBytes": receipt.GetTotalBytes(),
		},
	}
	if len(attributes) > 0 {
		output["attributes"] = attributes
	}
	if !finalChunkAt.IsZero() {
		output["stream_timing"] = map[string]interface{}{
			"final_chunk_ms": durationMillis(finalChunkAt.Sub(streamStart)),
			"stream_loop_ms": durationMillis(time.Since(streamStart)),
		}
	}
	return output, nil
}

// remoteQueryErrorOutput propagates a terminal error event without a receipt.
func remoteQueryErrorOutput(errEvent *pb.RemoteQueryStreamError) map[string]interface{} {
	return map[string]interface{}{
		"status": errEvent.GetCode(),
		"error": map[string]interface{}{
			"code":      errEvent.GetCode(),
			"message":   errEvent.GetMessage(),
			"retryable": errEvent.GetRetryable(),
		},
	}
}

func mergeStringAttributes(out map[string]interface{}, attrs map[string]string) {
	for key, value := range attrs {
		out[key] = value
	}
}

func durationMillis(duration time.Duration) float64 {
	if duration <= 0 {
		return 0
	}
	return duration.Seconds() * 1000
}
