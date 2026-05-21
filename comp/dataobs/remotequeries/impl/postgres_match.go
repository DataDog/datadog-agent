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
	// PostgresMatchEndpointPath is mounted under /agent by the Agent command API.
	PostgresMatchEndpointPath = "/remote-queries/postgres/match-check"
	// PostgresMatchEnabledConfig is disabled by default when the key is absent.
	PostgresMatchEnabledConfig = "remote_queries.postgres_match_check.enabled"

	statusOK             = "ok"
	statusTargetNotFound = "target_not_found"
	statusAmbiguous      = "ambiguous_target"
	statusInvalidRequest = "invalid_request"
	statusBridgeDisabled = "bridge_disabled"
)

var credentialShapedFields = map[string]struct{}{
	"app_key":           {},
	"apikey":            {},
	"api_key":           {},
	"conn_string":       {},
	"connection_string": {},
	"credential":        {},
	"credentials":       {},
	"dsn":               {},
	"pass":              {},
	"password":          {},
	"pwd":               {},
	"secret":            {},
	"sslcert":           {},
	"sslkey":            {},
	"token":             {},
	"user":              {},
	"username":          {},
}

// Requires defines dependencies for the Remote Queries POC endpoint provider.
type Requires struct {
	fx.In

	Cfg       config.Component
	Collector collector.Component
}

// NewPostgresMatchEndpointProvider registers the Postgres match endpoint on the internal Agent API.
func NewPostgresMatchEndpointProvider(reqs Requires) api.AgentEndpointProvider {
	h := &postgresMatchHandler{
		collector: reqs.Collector,
		enabled:   reqs.Cfg.GetBool(PostgresMatchEnabledConfig),
	}
	return api.NewAgentEndpointProvider(h.handle, PostgresMatchEndpointPath, http.MethodPost)
}

type postgresMatchHandler struct {
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

type postgresTarget struct {
	Host   string
	Port   int
	DBName string
}

type postgresInstanceTarget struct {
	host   string
	port   int
	dbname string
}

func (h *postgresMatchHandler) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if !h.enabled {
		writeMatchResponse(w, http.StatusServiceUnavailable, statusBridgeDisabled, 0, nil, "remote queries bridge is disabled")
		return
	}

	target, err := parseMatchRequest(r)
	if err != nil {
		writeMatchResponse(w, http.StatusBadRequest, statusInvalidRequest, 0, nil, err.Error())
		return
	}

	matches := h.findMatches(target)
	switch len(matches) {
	case 0:
		writeMatchResponse(w, http.StatusNotFound, statusTargetNotFound, 0, nil, "no matching Postgres check found")
	case 1:
		writeMatchResponse(w, http.StatusOK, statusOK, 1, &matches[0].sanitized, "")
	default:
		writeMatchResponse(w, http.StatusConflict, statusAmbiguous, len(matches), nil, "multiple matching Postgres checks found")
	}
}

func parseMatchRequest(r *http.Request) (postgresTarget, error) {
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		return postgresTarget{}, fmt.Errorf("content-type must be application/json")
	}

	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()

	var root map[string]json.RawMessage
	if err := decoder.Decode(&root); err != nil {
		return postgresTarget{}, fmt.Errorf("malformed JSON request")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return postgresTarget{}, fmt.Errorf("malformed JSON request")
	}

	for key := range root {
		if key != "target" {
			if isCredentialShapedField(key) {
				return postgresTarget{}, fmt.Errorf("request contains disallowed credential-shaped field")
			}
			return postgresTarget{}, fmt.Errorf("request contains unknown field")
		}
	}

	return parseTargetFromRoot(root)
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

func isCredentialShapedField(field string) bool {
	_, found := credentialShapedFields[strings.ToLower(field)]
	return found
}

type postgresCheckMatch struct {
	check     check.Check
	sanitized sanitizedMatch
}

func (h *postgresMatchHandler) findMatches(target postgresTarget) []postgresCheckMatch {
	return findPostgresMatches(h.collector, target)
}

func findPostgresMatches(collector collector.Component, target postgresTarget) []postgresCheckMatch {
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
