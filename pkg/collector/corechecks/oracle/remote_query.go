// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/oracle/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// This file is the Remote Queries boundary of the native Oracle check. It
// implements check.RemoteQuerier for one narrow, fail-closed proof of concept:
//
//   - the target is the check's own configured direct go-ora server (TCP, separate
//     port, explicit service_name), matched from configuration only;
//   - the query is one of a fixed set of fixture statements, never caller SQL;
//   - results stream into the existing JSON-cell intake upload session.
//
// Nothing here uses the monitoring connection or its state.

const (
	remoteOperationResolve = "resolve"
	remoteOperationExecute = "execute"

	// Stable failure codes carried in the integration answer. Messages are fixed text:
	// they never include SQL, credentials, DSNs, hosts, driver error text or row data.
	remoteCodeInvalidRequest     = "invalid_request"
	remoteCodeTargetNotFound     = "target_not_found"
	remoteCodeUnsupportedQuery   = "unsupported_query"
	remoteCodeCredentials        = "credentials_unavailable"
	remoteCodeConnectionFailed   = "connection_failed"
	remoteCodeQueryFailed        = "query_failed"
	remoteCodeUnsupportedColumn  = "unsupported_column"
	remoteCodeInvalidColumns     = "invalid_columns"
	remoteCodeUnsupportedValue   = "unsupported_value"
	remoteCodeRowTooLarge        = "row_too_large"
	remoteCodeResultTooLarge     = "result_too_large"
	remoteCodeUploadFailed       = "upload_failed"
	remoteCodeTimeout            = "timeout"
	remoteCodeCancelled          = "cancelled"
	remoteCodeInternalError      = "internal_error"
	remoteCodeDescriptorTooLarge = "descriptor_too_large"
)

// The only statements a remote query may run in this proof of concept. The caller's
// query text is normalised (whitespace runs collapsed) and must equal a key exactly;
// the executed statement is the value, not the caller's text. RQ_ORACLE_PROBE is the
// fixture table (ID NUMBER, LABEL VARCHAR2(100)) in the configured login's schema.
// ORDER BY both columns makes the output deterministic even when ID is not unique.
const remoteProbeSQL = "SELECT ID, LABEL FROM RQ_ORACLE_PROBE ORDER BY ID, LABEL"

var approvedRemoteQueries = map[string]string{
	remoteProbeSQL: remoteProbeSQL,
}

// approvedRemoteQuery returns the statement to execute for a caller query, or false.
func approvedRemoteQuery(query string) (string, bool) {
	sql, ok := approvedRemoteQueries[strings.Join(strings.Fields(query), " ")]
	return sql, ok
}

var (
	remoteUUIDPattern   = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	remoteUploadPattern = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,128}$`)
	remoteTraceIDPatten = regexp.MustCompile(`^[0-9]{1,20}$`)
	remoteOraCodeRegexp = regexp.MustCompile(`ORA-[0-9]{5}`)
)

// remoteTarget is the host/port/dbname identity. database_instance targets are not served.
type remoteTarget struct {
	Host             string `json:"host"`
	Port             int    `json:"port"`
	DBName           string `json:"dbname"`
	DatabaseInstance string `json:"database_instance"`
}

type remoteLimits struct {
	MaxFileBytes   int64
	MaxResultBytes int64
	MaxRowBytes    int64
	MaxColumns     int64
	MaxSchemaBytes int64
	MaxPages       int64
	TimeoutMs      int64
}

type remoteDelivery struct {
	CrawlID  string
	TaskID   string
	UploadID string
	BaseURL  string
	Limits   remoteLimits
}

type remoteTrace struct {
	TraceID          string
	ParentID         string
	SamplingPriority int
}

type remoteRequest struct {
	Operation     string
	Target        remoteTarget
	Query         string
	IncludeSchema bool
	Delivery      *remoteDelivery
	Trace         *remoteTrace
}

type remoteAnswer struct {
	Status string       `json:"status"`
	Error  *remoteError `json:"error,omitempty"`
}

type remoteError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// remoteFailure is the error type carried through a run; the answer is its only output.
type remoteFailure struct {
	code    string
	message string
}

func (f *remoteFailure) Error() string { return f.code }

func newRemoteFailure(code, message string) *remoteFailure {
	return &remoteFailure{code: code, message: message}
}

func encodeRemoteAnswer(a remoteAnswer) string {
	out, err := json.Marshal(a)
	if err != nil {
		return `{"status":"FAILED","error":{"code":"internal_error","message":"the answer could not be encoded"}}`
	}
	return string(out)
}

func remoteFailed(code, message string) string {
	return encodeRemoteAnswer(remoteAnswer{Status: "FAILED", Error: &remoteError{Code: code, Message: message}})
}

func rawFields(data []byte, what string, allowed ...string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields() // unknown keys are rejected explicitly below
	if err := dec.Decode(&fields); err != nil || fields == nil {
		return nil, errors.New(what + " must be an object")
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New(what + " must contain one object")
	}
	for key := range fields {
		known := false
		for _, a := range allowed {
			known = known || a == key
		}
		if !known {
			return nil, errors.New(what + " contains an unknown field")
		}
	}
	return fields, nil
}

func strictDecode(raw json.RawMessage, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("trailing data")
	}
	return nil
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
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil || strings.TrimSpace(value) == "" {
		return "", errors.New(path + " must be a non-empty string")
	}
	return value, nil
}

func parseRemoteDelivery(raw json.RawMessage) (*remoteDelivery, error) {
	fields, err := rawFields(raw, "resultDelivery", "crawlId", "taskId", "uploadId", "baseUrl", "limits")
	if err != nil {
		return nil, err
	}
	d := &remoteDelivery{}
	for _, p := range []struct {
		key  string
		dest *string
	}{{"crawlId", &d.CrawlID}, {"taskId", &d.TaskID}, {"uploadId", &d.UploadID}, {"baseUrl", &d.BaseURL}} {
		if *p.dest, err = requiredString(fields, p.key, "resultDelivery."+p.key); err != nil {
			return nil, err
		}
	}
	if !remoteUUIDPattern.MatchString(d.CrawlID) || !remoteUUIDPattern.MatchString(d.TaskID) {
		return nil, errors.New("resultDelivery crawlId and taskId must be UUIDs")
	}
	if !remoteUploadPattern.MatchString(d.UploadID) || d.UploadID == "." || d.UploadID == ".." {
		return nil, errors.New("resultDelivery.uploadId is invalid")
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

func parseRemoteTrace(raw json.RawMessage) *remoteTrace {
	var t struct {
		TraceID          string `json:"traceId"`
		ParentID         string `json:"parentId"`
		SamplingPriority int    `json:"samplingPriority"`
	}
	if strictDecode(raw, &t) != nil || (t.SamplingPriority != 1 && t.SamplingPriority != 2) {
		return nil
	}
	for _, id := range []string{t.TraceID, t.ParentID} {
		n, err := strconv.ParseUint(id, 10, 64)
		if !remoteTraceIDPatten.MatchString(id) || err != nil || n == 0 {
			return nil
		}
	}
	// A missing or invalid trace context never affects a run.
	return &remoteTrace{TraceID: t.TraceID, ParentID: t.ParentID, SamplingPriority: t.SamplingPriority}
}

// parseRemoteRequest strictly decodes the Agent's integration call.
func parseRemoteRequest(request string) (remoteRequest, error) {
	var req remoteRequest
	fields, err := rawFields([]byte(request), "request", "operation", "target", "query", "includeSchema", "resultDelivery", "traceContext")
	if err != nil {
		return req, err
	}
	if req.Operation, err = requiredString(fields, "operation", "operation"); err != nil {
		return req, err
	}
	targetFields, err := rawFields(fields["target"], "target", "host", "port", "dbname", "database_instance")
	if err != nil {
		return req, err
	}
	if _, instance := targetFields["database_instance"]; instance {
		if len(targetFields) != 1 {
			return req, errors.New("target.database_instance cannot be combined with host, port, or dbname")
		}
		req.Target.DatabaseInstance, err = requiredString(targetFields, "database_instance", "target.database_instance")
		if err != nil {
			return req, err
		}
	} else {
		if req.Target.Host, err = requiredString(targetFields, "host", "target.host"); err != nil {
			return req, err
		}
		if req.Target.DBName, err = requiredString(targetFields, "dbname", "target.dbname"); err != nil {
			return req, err
		}
		if json.Unmarshal(targetFields["port"], &req.Target.Port) != nil || req.Target.Port < 1 || req.Target.Port > 65535 {
			return req, errors.New("target.port must be an integer from 1 to 65535")
		}
	}
	switch req.Operation {
	case remoteOperationResolve:
		if len(fields) != 2 {
			return req, errors.New("resolve carries only the operation and the target")
		}
		return req, nil
	case remoteOperationExecute:
	default:
		return req, errors.New("unknown operation")
	}
	if req.Query, err = requiredString(fields, "query", "query"); err != nil {
		return req, err
	}
	if json.Unmarshal(fields["includeSchema"], &req.IncludeSchema) != nil {
		return req, errors.New("includeSchema must be a boolean")
	}
	if req.Delivery, err = parseRemoteDelivery(fields["resultDelivery"]); err != nil {
		return req, err
	}
	if raw, ok := fields["traceContext"]; ok {
		req.Trace = parseRemoteTrace(raw)
	}
	return req, nil
}

func normalizeRemoteHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}

// remoteServes reports whether this check's own configuration serves the target. It
// is pure configuration: no connection, no SQL, no state change. Configurations
// outside the proof of concept's scope never match, so unrelated Oracle instances on
// the same Agent cannot make a resolution ambiguous or fail.
func remoteServes(cfg *config.CheckConfig, t remoteTarget) bool {
	if cfg == nil || t.DatabaseInstance != "" {
		return false
	}
	c := cfg.ConnectionConfig
	switch {
	case c.TnsAlias != "", c.TnsAdmin != "", c.OracleClient, c.Wallet != "", c.ServerEmbedsPort:
		return false
	case c.Protocol != "" && !strings.EqualFold(strings.TrimSpace(c.Protocol), "TCP"):
		return false
	}
	service := strings.TrimSpace(c.ServiceName)
	host := normalizeRemoteHost(c.Server)
	if service == "" || host == "" || c.Port < 1 || c.Port > 65535 {
		return false
	}
	return host == normalizeRemoteHost(t.Host) && c.Port == t.Port && strings.EqualFold(service, strings.TrimSpace(t.DBName))
}

// remoteQueryState tracks the runs admitted on one check so unscheduling can stop them.
type remoteQueryState struct {
	mu      sync.Mutex
	closed  bool
	next    uint64
	cancels map[uint64]context.CancelFunc
	wg      sync.WaitGroup
}

func (s *remoteQueryState) admit(ctx context.Context) (context.Context, func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, nil, check.ErrRemoteQueryCancelled
	}
	runCtx, cancel := context.WithCancel(ctx)
	if s.cancels == nil {
		s.cancels = map[uint64]context.CancelFunc{}
	}
	id := s.next
	s.next++
	s.cancels[id] = cancel
	s.wg.Add(1)
	return runCtx, func() {
		s.mu.Lock()
		delete(s.cancels, id)
		s.mu.Unlock()
		cancel()
		s.wg.Done()
	}, nil
}

// close refuses new runs, cancels runs in progress and waits for them to end.
func (s *remoteQueryState) close() {
	s.mu.Lock()
	s.closed = true
	for _, cancel := range s.cancels {
		cancel()
	}
	s.mu.Unlock()
	s.wg.Wait()
}

// SupportsRemoteQueries reports that this check implements the remote query boundary.
func (c *Check) SupportsRemoteQueries() bool {
	return c.config != nil
}

// Cancel stops remote runs before the check is torn down. It cancels the runs'
// contexts, which closes their database sockets and aborts their upload sessions,
// and waits for them: each run's remaining work is bounded (one abort request with
// its own short budget). It does not claim the database stops the statement.
func (c *Check) Cancel() {
	c.remote.close()
	c.CheckBase.Cancel()
}

// RemoteQuery implements check.RemoteQuerier. Only the outcome crosses this boundary.
func (c *Check) RemoteQuery(ctx context.Context, request string) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	runCtx, release, err := c.remote.admit(ctx)
	if err != nil {
		return "", err
	}
	defer release()
	req, err := parseRemoteRequest(request)
	if err != nil {
		return remoteFailed(remoteCodeInvalidRequest, "the remote query request is invalid"), nil
	}
	if req.Operation == remoteOperationResolve {
		if !remoteServes(c.config, req.Target) {
			return remoteFailed(remoteCodeTargetNotFound, "this Oracle check does not serve the target"), nil
		}
		return encodeRemoteAnswer(remoteAnswer{Status: "MATCHED"}), nil
	}
	return c.executeRemote(runCtx, req), nil
}

func (c *Check) remoteHooks() remoteQueryHooks {
	if c.remoteTestHooks != nil {
		return *c.remoteTestHooks
	}
	return defaultRemoteQueryHooks()
}

// executeRemote revalidates the request from configuration, runs the approved
// statement and uploads its rows. It always returns a final answer.
func (c *Check) executeRemote(ctx context.Context, req remoteRequest) string {
	start := time.Now()
	run := &remoteRun{check: c, req: req, hooks: c.remoteHooks()}
	answer := run.execute(ctx)
	status, code := "SUCCEEDED", ""
	if run.failure != nil {
		status, code = "FAILED", run.failure.code
	}
	// Identifiers only: never SQL, credentials, DSNs, driver text or rows.
	log.Infof("oracle remote query check=%s crawl_id=%s task_id=%s upload_id=%s outcome=%s code=%s pages=%d rows=%d duration=%s",
		c.ID(), req.Delivery.CrawlID, req.Delivery.TaskID, req.Delivery.UploadID, status, code, run.pages, run.rows, time.Since(start))
	return answer
}

func intakeSiteBaseURL(site string) (string, bool) {
	site = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(site), "."))
	if site == "" || strings.ContainsAny(site, "/:@?#\\ \t\r\n%") {
		return "", false
	}
	u := url.URL{Scheme: "https", Host: "api." + site, Path: "/api/unstable/its-agent-intake"}
	return u.String(), true
}
