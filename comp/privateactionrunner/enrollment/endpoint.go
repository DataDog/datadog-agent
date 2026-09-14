// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package enrollment exposes split-mode PAR enrollment through the Core Agent API.
package enrollment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

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
	"github.com/DataDog/datadog-agent/pkg/version"
)

const route = "/private-action-runner/ensure-enrollment"

type enrollAndPersistFunc func(context.Context, config.Component, *parenrollment.AgentIdentifier) (*parenrollment.Result, error)

// Response is the identity par-control uses to authenticate with OPMS.
type Response struct {
	URN          string `json:"urn"`
	PrivateKey   string `json:"private_key"`
	OrgID        int64  `json:"org_id"`
	RunnerID     string `json:"runner_id"`
	AgentVersion string `json:"agent_version"`
}

// Requires contains the endpoint dependencies.
type Requires struct {
	Config   config.Component
	Hostname hostname.Component
}

// Provider registers the enrollment endpoint with the Core Agent API.
type Provider struct {
	Endpoint api.AgentEndpointProvider
}

// NewProvider returns the split-mode enrollment endpoint.
func NewProvider(requires Requires) Provider {
	return Provider{Endpoint: api.NewAgentEndpointProvider(handler(requires.Config, requires.Hostname, enrollAndPersist), route, http.MethodPost)}
}

func handler(cfg config.Component, hostnameComp hostname.Component, enroll enrollAndPersistFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response, err := ensure(r.Context(), cfg, hostnameComp, enroll)
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, "failed to encode enrollment response", http.StatusInternalServerError)
		}
	}
}

func ensure(ctx context.Context, cfg config.Component, hostnameComp hostname.Component, enroll enrollAndPersistFunc) (*Response, error) {
	if !cfg.GetBool(par.PAREnabled) || !cfg.GetBool(par.PARSplitEnabled) {
		return nil, errors.New("Private Action Runner split mode is disabled")
	}
	if cfg.GetBool("fips.enabled") {
		return nil, errors.New("private_action_runner.split_enabled is not supported with fips.enabled")
	}
	if enabled, err := fips.Enabled(); err == nil && enabled {
		return nil, errors.New("private_action_runner.split_enabled is not supported by the FIPS Agent")
	}

	agentID, err := parenrollment.GetAgentIdentifier(ctx, hostnameComp)
	if err != nil {
		return nil, err
	}
	identity, err := parenrollment.GetIdentityFromPreviousEnrollment(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if identity != nil && !parenrollment.ShouldReenroll(agentID, identity, cfg.GetString("api_key")) {
		applyIdentity(cfg, identity)
	} else if cfg.GetString(par.PARUrn) == "" || cfg.GetString(par.PARPrivateKey) == "" {
		if !cfg.GetBool(par.PARSelfEnroll) {
			return nil, errors.New("no Private Action Runner identity is configured and self-enrollment is disabled")
		}
		if _, err := enroll(ctx, cfg, agentID); err != nil {
			return nil, err
		}
		identity, err = parenrollment.GetIdentityFromPreviousEnrollment(ctx, cfg)
		if err != nil {
			return nil, err
		}
		applyIdentity(cfg, identity)
	}

	urn := cfg.GetString(par.PARUrn)
	privateKey := cfg.GetString(par.PARPrivateKey)
	if urn == "" || privateKey == "" {
		return nil, errors.New("the resolved Private Action Runner identity is incomplete")
	}
	parsed, err := parutil.ParseRunnerURN(urn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the Private Action Runner identity: %w", err)
	}
	return &Response{
		URN:          urn,
		PrivateKey:   privateKey,
		OrgID:        parsed.OrgID,
		RunnerID:     parsed.RunnerID,
		AgentVersion: version.AgentVersion,
	}, nil
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
