// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

// Package helm implements 'cluster-agent helm' subcommands.
package helm

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/helmactions"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

const (
	kubeClientTimeout = 30 * time.Second
	kubeClientQPS     = 5
	kubeClientBurst   = 10
)

type rollbackCliParams struct {
	release          string
	releaseNamespace string
	revision         int
	jobNamespace     string
	serviceAccount   string
	image            string
	driver           string
}

// Commands returns the 'helm' command tree for the cluster-agent CLI.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	helmCmd := &cobra.Command{
		Use:   "helm",
		Short: "Trigger Helm operations on the cluster",
	}
	helmCmd.AddCommand(rollbackCmd(globalParams))
	return []*cobra.Command{helmCmd}
}

func rollbackCmd(globalParams *command.GlobalParams) *cobra.Command {
	params := &rollbackCliParams{}

	cmd := &cobra.Command{
		Use:   "rollback",
		Short: "Roll a Helm release back to a previous revision via a Kubernetes Job",
		Long: `Creates a short-lived Kubernetes Job that runs ` + "`helm rollback`" + ` against the
specified release. The Job runs in --job-namespace as --service-account, which
must already have the RBAC permissions helm needs to act on the release.`,
		Example: "datadog-cluster-agent helm rollback --release myrel --namespace prod " +
			"--job-namespace ops --service-account helm-sa --revision 3",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(runRollback,
				fx.Supply(params),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewClusterAgentParams(globalParams.ConfFilePath),
					LogParams:    log.ForOneShot(command.LoggerName, command.DefaultLogLevel, true),
				}),
				core.Bundle(),
			)
		},
	}

	cmd.Flags().StringVar(&params.release, "release", "", "Name of the Helm release to roll back (required)")
	cmd.Flags().StringVar(&params.releaseNamespace, "namespace", "", "Namespace of the Helm release (required)")
	cmd.Flags().IntVar(&params.revision, "revision", 0, "Target revision number; 0 (default) means previous revision")
	cmd.Flags().StringVar(&params.jobNamespace, "job-namespace", "", "Namespace where the rollback Job is created (required)")
	cmd.Flags().StringVar(&params.serviceAccount, "service-account", "", "ServiceAccount the rollback Job runs as (required)")
	cmd.Flags().StringVar(&params.image, "image", "", "Helm container image (default: "+helmactions.DefaultHelmImage+")")
	cmd.Flags().StringVar(&params.driver, "driver", "", "Helm storage driver (secret|configmap|sql); empty inherits helm's default (secret)")

	for _, name := range []string{"release", "namespace", "job-namespace", "service-account"} {
		_ = cmd.MarkFlagRequired(name)
	}

	return cmd
}

func runRollback(_ log.Component, _ config.Component, params *rollbackCliParams) error {
	client, err := apiserver.GetKubeClient(kubeClientTimeout, kubeClientQPS, kubeClientBurst)
	if err != nil {
		return fmt.Errorf("create kubernetes client: %w", err)
	}

	executor := helmactions.NewRollbackExecutor(client)
	job, err := executor.Run(context.Background(), helmactions.RollbackOptions{
		Release:            params.release,
		ReleaseNamespace:   params.releaseNamespace,
		Revision:           params.revision,
		JobNamespace:       params.jobNamespace,
		ServiceAccountName: params.serviceAccount,
		Image:              params.image,
		Driver:             params.driver,
	})
	if err != nil {
		return err
	}

	fmt.Printf("Created helm rollback job %s/%s for release %s/%s (revision=%d)\n",
		job.Namespace, job.Name, params.releaseNamespace, params.release, params.revision)
	return nil
}
