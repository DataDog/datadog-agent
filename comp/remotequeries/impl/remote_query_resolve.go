// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remotequeriesimpl

import (
	"encoding/json"
	"net/http"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
)

const (
	// RemoteQueryResolveEndpointPath is mounted under /agent by the Agent command API.
	RemoteQueryResolveEndpointPath = "/remote-queries/resolve"
	// RemoteQueriesResolveEnabledConfig is disabled by default when the key is absent.
	RemoteQueriesResolveEnabledConfig = "remote_queries.resolve.enabled"
)

const (
	// RemoteQueryStatusMatched reports exactly one loaded check matching the
	// resolved target; the result always carries a non-empty match fingerprint.
	RemoteQueryStatusMatched = statusMatched
	// RemoteQueryStatusResolutionError reports a resolve operation that could not
	// complete matching: an internal or contract error, never a target miss.
	RemoteQueryStatusResolutionError = statusResolutionError
)

// NewRemoteQueryResolveEndpointProvider registers the remote query resolve endpoint
// on the internal Agent API. It mirrors the match-check diagnostic endpoint's shape
// (strict integration + target request, config-gated) but answers with the
// match-before-execute resolve statuses and the match fingerprint.
func NewRemoteQueryResolveEndpointProvider(reqs Requires) api.AgentEndpointProvider {
	h := &remoteQueryResolveHandler{
		service: NewRemoteQueryResolveService(reqs.Collector, reqs.Cfg.GetBool(RemoteQueriesResolveEnabledConfig)),
	}
	return api.NewAgentEndpointProvider(h.handle, RemoteQueryResolveEndpointPath, http.MethodPost)
}

type remoteQueryResolveHandler struct {
	service *RemoteQueryResolveService
}

// RemoteQueryResolveService resolves a Remote Queries target through the shared
// integration matcher without side effects: no query, no result delivery, no page
// writer, and no upload credentials ever reach this service. It answers the
// structured zero/one/many outcome plus an opaque match fingerprint for the unique
// case, and is shared by the HTTP diagnostic endpoint and the AgentSecure
// RemoteQueryResolve RPC.
type RemoteQueryResolveService struct {
	collector RemoteQueryCollector
	enabled   bool
}

// NewRemoteQueryResolveService creates the shared resolver used by the HTTP
// endpoint and the AgentSecure RPC.
func NewRemoteQueryResolveService(collector RemoteQueryCollector, enabled bool) *RemoteQueryResolveService {
	return &RemoteQueryResolveService{collector: collector, enabled: enabled}
}

// RemoteQueryResolveRequest is the typed resolve request: the integration and the
// target only. There is no field that could carry SQL, result-delivery data, or
// credentials.
type RemoteQueryResolveRequest struct {
	Integration string
	Target      RemoteQueryExecuteTarget
}

// RemoteQueryResolveResult is the resolve service result: the structured status,
// the opaque match fingerprint when exactly one check matched, and a sanitized
// error mirroring the status otherwise.
type RemoteQueryResolveResult struct {
	HTTPStatus       int
	Status           string
	MatchFingerprint string
	Error            *RemoteQueryExecuteError
}

// Resolve answers the structured zero/one/many outcome for the requested target.
// It reuses the exact matcher execute uses — never a second matcher — so the
// diagnostic match-check, resolve, and execute paths cannot disagree. Internal
// and contract failures answer resolution_error with a sanitized message, never a
// silent target miss: a malformed request cannot look like a missing target.
func (s *RemoteQueryResolveService) Resolve(req RemoteQueryResolveRequest) RemoteQueryResolveResult {
	if s == nil || !s.enabled {
		return remoteQueryResolveErrorResult(http.StatusServiceUnavailable, statusResolutionError, "remote queries resolve bridge is disabled")
	}
	if s.collector == nil {
		return remoteQueryResolveErrorResult(http.StatusFailedDependency, statusResolutionError, "remote query resolver is unavailable")
	}

	integration, err := parseIntegration(req.Integration)
	if err != nil {
		return remoteQueryResolveErrorResult(http.StatusBadRequest, statusResolutionError, err.Error())
	}
	target, err := parseExecuteTarget(req.Target)
	if err != nil {
		return remoteQueryResolveErrorResult(http.StatusBadRequest, statusResolutionError, err.Error())
	}

	matches := findIntegrationMatches(s.collector, integration, target)
	switch len(matches) {
	case 0:
		return remoteQueryResolveErrorResult(http.StatusNotFound, statusTargetNotFound, "no matching integration check found")
	case 1:
		fingerprint, err := computeMatchFingerprint(integration, target, matches[0])
		if err != nil {
			return remoteQueryResolveErrorResult(http.StatusFailedDependency, statusResolutionError, "could not compute match fingerprint")
		}
		return RemoteQueryResolveResult{HTTPStatus: http.StatusOK, Status: statusMatched, MatchFingerprint: fingerprint}
	default:
		return remoteQueryResolveErrorResult(http.StatusConflict, statusAmbiguous, "multiple matching integration checks found")
	}
}

func remoteQueryResolveErrorResult(httpStatus int, status string, message string) RemoteQueryResolveResult {
	return RemoteQueryResolveResult{
		HTTPStatus: httpStatus,
		Status:     status,
		Error:      &RemoteQueryExecuteError{Code: status, Message: message},
	}
}

func (h *remoteQueryResolveHandler) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if h.service == nil || !h.service.enabled {
		writeResolveError(w, http.StatusServiceUnavailable, statusBridgeDisabled, "remote queries resolve bridge is disabled")
		return
	}

	// The resolve request shape is the match-check request shape: strict
	// integration + target JSON with identical selector-mode validation, parsed
	// by the shared match request parser.
	req, err := parseMatchRequest(r)
	if err != nil {
		writeResolveParseError(w, err)
		return
	}

	result := h.service.Resolve(RemoteQueryResolveRequest{
		Integration: req.Integration,
		Target:      RemoteQueryExecuteTarget(req.Target),
	})
	writeResolveResponse(w, result)
}

// remoteQueryResolveResponseJSON is the HTTP wire shape of the resolve outcome:
// exactly the status, the optional match fingerprint, and the optional error
// object mirroring the AP action output contract.
type remoteQueryResolveResponseJSON struct {
	Status           string         `json:"status"`
	MatchFingerprint string         `json:"matchFingerprint,omitempty"`
	Error            *responseError `json:"error,omitempty"`
}

func writeResolveResponse(w http.ResponseWriter, result RemoteQueryResolveResult) {
	w.WriteHeader(result.HTTPStatus)
	resp := remoteQueryResolveResponseJSON{
		Status:           result.Status,
		MatchFingerprint: result.MatchFingerprint,
	}
	if result.Error != nil {
		resp.Error = &responseError{Code: result.Error.Code, Message: result.Error.Message}
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func writeResolveParseError(w http.ResponseWriter, err error) {
	parseErr, ok := err.(requestParseError)
	if !ok {
		writeResolveError(w, http.StatusBadRequest, statusInvalidRequest, err.Error())
		return
	}

	writeResolveError(w, http.StatusBadRequest, parseErr.status, parseErr.message)
}

func writeResolveError(w http.ResponseWriter, httpStatus int, status string, message string) {
	w.WriteHeader(httpStatus)
	_ = json.NewEncoder(w).Encode(remoteQueryResolveResponseJSON{
		Status: status,
		Error:  &responseError{Code: status, Message: message},
	})
}
