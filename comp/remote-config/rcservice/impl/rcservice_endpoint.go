// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package rcserviceimpl

import (
	"errors"
	"net/http"

	"google.golang.org/protobuf/encoding/protojson"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	rcservice "github.com/DataDog/datadog-agent/comp/remote-config/rcservice/def"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

// errRCNotInitialized is returned by the remote-config state endpoint when the
// remote configuration service is not running (for example, when remote config
// is disabled).
var errRCNotInitialized = errors.New("remote configuration service not initialized")

// rcStateEndpoint serves the remote config repositories state over the agent's
// authenticated IPC HTTP API. It exposes the same data as the existing
// AgentSecure.GetConfigState gRPC method, but as JSON over HTTP, so that callers
// that can only reach the agent through the local HTTP API — such as the
// co-located Private Action Runner — can read the remote-config state without a
// gRPC client.
type rcStateEndpoint struct {
	// svc is the remote config service. It is nil when remote config is
	// disabled or failed to start.
	svc rcservice.Component
}

// newRCStateEndpointProvider returns an agent-endpoint provider that serves the
// remote config state at GET /agent/remote-config/state. svc may be nil, in
// which case the endpoint reports that the service is not initialized.
func newRCStateEndpointProvider(svc rcservice.Component) api.AgentEndpointProvider {
	e := &rcStateEndpoint{svc: svc}
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

	body, err := protojson.Marshal(state)
	if err != nil {
		httputils.SetJSONError(w, err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
