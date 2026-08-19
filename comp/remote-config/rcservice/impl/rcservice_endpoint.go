// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package rcserviceimpl

import (
	"encoding/json"
	"errors"
	"net/http"

	"google.golang.org/protobuf/encoding/protojson"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	rcservice "github.com/DataDog/datadog-agent/comp/remote-config/rcservice/def"
	rcservicemrf "github.com/DataDog/datadog-agent/comp/remote-config/rcservicemrf/def"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// errRCNotInitialized is returned by the remote-config state endpoint when the
// remote configuration service is not running (for example, when remote config
// is disabled).
var errRCNotInitialized = errors.New("remote configuration service not initialized")

// rcStateEndpoint serves the remote config repositories state over the agent's
// authenticated IPC HTTP API. It exposes the same data as the existing
// AgentSecure.GetConfigState/GetConfigStateHA gRPC methods, but as JSON over
// HTTP, so that callers that can only reach the agent through the local HTTP
// API — such as the co-located Private Action Runner — can read the
// remote-config state without a gRPC client.
type rcStateEndpoint struct {
	// svc is the remote config service. It is nil when remote config is
	// disabled or failed to start.
	svc rcservice.Component
	// mrfSvc is the failover-DC remote config service. It is unset unless
	// multi_region_failover.enabled is true and the service started
	// successfully.
	mrfSvc option.Option[rcservicemrf.Component]
}

// rcState is the JSON payload returned by the endpoint. StateHA is omitted
// unless a failover-DC remote config service is running.
type rcState struct {
	State   json.RawMessage `json:"state"`
	StateHA json.RawMessage `json:"stateHA,omitempty"`
}

// newRCStateEndpointProvider returns an agent-endpoint provider that serves the
// remote config state at GET /agent/remote-config/state. svc may be nil, in
// which case the endpoint reports that the service is not initialized.
func newRCStateEndpointProvider(svc rcservice.Component, mrfSvc option.Option[rcservicemrf.Component]) api.AgentEndpointProvider {
	e := &rcStateEndpoint{svc: svc, mrfSvc: mrfSvc}
	return api.NewAgentEndpointProvider(e.handle, "/remote-config/state", "GET")
}

func (e *rcStateEndpoint) handle(w http.ResponseWriter, _ *http.Request) {
	if e.svc == nil {
		httputils.SetJSONError(w, errRCNotInitialized, http.StatusServiceUnavailable)
		return
	}

	state, err := e.svc.ConfigGetState()
	if err != nil {
		httputils.SetJSONError(w, err, http.StatusInternalServerError)
		return
	}

	resp := rcState{}
	resp.State, err = protojson.Marshal(state)
	if err != nil {
		httputils.SetJSONError(w, err, http.StatusInternalServerError)
		return
	}

	if mrfSvc, ok := e.mrfSvc.Get(); ok {
		stateHA, err := mrfSvc.ConfigGetState()
		if err != nil {
			httputils.SetJSONError(w, err, http.StatusInternalServerError)
			return
		}
		resp.StateHA, err = protojson.Marshal(stateHA)
		if err != nil {
			httputils.SetJSONError(w, err, http.StatusInternalServerError)
			return
		}
	}

	body, err := json.Marshal(resp)
	if err != nil {
		httputils.SetJSONError(w, err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
