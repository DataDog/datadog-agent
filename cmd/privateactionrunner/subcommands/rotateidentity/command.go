// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package rotateidentity implements the 'rotate-identity' subcommand for the private-action-runner.
package rotateidentity

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/privateactionrunner/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/autoconnections"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	parutil "github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Commands returns a slice of subcommands for the 'private-action-runner' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cmd := &cobra.Command{
		Use:   "rotate-identity",
		Short: "Rotate the Private Action Runner identity by performing a new enrollment",
		Long: `Generates fresh credentials and registers a new Private Action Runner identity.
The new identity is persisted to the configured storage (file or Kubernetes secret).
Restart the Private Action Runner process to apply the new identity.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(run,
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithExtraConfFiles(globalParams.ExtraConfFilePath)),
					LogParams:    log.ForOneShot(command.LoggerName, "info", true),
				}),
				core.Bundle(core.WithSecrets()),
				hostnameimpl.Module(),
			)
		},
	}
	return []*cobra.Command{cmd}
}

func run(logger log.Component, cfg config.Component, hostnameComp hostname.Component) error {
	ctx := context.Background()

	if !cfg.GetBool(pkgconfigsetup.PAREnabled) {
		return errors.New("private_action_runner.enabled is false - set it to true before rotating the identity")
	}

	// Match the running agent's hostname so ShouldReenroll keeps the rotated identity.
	agentIdentifier, err := enrollment.GetAgentIdentifier(ctx, hostnameComp)
	if err != nil {
		return fmt.Errorf("failed to get agent identifier: %w", err)
	}

	result, err := enrollment.Enroll(ctx, cfg, agentIdentifier)
	if err != nil {
		return fmt.Errorf("enrollment failed: %w", err)
	}

	if err := enrollment.RotateIdentity(ctx, cfg, result); err != nil {
		return fmt.Errorf("failed to persist new identity: %w", err)
	}

	// nil metrics client: identity rotation emits no metrics.
	parCfg, err := parconfig.FromDDConfig(cfg, nil)
	if err != nil {
		logger.Warnf("Identity rotated, but failed to load runner config for auto-connection: %v", err)
	} else if urnParts, err := parutil.ParseRunnerURN(result.URN); err != nil {
		logger.Warnf("Identity rotated, but failed to parse URN for auto-connection: %v", err)
	} else {
		autoconnections.CreateConnectionsIfEnabled(
			ctx, cfg, parCfg,
			cfg.GetString("api_key"), cfg.GetString("app_key"), urnParts.RunnerID,
			result, autoconnections.NewBasicTagsProvider(),
		)
	}

	logger.Infof("Identity successfully rotated. New URN: %s", result.URN)
	logger.Info("Restart the Private Action Runner to apply the new identity.")
	return nil
}
