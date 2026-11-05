// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remotequeriesimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

const (
	// RemoteQueryExecuteEndpointPath is mounted under /agent by the Agent command API.
	RemoteQueryExecuteEndpointPath = "/remote-queries/execute"
	// RemoteQueriesExecuteEnabledConfig is disabled by default when the key is absent.
	RemoteQueriesExecuteEnabledConfig = "remote_queries.execute.enabled"

	// RemoteQueryOperationProduceJSONPages is the one supported integration operation:
	// produce bounded JSON page files and upload them directly to its-agent-intake.
	RemoteQueryOperationProduceJSONPages = "produce_json_pages"

	statusExecutorUnavailable = "executor_unavailable"
)

// Transport bounds of the opaque session_info and upload receipt objects. The Agent
// only checks that each is one JSON object within its size bound; it never parses,
// validates or interprets a nested field, so the integration owns every semantic check.
const (
	// RemoteQuerySessionInfoMaxBytes bounds the serialized session_info object forwarded
	// to the integration.
	RemoteQuerySessionInfoMaxBytes = 64 << 10
	// RemoteQueryUploadReceiptMaxBytes bounds the serialized upload receipt object the
	// integration returns in its final event.
	RemoteQueryUploadReceiptMaxBytes = 16 << 10
)

// RemoteQueryTraceContext carries the optional trace-continuation metadata the
// private-action-runner attaches at the Remote Queries action boundary: the active
// trace ID, the action.run span's own ID as the downstream parent, and the effective
// sampling priority — restricted to the positive keep priorities 1 and 2, the exact
// domain the integration's strict carrier validation accepts. It is observability
// metadata only — never signed task data and never an authorization, routing, or
// result-validation input — and it is optional end to end: absent or invalid values
// leave the query executing exactly as before.
type RemoteQueryTraceContext struct {
	TraceID          uint64
	SpanID           uint64
	SamplingPriority int
}

const (
	// remoteQuerySamplingPriorityMin and remoteQuerySamplingPriorityMax bound the
	// only sampling-priority values that may cross the bridge: the tracer's positive
	// keep priorities, AutoKeep (1) and UserKeep (2), which are exactly the values the
	// integration's strict carrier validation accepts. A propagated priority outside
	// the keep domain — the tracer's drop priorities UserDrop (-1) and AutoDrop (0), or
	// any other integer — is unsupported trace context and is dropped, never fatal.
	remoteQuerySamplingPriorityMin = 1
	remoteQuerySamplingPriorityMax = 2
)

// NewRemoteQueryTraceContext validates the optional propagated trace context and
// returns the normalized value, or nil when the context is invalid: a zero trace or
// parent ID cannot identify a real trace, and only the positive keep sampling
// priorities 1 and 2 are supported — the integration's strict carrier validation
// accepts exactly those, so anything else (including the tracer's drop priorities
// -1 and 0) must never be emitted. Invalid context is dropped without failing the
// request, so an untraced or old caller executes exactly as before, and optional
// invalid metadata never becomes a strict query-validation failure downstream.
func NewRemoteQueryTraceContext(traceID, spanID uint64, samplingPriority int) *RemoteQueryTraceContext {
	if traceID == 0 || spanID == 0 {
		return nil
	}
	if samplingPriority < remoteQuerySamplingPriorityMin || samplingPriority > remoteQuerySamplingPriorityMax {
		return nil
	}
	return &RemoteQueryTraceContext{
		TraceID:          traceID,
		SpanID:           spanID,
		SamplingPriority: samplingPriority,
	}
}

// normalizeRemoteQueryTraceContext revalidates an already-typed trace context at the
// integration request boundary, so a hand-assembled typed request can emit only a
// supported context on the wire.
func normalizeRemoteQueryTraceContext(traceContext *RemoteQueryTraceContext) *RemoteQueryTraceContext {
	if traceContext == nil {
		return nil
	}
	return NewRemoteQueryTraceContext(traceContext.TraceID, traceContext.SpanID, traceContext.SamplingPriority)
}

type remoteQueryStreamRunner interface {
	RunRemoteQueryStream(integration string, requestJSON string, emit func(check.RemoteQueryStreamEvent) error) error
}

type remoteQueryCheckUnwrapper interface {
	Unwrap() check.Check
}

func remoteQueryStreamRunnerFor(chk check.Check) (remoteQueryStreamRunner, bool) {
	for chk != nil {
		if runner, ok := chk.(remoteQueryStreamRunner); ok {
			return runner, true
		}
		unwrapper, ok := chk.(remoteQueryCheckUnwrapper)
		if !ok {
			break
		}
		unwrapped := unwrapper.Unwrap()
		if unwrapped == chk {
			break
		}
		chk = unwrapped
	}
	return nil, false
}

// NewRemoteQueryExecuteEndpointProvider registers the remote query execute endpoint on the internal Agent API.
func NewRemoteQueryExecuteEndpointProvider(reqs Requires) api.AgentEndpointProvider {
	h := &remoteQueryExecuteHandler{
		service: NewRemoteQueryExecuteService(reqs.Collector, reqs.Cfg.GetBool(RemoteQueriesExecuteEnabledConfig), reqs.Cfg),
		cfg:     reqs.Cfg,
	}
	return api.NewAgentEndpointProvider(h.handle, RemoteQueryExecuteEndpointPath, http.MethodPost)
}

type remoteQueryExecuteHandler struct {
	service   *RemoteQueryExecuteService
	collector RemoteQueryCollector
	enabled   bool
	cfg       config.Component
}

// RemoteQueryExecuteService executes credential-free Remote Queries requests through loaded checks.
type RemoteQueryExecuteService struct {
	collector RemoteQueryCollector
	enabled   bool
	cfg       config.Component
}

// NewRemoteQueryExecuteService creates the shared executor used by the HTTP POC endpoint and AgentSecure RPC.
func NewRemoteQueryExecuteService(collector RemoteQueryCollector, enabled bool, cfg config.Component) *RemoteQueryExecuteService {
	return &RemoteQueryExecuteService{collector: collector, enabled: enabled, cfg: cfg}
}

// RemoteQueryExecuteTarget identifies the datastore target without carrying credentials.
type RemoteQueryExecuteTarget struct {
	Host             string
	Port             int
	DBName           string
	DatabaseInstance string
}

// RemoteQueryExecuteRequest is the typed request shared by the HTTP and gRPC callers. The
// result contract is fixed: operation produce_json_pages with a required session_info
// object; there is no inline result-byte path and no caller-provided format. There is no
// resolve-time binding: execute resolves the target fresh and executes on a unique match,
// whatever the check identity is at execute time.
//
// SessionInfo is the backend-injected session metadata: one arbitrary JSON object, carried
// as its exact bytes. The Agent only transports it to the integration, which owns its
// contents and validation; no nested field is parsed, defaulted or checked here.
type RemoteQueryExecuteRequest struct {
	Integration   string
	Target        RemoteQueryExecuteTarget
	Query         string
	IncludeSchema bool
	SessionInfo   json.RawMessage
	// TraceContext is the optional propagated trace-continuation metadata; nil
	// means the caller is untraced and the integration request omits the field.
	TraceContext *RemoteQueryTraceContext
}

// ValidateRemoteQueryJSONObject is the generic transport check for an opaque JSON object
// (session_info or an upload receipt): non-empty, at most maxBytes, and one syntactically
// valid JSON object. Nothing about the nested content is inspected: nested keys, arrays,
// nulls and unknown future fields all pass through. It returns the object's bytes with
// surrounding whitespace trimmed. Errors name the field and never echo its content.
func ValidateRemoteQueryJSONObject(name string, data []byte, maxBytes int) ([]byte, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("%s is required", name)
	}
	if len(trimmed) > maxBytes {
		return nil, fmt.Errorf("%s must not exceed %d bytes", name, maxBytes)
	}
	if trimmed[0] != '{' || !json.Valid(trimmed) {
		return nil, fmt.Errorf("%s must be a JSON object", name)
	}
	return trimmed, nil
}

// NewRemoteQueryExecuteRequest validates and normalizes a typed paged-JSON request. The
// session_info object is checked only for transport (a bounded JSON object).
func NewRemoteQueryExecuteRequest(integration string, target RemoteQueryExecuteTarget, query string, includeSchema bool, sessionInfo json.RawMessage) (RemoteQueryExecuteRequest, error) {
	parsedIntegration, err := parseIntegration(integration)
	if err != nil {
		return RemoteQueryExecuteRequest{}, err
	}
	parsedTarget, err := parseExecuteTarget(target)
	if err != nil {
		return RemoteQueryExecuteRequest{}, err
	}
	if query == "" {
		return RemoteQueryExecuteRequest{}, errors.New("query is required")
	}
	normalizedSessionInfo, err := ValidateRemoteQueryJSONObject("session_info", sessionInfo, RemoteQuerySessionInfoMaxBytes)
	if err != nil {
		return RemoteQueryExecuteRequest{}, err
	}
	req := remoteQueryExecuteRequestFromInternal(remoteQueryExecuteRequest{
		Integration:   parsedIntegration,
		Target:        parsedTarget,
		Query:         query,
		IncludeSchema: includeSchema,
		SessionInfo:   normalizedSessionInfo,
	})
	return req, nil
}

// RemoteQueryExecuteError is a sanitized remote query bridge error.
type RemoteQueryExecuteError struct {
	Code    string
	Message string
}

// RemoteQueryExecuteResult is the service result.
type RemoteQueryExecuteResult struct {
	HTTPStatus int
	Status     string
	Error      *RemoteQueryExecuteError
}

const (
	// RemoteQueryStatusInvalidRequest reports a malformed or disallowed request.
	RemoteQueryStatusInvalidRequest = statusInvalidRequest
	// RemoteQueryStatusExecutorUnavailable reports an unavailable matched executor or bridge dependency.
	RemoteQueryStatusExecutorUnavailable = statusExecutorUnavailable
)

type remoteQueryExecuteRequest struct {
	Integration   string
	Target        remoteQueryTarget
	Query         string
	IncludeSchema bool
	SessionInfo   json.RawMessage
	TraceContext  *RemoteQueryTraceContext
}

// remoteQueryExecuteRequestJSON is the HTTP wire shape. session_info stays raw bytes so
// no nested field is decoded, enumerated or defaulted by the transport.
type remoteQueryExecuteRequestJSON struct {
	Integration   string                        `json:"integration"`
	Target        *remoteQueryTargetRequestJSON `json:"target"`
	Query         string                        `json:"query"`
	IncludeSchema bool                          `json:"includeSchema"`
	SessionInfo   json.RawMessage               `json:"session_info"`
}

// remoteQueryProduceJSONPagesRequestJSON is the integration request marshaled for the
// rtloader bridge. Keys are the camelCase integration contract and the shape carries no
// credentials: the integration reads the org API/application keys from Agent config.
type remoteQueryProduceJSONPagesRequestJSON struct {
	Operation     string                       `json:"operation"`
	Target        remoteQueryTargetJSON        `json:"target"`
	Query         string                       `json:"query"`
	IncludeSchema bool                         `json:"includeSchema"`
	SessionInfo   json.RawMessage              `json:"session_info"`
	TraceContext  *remoteQueryTraceContextJSON `json:"traceContext,omitempty"`
}

// remoteQueryTraceContextJSON is the optional trace-continuation object of the
// integration request. The IDs cross as unsigned decimal strings so the exact
// 64-bit value survives without numeric precision or format drift, and the
// sampling priority is an integer. Absence stays absent.
type remoteQueryTraceContextJSON struct {
	TraceID          string `json:"traceId"`
	ParentID         string `json:"parentId"`
	SamplingPriority int    `json:"samplingPriority"`
}

type remoteQueryTargetJSON struct {
	Host             string `json:"host,omitempty"`
	Port             int    `json:"port,omitempty"`
	DBName           string `json:"dbname,omitempty"`
	DatabaseInstance string `json:"database_instance,omitempty"`
}

func (h *remoteQueryExecuteHandler) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	service := h.service
	if service == nil {
		service = NewRemoteQueryExecuteService(h.collector, h.enabled, h.cfg)
	}
	if service == nil || !service.enabled {
		writeExecuteError(w, http.StatusServiceUnavailable, statusBridgeDisabled, "remote queries bridge is disabled")
		return
	}

	req, err := parseExecuteRequest(r)
	if err != nil {
		writeExecuteParseError(w, err)
		return
	}

	result := service.Execute(req)
	if result.Error != nil {
		writeExecuteError(w, result.HTTPStatus, result.Error.Code, result.Error.Message)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, `{"status":"SUCCEEDED"}`)
}

func parseExecuteRequest(r *http.Request) (RemoteQueryExecuteRequest, error) {
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		return RemoteQueryExecuteRequest{}, invalidRequestError("content-type must be application/json")
	}

	defer r.Body.Close()
	var wireReq remoteQueryExecuteRequestJSON
	if err := decodeStrictJSON(r.Body, &wireReq); err != nil {
		return RemoteQueryExecuteRequest{}, parseJSONRequestError(err)
	}

	// The wire target carries field-presence information, so selector-mode validation
	// happens here; the reconstructed typed target is then unambiguous.
	parsedTarget, err := parseTarget(wireReq.Target)
	if err != nil {
		return RemoteQueryExecuteRequest{}, err
	}
	if wireReq.Query == "" {
		return RemoteQueryExecuteRequest{}, errors.New("query is required")
	}
	req, err := NewRemoteQueryExecuteRequest(
		wireReq.Integration,
		RemoteQueryExecuteTarget(parsedTarget),
		wireReq.Query,
		wireReq.IncludeSchema,
		wireReq.SessionInfo,
	)
	if err != nil {
		return RemoteQueryExecuteRequest{}, err
	}
	return req, nil
}

// Execute rejects inline HTTP execution: Remote Queries execute over the AgentSecure
// streaming RPC only, and bulk result bytes never cross any Agent boundary.
func (s *RemoteQueryExecuteService) Execute(_ RemoteQueryExecuteRequest) RemoteQueryExecuteResult {
	return remoteQueryExecuteErrorResult(http.StatusBadRequest, statusInvalidRequest, "remote queries execute only over the AgentSecure streaming RPC")
}

// ExecuteStream executes a paged-JSON request and emits metadata-only stream events. The
// Agent is a control-plane forwarder: it carries the backend-injected session_info object
// through to the integration request JSON verbatim and passes the emit callback straight
// through. It never parses session_info, so every semantic check — including the check
// that the upload base URL is safe to receive the org API key — belongs to the integration.
// The integration uploads bounded JSON page files itself; only progress metadata, the
// final compact run receipt, and errors come back through the stream.
func (s *RemoteQueryExecuteService) ExecuteStream(ctx context.Context, req RemoteQueryExecuteRequest, emit func(check.RemoteQueryStreamEvent) error) RemoteQueryExecuteResult {
	if emit == nil {
		return remoteQueryExecuteErrorResult(http.StatusFailedDependency, statusExecutorUnavailable, "remote query stream emitter is unavailable")
	}
	if s == nil || !s.enabled {
		return remoteQueryExecuteErrorResult(http.StatusServiceUnavailable, statusBridgeDisabled, "remote queries bridge is disabled")
	}
	if s.collector == nil {
		return remoteQueryExecuteErrorResult(http.StatusFailedDependency, statusExecutorUnavailable, "remote query executor is unavailable")
	}
	if req.Query == "" {
		return remoteQueryExecuteErrorResult(http.StatusBadRequest, statusInvalidRequest, "query is required")
	}
	// A typed request assembled without NewRemoteQueryExecuteRequest must not bypass the
	// transport bounds: only a bounded JSON object ever reaches the integration.
	sessionInfo, err := ValidateRemoteQueryJSONObject("session_info", req.SessionInfo, RemoteQuerySessionInfoMaxBytes)
	if err != nil {
		return remoteQueryExecuteErrorResult(http.StatusBadRequest, statusInvalidRequest, err.Error())
	}
	req.SessionInfo = sessionInfo

	internal := req.internal()
	match, result := s.resolveExecutionTarget(internal)
	if result.Error != nil {
		return result
	}
	runner, ok := remoteQueryStreamRunnerFor(match.check)
	if !ok {
		return remoteQueryExecuteErrorResult(http.StatusFailedDependency, statusExecutorUnavailable, "matched integration check does not support remote query streaming")
	}
	requestJSON, err := marshalExecuteRequest(internal)
	if err != nil {
		return remoteQueryExecuteErrorResult(http.StatusBadRequest, statusInvalidRequest, err.Error())
	}

	if ctx.Err() != nil {
		return remoteQueryExecuteErrorResult(http.StatusRequestTimeout, statusExecutorUnavailable, "remote query request was cancelled")
	}

	runErr := runner.RunRemoteQueryStream(internal.Integration, requestJSON, emit)
	if runErr != nil {
		return remoteQueryExecuteErrorResult(http.StatusBadGateway, statusExecutorUnavailable, "remote query stream executor failed")
	}
	return RemoteQueryExecuteResult{HTTPStatus: http.StatusOK, Status: "SUCCEEDED"}
}

// resolveExecutionTarget resolves the execution target: one complete integration-owned candidate sweep, then the plain
// zero/one/many outcome. A unique match proceeds; zero matches answers
// target_not_found and multiple matches answer ambiguous_target before
// marshalExecuteRequest, any SQL dispatch, or any database work. There is no
// resolve-time match binding: the target is resolved fresh here, so a match whose
// check identity changed since a resolve call still executes as long as it is
// still the unique match. A sweep failure fails closed as executor_unavailable
// with the sweep's sanitized message.
func (s *RemoteQueryExecuteService) resolveExecutionTarget(internal remoteQueryExecuteRequest) (integrationCheckMatch, RemoteQueryExecuteResult) {
	resolution := resolveIntegrationTargets(s.collector, internal.Integration, internal.Target)
	switch resolution.status {
	case statusMatched:
		return resolution.matches[0], RemoteQueryExecuteResult{HTTPStatus: http.StatusOK}
	case statusTargetNotFound:
		return integrationCheckMatch{}, remoteQueryExecuteErrorResult(http.StatusNotFound, statusTargetNotFound, resolution.message)
	case statusAmbiguous:
		return integrationCheckMatch{}, remoteQueryExecuteErrorResult(http.StatusConflict, statusAmbiguous, resolution.message)
	default:
		return integrationCheckMatch{}, remoteQueryExecuteErrorResult(http.StatusBadGateway, statusExecutorUnavailable, resolution.message)
	}
}

func remoteQueryExecuteErrorResult(httpStatus int, status string, message string) RemoteQueryExecuteResult {
	return RemoteQueryExecuteResult{
		HTTPStatus: httpStatus,
		Status:     status,
		Error:      &RemoteQueryExecuteError{Code: status, Message: message},
	}
}

func (r RemoteQueryExecuteRequest) internal() remoteQueryExecuteRequest {
	return remoteQueryExecuteRequest{
		Integration:   r.Integration,
		Target:        remoteQueryTarget{Host: r.Target.Host, Port: r.Target.Port, DBName: r.Target.DBName, DatabaseInstance: r.Target.DatabaseInstance},
		Query:         r.Query,
		IncludeSchema: r.IncludeSchema,
		SessionInfo:   r.SessionInfo,
		TraceContext:  r.TraceContext,
	}
}

func remoteQueryExecuteRequestFromInternal(req remoteQueryExecuteRequest) RemoteQueryExecuteRequest {
	return RemoteQueryExecuteRequest{
		Integration:   req.Integration,
		Target:        RemoteQueryExecuteTarget{Host: req.Target.Host, Port: req.Target.Port, DBName: req.Target.DBName, DatabaseInstance: req.Target.DatabaseInstance},
		Query:         req.Query,
		IncludeSchema: req.IncludeSchema,
		SessionInfo:   req.SessionInfo,
		TraceContext:  req.TraceContext,
	}
}

func parseExecuteTarget(target RemoteQueryExecuteTarget) (remoteQueryTarget, error) {
	wireTarget := &remoteQueryTargetRequestJSON{Host: target.Host, DBName: target.DBName}
	if target.Host != "" {
		wireTarget.hostSet = true
	}
	if target.Port != 0 {
		wireTarget.Port = &target.Port
		wireTarget.portSet = true
	}
	if target.DBName != "" {
		wireTarget.dbnameSet = true
	}
	if target.DatabaseInstance != "" {
		wireTarget.DatabaseInstance = &target.DatabaseInstance
		wireTarget.databaseInstanceSet = true
	}
	return parseTarget(wireTarget)
}

// marshalExecuteRequest builds the integration request JSON. It always emits the fixed
// operation and the explicit includeSchema flag, and carries the opaque session_info
// object verbatim as a top-level key, so the integration alone interprets it (including
// its upload target); the Agent adds, removes and rewrites nothing in it. The integration reads the org
// API/application keys from Agent config, so they never appear on the request wire. The
// optional propagated trace context is revalidated here and emitted as the top-level
// traceContext object — decimal-string IDs and the integer sampling priority — only
// when a supported keep-priority context is present; absent stays absent and invalid
// — including any non-keep priority — is dropped without failing the run, so the
// integration never receives a carrier its strict validation would reject.
func marshalExecuteRequest(req remoteQueryExecuteRequest) (string, error) {
	sessionInfo, err := ValidateRemoteQueryJSONObject("session_info", req.SessionInfo, RemoteQuerySessionInfoMaxBytes)
	if err != nil {
		return "", err
	}
	wireReq := remoteQueryProduceJSONPagesRequestJSON{
		Operation:     RemoteQueryOperationProduceJSONPages,
		Target:        remoteQueryTargetJSON{Host: req.Target.Host, Port: req.Target.Port, DBName: req.Target.DBName, DatabaseInstance: req.Target.DatabaseInstance},
		Query:         req.Query,
		IncludeSchema: req.IncludeSchema,
		SessionInfo:   sessionInfo,
	}
	if traceContext := normalizeRemoteQueryTraceContext(req.TraceContext); traceContext != nil {
		wireReq.TraceContext = &remoteQueryTraceContextJSON{
			TraceID:          strconv.FormatUint(traceContext.TraceID, 10),
			ParentID:         strconv.FormatUint(traceContext.SpanID, 10),
			SamplingPriority: traceContext.SamplingPriority,
		}
	}
	requestJSON, err := json.Marshal(wireReq)
	if err != nil {
		return "", err
	}
	return string(requestJSON), nil
}

func writeExecuteParseError(w http.ResponseWriter, err error) {
	parseErr, ok := err.(requestParseError)
	if !ok {
		writeExecuteError(w, http.StatusBadRequest, statusInvalidRequest, err.Error())
		return
	}

	writeExecuteError(w, http.StatusBadRequest, parseErr.status, parseErr.message)
}

func writeExecuteError(w http.ResponseWriter, httpStatus int, status string, message string) {
	w.WriteHeader(httpStatus)
	_ = json.NewEncoder(w).Encode(struct {
		Status string         `json:"status"`
		Error  *responseError `json:"error"`
	}{
		Status: status,
		Error:  &responseError{Code: status, Message: message},
	})
}
