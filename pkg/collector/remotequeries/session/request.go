// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package session is the integration side of a Remote Queries run for checks written
// in Go. It parses the core Agent's call into the check, uploads results to
// its-agent-intake in the JSON-cell format (descriptor, pages, finalize or abort), and
// encodes the check's answer. The database work stays in the check.
package session

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Operations the core Agent calls a check with.
const (
	OperationResolve = "resolve"
	OperationExecute = "execute"
)

var (
	uuidPattern     = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	uploadPattern   = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,128}$`)
	traceIDPattern  = regexp.MustCompile(`^[0-9]{1,20}$`)
	controlOrSpaces = regexp.MustCompile(`[\s\\\x00-\x1f\x7f]`)
)

// Target names the database a request is for: either a host, port, and database, or a
// database instance name.
type Target struct {
	Host             string
	Port             int
	DBName           string
	DatabaseInstance string
}

// Limits are the delivery instructions' limits, which intake returned when it created
// the session.
type Limits struct {
	MaxFileBytes   int64
	MaxResultBytes int64
	MaxRowBytes    int64
	MaxColumns     int64
	MaxSchemaBytes int64
	MaxPages       int64
	TimeoutMs      int64
}

// Delivery is where and within which limits a run uploads its result.
type Delivery struct {
	CrawlID  string
	TaskID   string
	UploadID string
	BaseURL  string
	Limits   Limits
}

// Trace is the runner's trace context, forwarded to intake.
type Trace struct {
	TraceID          string
	ParentID         string
	SamplingPriority int
}

// Request is one call from the core Agent.
type Request struct {
	Operation     string
	Target        Target
	Query         string
	IncludeSchema bool
	Delivery      *Delivery
	Trace         *Trace
}

// ParseRequest strictly decodes the core Agent's call. Any unknown field, or missing or
// invalid value, is an error; the caller answers it as an invalid request.
func ParseRequest(request string) (Request, error) {
	var req Request
	fields, err := rawFields([]byte(request), "request", "operation", "target", "query", "includeSchema", "resultDelivery", "traceContext")
	if err != nil {
		return req, err
	}
	if req.Operation, err = requiredString(fields, "operation", "operation"); err != nil {
		return req, err
	}
	if req.Target, err = parseTarget(fields["target"]); err != nil {
		return req, err
	}
	switch req.Operation {
	case OperationResolve:
		if len(fields) != 2 {
			return req, errors.New("resolve carries only the operation and the target")
		}
		return req, nil
	case OperationExecute:
	default:
		return req, errors.New("unknown operation")
	}
	for _, key := range []string{"query", "includeSchema", "resultDelivery"} {
		if _, ok := fields[key]; !ok {
			return req, errors.New(key + " is required")
		}
	}
	if req.Query, err = requiredString(fields, "query", "query"); err != nil {
		return req, err
	}
	if json.Unmarshal(fields["includeSchema"], &req.IncludeSchema) != nil || string(bytes.TrimSpace(fields["includeSchema"])) == "null" {
		return req, errors.New("includeSchema must be a boolean")
	}
	if req.Delivery, err = parseDelivery(fields["resultDelivery"]); err != nil {
		return req, err
	}
	if raw, ok := fields["traceContext"]; ok {
		if req.Trace, err = parseTrace(raw); err != nil {
			return req, err
		}
	}
	return req, nil
}

func parseTarget(raw json.RawMessage) (Target, error) {
	var t Target
	fields, err := rawFields(raw, "target", "host", "port", "dbname", "database_instance")
	if err != nil {
		return t, err
	}
	if _, instance := fields["database_instance"]; instance {
		if len(fields) != 1 {
			return t, errors.New("target.database_instance cannot be combined with host, port, or dbname")
		}
		if t.DatabaseInstance, err = requiredString(fields, "database_instance", "target.database_instance"); err != nil {
			return t, err
		}
		if t.DatabaseInstance != strings.TrimSpace(t.DatabaseInstance) {
			return t, errors.New("target.database_instance must not have surrounding spaces")
		}
		return t, nil
	}
	if len(fields) != 3 {
		return t, errors.New("target needs host, port, and dbname")
	}
	if t.Host, err = requiredString(fields, "host", "target.host"); err != nil {
		return t, err
	}
	if t.DBName, err = requiredString(fields, "dbname", "target.dbname"); err != nil {
		return t, err
	}
	if json.Unmarshal(fields["port"], &t.Port) != nil || t.Port < 1 || t.Port > 65535 {
		return t, errors.New("target.port must be an integer from 1 to 65535")
	}
	return t, nil
}

func parseDelivery(raw json.RawMessage) (*Delivery, error) {
	fields, err := rawFields(raw, "resultDelivery", "crawlId", "taskId", "uploadId", "baseUrl", "limits")
	if err != nil {
		return nil, err
	}
	if len(fields) != 5 {
		return nil, errors.New("resultDelivery needs crawlId, taskId, uploadId, baseUrl, and limits")
	}
	d := &Delivery{}
	for _, p := range []struct {
		key  string
		dest *string
	}{{"crawlId", &d.CrawlID}, {"taskId", &d.TaskID}, {"uploadId", &d.UploadID}, {"baseUrl", &d.BaseURL}} {
		if *p.dest, err = requiredString(fields, p.key, "resultDelivery."+p.key); err != nil {
			return nil, err
		}
	}
	if !uuidPattern.MatchString(d.CrawlID) || !uuidPattern.MatchString(d.TaskID) {
		return nil, errors.New("resultDelivery crawlId and taskId must be UUIDs")
	}
	if !uploadPattern.MatchString(d.UploadID) || d.UploadID == "." || d.UploadID == ".." {
		return nil, errors.New("resultDelivery.uploadId is invalid")
	}
	if !validBaseURL(d.BaseURL) {
		return nil, errors.New("resultDelivery.baseUrl is invalid")
	}
	limits, err := rawFields(fields["limits"], "resultDelivery.limits", "maxFileBytes", "maxResultBytes", "maxRowBytes", "maxColumns", "maxSchemaBytes", "maxPages", "timeoutMs")
	if err != nil {
		return nil, err
	}
	for _, p := range []struct {
		key  string
		dest *int64
	}{
		{"maxFileBytes", &d.Limits.MaxFileBytes}, {"maxResultBytes", &d.Limits.MaxResultBytes},
		{"maxRowBytes", &d.Limits.MaxRowBytes}, {"maxColumns", &d.Limits.MaxColumns},
		{"maxSchemaBytes", &d.Limits.MaxSchemaBytes}, {"maxPages", &d.Limits.MaxPages},
		{"timeoutMs", &d.Limits.TimeoutMs},
	} {
		if *p.dest, err = positiveLimit(limits, p.key); err != nil {
			return nil, err
		}
	}
	return d, nil
}

// validBaseURL checks only the syntax of the upload address. Which site the address
// names is the core Agent's decision (Agent INPUT-05); this rejects what could change
// the appended path or the request's headers.
func validBaseURL(address string) bool {
	u, err := url.Parse(address)
	if err != nil || controlOrSpaces.MatchString(address) {
		return false
	}
	return (u.Scheme == "https" || u.Scheme == "http") && u.Hostname() != "" && u.User == nil &&
		u.RawQuery == "" && u.Fragment == "" && !u.ForceQuery && u.Opaque == ""
}

func parseTrace(raw json.RawMessage) (*Trace, error) {
	fields, err := rawFields(raw, "traceContext", "traceId", "parentId", "samplingPriority")
	if err != nil || len(fields) != 3 {
		return nil, errors.New("traceContext needs traceId, parentId, and samplingPriority")
	}
	t := &Trace{}
	if json.Unmarshal(fields["samplingPriority"], &t.SamplingPriority) != nil || (t.SamplingPriority != 1 && t.SamplingPriority != 2) {
		return nil, errors.New("traceContext.samplingPriority must be 1 or 2")
	}
	for _, p := range []struct {
		key  string
		dest *string
	}{{"traceId", &t.TraceID}, {"parentId", &t.ParentID}} {
		if *p.dest, err = requiredString(fields, p.key, "traceContext."+p.key); err != nil {
			return nil, err
		}
		n, err := strconv.ParseUint(*p.dest, 10, 64)
		if !traceIDPattern.MatchString(*p.dest) || err != nil || n == 0 {
			return nil, errors.New("traceContext." + p.key + " must be a non-zero unsigned 64-bit integer")
		}
	}
	return t, nil
}

// rawFields decodes exactly one JSON object whose keys are all allowed and unique.
func rawFields(data []byte, what string, allowed ...string) (map[string]json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, errors.New(what + " must be an object")
	}
	fields := map[string]json.RawMessage{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, errors.New(what + " must be an object")
		}
		key, _ := tok.(string)
		known := false
		for _, a := range allowed {
			known = known || a == key
		}
		if !known {
			return nil, errors.New(what + " contains an unknown field")
		}
		if _, dup := fields[key]; dup {
			return nil, errors.New(what + " contains a repeated field")
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, errors.New(what + " must be an object")
		}
		fields[key] = value
	}
	if tok, err := dec.Token(); err != nil || tok != json.Delim('}') {
		return nil, errors.New(what + " must be an object")
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New(what + " must contain one object")
	}
	return fields, nil
}

func positiveLimit(fields map[string]json.RawMessage, key string) (int64, error) {
	raw, ok := fields[key]
	if !ok {
		return 0, errors.New("resultDelivery.limits." + key + " is required")
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil || n < 1 {
		return 0, errors.New("resultDelivery.limits." + key + " must be a positive integer")
	}
	return n, nil
}

func requiredString(fields map[string]json.RawMessage, key, path string) (string, error) {
	var value string
	raw := fields[key]
	if len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &value) != nil || strings.TrimSpace(value) == "" {
		return "", errors.New(path + " must be a non-empty string")
	}
	return value, nil
}
