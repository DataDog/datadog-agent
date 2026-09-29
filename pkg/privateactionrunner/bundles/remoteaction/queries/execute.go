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
	"strconv"
	"strings"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/devtracing"
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

// The fleet mini-tracer's trace-propagation environment contract (see
// telemetry.EnvFromContext): the decimal DATADOG_* variables a child of the
// private-action-runner reads to continue the runner's trace. The bundle reads
// the same contract on the in-process side of the boundary.
const (
	miniTracerTraceIDEnv          = "DATADOG_TRACE_ID"
	miniTracerParentIDEnv         = "DATADOG_PARENT_ID"
	miniTracerSamplingPriorityEnv = "DATADOG_SAMPLING_PRIORITY"
)

const (
	// miniTracerDropTraceID is the trace ID the fleet mini-tracer assigns to a trace
	// it will not report — head-sampled fresh traces and traces carrying a drop
	// sampling priority (it mirrors the tracer's unexported dropTraceID). Such a
	// trace has no real identity to continue, so the propagation field stays
	// absent for it.
	miniTracerDropTraceID = 1
	// miniTracerDefaultSamplingPriority is the mini-tracer's effective sampling
	// priority for a trace that propagated none: the flush stamps 2 (user keep) on
	// every completed span without one, so 2 is also the effective keep priority
	// reported for such traces.
	miniTracerDefaultSamplingPriority = 2
)

// ---------------------------------------------------------------------------
// Development tracing — Remote Queries POC. DEVELOPMENT-ONLY: delete this whole
// section with the POC (and the devtracing package); the trace-context
// propagation above and below it stays. The tracer setup and the explicit
// development endpoint live in the devtracing package.
//
// While development tracing is enabled, the bundle wraps every remote-query
// execution in a normal dd-trace-go span.

const (
	// developmentExecuteOperation is the span wrapping one remote-query
	// execution; the integration's producer and upload spans nest beneath it.
	developmentExecuteOperation = "remote_queries.agent_execute"

	// The development span's identity tags: the run's identity and its terminal
	// status only — never the query text, the target, or any credential, token,
	// or upload instruction.
	developmentRunIDTag       = "run_id"
	developmentTaskIDTag      = "task_id"
	developmentIntegrationTag = "integration"
	developmentStatusTag      = "status"

	// developmentDispatchErrorStatus is the terminal status of a dispatch that
	// failed before the AgentSecure stream produced a terminal event.
	developmentDispatchErrorStatus = "ERROR"
)

// startDevelopmentExecutionSpan starts the development remote_queries.agent_execute
// span wrapping one remote-query execution, continuing the runner's trace: the
// span is a child of the action.run span active on ctx (read through the
// mini-tracer's environment contract), inherits the trace's sampling priority,
// and is tagged with the run's identity only. It returns the span (nil when
// development tracing is disabled), the context carrying it, and the trace
// context the AgentSecure request must propagate: the span's own identity when
// the span is real, so the integration's spans nest beneath the agent_execute
// span; otherwise the runner's identity, which is also the disabled path's
// behavior. With no runner trace the span is a fresh root of its own.
func startDevelopmentExecutionSpan(ctx context.Context, inputs ExecuteInputs) (*tracer.Span, context.Context, *pb.RemoteQueryTraceContext) {
	runnerTraceContext := remoteQueryTraceContextFromContext(ctx)
	if !devtracing.Enabled() {
		return nil, ctx, runnerTraceContext
	}
	devtracing.EnsureTracer()
	// The standard Datadog propagation headers: the propagator extracts the
	// runner's trace and sampling priority, making the development span their
	// child. Without a runner trace the empty carrier starts a fresh root span.
	carrier := tracer.TextMapCarrier{}
	if runnerTraceContext != nil {
		carrier[tracer.DefaultTraceIDHeader] = strconv.FormatUint(runnerTraceContext.GetTraceId(), 10)
		carrier[tracer.DefaultParentIDHeader] = strconv.FormatUint(runnerTraceContext.GetParentId(), 10)
		carrier[tracer.DefaultPriorityHeader] = strconv.FormatInt(int64(runnerTraceContext.GetSamplingPriority()), 10)
	}
	span, spanCtx := tracer.StartSpanFromPropagatedContext(ctx, developmentExecuteOperation, carrier)
	if delivery := inputs.ResultDelivery; delivery != nil {
		span.SetTag(developmentRunIDTag, delivery.RunID)
		span.SetTag(developmentTaskIDTag, delivery.TaskID)
	}
	span.SetTag(developmentIntegrationTag, inputs.Integration)
	if spanTraceContext := remoteQueryTraceContextFromSpan(span); spanTraceContext != nil {
		return span, spanCtx, spanTraceContext
	}
	// The tracer did not start (fail-open): the span is a no-op with no identity,
	// so the request carries the runner's own identity as in the disabled path.
	return span, spanCtx, runnerTraceContext
}

// remoteQueryTraceContextFromSpan reads the development span's own identity for
// downstream propagation: the integration's spans become children of this
// span. It returns nil when the span carries no identity — the tracer did not
// start and the span is a no-op.
func remoteQueryTraceContextFromSpan(span *tracer.Span) *pb.RemoteQueryTraceContext {
	if span == nil {
		return nil
	}
	spanContext := span.Context()
	traceID := spanContext.TraceIDLower()
	spanID := spanContext.SpanID()
	if traceID == 0 || spanID == 0 {
		return nil
	}
	samplingPriority := miniTracerDefaultSamplingPriority
	if propagated, ok := spanContext.SamplingPriority(); ok {
		samplingPriority = propagated
	}
	return &pb.RemoteQueryTraceContext{
		TraceId:          traceID,
		ParentId:         spanID,
		SamplingPriority: int32(samplingPriority),
	}
}

// remoteQueryTerminalStatus maps the dispatch outcome to the development span's
// terminal-status tag: the AgentSecure terminal status on success and on
// terminal error events (the error code), and ERROR when the dispatch failed
// before a terminal event existed.
func remoteQueryTerminalStatus(output interface{}, err error) string {
	if err != nil {
		return developmentDispatchErrorStatus
	}
	if out, ok := output.(map[string]interface{}); ok {
		if status, ok := out["status"].(string); ok && status != "" {
			return status
		}
	}
	return developmentDispatchErrorStatus
}

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
// outcome; a matched answer carries the status only, because there is no resolve-time
// binding to revalidate on execute. Resolve mode never dispatches a query, never
// creates a page writer, and never touches upload credentials — enforced by the
// mode's input contract and request mapping.
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
// API/application keys are read by the integration from Agent config, and the session
// is identified solely by its upload id — there is no per-session upload token.
// resolveOnly selects the side-effect-free resolution mode; when true the input is
// target-only and the execute-only fields are forbidden (see validateResolveOnlyInputs).
type ExecuteInputs struct {
	Integration    string                `json:"integration"`
	Target         TargetInputs          `json:"target"`
	Query          string                `json:"query"`
	IncludeSchema  bool                  `json:"includeSchema"`
	ResultDelivery *ResultDeliveryInputs `json:"resultDelivery"`
	ResolveOnly    bool                  `json:"resolveOnly"`

	// Presence of the execute-only fields on the wire. Resolve mode forbids those
	// fields by presence — an empty query or a null resultDelivery is still a
	// contract violation — so the decode records presence alongside the values.
	querySet          bool
	includeSchemaSet  bool
	resultDeliverySet bool
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
		Integration    string                `json:"integration"`
		Target         TargetInputs          `json:"target"`
		Query          string                `json:"query"`
		IncludeSchema  bool                  `json:"includeSchema"`
		ResultDelivery *ResultDeliveryInputs `json:"resultDelivery"`
		ResolveOnly    bool                  `json:"resolveOnly"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}

	*i = ExecuteInputs{
		Integration:    wire.Integration,
		Target:         wire.Target,
		Query:          wire.Query,
		IncludeSchema:  wire.IncludeSchema,
		ResultDelivery: wire.ResultDelivery,
		ResolveOnly:    wire.ResolveOnly,
	}
	_, i.querySet = raw["query"]
	_, i.includeSchemaSet = raw["includeSchema"]
	_, i.resultDeliverySet = raw["resultDelivery"]
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
// execute-only fields — query, includeSchema, and resultDelivery — are forbidden.
// Presence-based rejection is the conditional equivalent of the structural
// guarantee the retired standalone resolve action enforced by decoding with
// DisallowUnknownFields: a resolve-mode task cannot carry SQL or upload instructions
// even when their values are empty, because the fields themselves are the contract
// violation. The check runs after decode but before the bridge client is ever
// created, so a malformed task never reaches the Agent IPC.
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

	return a.executeRemoteQuery(ctx, client, inputs)
}

// executeRemoteQuery dispatches the customer SQL through the streaming AgentSecure
// execute RPC and maps the response stream to the AP action output. When development
// tracing is enabled, the dispatch is wrapped in the remote_queries.agent_execute
// span continuing the runner's trace, and the request's trace context carries that
// span's identity so the integration's spans join the same trace beneath it. The
// development span is observability only: it never gates or fails the execution.
func (a *ExecuteAction) executeRemoteQuery(ctx context.Context, client BridgeClient, inputs ExecuteInputs) (interface{}, error) {
	span, spanCtx, traceContext := startDevelopmentExecutionSpan(ctx, inputs)
	output, err := dispatchRemoteQueryExecute(spanCtx, client, inputs, traceContext)
	if span != nil {
		span.SetTag(developmentStatusTag, remoteQueryTerminalStatus(output, err))
		span.Finish()
	}
	return output, err
}

// dispatchRemoteQueryExecute runs the streaming AgentSecure execute RPC with the
// backend-injected upload contract and maps the response stream to the AP action
// output, failing closed on transport and contract violations.
func dispatchRemoteQueryExecute(ctx context.Context, client BridgeClient, inputs ExecuteInputs, traceContext *pb.RemoteQueryTraceContext) (interface{}, error) {
	stream, err := client.RemoteQueryExecuteStream(ctx, remoteQueryExecuteRequestFromInputs(inputs, traceContext))
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
// flag, and the backend-owned result delivery. The active trace context rides along
// as optional observability metadata: with development tracing it is the wrapping
// agent_execute span's own identity, so the integration's spans nest beneath it;
// otherwise it is the runner's identity read from the mini-tracer's environment
// contract. An absent or dropped trace context leaves the field unset so the request
// executes exactly as before.
func remoteQueryExecuteRequestFromInputs(inputs ExecuteInputs, traceContext *pb.RemoteQueryTraceContext) *pb.RemoteQueryExecuteRequest {
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
	req.TraceContext = traceContext
	return req
}

// remoteQueryTraceContextFromContext reads the runner's active trace identity through
// the fleet mini-tracer's environment-variable contract (telemetry.EnvFromContext):
// the same DATADOG_TRACE_ID, DATADOG_PARENT_ID, and DATADOG_SAMPLING_PRIORITY values
// the mini-tracer propagates to child processes. The parent is the span currently
// active on the context — the action.run span's own ID, never the AP-supplied parent.
// The field stays absent when there is no identity to continue: no active span, the
// mini-tracer's drop sentinel (a head-sampled or priority-dropped trace), or malformed
// values — propagation is fail-open and never gates execution.
func remoteQueryTraceContextFromContext(ctx context.Context) *pb.RemoteQueryTraceContext {
	var traceID, parentID uint64
	var samplingPriority int
	haveTraceID, haveParentID, haveSamplingPriority := false, false, false
	for _, entry := range telemetry.EnvFromContext(ctx) {
		key, value, _ := strings.Cut(entry, "=")
		var err error
		switch key {
		case miniTracerTraceIDEnv:
			traceID, err = strconv.ParseUint(value, 10, 64)
			haveTraceID = err == nil
		case miniTracerParentIDEnv:
			parentID, err = strconv.ParseUint(value, 10, 64)
			haveParentID = err == nil
		case miniTracerSamplingPriorityEnv:
			samplingPriority, err = strconv.Atoi(value)
			haveSamplingPriority = err == nil
		}
		if err != nil {
			return nil
		}
	}
	if !haveTraceID || !haveParentID {
		return nil
	}
	if traceID == 0 || parentID == 0 || traceID == miniTracerDropTraceID {
		return nil
	}
	if !haveSamplingPriority {
		samplingPriority = miniTracerDefaultSamplingPriority
	}
	return &pb.RemoteQueryTraceContext{
		TraceId:          traceID,
		ParentId:         parentID,
		SamplingPriority: int32(samplingPriority),
	}
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

	output := map[string]interface{}{
		"status": finalEvent.GetStatus(),
		"uploadReceipt": map[string]interface{}{
			"uploadId":   receipt.GetUploadId(),
			"pageCount":  receipt.GetPageCount(),
			"totalRows":  receipt.GetTotalRows(),
			"totalBytes": receipt.GetTotalBytes(),
		},
	}
	return output, nil
}

// remoteQueryErrorOutput propagates a terminal error event without a receipt. The error
// object carries exactly code and message: the AP metadata ExecutionError schema is
// strict and the worker reads only those two fields.
func remoteQueryErrorOutput(errEvent *pb.RemoteQueryStreamError) map[string]interface{} {
	output := map[string]interface{}{
		"status": errEvent.GetCode(),
		"error": map[string]interface{}{
			"code":    errEvent.GetCode(),
			"message": errEvent.GetMessage(),
		},
	}
	return output
}

// remoteQueryResolveRequestFromInputs maps the resolveOnly AP action input to the
// credential-free AgentSecure resolve request: exactly the integration and the
// target, mirroring execute's target mapping. The resolve request mapping carries
// no query and no result delivery, so the side-effect-free guarantee holds by
// construction of the mapping as well as of the input contract.
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
// {status}, plus the error object when the response carries one. A matched answer
// carries the status only — there is no resolve-time binding to revalidate on
// execute. The mapping is deliberately opaque — an unknown status passes through
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
	if code := resp.GetErrorCode(); code != "" {
		output["error"] = map[string]interface{}{
			"code":    code,
			"message": resp.GetErrorMessage(),
		}
	}
	return output, nil
}
