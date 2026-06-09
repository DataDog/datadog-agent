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
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
)

// Commands returns a slice of subcommands for the 'cluster-agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cmd := &cobra.Command{
		Use:   "rotate-par-identity",
		Short: "Rotate the Private Action Runner identity for this cluster",
		Long: `Generates fresh credentials and registers a new Private Action Runner identity.
The new identity is written to the shared Kubernetes secret. Running cluster agent
replicas will detect the change and reload their PAR connection automatically.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(run,
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewClusterAgentParams(globalParams.ConfFilePath),
					LogParams:    log.ForOneShot(command.LoggerName, command.DefaultLogLevel, true),
				}),
				core.Bundle(core.WithSecrets()),
			)
		},
	}
	return []*cobra.Command{cmd}
}

func run(_ log.Component, cfg config.Component) error {
	ctx := context.Background()

	if !cfg.GetBool(pkgconfigsetup.PAREnabled) {
		return errors.New("private_action_runner.enabled is false — set it to true before rotating the identity")
	}

	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("failed to get hostname: %w", err)
	}
	orchClusterID, err := clustername.GetClusterID()
	if err != nil || orchClusterID == "" {
		return fmt.Errorf("failed to get cluster ID: %w", err)
	}
	agentIdentifier := &enrollment.AgentIdentifier{Hostname: hostname, OrchClusterID: orchClusterID}

	result, err := enrollment.Enroll(ctx, cfg, agentIdentifier)
	if err != nil {
		return fmt.Errorf("enrollment failed: %w", err)
	}

	if err := enrollment.RotateIdentity(ctx, cfg, result); err != nil {
		return fmt.Errorf("failed to persist new identity: %w", err)
	}

	fmt.Printf("Identity successfully rotated. New URN: %s\n", result.URN)
	fmt.Println("The new identity has been written to the Kubernetes secret.")
	fmt.Println("Running cluster agent replicas will detect the change and reload automatically.")
	return nil
}
