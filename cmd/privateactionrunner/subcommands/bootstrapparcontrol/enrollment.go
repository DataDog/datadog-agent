// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package bootstrapparcontrol

import (
	"context"
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	par "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/autoconnections"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	parutil "github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
)

type enrollAndPersistFunc func(context.Context, config.Component, *enrollment.AgentIdentifier) (*enrollment.Result, error)

func ensureEnrollment(ctx context.Context, cfg config.Component, hostnameComp hostname.Component, enroll enrollAndPersistFunc) error {
	agentID, err := enrollment.GetAgentIdentifier(ctx, hostnameComp)
	if err != nil {
		return err
	}

	identity, err := enrollment.GetIdentityFromPreviousEnrollment(ctx, cfg)
	if err != nil {
		return err
	}
	if identity != nil && !enrollment.ShouldReenroll(agentID, identity) {
		applyIdentity(cfg, identity)
		return nil
	}

	if cfg.GetString(par.PARUrn) != "" && cfg.GetString(par.PARPrivateKey) != "" {
		return nil
	}
	if !cfg.GetBool(par.PARSelfEnroll) {
		return errors.New("no Private Action Runner identity is configured and self-enrollment is disabled")
	}

	if _, err := enroll(ctx, cfg, agentID); err != nil {
		return err
	}
	identity, err = enrollment.GetIdentityFromPreviousEnrollment(ctx, cfg)
	if err != nil {
		return err
	}
	applyIdentity(cfg, identity)
	return nil
}

func applyIdentity(cfg config.Component, identity *enrollment.PersistedIdentity) {
	if identity == nil {
		return
	}
	cfg.Set(par.PARUrn, identity.URN, model.SourceAgentRuntime)
	cfg.Set(par.PARPrivateKey, identity.PrivateKey, model.SourceAgentRuntime)
}

func enrollAndPersist(ctx context.Context, cfg config.Component, agentID *enrollment.AgentIdentifier) (*enrollment.Result, error) {
	result, err := enrollment.Enroll(ctx, cfg, agentID)
	if err != nil {
		return nil, fmt.Errorf("enrollment failed: %w", err)
	}
	if err := enrollment.RotateIdentity(ctx, cfg, result); err != nil {
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
