// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package identity contains enrollment operations shared by identity subcommands.
package identity

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/autoconnections"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	parutil "github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
)

// EnrollAndPersist enrolls a new identity, persists it, and creates any enabled
// automatic connections. The identity is persisted before best-effort connection
// creation so a connection failure cannot discard valid enrollment credentials.
func EnrollAndPersist(ctx context.Context, logger log.Component, cfg config.Component, agentIdentifier *enrollment.AgentIdentifier) (*enrollment.Result, error) {
	result, err := enrollment.Enroll(ctx, cfg, agentIdentifier)
	if err != nil {
		return nil, fmt.Errorf("enrollment failed: %w", err)
	}

	if err := enrollment.RotateIdentity(ctx, cfg, result); err != nil {
		return nil, fmt.Errorf("failed to persist new identity: %w", err)
	}

	// nil metrics client: identity commands emit no metrics.
	parCfg, err := parconfig.FromDDConfig(cfg, nil)
	if err != nil {
		logger.Warnf("Identity persisted, but failed to load runner config for auto-connection: %v", err)
	} else if urnParts, err := parutil.ParseRunnerURN(result.URN); err != nil {
		logger.Warnf("Identity persisted, but failed to parse URN for auto-connection: %v", err)
	} else {
		autoconnections.CreateConnectionsIfEnabled(
			ctx, cfg, parCfg,
			cfg.GetString("api_key"), cfg.GetString("app_key"), urnParts.RunnerID,
			result, autoconnections.NewBasicTagsProvider(),
		)
	}

	return result, nil
}
