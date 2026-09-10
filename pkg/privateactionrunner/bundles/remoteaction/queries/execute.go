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

// BridgeClient is the narrow AgentSecure gRPC client surface required by this
// bundle: the streaming execute RPC and the unary side-effect-free resolve RPC that
// backs the execute action's resolveOnly mode. Bulk result bytes never traverse
// AgentSecure, so there is no unary inline-result call.
type BridgeClient interface {
	RemoteQueryExecuteStream(ctx context.Context, in *pb.RemoteQueryExecuteRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[pb.RemoteQueryExecuteChunk], error)
	RemoteQueryResolve(ctx context.Context, in *pb.RemoteQueryResolveRequest, opts ...grpc.CallOption) (*pb.RemoteQueryResolveResponse, error)
}

// BridgeClientFactory returns an authenticated AgentSecure client over the local Agent IPC channel.
type BridgeClientFactory func() (BridgeClient, error)

// ExecuteAction runs the single Remote Queries AP action in one of two modes selected
// by the resolveOnly input. The default execute mode dispatches customer SQL through
// the streaming AgentSecure execute RPC with the backend-injected upload contract.
// The resolveOnly mode is the side-effect-free target resolution: the Agent re-runs
// the same integration matcher execute uses and answers the structured zero/one/many
// outcome, with the opaque match fingerprint for the unique case. Resolve mode never
// dispatches a query, never creates a page writer, and never touches upload
// credentials — enforced by the mode's input contract and request mapping.
type ExecuteAction struct {
	newBridgeClient BridgeClientFactory
}

func NewExecuteAction(newBridgeClient BridgeClientFactory) *ExecuteAction {
	return &ExecuteAction{newBridgeClient: newBridgeClient}
}

// ExecuteInputs is the AP action input injected by the backend: the integration,
// target, query, the explicit includeSchema flag, the backend-owned resultDelivery
// (authoritative run/task identity, artifact version, scoped upload instructions,
// and effective limits), and the optional resolve-time matchFingerprint the Agent
// revalidates before any SQL execution. The input carries no credentials: the org
// API/application keys are read by the integration from Agent config, and the session
// is identified solely by its upload id — there is no per-session upload token.
// resolveOnly selects the side-effect-free resolution mode; when true the input is
// target-only and the execute-only fields are forbidden (see validateResolveOnlyInputs).
type ExecuteInputs struct {
	Integration      string                `json:"integration"`
	Target           TargetInputs          `json:"target"`
	Query            string                `json:"query"`
	IncludeSchema    bool                  `json:"includeSchema"`
	ResultDelivery   *ResultDeliveryInputs `json:"resultDelivery"`
	MatchFingerprint string                `json:"matchFingerprint"`
	ResolveOnly      bool                  `json:"resolveOnly"`

	// Presence of the execute-only fields on the wire. Resolve mode forbids those
	// fields by presence — an empty query or a null resultDelivery is still a
	// contract violation — so the decode records presence alongside the values.
	querySet            bool
	includeSchemaSet    bool
	resultDeliverySet   bool
	matchFingerprintSet bool
}

// UnmarshalJSON decodes the AP action input tolerantly — execute-mode inputs keep
// accepting unknown fields, exactly as before — while recording whether the
// execute-only fields were present on the wire, so validateResolveOnlyInputs can
// distinguish "absent" from "present but zero-valued".
func (i *ExecuteInputs) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	var wire struct {
		Integration      string                `json:"integration"`
		Target           TargetInputs          `json:"target"`
		Query            string                `json:"query"`
		IncludeSchema    bool                  `json:"includeSchema"`
		ResultDelivery   *ResultDeliveryInputs `json:"resultDelivery"`
		MatchFingerprint string                `json:"matchFingerprint"`
		ResolveOnly      bool                  `json:"resolveOnly"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}

	*i = ExecuteInputs{
		Integration:      wire.Integration,
		Target:           wire.Target,
		Query:            wire.Query,
		IncludeSchema:    wire.IncludeSchema,
		ResultDelivery:   wire.ResultDelivery,
		MatchFingerprint: wire.MatchFingerprint,
		ResolveOnly:      wire.ResolveOnly,
	}
	_, i.querySet = raw["query"]
	_, i.includeSchemaSet = raw["includeSchema"]
	_, i.resultDeliverySet = raw["resultDelivery"]
	_, i.matchFingerprintSet = raw["matchFingerprint"]
	return nil
}

type ResultDeliveryInputs struct {
	RunID           string                `json:"runId"`
	TaskID          string                `json:"taskId"`
	ArtifactVersion int64                 `json:"artifactVersion"`
	UploadID        string                `json:"uploadId"`
	BaseURL         string                `json:"baseUrl"`
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

// validateTargetInputs enforces the target selector modes: the tuple
// {host, port, dbname} or the managed instance {database_instance}. The
// database_instance and host/port endpoint selectors stay mutually exclusive, and
// dbname alongside database_instance is rejected: it would select a check
// identity and then override its materialized configured database, bypassing the
// configured-scope contract. Presence is rejected even when the value is empty or
// null.
func validateTargetInputs(target TargetInputs) error {
	databaseInstance := target.DatabaseInstance
	hasHost := strings.TrimSpace(target.Host) != ""
	hasDBName := target.DBName != ""
	if target.databaseInstanceSet {
		if databaseInstance == "" {
			return errors.New("target.database_instance is required")
		}
		if strings.TrimSpace(databaseInstance) != databaseInstance {
			return errors.New("target.database_instance must not contain surrounding whitespace")
		}
		if target.hostSet || target.portSet {
			return errors.New("target must specify exactly one selector mode")
		}
		if target.dbnameSet {
			return errors.New("target.database_instance must not be combined with dbname")
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

// validateResolveOnlyInputs is the targeted strict validation of the resolveOnly
// mode: a resolution dispatch carries exactly the integration and target, so the
// execute-only fields — query, includeSchema, resultDelivery, and matchFingerprint —
// are forbidden. Presence-based rejection is the conditional equivalent of the
// structural guarantee the retired standalone resolve action enforced by decoding
// with DisallowUnknownFields: a resolve-mode task cannot carry SQL, upload
// instructions, or a fingerprint even when their values are empty, because the
// fields themselves are the contract violation. The check runs after decode but
// before the bridge client is ever created, so a malformed task never reaches the
// Agent IPC.
func validateResolveOnlyInputs(inputs ExecuteInputs) error {
	if inputs.querySet {
		return errors.New("resolveOnly input must not carry query")
	}
	if inputs.includeSchemaSet {
		return errors.New("resolveOnly input must not carry includeSchema")
	}
	if inputs.resultDeliverySet {
		return errors.New("resolveOnly input must not carry resultDelivery")
	}
	if inputs.matchFingerprintSet {
		return errors.New("resolveOnly input must not carry matchFingerprint")
	}
	return nil
}

func (a *ExecuteAction) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
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
	if inputs.ResolveOnly {
		if err := validateResolveOnlyInputs(inputs); err != nil {
			return nil, util.DefaultActionErrorWithDisplayError(
				errors.New("invalid remote query action inputs"),
				"invalid remote query action inputs",
			)
		}
	} else if err := validateDeliveryInputs(inputs.ResultDelivery); err != nil {
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

	if inputs.ResolveOnly {
		resp, err := client.RemoteQueryResolve(ctx, remoteQueryResolveRequestFromInputs(inputs))
		if err != nil {
			return nil, util.DefaultActionErrorWithDisplayError(err, "remote query AgentSecure resolve RPC failed")
		}
		output, err := remoteQueryResolveOutputFromResponse(resp)
		if err != nil {
			return nil, util.DefaultActionErrorWithDisplayError(err, "remote query AgentSecure resolve RPC response was invalid")
		}
		return output, nil
	}

	stream, err := client.RemoteQueryExecuteStream(ctx, remoteQueryExecuteRequestFromInputs(inputs))
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(err, "remote query AgentSecure streaming RPC failed")
	}
	output, err := remoteQueryExecuteOutputFromStream(stream, inputs.ResultDelivery.UploadID)
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(err, "remote query AgentSecure streaming RPC response was invalid")
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
		Query:            inputs.Query,
		IncludeSchema:    inputs.IncludeSchema,
		MatchFingerprint: inputs.MatchFingerprint,
	}
	if delivery := inputs.ResultDelivery; delivery != nil {
		protoDelivery := &pb.RemoteQueryResultDelivery{
			RunId:           delivery.RunID,
			TaskId:          delivery.TaskID,
			ArtifactVersion: int32(delivery.ArtifactVersion),
			UploadId:        delivery.UploadID,
			BaseUrl:         delivery.BaseURL,
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
// AP action output. The output matches the strict AP metadata schema exactly:
// {status, uploadReceipt} on success and {status, error{code, message}} on terminal
// error — no other key, because the AP output validation rejects unknown fields.
// Progress metadata and agent timing stay on the internal AgentSecure stream events;
// bulk result bytes never appear because the integration uploads page files directly.
// A successful final must carry the compact receipt, and its uploadId must match the
// injected upload session.
func remoteQueryExecuteOutputFromStream(stream grpc.ServerStreamingClient[pb.RemoteQueryExecuteChunk], requestedUploadID string) (map[string]interface{}, error) {
	if stream == nil {
		return nil, errors.New("remote query response stream missing")
	}

	var finalEvent *pb.RemoteQueryStreamFinal
	var errorEvent *pb.RemoteQueryStreamError
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
				// Progress metadata travels on the internal stream only; the AP
				// output schema has no field for it.
			case *pb.RemoteQueryExecuteStreamEvent_Final:
				finalEvent = e.Final
			case *pb.RemoteQueryExecuteStreamEvent_Error:
				errorEvent = e.Error
			default:
				return nil, errors.New("remote query response stream contained unknown event")
			}
		} else if !chunk.GetFinal() {
			return nil, errors.New("remote query response stream chunk missing typed event")
		}
		seenFinal = chunk.GetFinal()
		expectedChunkIndex++
	}
	if !seenFinal {
		return nil, errors.New("remote query response stream missing final chunk")
	}

	if finalEvent == nil {
		// Terminal-error propagation: an error event replaces the final event and the
		// run reports no receipt.
		if errorEvent != nil {
			return remoteQueryErrorOutput(errorEvent), nil
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

	return map[string]interface{}{
		"status": finalEvent.GetStatus(),
		"uploadReceipt": map[string]interface{}{
			"uploadId":   receipt.GetUploadId(),
			"pageCount":  receipt.GetPageCount(),
			"totalRows":  receipt.GetTotalRows(),
			"totalBytes": receipt.GetTotalBytes(),
		},
	}, nil
}

// remoteQueryErrorOutput propagates a terminal error event without a receipt. The error
// object carries exactly code and message: the AP metadata ExecutionError schema is
// strict and the worker reads only those two fields.
func remoteQueryErrorOutput(errEvent *pb.RemoteQueryStreamError) map[string]interface{} {
	return map[string]interface{}{
		"status": errEvent.GetCode(),
		"error": map[string]interface{}{
			"code":    errEvent.GetCode(),
			"message": errEvent.GetMessage(),
		},
	}
}

// remoteQueryResolveRequestFromInputs maps the resolveOnly AP action input to the
// credential-free AgentSecure resolve request: exactly the integration and the
// target, mirroring execute's target mapping. The resolve request mapping carries
// no query, no result delivery, and no fingerprint, so the side-effect-free
// guarantee holds by construction of the mapping as well as of the input contract.
func remoteQueryResolveRequestFromInputs(inputs ExecuteInputs) *pb.RemoteQueryResolveRequest {
	return &pb.RemoteQueryResolveRequest{
		Integration: inputs.Integration,
		Target: &pb.RemoteQueryTarget{
			Host:             inputs.Target.Host,
			Port:             int32(inputs.Target.Port),
			Dbname:           inputs.Target.DBName,
			DatabaseInstance: inputs.Target.DatabaseInstance,
		},
	}
}

// remoteQueryResolveOutputFromResponse maps the typed AgentSecure resolve response
// to the AP action output, mirroring execute's result-object conventions: exactly
// {status}, plus matchFingerprint when the response carries one and the error
// object when the response carries one. The mapping is deliberately opaque — an
// unknown status or a matched response without a fingerprint passes through
// unchanged so the dispatcher classifies well-formed contract violations itself
// instead of the bundle collapsing them into a transport failure. Only a
// structurally missing response or status fails closed.
func remoteQueryResolveOutputFromResponse(resp *pb.RemoteQueryResolveResponse) (map[string]interface{}, error) {
	if resp == nil {
		return nil, errors.New("remote query resolve response missing")
	}
	status := resp.GetStatus()
	if status == "" {
		return nil, errors.New("remote query resolve response missing status")
	}
	output := map[string]interface{}{"status": status}
	if fingerprint := resp.GetMatchFingerprint(); fingerprint != "" {
		output["matchFingerprint"] = fingerprint
	}
	if code := resp.GetErrorCode(); code != "" {
		output["error"] = map[string]interface{}{
			"code":    code,
			"message": resp.GetErrorMessage(),
		}
	}
	return output, nil
}
