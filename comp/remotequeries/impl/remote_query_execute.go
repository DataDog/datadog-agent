// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remotequeriesimpl

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

const (
	// RemoteQueryExecuteEndpointPath is mounted under /agent by the Agent command API.
	RemoteQueryExecuteEndpointPath = "/remote-queries/execute"
	// RemoteQueriesExecuteEnabledConfig is disabled by default when the key is absent.
	RemoteQueriesExecuteEnabledConfig = "remote_queries.execute.enabled"

	remoteQueryProofSeedQuery         = "SELECT 1 AS value"
	remoteQueryFixtureTableProofQuery = "SELECT city, country FROM cities ORDER BY city"

	statusExecutorUnavailable = "executor_unavailable"
)

var remoteQueryLargePayloadProofQueries = map[string]int{
	"SELECT repeat('x', 1048576) AS payload":  1 << 20,
	"SELECT repeat('x', 2097152) AS payload":  2 << 20,
	"SELECT repeat('x', 4194304) AS payload":  4 << 20,
	"SELECT repeat('x', 8388608) AS payload":  8 << 20,
	"SELECT repeat('x', 16777216) AS payload": 16 << 20,
	"SELECT repeat('x', 33554432) AS payload": 32 << 20,
}

type remoteQueryRunner interface {
	RunRemoteQueryJSON(integration string, requestJSON string) (string, error)
}

type remoteQueryStreamRunner interface {
	RunRemoteQueryStream(integration string, requestJSON string, emit func(string) error) error
}

func isRemoteQueryAllowedProofQuery(query string) bool {
	switch query {
	case remoteQueryProofSeedQuery, remoteQueryFixtureTableProofQuery:
		return true
	default:
		_, ok := remoteQueryLargePayloadProofQueries[query]
		return ok
	}
}

type remoteQueryCheckUnwrapper interface {
	Unwrap() check.Check
}

func remoteQueryRunnerFor(chk check.Check) (remoteQueryRunner, bool) {
	for chk != nil {
		if runner, ok := chk.(remoteQueryRunner); ok {
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
		service: NewRemoteQueryExecuteService(reqs.Collector, reqs.Cfg.GetBool(RemoteQueriesExecuteEnabledConfig)),
	}
	return api.NewAgentEndpointProvider(h.handle, RemoteQueryExecuteEndpointPath, http.MethodPost)
}

type remoteQueryExecuteHandler struct {
	service   *RemoteQueryExecuteService
	collector collector.Component
	enabled   bool
}

// RemoteQueryExecuteService executes credential-free Remote Queries requests through loaded checks.
type RemoteQueryExecuteService struct {
	collector collector.Component
	enabled   bool
}

// NewRemoteQueryExecuteService creates the shared executor used by the HTTP POC endpoint and AgentSecure RPC.
func NewRemoteQueryExecuteService(collector collector.Component, enabled bool) *RemoteQueryExecuteService {
	return &RemoteQueryExecuteService{collector: collector, enabled: enabled}
}

// RemoteQueryExecuteTarget identifies the datastore target without carrying credentials.
type RemoteQueryExecuteTarget struct {
	Host   string
	Port   int
	DBName string
}

// RemoteQueryExecuteLimits contains optional execution limits for a remote query.
type RemoteQueryExecuteLimits struct {
	MaxRows   int
	MaxBytes  int
	TimeoutMs int
}

// RemoteQueryExecuteCopyLimits contains COPY stream execution limits.
type RemoteQueryExecuteCopyLimits struct {
	ChunkBytes  int
	MaxBytes    int
	MaxRowBytes int
	TimeoutMs   int
}

// RemoteQueryExecuteRequest is the typed internal request shape shared by HTTP and gRPC callers.
type RemoteQueryExecuteRequest struct {
	Integration string
	Operation   string
	Target      RemoteQueryExecuteTarget
	Query       string
	Format      string
	Limits      *RemoteQueryExecuteLimits
	CopyLimits  *RemoteQueryExecuteCopyLimits
}

// NewRemoteQueryCopyStreamExecuteRequest validates and normalizes a typed COPY stream request.
func NewRemoteQueryCopyStreamExecuteRequest(integration string, target RemoteQueryExecuteTarget, query string, format string, limits *RemoteQueryExecuteCopyLimits) (RemoteQueryExecuteRequest, error) {
	parsedIntegration, err := parseIntegration(integration)
	if err != nil {
		return RemoteQueryExecuteRequest{}, err
	}
	parsedTarget, err := parseTarget(&remoteQueryTargetRequestJSON{Host: target.Host, Port: &target.Port, DBName: target.DBName})
	if err != nil {
		return RemoteQueryExecuteRequest{}, err
	}
	if query == "" {
		return RemoteQueryExecuteRequest{}, fmt.Errorf("query is required")
	}
	if !isRemoteQueryAllowedProofQuery(query) {
		return RemoteQueryExecuteRequest{}, fmt.Errorf("query is not allowed")
	}
	if format == "" {
		format = "csv"
	}
	if format != "csv" {
		return RemoteQueryExecuteRequest{}, fmt.Errorf("format must be csv")
	}
	var parsedLimits *remoteQueryExecuteCopyLimits
	if limits != nil {
		parsedLimits, err = parseExecuteCopyLimits(&remoteQueryExecuteCopyLimitsRequestJSON{
			ChunkBytes:  &limits.ChunkBytes,
			MaxBytes:    &limits.MaxBytes,
			MaxRowBytes: &limits.MaxRowBytes,
			TimeoutMs:   &limits.TimeoutMs,
		})
		if err != nil {
			return RemoteQueryExecuteRequest{}, err
		}
	}
	return remoteQueryExecuteRequestFromInternal(remoteQueryExecuteRequest{Integration: parsedIntegration, Operation: "copy_stream", Target: parsedTarget, Query: query, Format: format, CopyLimits: parsedLimits}), nil
}

// RemoteQueryExecuteError is a sanitized remote query bridge error.
type RemoteQueryExecuteError struct {
	Code    string
	Message string
}

// RemoteQueryExecuteResult is the service result. ResponseJSON is set only for successful executor responses.
type RemoteQueryExecuteResult struct {
	HTTPStatus   int
	Status       string
	Error        *RemoteQueryExecuteError
	ResponseJSON string
}

const (
	// RemoteQueryStatusInvalidRequest reports a malformed or disallowed request.
	RemoteQueryStatusInvalidRequest = statusInvalidRequest
	// RemoteQueryStatusExecutorUnavailable reports an unavailable matched executor or bridge dependency.
	RemoteQueryStatusExecutorUnavailable = statusExecutorUnavailable
)

// NewRemoteQueryExecuteRequest validates and normalizes a typed Remote Queries execute request.
func NewRemoteQueryExecuteRequest(integration string, target RemoteQueryExecuteTarget, query string, limits *RemoteQueryExecuteLimits) (RemoteQueryExecuteRequest, error) {
	parsedIntegration, err := parseIntegration(integration)
	if err != nil {
		return RemoteQueryExecuteRequest{}, err
	}

	parsedTarget, err := parseTarget(&remoteQueryTargetRequestJSON{Host: target.Host, Port: &target.Port, DBName: target.DBName})
	if err != nil {
		return RemoteQueryExecuteRequest{}, err
	}

	if query == "" {
		return RemoteQueryExecuteRequest{}, fmt.Errorf("query is required")
	}
	if !isRemoteQueryAllowedProofQuery(query) {
		return RemoteQueryExecuteRequest{}, fmt.Errorf("query is not allowed")
	}

	var parsedLimits *remoteQueryExecuteLimits
	if limits != nil {
		parsedLimits, err = parseExecuteLimits(&remoteQueryExecuteLimitsRequestJSON{
			MaxRows:   &limits.MaxRows,
			MaxBytes:  &limits.MaxBytes,
			TimeoutMs: &limits.TimeoutMs,
		})
		if err != nil {
			return RemoteQueryExecuteRequest{}, err
		}
	}

	return remoteQueryExecuteRequestFromInternal(remoteQueryExecuteRequest{
		Integration: parsedIntegration,
		Target:      parsedTarget,
		Query:       query,
		Limits:      parsedLimits,
	}), nil
}

type remoteQueryExecuteRequest struct {
	Integration string
	Operation   string
	Target      remoteQueryTarget
	Query       string
	Format      string
	Limits      *remoteQueryExecuteLimits
	CopyLimits  *remoteQueryExecuteCopyLimits
}

type remoteQueryExecuteRequestJSON struct {
	Integration string                                   `json:"integration"`
	Operation   string                                   `json:"operation,omitempty"`
	Target      *remoteQueryTargetRequestJSON            `json:"target"`
	Query       string                                   `json:"query"`
	Format      string                                   `json:"format,omitempty"`
	Limits      *remoteQueryExecuteLimitsRequestJSON     `json:"limits,omitempty"`
	CopyLimits  *remoteQueryExecuteCopyLimitsRequestJSON `json:"copyLimits,omitempty"`
}

type remoteQueryExecuteLimitsRequestJSON struct {
	MaxRows   *int `json:"maxRows"`
	MaxBytes  *int `json:"maxBytes"`
	TimeoutMs *int `json:"timeoutMs"`
}

type remoteQueryExecuteLimits struct {
	MaxRows   int
	MaxBytes  int
	TimeoutMs int
}

type remoteQueryExecuteCopyLimitsRequestJSON struct {
	ChunkBytes  *int `json:"chunkBytes"`
	MaxBytes    *int `json:"maxBytes"`
	MaxRowBytes *int `json:"maxRowBytes"`
	TimeoutMs   *int `json:"timeoutMs"`
}

type remoteQueryExecuteCopyLimits struct {
	ChunkBytes  int
	MaxBytes    int
	MaxRowBytes int
	TimeoutMs   int
}

type remoteQueryExecutorRequestJSON struct {
	Target remoteQueryTargetJSON         `json:"target"`
	Query  string                        `json:"query"`
	Limits *remoteQueryExecuteLimitsJSON `json:"limits,omitempty"`
}

type remoteQueryCopyExecutorRequestJSON struct {
	Operation string                            `json:"operation"`
	Target    remoteQueryTargetJSON             `json:"target"`
	Query     string                            `json:"query"`
	Format    string                            `json:"format"`
	Limits    *remoteQueryExecuteCopyLimitsJSON `json:"limits,omitempty"`
}

type remoteQueryExecuteCopyLimitsJSON struct {
	ChunkBytes  int `json:"chunkBytes"`
	MaxBytes    int `json:"maxBytes"`
	MaxRowBytes int `json:"maxRowBytes"`
	TimeoutMs   int `json:"timeoutMs"`
}

type remoteQueryTargetJSON struct {
	Host   string `json:"host"`
	Port   int    `json:"port"`
	DBName string `json:"dbname"`
}

type remoteQueryExecuteLimitsJSON struct {
	MaxRows   int `json:"maxRows"`
	MaxBytes  int `json:"maxBytes"`
	TimeoutMs int `json:"timeoutMs"`
}

func (h *remoteQueryExecuteHandler) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	service := h.service
	if service == nil {
		service = NewRemoteQueryExecuteService(h.collector, h.enabled)
	}
	if service == nil || !service.enabled {
		writeExecuteError(w, http.StatusServiceUnavailable, statusBridgeDisabled, "remote queries bridge is disabled")
		return
	}

	req, _, err := parseExecuteRequest(r)
	if err != nil {
		writeExecuteParseError(w, err)
		return
	}

	result := service.Execute(remoteQueryExecuteRequestFromInternal(req))
	if result.Error != nil {
		writeExecuteError(w, result.HTTPStatus, result.Error.Code, result.Error.Message)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, result.ResponseJSON)
}

func parseExecuteRequest(r *http.Request) (remoteQueryExecuteRequest, string, error) {
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		return remoteQueryExecuteRequest{}, "", invalidRequestError("content-type must be application/json")
	}

	defer r.Body.Close()
	var wireReq remoteQueryExecuteRequestJSON
	if err := decodeStrictJSON(r.Body, &wireReq); err != nil {
		return remoteQueryExecuteRequest{}, "", parseJSONRequestError(err)
	}

	integration, err := parseIntegration(wireReq.Integration)
	if err != nil {
		return remoteQueryExecuteRequest{}, "", err
	}

	target, err := parseTarget(wireReq.Target)
	if err != nil {
		return remoteQueryExecuteRequest{}, "", err
	}

	if wireReq.Query == "" {
		return remoteQueryExecuteRequest{}, "", fmt.Errorf("query is required")
	}
	if !isRemoteQueryAllowedProofQuery(wireReq.Query) {
		return remoteQueryExecuteRequest{}, "", fmt.Errorf("query is not allowed")
	}

	limits, err := parseExecuteLimits(wireReq.Limits)
	if err != nil {
		return remoteQueryExecuteRequest{}, "", err
	}
	copyLimits, err := parseExecuteCopyLimits(wireReq.CopyLimits)
	if err != nil {
		return remoteQueryExecuteRequest{}, "", err
	}

	req := remoteQueryExecuteRequest{Integration: integration, Operation: wireReq.Operation, Target: target, Query: wireReq.Query, Format: wireReq.Format, Limits: limits, CopyLimits: copyLimits}
	requestJSON, err := marshalExecuteRequest(req)
	if err != nil {
		return remoteQueryExecuteRequest{}, "", fmt.Errorf("malformed JSON request")
	}
	return req, requestJSON, nil
}

var (
	errLimitsUnknownField = errors.New("limits contains unknown field")
	errLimitsMustBeObject = errors.New("limits must be an object")
)

func (l *remoteQueryExecuteCopyLimitsRequestJSON) UnmarshalJSON(data []byte) error {
	if !isJSONObject(data) {
		return errLimitsMustBeObject
	}

	type limitsAlias remoteQueryExecuteCopyLimitsRequestJSON
	var limits limitsAlias
	if err := decodeStrictJSON(bytes.NewReader(data), &limits); err != nil {
		if isUnknownJSONFieldError(err) {
			return errLimitsUnknownField
		}
		return err
	}
	*l = remoteQueryExecuteCopyLimitsRequestJSON(limits)
	return nil
}

func (l *remoteQueryExecuteLimitsRequestJSON) UnmarshalJSON(data []byte) error {
	if !isJSONObject(data) {
		return errLimitsMustBeObject
	}

	type limitsAlias remoteQueryExecuteLimitsRequestJSON
	var limits limitsAlias
	if err := decodeStrictJSON(bytes.NewReader(data), &limits); err != nil {
		if isUnknownJSONFieldError(err) {
			return errLimitsUnknownField
		}
		return err
	}
	*l = remoteQueryExecuteLimitsRequestJSON(limits)
	return nil
}

func parseExecuteCopyLimits(limits *remoteQueryExecuteCopyLimitsRequestJSON) (*remoteQueryExecuteCopyLimits, error) {
	if limits == nil {
		return nil, nil
	}
	chunkBytes, err := parseRequiredPositiveInt(limits.ChunkBytes, "copyLimits.chunkBytes")
	if err != nil {
		return nil, err
	}
	maxBytes, err := parseRequiredPositiveInt(limits.MaxBytes, "copyLimits.maxBytes")
	if err != nil {
		return nil, err
	}
	maxRowBytes, err := parseRequiredPositiveInt(limits.MaxRowBytes, "copyLimits.maxRowBytes")
	if err != nil {
		return nil, err
	}
	timeoutMs, err := parseRequiredPositiveInt(limits.TimeoutMs, "copyLimits.timeoutMs")
	if err != nil {
		return nil, err
	}
	return &remoteQueryExecuteCopyLimits{ChunkBytes: chunkBytes, MaxBytes: maxBytes, MaxRowBytes: maxRowBytes, TimeoutMs: timeoutMs}, nil
}

func parseExecuteLimits(limits *remoteQueryExecuteLimitsRequestJSON) (*remoteQueryExecuteLimits, error) {
	if limits == nil {
		return nil, nil
	}

	maxRows, err := parseRequiredPositiveInt(limits.MaxRows, "limits.maxRows")
	if err != nil {
		return nil, err
	}
	maxBytes, err := parseRequiredPositiveInt(limits.MaxBytes, "limits.maxBytes")
	if err != nil {
		return nil, err
	}
	timeoutMs, err := parseRequiredPositiveInt(limits.TimeoutMs, "limits.timeoutMs")
	if err != nil {
		return nil, err
	}

	return &remoteQueryExecuteLimits{MaxRows: maxRows, MaxBytes: maxBytes, TimeoutMs: timeoutMs}, nil
}

func parseRequiredPositiveInt(value *int, name string) (int, error) {
	if value == nil {
		return 0, fmt.Errorf("%s is required", name)
	}
	if *value < 1 {
		return 0, fmt.Errorf("%s must be at least 1", name)
	}
	return *value, nil
}

func (s *RemoteQueryExecuteService) Execute(req RemoteQueryExecuteRequest) RemoteQueryExecuteResult {
	if req.Operation == "copy_stream" {
		return remoteQueryExecuteErrorResult(http.StatusBadRequest, statusInvalidRequest, "copy_stream requires the streaming executor")
	}
	if s == nil || !s.enabled {
		return remoteQueryExecuteErrorResult(http.StatusServiceUnavailable, statusBridgeDisabled, "remote queries bridge is disabled")
	}
	if s.collector == nil {
		return remoteQueryExecuteErrorResult(http.StatusFailedDependency, statusExecutorUnavailable, "remote query executor is unavailable")
	}

	internal := req.internal()
	match, result := s.matchExecutor(internal)
	if result.Error != nil {
		return result
	}

	runner, ok := remoteQueryRunnerFor(match.check)
	if !ok {
		return remoteQueryExecuteErrorResult(http.StatusFailedDependency, statusExecutorUnavailable, "matched integration check does not support remote query execution")
	}

	requestJSON, err := marshalExecuteRequest(internal)
	if err != nil {
		return remoteQueryExecuteErrorResult(http.StatusBadRequest, statusInvalidRequest, "malformed JSON request")
	}

	responseJSON, err := runner.RunRemoteQueryJSON(internal.Integration, requestJSON)
	if err != nil {
		return remoteQueryExecuteErrorResult(http.StatusBadGateway, statusExecutorUnavailable, "remote query executor failed")
	}

	return RemoteQueryExecuteResult{HTTPStatus: http.StatusOK, ResponseJSON: responseJSON}
}

// ExecuteStream executes a COPY streaming request and emits serialized stream events without materializing the full result.
func (s *RemoteQueryExecuteService) ExecuteStream(req RemoteQueryExecuteRequest, emit func(string) error) RemoteQueryExecuteResult {
	if req.Operation != "copy_stream" {
		return s.Execute(req)
	}
	if emit == nil {
		return remoteQueryExecuteErrorResult(http.StatusFailedDependency, statusExecutorUnavailable, "remote query stream emitter is unavailable")
	}
	if s == nil || !s.enabled {
		return remoteQueryExecuteErrorResult(http.StatusServiceUnavailable, statusBridgeDisabled, "remote queries bridge is disabled")
	}
	if s.collector == nil {
		return remoteQueryExecuteErrorResult(http.StatusFailedDependency, statusExecutorUnavailable, "remote query executor is unavailable")
	}

	internal := req.internal()
	match, result := s.matchExecutor(internal)
	if result.Error != nil {
		return result
	}
	runner, ok := remoteQueryStreamRunnerFor(match.check)
	if !ok {
		return remoteQueryExecuteErrorResult(http.StatusFailedDependency, statusExecutorUnavailable, "matched integration check does not support remote query streaming")
	}
	requestJSON, err := marshalExecuteRequest(internal)
	if err != nil {
		return remoteQueryExecuteErrorResult(http.StatusBadRequest, statusInvalidRequest, "malformed JSON request")
	}
	if err := runner.RunRemoteQueryStream(internal.Integration, requestJSON, emit); err != nil {
		return remoteQueryExecuteErrorResult(http.StatusBadGateway, statusExecutorUnavailable, "remote query stream executor failed")
	}
	return RemoteQueryExecuteResult{HTTPStatus: http.StatusOK, Status: "SUCCEEDED"}
}

func (s *RemoteQueryExecuteService) matchExecutor(internal remoteQueryExecuteRequest) (integrationCheckMatch, RemoteQueryExecuteResult) {
	matches := findIntegrationMatches(s.collector, internal.Integration, internal.Target)
	switch len(matches) {
	case 0:
		return integrationCheckMatch{}, remoteQueryExecuteErrorResult(http.StatusNotFound, statusTargetNotFound, "no matching integration check found")
	case 1:
		return matches[0], RemoteQueryExecuteResult{HTTPStatus: http.StatusOK}
	default:
		return integrationCheckMatch{}, remoteQueryExecuteErrorResult(http.StatusConflict, statusAmbiguous, "multiple matching integration checks found")
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
	internal := remoteQueryExecuteRequest{
		Integration: r.Integration,
		Operation:   r.Operation,
		Target:      remoteQueryTarget{Host: r.Target.Host, Port: r.Target.Port, DBName: r.Target.DBName},
		Query:       r.Query,
		Format:      r.Format,
	}
	if r.Limits != nil {
		internal.Limits = &remoteQueryExecuteLimits{MaxRows: r.Limits.MaxRows, MaxBytes: r.Limits.MaxBytes, TimeoutMs: r.Limits.TimeoutMs}
	}
	if r.CopyLimits != nil {
		internal.CopyLimits = &remoteQueryExecuteCopyLimits{ChunkBytes: r.CopyLimits.ChunkBytes, MaxBytes: r.CopyLimits.MaxBytes, MaxRowBytes: r.CopyLimits.MaxRowBytes, TimeoutMs: r.CopyLimits.TimeoutMs}
	}
	return internal
}

func remoteQueryExecuteRequestFromInternal(req remoteQueryExecuteRequest) RemoteQueryExecuteRequest {
	out := RemoteQueryExecuteRequest{
		Integration: req.Integration,
		Operation:   req.Operation,
		Target:      RemoteQueryExecuteTarget{Host: req.Target.Host, Port: req.Target.Port, DBName: req.Target.DBName},
		Query:       req.Query,
		Format:      req.Format,
	}
	if req.Limits != nil {
		out.Limits = &RemoteQueryExecuteLimits{MaxRows: req.Limits.MaxRows, MaxBytes: req.Limits.MaxBytes, TimeoutMs: req.Limits.TimeoutMs}
	}
	if req.CopyLimits != nil {
		out.CopyLimits = &RemoteQueryExecuteCopyLimits{ChunkBytes: req.CopyLimits.ChunkBytes, MaxBytes: req.CopyLimits.MaxBytes, MaxRowBytes: req.CopyLimits.MaxRowBytes, TimeoutMs: req.CopyLimits.TimeoutMs}
	}
	return out
}

func marshalExecuteRequest(req remoteQueryExecuteRequest) (string, error) {
	if req.Operation == "copy_stream" {
		format := req.Format
		if format == "" {
			format = "csv"
		}
		wireReq := remoteQueryCopyExecutorRequestJSON{
			Operation: req.Operation,
			Target:    remoteQueryTargetJSON{Host: req.Target.Host, Port: req.Target.Port, DBName: req.Target.DBName},
			Query:     req.Query,
			Format:    format,
		}
		if req.CopyLimits != nil {
			wireReq.Limits = &remoteQueryExecuteCopyLimitsJSON{
				ChunkBytes:  req.CopyLimits.ChunkBytes,
				MaxBytes:    req.CopyLimits.MaxBytes,
				MaxRowBytes: req.CopyLimits.MaxRowBytes,
				TimeoutMs:   req.CopyLimits.TimeoutMs,
			}
		}
		requestJSON, err := json.Marshal(wireReq)
		if err != nil {
			return "", err
		}
		return string(requestJSON), nil
	}

	wireReq := remoteQueryExecutorRequestJSON{
		Target: remoteQueryTargetJSON{Host: req.Target.Host, Port: req.Target.Port, DBName: req.Target.DBName},
		Query:  req.Query,
	}
	if req.Limits != nil {
		wireReq.Limits = &remoteQueryExecuteLimitsJSON{
			MaxRows:   req.Limits.MaxRows,
			MaxBytes:  req.Limits.MaxBytes,
			TimeoutMs: req.Limits.TimeoutMs,
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
