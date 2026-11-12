// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remotequeries

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
)

// Error is the outcome's only failure detail.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Answer is the complete action output. It never carries result data.
type Answer struct {
	Status string `json:"status"`
	Error  *Error `json:"error,omitempty"`
}

// Logger is the Agent's structured-summary logging boundary.
type Logger interface {
	Infof(string, ...interface{})
}

// Service resolves against a fresh loaded-check inventory for every request.
// Its handler must be mounted behind the command API's IPC authentication middleware.
type Service struct {
	Config model.Reader
	Checks func() []check.Check
	Log    Logger
}

type summary struct {
	candidates int
	matched    string
}

func failure(resolve bool, code, message string) Answer {
	status := "FAILED"
	if resolve {
		status = code
	}
	return Answer{Status: status, Error: &Error{Code: code, Message: message}}
}

// ServeHTTP serves the resolve and execute operations on the local command API.
func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// The command API is loopback-bound. Also refuse network-originated requests
	// if a future listener configuration exposes this route more widely.
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || !net.ParseIP(host).IsLoopback() {
		http.Error(w, "local IPC required", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	start := time.Now()
	var req Request
	var integration, runID, taskID, uploadID string
	var stats summary
	answer := failure(false, "invalid_request", "input is malformed")
	defer func() {
		kind := "execute"
		if req.ResolveOnly {
			kind = "resolve"
		}
		code := ""
		if answer.Error != nil {
			code = answer.Error.Code
		}
		if s.Log != nil {
			// Values use quoted formatting; neither check errors nor request bodies are logged.
			s.Log.Infof("remote query integration=%q run_id=%q task_id=%q upload_id=%q kind=%s outcome=%s code=%s duration=%s candidates=%d matched_check=%q", integration, runID, taskID, uploadID, kind, answer.Status, code, time.Since(start), stats.candidates, stats.matched)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(answer)
	}()
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return
	}
	fields, err := object(data, "request", "input", "traceContext")
	if err != nil {
		return
	}
	integration, runID, taskID, uploadID = requestIdentity(fields["input"])
	var validationErr error
	req, validationErr = Parse(fields["input"], s.Config.GetString("site"))
	allowed := parconfig.NewActionsAllowlist(s.Config.GetStringSlice("private_action_runner.actions_allowlist")).IsActionAllowed("com.datadoghq.remoteaction.queries", "execute")
	if !s.Config.GetBool("remote_queries.enabled") || !allowed {
		code := "disabled"
		if req.ResolveOnly {
			code = "resolution_error"
		}
		answer = failure(req.ResolveOnly, code, "remote queries are disabled or the action is not allowed")
		return
	}
	if validationErr != nil {
		code := "invalid_request"
		if req.ResolveOnly {
			code = "resolution_error"
		}
		answer = failure(req.ResolveOnly, code, validationErr.Error())
		return
	}
	var trace TraceContext
	if json.Unmarshal(fields["traceContext"], &trace) == nil && trace.Valid() {
		req.TraceContext = &trace
	}
	answer = s.run(r.Context(), req, &stats)
}

// Correlation fields are safe to log even if a different field fails validation.
// Never log arbitrary malformed values supplied in place of an identifier.
func requestIdentity(raw []byte) (integration, runID, taskID, uploadID string) {
	var identity struct {
		Integration string `json:"integration"`
		Delivery    struct {
			RunID    string `json:"runId"`
			TaskID   string `json:"taskId"`
			UploadID string `json:"uploadId"`
		} `json:"resultDelivery"`
	}
	_ = json.Unmarshal(raw, &identity)
	name := strings.ToLower(strings.TrimSpace(identity.Integration))
	if integrationPattern.MatchString(name) {
		integration = name
	}
	if uuidPattern.MatchString(identity.Delivery.RunID) {
		runID = identity.Delivery.RunID
	}
	if uuidPattern.MatchString(identity.Delivery.TaskID) {
		taskID = identity.Delivery.TaskID
	}
	if uploadPattern.MatchString(identity.Delivery.UploadID) {
		uploadID = identity.Delivery.UploadID
	}
	return
}

type integrationRequest struct {
	Operation      string        `json:"operation"`
	Target         Target        `json:"target"`
	Query          string        `json:"query,omitempty"`
	IncludeSchema  *bool         `json:"includeSchema,omitempty"`
	ResultDelivery *Delivery     `json:"resultDelivery,omitempty"`
	TraceContext   *TraceContext `json:"traceContext,omitempty"`
}

type callResult struct {
	answer Answer
	err    error
}

func invoke(ctx context.Context, c check.RemoteQuerier, input string, resolve bool) (out callResult) {
	defer func() {
		if recover() != nil {
			out = callResult{err: errors.New("remote_query call panicked")}
		}
	}()
	if !c.SupportsRemoteQueries() {
		return callResult{err: errors.New("remote queries are not supported")}
	}
	raw, err := c.RemoteQuery(ctx, input)
	if err != nil {
		if errors.Is(err, check.ErrRemoteQueryCancelled) {
			return callResult{err: check.ErrRemoteQueryCancelled}
		}
		return callResult{err: errors.New("remote_query call failed without a final string outcome")}
	}
	answer, err := parseAnswer([]byte(raw), resolve)
	return callResult{answer, err}
}

func parseAnswer(raw []byte, resolve bool) (Answer, error) {
	fields, err := object(raw, "answer", "status", "error")
	if err != nil {
		return Answer{}, err
	}
	status, err := stringField(fields, "status", "answer.status")
	if err != nil {
		return Answer{}, err
	}
	success := "SUCCEEDED"
	if resolve {
		success = "MATCHED"
	}
	if status == success && len(fields) == 1 {
		return Answer{Status: status}, nil
	}
	if status != "FAILED" || len(fields) != 2 {
		return Answer{}, errors.New("answer must contain exactly one final outcome")
	}
	detail, err := object(fields["error"], "answer.error", "code", "message")
	if err != nil {
		return Answer{}, err
	}
	code, err := stringField(detail, "code", "answer.error.code")
	if err != nil {
		return Answer{}, err
	}
	message, err := stringField(detail, "message", "answer.error.message")
	if err != nil {
		return Answer{}, err
	}
	return Answer{Status: status, Error: &Error{code, message}}, nil
}

// Cancellation can race the integration registering its run. Retrying the
// idempotent signal until the call returns covers that race, including Python
// interpreter thread switches before registration. Waiting never holds the GIL.
func cancelUntilDone(querier check.RemoteQuerier, runID string, answer <-chan callResult) {
	// A buggy check must not panic out of this detached cleanup goroutine.
	defer func() { _ = recover() }()
	for {
		select {
		case <-answer:
			return
		default:
		}
		if err := querier.CancelRemoteQuery(runID); err != nil {
			return
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-answer:
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func resolutionCode(matches int) string {
	if matches == 0 {
		return "target_not_found"
	}
	if matches == 1 {
		return "matched"
	}
	return "ambiguous_target"
}

// ParseActionAnswer rejects any output outside the action's documented fields.
func ParseActionAnswer(raw []byte, resolve bool) (Answer, error) {
	if !resolve {
		return parseAnswer(raw, false)
	}
	fields, err := object(raw, "answer", "status", "error")
	if err != nil {
		return Answer{}, err
	}
	status, err := stringField(fields, "status", "answer.status")
	if err != nil {
		return Answer{}, err
	}
	if status == "matched" && len(fields) == 1 {
		return Answer{Status: status}, nil
	}
	if status != "target_not_found" && status != "ambiguous_target" && status != "resolution_error" {
		return Answer{}, errors.New("unknown resolution status")
	}
	var answer Answer
	if json.Unmarshal(raw, &answer) != nil || answer.Error == nil || answer.Error.Code != status || strings.TrimSpace(answer.Error.Message) == "" {
		return Answer{}, errors.New("invalid resolution error")
	}
	if _, err := object(fields["error"], "answer.error", "code", "message"); err != nil {
		return Answer{}, err
	}
	return answer, nil
}

func (s *Service) run(ctx context.Context, req Request, stats *summary) Answer {
	var candidates []check.Check
	for _, c := range s.Checks() {
		if strings.EqualFold(c.String(), req.Integration) {
			candidates = append(candidates, c)
		}
	}
	stats.candidates = len(candidates)
	resolveInput, _ := json.Marshal(integrationRequest{Operation: "resolve", Target: req.Target})
	type resolved struct {
		c      check.Check
		result callResult
	}
	answers := make(chan resolved, len(candidates))
	for _, candidate := range candidates {
		go func(c check.Check) {
			querier, ok := c.(check.RemoteQuerier)
			if !ok {
				answers <- resolved{c, callResult{err: errors.New("remote queries are not supported")}}
				return
			}
			answers <- resolved{c, invoke(ctx, querier, string(resolveInput), true)}
		}(candidate)
	}
	var matches []check.Check
	var resolutionError string
	for range candidates {
		select {
		case <-ctx.Done():
			code := "cancelled"
			if req.ResolveOnly {
				code = "resolution_error"
			}
			return failure(req.ResolveOnly, code, "caller ended while resolving target")
		case result := <-answers:
			if result.result.err != nil {
				resolutionError = fmt.Sprintf("candidate %s: %s", result.c.ID(), result.result.err)
			} else if result.result.answer.Status == "MATCHED" {
				matches = append(matches, result.c)
			} else if result.result.answer.Error.Code != "target_not_found" {
				resolutionError = fmt.Sprintf("candidate %s: %s", result.c.ID(), result.result.answer.Error.Code)
			}
		}
	}
	if resolutionError != "" {
		return failure(req.ResolveOnly, "resolution_error", resolutionError)
	}
	switch resolutionCode(len(matches)) {
	case "target_not_found":
		return failure(req.ResolveOnly, "target_not_found", "no loaded check matches the target")
	case "ambiguous_target":
		return failure(req.ResolveOnly, "ambiguous_target", "more than one loaded check matches the target")
	}
	matched := matches[0]
	stats.matched = string(matched.ID())
	if req.ResolveOnly {
		return Answer{Status: "matched"}
	}
	querier, ok := matched.(check.RemoteQuerier)
	if !ok {
		return failure(false, "integration_unavailable", "matched check does not support remote queries")
	}
	if ctx.Err() != nil {
		return failure(false, "cancelled", "caller ended before execution")
	}
	executeInput, _ := json.Marshal(integrationRequest{Operation: "execute", Target: req.Target, Query: req.Query, IncludeSchema: &req.IncludeSchema, ResultDelivery: req.ResultDelivery, TraceContext: req.TraceContext})
	answer := make(chan callResult, 1)
	go func() { answer <- invoke(ctx, querier, string(executeInput), false) }()
	select {
	case <-ctx.Done():
		// Runs with the same run ID are cancelled together by the integration.
		// Cancellation is prompt, but must not extend the caller's task deadline.
		go cancelUntilDone(querier, req.ResultDelivery.RunID, answer)
		return failure(false, "cancelled", "caller ended during execution")
	case result := <-answer:
		if ctx.Err() != nil {
			return failure(false, "cancelled", "caller ended during execution")
		}
		if result.err != nil {
			if errors.Is(result.err, check.ErrRemoteQueryCancelled) {
				return failure(false, "cancelled", "check is being unscheduled")
			}
			return failure(false, "integration_unavailable", result.err.Error())
		}
		return result.answer
	}
}
