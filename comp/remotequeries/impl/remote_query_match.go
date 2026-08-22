// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package remotequeriesimpl implements Remote Queries POC endpoints.
package remotequeriesimpl

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"go.uber.org/fx"
	"gopkg.in/yaml.v3"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

const (
	// RemoteQueryMatchEndpointPath is mounted under /agent by the Agent command API.
	RemoteQueryMatchEndpointPath = "/remote-queries/match-check"
	// RemoteQueriesMatchEnabledConfig is disabled by default when the key is absent.
	RemoteQueriesMatchEnabledConfig = "remote_queries.match_check.enabled"

	statusOK             = "ok"
	statusTargetNotFound = "target_not_found"
	statusAmbiguous      = "ambiguous_target"
	statusInvalidRequest = "invalid_request"
	statusBridgeDisabled = "bridge_disabled"
)

// Requires defines dependencies for the Remote Queries POC endpoint provider.
type Requires struct {
	fx.In

	Cfg       config.Component
	Collector RemoteQueryCollector
}

// NewRemoteQueryMatchEndpointProvider registers the remote query match endpoint on the internal Agent API.
func NewRemoteQueryMatchEndpointProvider(reqs Requires) api.AgentEndpointProvider {
	h := &remoteQueryMatchHandler{
		collector: reqs.Collector,
		enabled:   reqs.Cfg.GetBool(RemoteQueriesMatchEnabledConfig),
	}
	return api.NewAgentEndpointProvider(h.handle, RemoteQueryMatchEndpointPath, http.MethodPost)
}

type remoteQueryMatchHandler struct {
	collector RemoteQueryCollector
	enabled   bool
}

// RemoteQueryCollector is the narrow collector surface Remote Queries needs.
// The Agent command provides its collector.Component as this interface at the application boundary
// so this package does not force Bazel onboarding for the full collector component package.
type RemoteQueryCollector interface {
	GetChecks() []check.Check
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

type remoteQueryMatchRequestJSON struct {
	Integration string                        `json:"integration"`
	Target      *remoteQueryTargetRequestJSON `json:"target"`
}

type remoteQueryTargetRequestJSON struct {
	Host                string  `json:"host"`
	Port                *int    `json:"port"`
	DBName              string  `json:"dbname"`
	DatabaseInstance    *string `json:"database_instance"`
	hostSet             bool
	portSet             bool
	dbnameSet           bool
	databaseInstanceSet bool
}

type remoteQueryTarget struct {
	Host             string
	Port             int
	DBName           string
	DatabaseInstance string
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

var integrationNamePattern = regexp.MustCompile(`^[a-z0-9_]+$`)

type integrationInstanceTarget struct {
	host             string
	port             int
	dbname           string
	databaseInstance string
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
		writeMatchResponse(w, http.StatusNotFound, statusTargetNotFound, 0, nil, "no matching integration check found")
	case 1:
		writeMatchResponse(w, http.StatusOK, statusOK, 1, &matches[0].sanitized, "")
	default:
		writeMatchResponse(w, http.StatusConflict, statusAmbiguous, len(matches), nil, "multiple matching integration checks found")
	}
}

func parseMatchRequest(r *http.Request) (remoteQueryMatchRequest, error) {
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		return remoteQueryMatchRequest{}, invalidRequestError("content-type must be application/json")
	}

	defer r.Body.Close()
	var wireReq remoteQueryMatchRequestJSON
	if err := decodeStrictJSON(r.Body, &wireReq); err != nil {
		return remoteQueryMatchRequest{}, parseJSONRequestError(err)
	}

	integration, err := parseIntegration(wireReq.Integration)
	if err != nil {
		return remoteQueryMatchRequest{}, err
	}
	target, err := parseTarget(wireReq.Target)
	if err != nil {
		return remoteQueryMatchRequest{}, err
	}
	return remoteQueryMatchRequest{Integration: integration, Target: target}, nil
}

func parseIntegration(integration string) (string, error) {
	integration = strings.ToLower(strings.TrimSpace(integration))
	if integration == "" {
		return "", errors.New("integration is required")
	}
	if !integrationNamePattern.MatchString(integration) {
		return "", invalidRequestError("integration contains invalid characters")
	}
	return integration, nil
}

func isJSONContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mediaType == "application/json"
}

var (
	errMultipleJSONValues = errors.New("multiple JSON values")
	errTargetUnknownField = errors.New("target contains unknown field")
	errTargetMustBeObject = errors.New("target must be an object")
)

func (t *remoteQueryTargetRequestJSON) UnmarshalJSON(data []byte) error {
	if !isJSONObject(data) {
		return errTargetMustBeObject
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	type targetAlias remoteQueryTargetRequestJSON
	var target targetAlias
	if err := decodeStrictJSON(bytes.NewReader(data), &target); err != nil {
		if isUnknownJSONFieldError(err) {
			return errTargetUnknownField
		}
		return err
	}
	_, target.hostSet = raw["host"]
	_, target.portSet = raw["port"]
	_, target.dbnameSet = raw["dbname"]
	_, target.databaseInstanceSet = raw["database_instance"]
	*t = remoteQueryTargetRequestJSON(target)
	return nil
}

func parseTarget(target *remoteQueryTargetRequestJSON) (remoteQueryTarget, error) {
	if target == nil {
		return remoteQueryTarget{}, errors.New("target is required")
	}

	host := normalizeHost(target.Host)
	hasTupleSelectorField := target.hostSet || target.portSet || target.dbnameSet

	if target.databaseInstanceSet {
		if target.DatabaseInstance == nil {
			return remoteQueryTarget{}, errors.New("target.database_instance is required")
		}
		databaseInstance := *target.DatabaseInstance
		if databaseInstance == "" {
			return remoteQueryTarget{}, errors.New("target.database_instance is required")
		}
		if strings.TrimSpace(databaseInstance) != databaseInstance {
			return remoteQueryTarget{}, errors.New("target.database_instance must not contain surrounding whitespace")
		}
		if hasTupleSelectorField {
			return remoteQueryTarget{}, errors.New("target must specify exactly one selector mode")
		}
		return remoteQueryTarget{DatabaseInstance: databaseInstance}, nil
	}

	if host == "" {
		return remoteQueryTarget{}, errors.New("target.host is required")
	}

	port, err := parseRequiredPort(target.Port)
	if err != nil {
		return remoteQueryTarget{}, err
	}

	if target.DBName == "" {
		return remoteQueryTarget{}, errors.New("target.dbname is required")
	}

	return remoteQueryTarget{Host: host, Port: port, DBName: target.DBName}, nil
}

func parseRequiredPort(port *int) (int, error) {
	if port == nil {
		return 0, errors.New("target.port is required")
	}
	if *port < 1 || *port > 65535 {
		return 0, errors.New("target.port is out of range")
	}
	return *port, nil
}

func decodeStrictJSON(r io.Reader, value any) error {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errMultipleJSONValues
	}
	return nil
}

func parseJSONRequestError(err error) error {
	switch {
	case errors.Is(err, errMultipleJSONValues):
		return invalidRequestError("malformed JSON request")
	case errors.Is(err, errTargetUnknownField):
		return errTargetUnknownField
	case errors.Is(err, errTargetMustBeObject):
		return errTargetMustBeObject
	case errors.Is(err, errLimitsUnknownField):
		return errLimitsUnknownField
	case errors.Is(err, errLimitsMustBeObject):
		return errLimitsMustBeObject
	case isUnknownJSONFieldError(err):
		return invalidRequestError("request contains unknown field")
	}

	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		switch typeErr.Field {
		case "port", "target.port":
			return errors.New("target.port must be an integer")
		case "database_instance", "target.database_instance":
			return errors.New("target.database_instance must be a string")
		case "target":
			return errTargetMustBeObject
		case "maxRows", "limits.maxRows":
			return errors.New("limits.maxRows must be an integer")
		case "maxBytes", "limits.maxBytes":
			return errors.New("limits.maxBytes must be an integer")
		case "timeoutMs", "limits.timeoutMs":
			return errors.New("limits.timeoutMs must be an integer")
		case "limits":
			return errLimitsMustBeObject
		}
	}

	return invalidRequestError("malformed JSON request")
}

func isUnknownJSONFieldError(err error) bool {
	return strings.HasPrefix(err.Error(), "json: unknown field ")
}

func isJSONObject(data []byte) bool {
	return bytes.HasPrefix(bytes.TrimSpace(data), []byte("{"))
}

func normalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	return strings.TrimSuffix(host, ".")
}

type integrationCheckMatch struct {
	check     check.Check
	sanitized sanitizedMatch
}

func (h *remoteQueryMatchHandler) findMatches(integration string, target remoteQueryTarget) []integrationCheckMatch {
	return findIntegrationMatches(h.collector, integration, target)
}

func findIntegrationMatches(collector RemoteQueryCollector, integration string, target remoteQueryTarget) []integrationCheckMatch {
	checks := collector.GetChecks()
	matches := make([]integrationCheckMatch, 0, 1)
	for _, chk := range checks {
		if normalizeIntegrationName(chk.String()) != integration {
			continue
		}

		instanceTarget, ok := parseIntegrationInstanceTarget(chk.InstanceConfig())
		if !ok {
			continue
		}

		if !instanceTarget.matches(target) {
			continue
		}

		matches = append(matches, integrationCheckMatch{
			check: chk,
			sanitized: sanitizedMatch{
				Integration:    integration,
				Loader:         chk.Loader(),
				ConfigProvider: chk.ConfigProvider(),
			},
		})
	}
	return matches
}

func normalizeIntegrationName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func (t integrationInstanceTarget) matches(target remoteQueryTarget) bool {
	if target.DatabaseInstance != "" {
		return t.databaseInstance != "" && t.databaseInstance == target.DatabaseInstance
	}
	return t.host == target.Host && t.port == target.Port && t.dbname == target.DBName
}

func parseIntegrationInstanceTarget(instanceConfig string) (integrationInstanceTarget, bool) {
	var fields map[string]any
	if err := yaml.Unmarshal([]byte(instanceConfig), &fields); err != nil || fields == nil {
		return integrationInstanceTarget{}, false
	}

	host, ok := fields["host"].(string)
	if !ok {
		return integrationInstanceTarget{}, false
	}
	host = normalizeHost(host)
	if host == "" {
		return integrationInstanceTarget{}, false
	}

	port, ok := yamlInt(fields["port"])
	if !ok || port < 1 || port > 65535 {
		return integrationInstanceTarget{}, false
	}

	dbname, ok := fields["dbname"].(string)
	if !ok || dbname == "" {
		return integrationInstanceTarget{}, false
	}

	databaseInstance, _ := renderPostgresDatabaseIdentifier(fields, host, port)

	return integrationInstanceTarget{host: host, port: port, dbname: dbname, databaseInstance: databaseInstance}, true
}

func renderPostgresDatabaseIdentifier(fields map[string]any, host string, port int) (string, bool) {
	template := "$resolved_hostname"
	if rawIdentifier, ok := fields["database_identifier"]; ok {
		identifier, ok := rawIdentifier.(map[string]any)
		if !ok {
			return "", false
		}
		rawTemplate, ok := identifier["template"]
		if !ok {
			return "", false
		}
		parsedTemplate, ok := rawTemplate.(string)
		if !ok || parsedTemplate == "" {
			return "", false
		}
		template = parsedTemplate
	}

	values := postgresDatabaseIdentifierTemplateValues(fields, host, port)
	rendered, ok := renderPythonTemplate(template, values)
	if !ok || rendered == "" {
		return "", false
	}
	return rendered, true
}

func postgresDatabaseIdentifierTemplateValues(fields map[string]any, host string, port int) map[string]string {
	values := make(map[string]string)
	if tags, ok := fields["tags"].([]any); ok {
		stringTags := make([]string, 0, len(tags))
		for _, rawTag := range tags {
			tag, ok := rawTag.(string)
			if !ok {
				continue
			}
			stringTags = append(stringTags, tag)
		}
		sort.Strings(stringTags)
		for _, tag := range stringTags {
			key, value, ok := strings.Cut(tag, ":")
			if !ok || key == "" {
				continue
			}
			if existing, found := values[key]; found {
				values[key] = existing + "," + value
			} else {
				values[key] = value
			}
		}
	}
	values["host"] = host
	values["port"] = strconv.Itoa(port)
	if reportedHostname, ok := fields["reported_hostname"].(string); ok && strings.TrimSpace(reportedHostname) != "" {
		values["resolved_hostname"] = strings.TrimSpace(reportedHostname)
	}
	return values
}

func renderPythonTemplate(template string, values map[string]string) (string, bool) {
	var out strings.Builder
	for i := 0; i < len(template); {
		if template[i] != '$' {
			out.WriteByte(template[i])
			i++
			continue
		}
		if i+1 < len(template) && template[i+1] == '$' {
			out.WriteByte('$')
			i += 2
			continue
		}
		name := ""
		next := i + 1
		if next < len(template) && template[next] == '{' {
			end := next + 1
			for end < len(template) && template[end] != '}' {
				end++
			}
			if end >= len(template) || end == next+1 {
				return "", false
			}
			name = template[next+1 : end]
			next = end + 1
		} else {
			end := next
			if end >= len(template) || !isTemplateIdentifierStart(template[end]) {
				return "", false
			}
			end++
			for end < len(template) && isTemplateIdentifierChar(template[end]) {
				end++
			}
			name = template[next:end]
			next = end
		}
		value, ok := values[name]
		if !ok {
			return "", false
		}
		out.WriteString(value)
		i = next
	}
	return out.String(), true
}

func isTemplateIdentifierStart(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || b == '_'
}

func isTemplateIdentifierChar(b byte) bool {
	return isTemplateIdentifierStart(b) || (b >= '0' && b <= '9')
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

	writeMatchResponse(w, http.StatusBadRequest, parseErr.status, 0, nil, parseErr.message)
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
