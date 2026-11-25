// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remotequeries

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// tunnelResolveTimeout keeps the synchronous resolve step inside the RC
	// transport's handler deadline. The detached run is not bound by it.
	tunnelResolveTimeout = 20 * time.Second
	// tunnelMaxInFlight bounds the runs a tunnel can have executing at once.
	tunnelMaxInFlight = 4
)

// TunnelStatus is the transport-neutral outcome of one tunnel Execute request.
type TunnelStatus int

// The possible outcomes of a tunnel Execute request.
const (
	// TunnelRejected means no run was started.
	TunnelRejected TunnelStatus = iota
	// TunnelAccepted means the run was admitted and handed to the integration.
	TunnelAccepted
	// TunnelMatched means a resolve-only request found exactly one check.
	TunnelMatched
)

// TunnelOutcome is the sanitized result of a tunnel Execute request.
type TunnelOutcome struct {
	Status TunnelStatus
	// Code and Message are set only for TunnelRejected. Message is fixed text
	// per code and never carries request or check content.
	Code    string
	Message string
}

// TunnelTrust is the identity the transport itself established for the
// current dispatch. The request's own org_id and connection_id are claims that
// must match it, never proof.
type TunnelTrust struct {
	OrgID        int64
	ConnectionID string
}

// TunnelAuthority establishes, independently of the request body, that a
// dispatch comes from the authorized Remote Queries worker for an org and that
// this Agent may run intake-backed queries from it. It returns an error when
// the dispatch is not authorized. There is deliberately no implementation
// that always allows.
type TunnelAuthority interface {
	Authorize(ctx context.Context, correlationID uint64) (TunnelTrust, error)
}

// TunnelLimits mirrors the worker-owned limits, verbatim and unbounded by the Agent.
type TunnelLimits struct {
	MaxFileBytes, MaxResultBytes, MaxRowBytes, MaxColumns, MaxSchemaBytes, MaxPages, TimeoutMs int64
}

// TunnelRequest is the decoded tunnel Execute request, before validation.
type TunnelRequest struct {
	ConnectionID  string
	OrgID         int64
	Integration   string
	Target        Target
	Query         string
	IncludeSchema bool
	TaskID        string
	ResolveOnly   bool
	RunID         string
	// Limits is nil when the request carries none.
	Limits *TunnelLimits
}

// Tunnel admits Remote Queries started over the RC tunnel. It reuses the
// Service's target resolution, upload destination and detached hand-off, and
// adds only the tunnel's identity, authorization and in-flight bound. It keeps
// no per-run record, so it does not detect duplicate run IDs.
type Tunnel struct {
	service   *Service
	authority TunnelAuthority

	mu       sync.Mutex
	inFlight int
}

// NewTunnel returns a tunnel adapter. It fails closed when there is no
// authority to establish the trusted org and connection.
func NewTunnel(service *Service, authority TunnelAuthority) (*Tunnel, error) {
	if service == nil {
		return nil, errors.New("remote query service is required")
	}
	if authority == nil {
		return nil, errors.New("remote query tunnel requires a trusted authority")
	}
	return &Tunnel{service: service, authority: authority}, nil
}

func reject(code, message string) TunnelOutcome {
	return TunnelOutcome{Status: TunnelRejected, Code: code, Message: message}
}

var tunnelMessages = map[string]string{
	"invalid_request":         "request is invalid",
	"unauthorized":            "request is not authorized",
	"disabled":                "remote queries are disabled",
	"busy":                    "too many runs in progress",
	"target_not_found":        "no loaded check matches the target",
	"ambiguous_target":        "more than one loaded check matches the target",
	"resolution_error":        "target resolution failed",
	"integration_unavailable": "integration is unavailable",
	"cancelled":               "request ended before execution",
}

func rejectCode(code string) TunnelOutcome {
	message, ok := tunnelMessages[code]
	if !ok {
		code, message = "integration_unavailable", tunnelMessages["integration_unavailable"]
	}
	return reject(code, message)
}

// Execute validates and admits one tunnel request. It never starts a run
// unless the transport-established identity, local opt-in and request all agree.
func (t *Tunnel) Execute(ctx context.Context, correlationID uint64, tr TunnelRequest) TunnelOutcome {
	trust, err := t.authority.Authorize(ctx, correlationID)
	if err != nil || trust.OrgID <= 0 || trust.ConnectionID == "" {
		return rejectCode("unauthorized")
	}
	if tr.OrgID != trust.OrgID || tr.ConnectionID != trust.ConnectionID {
		return rejectCode("unauthorized")
	}
	if t.service.Config == nil || !t.service.Config.GetBool("remote_queries.enabled") {
		return rejectCode("disabled")
	}
	req, err := tunnelRequest(tr)
	if err != nil {
		return rejectCode("invalid_request")
	}
	if tr.ResolveOnly {
		return t.resolve(ctx, req)
	}
	return t.execute(ctx, req)
}

func (t *Tunnel) resolve(ctx context.Context, req Request) TunnelOutcome {
	ctx, cancel := context.WithTimeout(ctx, tunnelResolveTimeout)
	defer cancel()
	answer := t.service.run(ctx, req, &summary{})
	if answer.Status == "matched" {
		return TunnelOutcome{Status: TunnelMatched}
	}
	return t.rejection(answer)
}

func (t *Tunnel) rejection(answer Answer) TunnelOutcome {
	if answer.Error == nil {
		return rejectCode("")
	}
	return rejectCode(answer.Error.Code)
}

func (t *Tunnel) execute(ctx context.Context, req Request) TunnelOutcome {
	t.mu.Lock()
	if t.inFlight >= tunnelMaxInFlight {
		t.mu.Unlock()
		return rejectCode("busy")
	}
	t.inFlight++
	t.mu.Unlock()

	// The detached run releases its slot when it ends, possibly before start returns.
	finish := func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		t.inFlight--
	}
	ctx, cancel := context.WithTimeout(ctx, tunnelResolveTimeout)
	defer cancel()
	answer := t.service.start(ctx, req, &summary{}, finish)
	if answer.Status != "SUCCEEDED" {
		// start spawns the detached goroutine only on success, so release the slot here.
		t.mu.Lock()
		t.inFlight--
		t.mu.Unlock()
		return t.rejection(answer)
	}
	return TunnelOutcome{Status: TunnelAccepted}
}

// tunnelRequest validates a tunnel request with the same rules as the action
// input and builds the executor's Request. The delivery has no crawl ID.
func tunnelRequest(tr TunnelRequest) (Request, error) {
	req := Request{ResolveOnly: tr.ResolveOnly}
	integration := strings.ToLower(strings.TrimSpace(tr.Integration))
	if !integrationPattern.MatchString(integration) {
		return req, errors.New("invalid integration")
	}
	req.Integration = integration
	targetJSON := map[string]interface{}{}
	if tr.Target.DatabaseInstance != "" {
		targetJSON["database_instance"] = tr.Target.DatabaseInstance
	}
	if tr.Target.Host != "" {
		targetJSON["host"] = tr.Target.Host
	}
	if tr.Target.DBName != "" {
		targetJSON["dbname"] = tr.Target.DBName
	}
	if tr.Target.Port != 0 {
		targetJSON["port"] = tr.Target.Port
	}
	// Marshaling a map of strings and an int cannot fail.
	raw, _ := json.Marshal(targetJSON)
	target, err := parseTarget(raw)
	if err != nil {
		return req, err
	}
	req.Target = target
	if tr.ResolveOnly {
		// A resolve carries the selector only: no SQL, limits or run identity.
		if tr.Query != "" || tr.IncludeSchema || tr.TaskID != "" || tr.RunID != "" || tr.Limits != nil {
			return req, errors.New("resolve_only request carries execution fields")
		}
		return req, nil
	}
	if strings.TrimSpace(tr.Query) == "" {
		return req, errors.New("query is required")
	}
	if !uuidPattern.MatchString(tr.TaskID) || !uuidPattern.MatchString(tr.RunID) {
		return req, errors.New("task_id and run_id must be UUIDs")
	}
	if tr.Limits == nil {
		return req, errors.New("limits are required")
	}
	var limits Limits
	for _, f := range []struct {
		value int64
		dest  *json.Number
	}{
		{tr.Limits.MaxFileBytes, &limits.MaxFileBytes}, {tr.Limits.MaxResultBytes, &limits.MaxResultBytes},
		{tr.Limits.MaxRowBytes, &limits.MaxRowBytes}, {tr.Limits.MaxColumns, &limits.MaxColumns},
		{tr.Limits.MaxSchemaBytes, &limits.MaxSchemaBytes}, {tr.Limits.MaxPages, &limits.MaxPages},
		{tr.Limits.TimeoutMs, &limits.TimeoutMs},
	} {
		if f.value <= 0 {
			return req, errors.New("limits must be positive")
		}
		*f.dest = json.Number(strconv.FormatInt(f.value, 10))
	}
	req.Query = tr.Query
	req.IncludeSchema = tr.IncludeSchema
	// The run ID is the intake upload ID; the crawl ID is deliberately absent.
	req.ResultDelivery = &Delivery{TaskID: tr.TaskID, UploadID: tr.RunID, Limits: limits}
	return req, nil
}
