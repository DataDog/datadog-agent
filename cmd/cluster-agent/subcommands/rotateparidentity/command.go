// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !windows && kubeapiserver

// Package rotateparidentity implements 'cluster-agent rotate-par-identity'.
package rotateparidentity

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/command"
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
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
)

// Commands returns a slice of subcommands for the 'cluster-agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cmd := &cobra.Command{
		Use:   "rotate-par-identity",
		Short: "Rotate the Private Action Runner identity for this cluster",
		Long: `Generates fresh credentials and registers a new Private Action Runner identity.
The new identity is written to the shared Kubernetes secret. After this command
completes, run a Kubernetes rollout restart of the Cluster Agent deployment to
apply the new identity.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(run,
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewClusterAgentParams(globalParams.ConfFilePath, config.WithExtraConfFiles(globalParams.ExtraConfFilePath)),
					LogParams:    log.ForOneShot(command.LoggerName, command.DefaultLogLevel, true),
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
	if !cfg.GetBool(pkgconfigsetup.PARIdentityUseK8sSecret) {
		return errors.New("private_action_runner.identity_use_k8s_secret is false - set it to true before rotating the shared Kubernetes secret identity")
	}

	// Match the running agent's hostname so ShouldReenroll keeps the rotated identity.
	hostnameVal, err := hostnameComp.Get(ctx)
	if err != nil {
		return fmt.Errorf("failed to get hostname: %w", err)
	}

	// clustername.GetClusterID would call the DCA HTTP client (no cross-node TLS in one-shot).
	apiClient, err := apiserver.GetAPIClient()
	if err != nil {
		return fmt.Errorf("failed to get Kubernetes client: %w", err)
	}
	orchClusterID, err := common.GetOrCreateClusterID(apiClient.Cl.CoreV1())
	if err != nil {
		return fmt.Errorf("failed to get cluster ID: %w", err)
	}

	agentIdentifier := &enrollment.AgentIdentifier{Hostname: hostnameVal, OrchClusterID: orchClusterID}

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
	logger.Info("The new identity has been written to the Kubernetes secret.")
	logger.Info("To apply the new identity, run `kubectl rollout restart deployment/<cluster-agent-deployment> -n <namespace>`.")
	return nil
}
