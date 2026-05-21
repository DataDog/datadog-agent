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
	// PostgresExecuteEndpointPath is mounted under /agent by the Agent command API.
	PostgresExecuteEndpointPath = "/remote-queries/postgres/execute"
	// PostgresExecuteEnabledConfig is disabled by default when the key is absent.
	PostgresExecuteEnabledConfig = "remote_queries.postgres_execute.enabled"

	postgresRemoteQueryProofQuery = "SELECT 1 AS value"

	statusExecutorUnavailable = "executor_unavailable"
)

type postgresRemoteQueryRunner interface {
	RunPostgresRemoteQueryJSON(requestJSON string) (string, error)
}

type postgresCheckUnwrapper interface {
	Unwrap() check.Check
}

func postgresRemoteQueryRunnerFor(chk check.Check) (postgresRemoteQueryRunner, bool) {
	for chk != nil {
		if runner, ok := chk.(postgresRemoteQueryRunner); ok {
			return runner, true
		}
		unwrapper, ok := chk.(postgresCheckUnwrapper)
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

// NewPostgresExecuteEndpointProvider registers the Postgres execute endpoint on the internal Agent API.
func NewPostgresExecuteEndpointProvider(reqs Requires) api.AgentEndpointProvider {
	h := &postgresExecuteHandler{
		collector: reqs.Collector,
		enabled:   reqs.Cfg.GetBool(PostgresExecuteEnabledConfig),
	}
	return api.NewAgentEndpointProvider(h.handle, PostgresExecuteEndpointPath, http.MethodPost)
}

type postgresExecuteHandler struct {
	collector collector.Component
	enabled   bool
}

type postgresExecuteRequest struct {
	Target postgresTarget
	Query  string
	Limits *postgresExecuteLimits
}

type postgresExecuteLimits struct {
	MaxRows   int
	MaxBytes  int
	TimeoutMs int
}

type postgresExecuteRequestJSON struct {
	Target postgresTargetJSON         `json:"target"`
	Query  string                     `json:"query"`
	Limits *postgresExecuteLimitsJSON `json:"limits,omitempty"`
}

type postgresTargetJSON struct {
	Host   string `json:"host"`
	Port   int    `json:"port"`
	DBName string `json:"dbname"`
}

type postgresExecuteLimitsJSON struct {
	MaxRows   int `json:"maxRows"`
	MaxBytes  int `json:"maxBytes"`
	TimeoutMs int `json:"timeoutMs"`
}

func (h *postgresExecuteHandler) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if !h.enabled {
		writeExecuteError(w, http.StatusServiceUnavailable, statusBridgeDisabled, "remote queries bridge is disabled")
		return
	}

	req, requestJSON, err := parseExecuteRequest(r)
	if err != nil {
		writeExecuteError(w, http.StatusBadRequest, statusInvalidRequest, err.Error())
		return
	}

	matches := findPostgresMatches(h.collector, req.Target)
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

	runner, ok := postgresRemoteQueryRunnerFor(matches[0].check)
	if !ok {
		writeExecuteError(w, http.StatusFailedDependency, statusExecutorUnavailable, "matched Postgres check does not support remote query execution")
		return
	}

	responseJSON, err := runner.RunPostgresRemoteQueryJSON(requestJSON)
	if err != nil {
		writeExecuteError(w, http.StatusBadGateway, statusExecutorUnavailable, "remote query executor failed")
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, responseJSON)
}

func parseExecuteRequest(r *http.Request) (postgresExecuteRequest, string, error) {
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		return postgresExecuteRequest{}, "", fmt.Errorf("content-type must be application/json")
	}

	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()

	var root map[string]json.RawMessage
	if err := decoder.Decode(&root); err != nil {
		return postgresExecuteRequest{}, "", fmt.Errorf("malformed JSON request")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return postgresExecuteRequest{}, "", fmt.Errorf("malformed JSON request")
	}

	for key := range root {
		switch key {
		case "target", "query", "limits":
			continue
		default:
			if isCredentialShapedField(key) {
				return postgresExecuteRequest{}, "", fmt.Errorf("request contains disallowed credential-shaped field")
			}
			return postgresExecuteRequest{}, "", fmt.Errorf("request contains unknown field")
		}
	}

	target, err := parseTargetFromRoot(root)
	if err != nil {
		return postgresExecuteRequest{}, "", err
	}

	query, err := parseRequiredString(root, "query")
	if err != nil {
		return postgresExecuteRequest{}, "", err
	}
	if query != postgresRemoteQueryProofQuery {
		return postgresExecuteRequest{}, "", fmt.Errorf("query is not allowed")
	}

	limits, err := parseExecuteLimits(root)
	if err != nil {
		return postgresExecuteRequest{}, "", err
	}

	req := postgresExecuteRequest{Target: target, Query: query, Limits: limits}
	requestJSON, err := marshalExecuteRequest(req)
	if err != nil {
		return postgresExecuteRequest{}, "", fmt.Errorf("malformed JSON request")
	}
	return req, requestJSON, nil
}

func parseTargetFromRoot(root map[string]json.RawMessage) (postgresTarget, error) {
	rawTarget, ok := root["target"]
	if !ok {
		return postgresTarget{}, fmt.Errorf("target is required")
	}

	var targetFields map[string]json.RawMessage
	if err := json.Unmarshal(rawTarget, &targetFields); err != nil || targetFields == nil {
		return postgresTarget{}, fmt.Errorf("target must be an object")
	}
	return parseTargetFields(targetFields)
}

func parseTargetFields(targetFields map[string]json.RawMessage) (postgresTarget, error) {
	for key := range targetFields {
		switch key {
		case "host", "port", "dbname":
			continue
		default:
			if isCredentialShapedField(key) {
				return postgresTarget{}, fmt.Errorf("request contains disallowed credential-shaped field")
			}
			return postgresTarget{}, fmt.Errorf("target contains unknown field")
		}
	}

	host, err := parseTargetString(targetFields, "host")
	if err != nil {
		return postgresTarget{}, err
	}
	host = normalizeHost(host)
	if host == "" {
		return postgresTarget{}, fmt.Errorf("target.host is required")
	}

	port, err := parseTargetPort(targetFields)
	if err != nil {
		return postgresTarget{}, err
	}

	dbname, err := parseTargetString(targetFields, "dbname")
	if err != nil {
		return postgresTarget{}, err
	}
	if dbname == "" {
		return postgresTarget{}, fmt.Errorf("target.dbname is required")
	}

	return postgresTarget{Host: host, Port: port, DBName: dbname}, nil
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

func parseExecuteLimits(root map[string]json.RawMessage) (*postgresExecuteLimits, error) {
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
			if isCredentialShapedField(key) {
				return nil, fmt.Errorf("request contains disallowed credential-shaped field")
			}
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

	return &postgresExecuteLimits{MaxRows: maxRows, MaxBytes: maxBytes, TimeoutMs: timeoutMs}, nil
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

func marshalExecuteRequest(req postgresExecuteRequest) (string, error) {
	wireReq := postgresExecuteRequestJSON{
		Target: postgresTargetJSON{Host: req.Target.Host, Port: req.Target.Port, DBName: req.Target.DBName},
		Query:  req.Query,
	}
	if req.Limits != nil {
		wireReq.Limits = &postgresExecuteLimitsJSON{
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
