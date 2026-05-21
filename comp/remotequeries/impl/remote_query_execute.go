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

type remoteQueryRunner interface {
	RunRemoteQueryJSON(integration string, requestJSON string) (string, error)
}

func isRemoteQueryAllowedProofQuery(query string) bool {
	switch query {
	case remoteQueryProofSeedQuery, remoteQueryFixtureTableProofQuery:
		return true
	default:
		return false
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

// RemoteQueryExecuteRequest is the typed internal request shape shared by HTTP and gRPC callers.
type RemoteQueryExecuteRequest struct {
	Integration string
	Target      RemoteQueryExecuteTarget
	Query       string
	Limits      *RemoteQueryExecuteLimits
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
	Target      remoteQueryTarget
	Query       string
	Limits      *remoteQueryExecuteLimits
}

type remoteQueryExecuteRequestJSON struct {
	Integration string                               `json:"integration"`
	Target      *remoteQueryTargetRequestJSON        `json:"target"`
	Query       string                               `json:"query"`
	Limits      *remoteQueryExecuteLimitsRequestJSON `json:"limits,omitempty"`
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

type remoteQueryExecutorRequestJSON struct {
	Target remoteQueryTargetJSON         `json:"target"`
	Query  string                        `json:"query"`
	Limits *remoteQueryExecuteLimitsJSON `json:"limits,omitempty"`
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

	req := remoteQueryExecuteRequest{Integration: integration, Target: target, Query: wireReq.Query, Limits: limits}
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
	if s == nil || !s.enabled {
		return remoteQueryExecuteErrorResult(http.StatusServiceUnavailable, statusBridgeDisabled, "remote queries bridge is disabled")
	}
	if s.collector == nil {
		return remoteQueryExecuteErrorResult(http.StatusFailedDependency, statusExecutorUnavailable, "remote query executor is unavailable")
	}

	internal := req.internal()
	matches := findIntegrationMatches(s.collector, internal.Integration, internal.Target)
	switch len(matches) {
	case 0:
		return remoteQueryExecuteErrorResult(http.StatusNotFound, statusTargetNotFound, "no matching integration check found")
	case 1:
		// continue below
	default:
		return remoteQueryExecuteErrorResult(http.StatusConflict, statusAmbiguous, "multiple matching integration checks found")
	}

	runner, ok := remoteQueryRunnerFor(matches[0].check)
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
		Target:      remoteQueryTarget{Host: r.Target.Host, Port: r.Target.Port, DBName: r.Target.DBName},
		Query:       r.Query,
	}
	if r.Limits != nil {
		internal.Limits = &remoteQueryExecuteLimits{MaxRows: r.Limits.MaxRows, MaxBytes: r.Limits.MaxBytes, TimeoutMs: r.Limits.TimeoutMs}
	}
	return internal
}

func remoteQueryExecuteRequestFromInternal(req remoteQueryExecuteRequest) RemoteQueryExecuteRequest {
	out := RemoteQueryExecuteRequest{
		Integration: req.Integration,
		Target:      RemoteQueryExecuteTarget{Host: req.Target.Host, Port: req.Target.Port, DBName: req.Target.DBName},
		Query:       req.Query,
	}
	if req.Limits != nil {
		out.Limits = &RemoteQueryExecuteLimits{MaxRows: req.Limits.MaxRows, MaxBytes: req.Limits.MaxBytes, TimeoutMs: req.Limits.TimeoutMs}
	}
	return out
}

func marshalExecuteRequest(req remoteQueryExecuteRequest) (string, error) {
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
