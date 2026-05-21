// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package remotequeriesimpl implements Remote Queries POC endpoints.
package remotequeriesimpl

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"go.uber.org/fx"
	"gopkg.in/yaml.v3"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

const (
	// RemoteQueryMatchEndpointPath is mounted under /agent by the Agent command API.
	RemoteQueryMatchEndpointPath = "/remote-queries/match-check"
	// RemoteQueriesMatchEnabledConfig is disabled by default when the key is absent.
	RemoteQueriesMatchEnabledConfig = "remote_queries.match_check.enabled"
	// legacyPostgresMatchEnabledConfig preserves compatibility with the earlier POC key.
	legacyPostgresMatchEnabledConfig = "remote_queries.postgres_match_check.enabled"

	integrationPostgres = "postgres"

	statusOK                     = "ok"
	statusTargetNotFound         = "target_not_found"
	statusAmbiguous              = "ambiguous_target"
	statusInvalidRequest         = "invalid_request"
	statusUnsupportedIntegration = "unsupported_integration"
	statusBridgeDisabled         = "bridge_disabled"
)

// Requires defines dependencies for the Remote Queries POC endpoint provider.
type Requires struct {
	fx.In

	Cfg       config.Component
	Collector collector.Component
}

// NewRemoteQueryMatchEndpointProvider registers the remote query match endpoint on the internal Agent API.
func NewRemoteQueryMatchEndpointProvider(reqs Requires) api.AgentEndpointProvider {
	h := &remoteQueryMatchHandler{
		collector: reqs.Collector,
		enabled:   remoteQueryEnabled(reqs.Cfg, RemoteQueriesMatchEnabledConfig, legacyPostgresMatchEnabledConfig),
	}
	return api.NewAgentEndpointProvider(h.handle, RemoteQueryMatchEndpointPath, http.MethodPost)
}

func remoteQueryEnabled(cfg config.Component, genericKey string, legacyKey string) bool {
	if cfg.IsConfigured(genericKey) {
		return cfg.GetBool(genericKey)
	}
	return cfg.GetBool(legacyKey)
}

type remoteQueryMatchHandler struct {
	collector collector.Component
	enabled   bool
}

type matchResponse struct {
	Status       string          `json:"status"`
	MatchedCount int             `json:"matched_count"`
	Match        *sanitizedMatch `json:"match,omitempty"`
	Error        *responseError  `json:"error,omitempty"`
}

type sanitizedMatch struct {
	Integration    string `json:"integration"`
	Loader         string `json:"loader"`
	ConfigProvider string `json:"config_provider"`
}

type responseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type remoteQueryMatchRequest struct {
	Integration string
	Target      remoteQueryTarget
}

type remoteQueryTarget struct {
	Host   string
	Port   int
	DBName string
}

type requestParseError struct {
	status  string
	message string
}

func (e requestParseError) Error() string {
	return e.message
}

func invalidRequestError(message string) error {
	return requestParseError{status: statusInvalidRequest, message: message}
}

func unsupportedIntegrationError() error {
	return requestParseError{status: statusUnsupportedIntegration, message: "unsupported integration"}
}

type postgresInstanceTarget struct {
	host   string
	port   int
	dbname string
}

func (h *remoteQueryMatchHandler) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if !h.enabled {
		writeMatchResponse(w, http.StatusServiceUnavailable, statusBridgeDisabled, 0, nil, "remote queries bridge is disabled")
		return
	}

	req, err := parseMatchRequest(r)
	if err != nil {
		writeMatchParseError(w, err)
		return
	}

	matches := h.findMatches(req.Integration, req.Target)
	switch len(matches) {
	case 0:
		writeMatchResponse(w, http.StatusNotFound, statusTargetNotFound, 0, nil, "no matching Postgres check found")
	case 1:
		writeMatchResponse(w, http.StatusOK, statusOK, 1, &matches[0].sanitized, "")
	default:
		writeMatchResponse(w, http.StatusConflict, statusAmbiguous, len(matches), nil, "multiple matching Postgres checks found")
	}
}

func parseMatchRequest(r *http.Request) (remoteQueryMatchRequest, error) {
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		return remoteQueryMatchRequest{}, invalidRequestError("content-type must be application/json")
	}

	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()

	var root map[string]json.RawMessage
	if err := decoder.Decode(&root); err != nil {
		return remoteQueryMatchRequest{}, invalidRequestError("malformed JSON request")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return remoteQueryMatchRequest{}, invalidRequestError("malformed JSON request")
	}

	for key := range root {
		switch key {
		case "integration", "target":
			continue
		default:
			return remoteQueryMatchRequest{}, invalidRequestError("request contains unknown field")
		}
	}

	integration, err := parseIntegrationFromRoot(root)
	if err != nil {
		return remoteQueryMatchRequest{}, err
	}
	target, err := parseTargetFromRoot(root)
	if err != nil {
		return remoteQueryMatchRequest{}, err
	}
	return remoteQueryMatchRequest{Integration: integration, Target: target}, nil
}

func parseIntegrationFromRoot(root map[string]json.RawMessage) (string, error) {
	integration, err := parseRequiredString(root, "integration")
	if err != nil {
		return "", err
	}
	switch strings.ToLower(strings.TrimSpace(integration)) {
	case integrationPostgres, "postgresql":
		return integrationPostgres, nil
	default:
		return "", unsupportedIntegrationError()
	}
}

func isJSONContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mediaType == "application/json"
}

func parseTargetString(fields map[string]json.RawMessage, field string) (string, error) {
	raw, ok := fields[field]
	if !ok {
		return "", fmt.Errorf("target.%s is required", field)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("target.%s must be a string", field)
	}
	return value, nil
}

func parseTargetPort(fields map[string]json.RawMessage) (int, error) {
	raw, ok := fields["port"]
	if !ok {
		return 0, fmt.Errorf("target.port is required")
	}

	var port int
	if err := json.Unmarshal(raw, &port); err != nil {
		return 0, fmt.Errorf("target.port must be an integer")
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("target.port is out of range")
	}
	return port, nil
}

func normalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	return strings.TrimSuffix(host, ".")
}

type postgresCheckMatch struct {
	check     check.Check
	sanitized sanitizedMatch
}

func (h *remoteQueryMatchHandler) findMatches(integration string, target remoteQueryTarget) []postgresCheckMatch {
	switch integration {
	case integrationPostgres:
		return findPostgresMatches(h.collector, target)
	default:
		return nil
	}
}

func findPostgresMatches(collector collector.Component, target remoteQueryTarget) []postgresCheckMatch {
	checks := collector.GetChecks()
	matches := make([]postgresCheckMatch, 0, 1)
	for _, chk := range checks {
		if !isPostgresCheck(chk) {
			continue
		}

		instanceTarget, ok := parsePostgresInstanceTarget(chk.InstanceConfig())
		if !ok {
			continue
		}

		if instanceTarget.host == target.Host && instanceTarget.port == target.Port && instanceTarget.dbname == target.DBName {
			matches = append(matches, postgresCheckMatch{
				check: chk,
				sanitized: sanitizedMatch{
					Integration:    "postgres",
					Loader:         chk.Loader(),
					ConfigProvider: chk.ConfigProvider(),
				},
			})
		}
	}
	return matches
}

func isPostgresCheck(chk check.Check) bool {
	name := strings.ToLower(strings.TrimSpace(chk.String()))
	return name == "postgres" || name == "postgresql"
}

func parsePostgresInstanceTarget(instanceConfig string) (postgresInstanceTarget, bool) {
	var fields map[string]any
	if err := yaml.Unmarshal([]byte(instanceConfig), &fields); err != nil || fields == nil {
		return postgresInstanceTarget{}, false
	}

	host, ok := fields["host"].(string)
	if !ok {
		return postgresInstanceTarget{}, false
	}
	host = normalizeHost(host)
	if host == "" {
		return postgresInstanceTarget{}, false
	}

	port, ok := yamlInt(fields["port"])
	if !ok || port < 1 || port > 65535 {
		return postgresInstanceTarget{}, false
	}

	dbname, ok := fields["dbname"].(string)
	if !ok || dbname == "" {
		return postgresInstanceTarget{}, false
	}

	return postgresInstanceTarget{host: host, port: port, dbname: dbname}, true
}

func yamlInt(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case uint64:
		if v > uint64(^uint(0)>>1) {
			return 0, false
		}
		return int(v), true
	default:
		return 0, false
	}
}

func writeMatchParseError(w http.ResponseWriter, err error) {
	parseErr, ok := err.(requestParseError)
	if !ok {
		writeMatchResponse(w, http.StatusBadRequest, statusInvalidRequest, 0, nil, err.Error())
		return
	}

	httpStatus := http.StatusBadRequest
	if parseErr.status == statusUnsupportedIntegration {
		httpStatus = http.StatusUnprocessableEntity
	}
	writeMatchResponse(w, httpStatus, parseErr.status, 0, nil, parseErr.message)
}

func writeMatchResponse(w http.ResponseWriter, httpStatus int, status string, matchedCount int, match *sanitizedMatch, message string) {
	w.WriteHeader(httpStatus)
	resp := matchResponse{
		Status:       status,
		MatchedCount: matchedCount,
		Match:        match,
	}
	if status != statusOK {
		resp.Error = &responseError{Code: status, Message: message}
	}
	_ = json.NewEncoder(w).Encode(resp)
}
