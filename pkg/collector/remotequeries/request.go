// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package remotequeries handles outcome-only remote queries on loaded checks.
package remotequeries

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var (
	integrationPattern = regexp.MustCompile(`^[a-z0-9_]+$`)
	uuidPattern        = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	uploadPattern      = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
	positiveInteger    = regexp.MustCompile(`^[1-9][0-9]*$`)
)

// Target is one of the two database identities understood by integrations.
type Target struct {
	Host             string `json:"host,omitempty"`
	Port             int    `json:"port,omitempty"`
	DBName           string `json:"dbname,omitempty"`
	DatabaseInstance string `json:"database_instance,omitempty"`
}

// Limits belong to the worker and intake. Numbers are kept verbatim, without an Agent bound.
type Limits struct {
	MaxFileBytes   json.Number `json:"maxFileBytes"`
	MaxResultBytes json.Number `json:"maxResultBytes"`
	MaxRowBytes    json.Number `json:"maxRowBytes"`
	MaxColumns     json.Number `json:"maxColumns"`
	MaxSchemaBytes json.Number `json:"maxSchemaBytes"`
	MaxPages       json.Number `json:"maxPages"`
	TimeoutMs      json.Number `json:"timeoutMs"`
}

// Delivery identifies the upload session; it never contains a credential.
type Delivery struct {
	RunID    string `json:"runId"`
	TaskID   string `json:"taskId"`
	UploadID string `json:"uploadId"`
	BaseURL  string `json:"baseUrl"`
	Limits   Limits `json:"limits"`
}

// TraceContext carries only a valid kept trace from the active runner action span.
type TraceContext struct {
	TraceID          string `json:"traceId"`
	ParentID         string `json:"parentId"`
	SamplingPriority int    `json:"samplingPriority"`
}

// Valid reports whether the integration may receive this context.
func (t *TraceContext) Valid() bool {
	if t == nil || (t.SamplingPriority != 1 && t.SamplingPriority != 2) {
		return false
	}
	trace, e1 := strconv.ParseUint(t.TraceID, 10, 64)
	parent, e2 := strconv.ParseUint(t.ParentID, 10, 64)
	return e1 == nil && e2 == nil && trace != 0 && parent != 0
}

// Request is the validated action input, without the runner-only timeout.
type Request struct {
	Integration    string
	ResolveOnly    bool
	Target         Target
	Query          string
	IncludeSchema  bool
	ResultDelivery *Delivery
	TraceContext   *TraceContext
}

// WireRequest is the local IPC envelope. Trace context is separate from action inputs.
type WireRequest struct {
	Input        json.RawMessage `json:"input"`
	TraceContext *TraceContext   `json:"traceContext,omitempty"`
}

func object(data []byte, field string, allowed ...string) (map[string]json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	token, err := dec.Token()
	if err != nil || token != json.Delim('{') {
		return nil, fmt.Errorf("%s must be an object", field)
	}
	result := make(map[string]json.RawMessage)
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("%s is malformed", field)
		}
		name, ok := key.(string)
		if !ok {
			return nil, fmt.Errorf("%s is malformed", field)
		}
		known := false
		for _, a := range allowed {
			if name == a {
				known = true
				break
			}
		}
		if !known {
			return nil, fmt.Errorf("%s contains an unknown field", field)
		}
		if _, exists := result[name]; exists {
			return nil, fmt.Errorf("%s contains a duplicate field", field)
		}
		var value json.RawMessage
		if dec.Decode(&value) != nil {
			return nil, fmt.Errorf("%s is malformed", field)
		}
		result[name] = value
	}
	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("%s is malformed", field)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("%s must contain one object", field)
	}
	return result, nil
}

func stringField(fields map[string]json.RawMessage, key, path string) (string, error) {
	var value string
	raw := fields[key]
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) || json.Unmarshal(raw, &value) != nil || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s must be a non-empty string", path)
	}
	return value, nil
}

// Parse validates the action input before any check is called.
func Parse(data []byte, site string) (Request, error) {
	var req Request
	// Recover the kind even when another input field is invalid.
	var kind struct {
		ResolveOnly bool `json:"resolveOnly"`
	}
	_ = json.Unmarshal(data, &kind)
	req.ResolveOnly = kind.ResolveOnly
	fields, err := object(data, "input", "integration", "target", "resolveOnly", "query", "includeSchema", "resultDelivery", "timeout")
	if err != nil {
		return req, err
	}
	if raw := fields["resolveOnly"]; len(raw) == 0 || bytes.Equal(raw, []byte("null")) || json.Unmarshal(raw, &req.ResolveOnly) != nil {
		return req, errors.New("resolveOnly must be a boolean")
	}
	integration, err := stringField(fields, "integration", "integration")
	if err != nil {
		return req, err
	}
	integration = strings.ToLower(strings.TrimSpace(integration))
	if !integrationPattern.MatchString(integration) {
		return req, errors.New("integration must contain only letters, digits, and underscores")
	}
	req.Integration = integration
	if req.Target, err = parseTarget(fields["target"]); err != nil {
		return req, err
	}
	if req.ResolveOnly {
		for _, key := range []string{"query", "includeSchema", "resultDelivery", "timeout"} {
			if _, exists := fields[key]; exists {
				return req, fmt.Errorf("%s is not allowed with resolveOnly", key)
			}
		}
		return req, nil
	}
	if req.Query, err = stringField(fields, "query", "query"); err != nil {
		return req, err
	}
	raw := fields["includeSchema"]
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) || json.Unmarshal(raw, &req.IncludeSchema) != nil {
		return req, errors.New("includeSchema must be a boolean")
	}
	if raw, exists := fields["timeout"]; exists && !positiveInteger.Match(raw) {
		return req, errors.New("timeout must be a positive integer in seconds")
	}
	req.ResultDelivery, err = parseDelivery(fields["resultDelivery"], site)
	return req, err
}

func parseTarget(raw []byte) (Target, error) {
	var target Target
	fields, err := object(raw, "target", "host", "port", "dbname", "database_instance")
	if err != nil {
		return target, err
	}
	if _, exists := fields["database_instance"]; exists {
		if len(fields) != 1 {
			return target, errors.New("target.database_instance cannot be combined with host, port, or dbname")
		}
		target.DatabaseInstance, err = stringField(fields, "database_instance", "target.database_instance")
		if err == nil && strings.TrimSpace(target.DatabaseInstance) != target.DatabaseInstance {
			err = errors.New("target.database_instance must not have surrounding spaces")
		}
		return target, err
	}
	if target.Host, err = stringField(fields, "host", "target.host"); err != nil {
		return target, err
	}
	// Checks own matching (CHECK-02). Preserve the original operand so the
	// integration removes one trailing dot, not a second dot after the Agent.
	if strings.TrimSuffix(strings.ToLower(strings.TrimSpace(target.Host)), ".") == "" {
		return target, errors.New("target.host must not be empty")
	}
	if target.DBName, err = stringField(fields, "dbname", "target.dbname"); err != nil {
		return target, err
	}
	if json.Unmarshal(fields["port"], &target.Port) != nil || target.Port < 1 || target.Port > 65535 {
		return target, errors.New("target.port must be an integer from 1 to 65535")
	}
	return target, nil
}

func parseDelivery(raw []byte, site string) (*Delivery, error) {
	fields, err := object(raw, "resultDelivery", "runId", "taskId", "uploadId", "baseUrl", "limits")
	if err != nil {
		return nil, err
	}
	d := &Delivery{}
	for _, pair := range []struct {
		key  string
		dest *string
	}{{"runId", &d.RunID}, {"taskId", &d.TaskID}, {"uploadId", &d.UploadID}, {"baseUrl", &d.BaseURL}} {
		*pair.dest, err = stringField(fields, pair.key, "resultDelivery."+pair.key)
		if err != nil {
			return nil, err
		}
	}
	if !uuidPattern.MatchString(d.RunID) {
		return nil, errors.New("resultDelivery.runId must be a UUID")
	}
	if !uuidPattern.MatchString(d.TaskID) {
		return nil, errors.New("resultDelivery.taskId must be a UUID")
	}
	if !uploadPattern.MatchString(d.UploadID) {
		return nil, errors.New("resultDelivery.uploadId contains an invalid character")
	}
	if !validBaseURL(d.BaseURL, site) {
		return nil, errors.New("resultDelivery.baseUrl must be an HTTPS address on the Agent site")
	}
	limits, err := object(fields["limits"], "resultDelivery.limits", "maxFileBytes", "maxResultBytes", "maxRowBytes", "maxColumns", "maxSchemaBytes", "maxPages", "timeoutMs")
	if err != nil {
		return nil, err
	}
	for _, key := range []string{"maxFileBytes", "maxResultBytes", "maxRowBytes", "maxColumns", "maxSchemaBytes", "maxPages", "timeoutMs"} {
		if !positiveInteger.Match(limits[key]) {
			return nil, fmt.Errorf("resultDelivery.limits.%s must be a positive integer", key)
		}
	}
	if json.Unmarshal(fields["limits"], &d.Limits) != nil {
		return nil, errors.New("resultDelivery.limits is malformed")
	}
	return d, nil
}

func validBaseURL(address, site string) bool {
	if site == "" || strings.ContainsAny(address, `\`) || strings.IndexFunc(address, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return false
	}
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(address, "#") || u.Opaque != "" {
		return false
	}
	if strings.HasSuffix(u.Host, ":") {
		return false
	}
	if port := u.Port(); port != "" {
		number, err := strconv.ParseUint(port, 10, 16)
		if err != nil || number != 443 {
			return false
		}
	}
	host := strings.ToLower(u.Hostname())
	site = strings.ToLower(site)
	return host == site || strings.HasSuffix(host, "."+site)
}
