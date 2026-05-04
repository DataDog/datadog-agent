// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows && kubeapiserver

// Package helm implements the 'cluster-agent helm' subcommands.
package helm

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	helm "github.com/DataDog/datadog-agent/comp/helm/def"
	helmfx "github.com/DataDog/datadog-agent/comp/helm/fx"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type rollbackParams struct {
	releaseName string
	namespace   string
	revision    int
}

// Commands returns 'cluster-agent helm' and its sub-subcommands.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	helmCmd := &cobra.Command{
		Use:   "helm",
		Short: "Manage Helm releases from the cluster-agent",
	}

	helmCmd.AddCommand(rollbackCommand(globalParams))

	return []*cobra.Command{helmCmd}
}

func rollbackCommand(globalParams *command.GlobalParams) *cobra.Command {
	params := &rollbackParams{}

	cmd := &cobra.Command{
		Use:   "rollback RELEASE",
		Short: "Roll back a Helm release to a previous revision",
		Long: `Roll back a Helm release to a previous revision using the cluster-agent's
in-cluster service account. A revision of 0 (the default) rolls back to the
immediately previous successful release.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			params.releaseName = args[0]
			return fxutil.OneShot(runRollback,
				fx.Supply(params),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewClusterAgentParams(globalParams.ConfFilePath),
					LogParams:    log.ForOneShot(command.LoggerName, command.DefaultLogLevel, true),
				}),
				core.Bundle(core.WithSecrets()),
				helmfx.Module(),
			)
		},
	}

	cmd.Flags().StringVar(&params.namespace, "namespace", "default", "Namespace of the release")
	cmd.Flags().IntVar(&params.revision, "revision", 0, "Revision to roll back to (0 means previous)")

	return cmd
}

func runRollback(params *rollbackParams, helmComp helm.Component) error {
	if err := helmComp.Rollback(context.Background(), params.releaseName, params.namespace, params.revision); err != nil {
		return err
	}
	fmt.Printf("Rolled back release %q in namespace %q (revision=%d)\n", params.releaseName, params.namespace, params.revision)
	return nil
}
