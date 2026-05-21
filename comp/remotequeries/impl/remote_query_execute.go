// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remotequeriesimpl

import (
	"bytes"
	"encoding/json"
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
	// legacyPostgresExecuteEnabledConfig preserves compatibility with the earlier POC key.
	legacyPostgresExecuteEnabledConfig = "remote_queries.postgres_execute.enabled"

	remoteQueryProofQuery = "SELECT 1 AS value"

	statusExecutorUnavailable = "executor_unavailable"
)

type remoteQueryRunner interface {
	RunRemoteQueryJSON(integration string, requestJSON string) (string, error)
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
		collector: reqs.Collector,
		enabled:   remoteQueryEnabled(reqs.Cfg, RemoteQueriesExecuteEnabledConfig, legacyPostgresExecuteEnabledConfig),
	}
	return api.NewAgentEndpointProvider(h.handle, RemoteQueryExecuteEndpointPath, http.MethodPost)
}

type remoteQueryExecuteHandler struct {
	collector collector.Component
	enabled   bool
}

type remoteQueryExecuteRequest struct {
	Integration string
	Target      remoteQueryTarget
	Query       string
	Limits      *remoteQueryExecuteLimits
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

	if !h.enabled {
		writeExecuteError(w, http.StatusServiceUnavailable, statusBridgeDisabled, "remote queries bridge is disabled")
		return
	}

	req, requestJSON, err := parseExecuteRequest(r)
	if err != nil {
		writeExecuteParseError(w, err)
		return
	}

	matches := h.findMatches(req.Integration, req.Target)
	switch len(matches) {
	case 0:
		writeExecuteError(w, http.StatusNotFound, statusTargetNotFound, "no matching Postgres check found")
		return
	case 1:
		// continue below
	default:
		writeExecuteError(w, http.StatusConflict, statusAmbiguous, "multiple matching Postgres checks found")
		return
	}

	runner, ok := remoteQueryRunnerFor(matches[0].check)
	if !ok {
		writeExecuteError(w, http.StatusFailedDependency, statusExecutorUnavailable, "matched Postgres check does not support remote query execution")
		return
	}

	responseJSON, err := runner.RunRemoteQueryJSON(req.Integration, requestJSON)
	if err != nil {
		writeExecuteError(w, http.StatusBadGateway, statusExecutorUnavailable, "remote query executor failed")
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, responseJSON)
}

func parseExecuteRequest(r *http.Request) (remoteQueryExecuteRequest, string, error) {
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		return remoteQueryExecuteRequest{}, "", invalidRequestError("content-type must be application/json")
	}

	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()

	var root map[string]json.RawMessage
	if err := decoder.Decode(&root); err != nil {
		return remoteQueryExecuteRequest{}, "", invalidRequestError("malformed JSON request")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return remoteQueryExecuteRequest{}, "", invalidRequestError("malformed JSON request")
	}

	for key := range root {
		switch key {
		case "integration", "target", "query", "limits":
			continue
		default:
			return remoteQueryExecuteRequest{}, "", invalidRequestError("request contains unknown field")
		}
	}

	integration, err := parseIntegrationFromRoot(root)
	if err != nil {
		return remoteQueryExecuteRequest{}, "", err
	}

	target, err := parseTargetFromRoot(root)
	if err != nil {
		return remoteQueryExecuteRequest{}, "", err
	}

	query, err := parseRequiredString(root, "query")
	if err != nil {
		return remoteQueryExecuteRequest{}, "", err
	}
	if query != remoteQueryProofQuery {
		return remoteQueryExecuteRequest{}, "", fmt.Errorf("query is not allowed")
	}

	limits, err := parseExecuteLimits(root)
	if err != nil {
		return remoteQueryExecuteRequest{}, "", err
	}

	req := remoteQueryExecuteRequest{Integration: integration, Target: target, Query: query, Limits: limits}
	requestJSON, err := marshalExecuteRequest(req)
	if err != nil {
		return remoteQueryExecuteRequest{}, "", fmt.Errorf("malformed JSON request")
	}
	return req, requestJSON, nil
}

func parseTargetFromRoot(root map[string]json.RawMessage) (remoteQueryTarget, error) {
	rawTarget, ok := root["target"]
	if !ok {
		return remoteQueryTarget{}, fmt.Errorf("target is required")
	}

	var targetFields map[string]json.RawMessage
	if err := json.Unmarshal(rawTarget, &targetFields); err != nil || targetFields == nil {
		return remoteQueryTarget{}, fmt.Errorf("target must be an object")
	}
	return parseTargetFields(targetFields)
}

func parseTargetFields(targetFields map[string]json.RawMessage) (remoteQueryTarget, error) {
	for key := range targetFields {
		switch key {
		case "host", "port", "dbname":
			continue
		default:
			return remoteQueryTarget{}, fmt.Errorf("target contains unknown field")
		}
	}

	host, err := parseTargetString(targetFields, "host")
	if err != nil {
		return remoteQueryTarget{}, err
	}
	host = normalizeHost(host)
	if host == "" {
		return remoteQueryTarget{}, fmt.Errorf("target.host is required")
	}

	port, err := parseTargetPort(targetFields)
	if err != nil {
		return remoteQueryTarget{}, err
	}

	dbname, err := parseTargetString(targetFields, "dbname")
	if err != nil {
		return remoteQueryTarget{}, err
	}
	if dbname == "" {
		return remoteQueryTarget{}, fmt.Errorf("target.dbname is required")
	}

	return remoteQueryTarget{Host: host, Port: port, DBName: dbname}, nil
}

func parseRequiredString(root map[string]json.RawMessage, field string) (string, error) {
	raw, ok := root[field]
	if !ok {
		return "", fmt.Errorf("%s is required", field)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("%s must be a string", field)
	}
	return value, nil
}

func parseExecuteLimits(root map[string]json.RawMessage) (*remoteQueryExecuteLimits, error) {
	rawLimits, ok := root["limits"]
	if !ok {
		return nil, nil
	}

	var limitFields map[string]json.RawMessage
	if err := json.Unmarshal(rawLimits, &limitFields); err != nil || limitFields == nil {
		return nil, fmt.Errorf("limits must be an object")
	}

	for key := range limitFields {
		switch key {
		case "maxRows", "maxBytes", "timeoutMs":
			continue
		default:
			return nil, fmt.Errorf("limits contains unknown field")
		}
	}

	maxRows, err := parsePositiveJSONInt(limitFields, "limits.maxRows", "maxRows")
	if err != nil {
		return nil, err
	}
	maxBytes, err := parsePositiveJSONInt(limitFields, "limits.maxBytes", "maxBytes")
	if err != nil {
		return nil, err
	}
	timeoutMs, err := parsePositiveJSONInt(limitFields, "limits.timeoutMs", "timeoutMs")
	if err != nil {
		return nil, err
	}

	return &remoteQueryExecuteLimits{MaxRows: maxRows, MaxBytes: maxBytes, TimeoutMs: timeoutMs}, nil
}

func parsePositiveJSONInt(fields map[string]json.RawMessage, displayName string, wireName string) (int, error) {
	raw, ok := fields[wireName]
	if !ok {
		return 0, fmt.Errorf("%s is required", displayName)
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value int
	if err := decoder.Decode(&value); err != nil {
		return 0, fmt.Errorf("%s must be an integer", displayName)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return 0, fmt.Errorf("%s must be an integer", displayName)
	}
	if value < 1 {
		return 0, fmt.Errorf("%s must be at least 1", displayName)
	}
	return value, nil
}

func (h *remoteQueryExecuteHandler) findMatches(integration string, target remoteQueryTarget) []postgresCheckMatch {
	switch integration {
	case integrationPostgres:
		return findPostgresMatches(h.collector, target)
	default:
		return nil
	}
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

	httpStatus := http.StatusBadRequest
	if parseErr.status == statusUnsupportedIntegration {
		httpStatus = http.StatusUnprocessableEntity
	}
	writeExecuteError(w, httpStatus, parseErr.status, parseErr.message)
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
