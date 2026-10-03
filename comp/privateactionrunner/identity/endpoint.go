// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package identity resolves the split-mode PAR identity through the Core Agent API.
package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	par "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/fips"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/autoconnections"
	parenrollment "github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	parutil "github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
)

const route = "/private-action-runner/resolve-identity"

var resolutionMu sync.Mutex

type enrollAndPersistFunc func(context.Context, config.Component, *parenrollment.AgentIdentifier) (*parenrollment.Result, error)

type responseError struct {
	status int
	err    error
}

func (e *responseError) Error() string { return e.err.Error() }

func withStatus(status int, err error) error {
	return &responseError{status: status, err: err}
}

func statusForError(err error) int {
	var responseErr *responseError
	if errors.As(err, &responseErr) {
		return responseErr.status
	}
	return http.StatusInternalServerError
}

// Request indicates whether par-control has a sidecar-local configured identity.
type Request struct {
	HasLocalIdentity bool `json:"has_local_identity"`
}

// Response tells par-control whether to use its configured identity or one managed by the Agent.
type Response struct {
	Source     string `json:"source"`
	URN        string `json:"urn,omitempty"`
	PrivateKey string `json:"private_key,omitempty"`
}

// Requires contains the endpoint dependencies.
type Requires struct {
	Config   config.Component
	Hostname hostname.Component
}

// Provider registers the identity endpoint with the Core Agent API.
type Provider struct {
	Endpoint api.AgentEndpointProvider
}

// NewProvider returns the split-mode identity endpoint.
func NewProvider(requires Requires) Provider {
	return Provider{Endpoint: api.NewAgentEndpointProvider(handler(requires.Config, requires.Hostname, enrollAndPersist), route, http.MethodPost)}
}

func handler(cfg config.Component, hostnameComp hostname.Component, enroll enrollAndPersistFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request Request
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			http.Error(w, "invalid identity request", http.StatusBadRequest)
			return
		}

		response, err := resolve(r.Context(), cfg, hostnameComp, request, enroll)
		if err != nil {
			http.Error(w, err.Error(), statusForError(err))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, "failed to encode identity response", http.StatusInternalServerError)
		}
	}
}

func resolve(ctx context.Context, cfg config.Component, hostnameComp hostname.Component, request Request, enroll enrollAndPersistFunc) (*Response, error) {
	resolutionMu.Lock()
	defer resolutionMu.Unlock()

	// validation
	if !cfg.GetBool(par.PAREnabled) || !cfg.GetBool(par.PARSplitEnabled) {
		return nil, withStatus(http.StatusConflict, errors.New("Private Action Runner split mode is disabled"))
	}
	if cfg.GetBool("fips.enabled") {
		return nil, withStatus(http.StatusConflict, errors.New("private_action_runner.split_enabled is not supported with fips.enabled"))
	}
	if enabled, err := fips.Enabled(); err == nil && enabled {
		return nil, withStatus(http.StatusConflict, errors.New("private_action_runner.split_enabled is not supported by the FIPS Agent"))
	}

	// use existing identity if it's valid
	agentID, err := parenrollment.GetAgentIdentifier(ctx, hostnameComp)
	if err != nil {
		return nil, withStatus(http.StatusServiceUnavailable, err)
	}
	persisted, err := parenrollment.GetIdentityFromPreviousEnrollment(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if persisted != nil && !parenrollment.ShouldReenroll(agentID, persisted, cfg.GetString("api_key")) {
		applyIdentity(cfg, persisted)
		return providedResponse(persisted.URN, persisted.PrivateKey)
	}
	if request.HasLocalIdentity {
		return &Response{Source: "local"}, nil
	}

	// enroll and apply new identity
	if !cfg.GetBool(par.PARSelfEnroll) {
		return nil, withStatus(http.StatusConflict, errors.New("no Private Action Runner identity is configured and self-enrollment is disabled"))
	}
	if _, err := enroll(ctx, cfg, agentID); err != nil {
		return nil, withStatus(http.StatusServiceUnavailable, err)
	}
	persisted, err = parenrollment.GetIdentityFromPreviousEnrollment(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if persisted == nil {
		return nil, errors.New("the resolved Private Action Runner identity is incomplete")
	}
	applyIdentity(cfg, persisted)
	return providedResponse(persisted.URN, persisted.PrivateKey)
}

func providedResponse(urn, privateKey string) (*Response, error) {
	if urn == "" || privateKey == "" {
		return nil, errors.New("the resolved Private Action Runner identity is incomplete")
	}
	if _, err := parutil.ParseRunnerURN(urn); err != nil {
		return nil, fmt.Errorf("failed to parse the Private Action Runner identity: %w", err)
	}
	return &Response{Source: "provided", URN: urn, PrivateKey: privateKey}, nil
}

func applyIdentity(cfg config.Component, identity *parenrollment.PersistedIdentity) {
	if identity == nil {
		return
	}
	cfg.Set(par.PARUrn, identity.URN, model.SourceAgentRuntime)
	cfg.Set(par.PARPrivateKey, identity.PrivateKey, model.SourceAgentRuntime)
}

func enrollAndPersist(ctx context.Context, cfg config.Component, agentID *parenrollment.AgentIdentifier) (*parenrollment.Result, error) {
	result, err := parenrollment.Enroll(ctx, cfg, agentID)
	if err != nil {
		return nil, fmt.Errorf("enrollment failed: %w", err)
	}
	if err := parenrollment.RotateIdentity(ctx, cfg, result); err != nil {
		return nil, fmt.Errorf("failed to persist new identity: %w", err)
	}

	parCfg, err := parconfig.FromDDConfig(cfg, nil)
	if err == nil {
		if urn, err := parutil.ParseRunnerURN(result.URN); err == nil {
			autoconnections.CreateConnectionsIfEnabled(
				ctx, cfg, parCfg, cfg.GetString("api_key"), cfg.GetString("app_key"), urn.RunnerID,
				result, autoconnections.NewBasicTagsProvider(),
			)
		}
	}
	return result, nil
}
