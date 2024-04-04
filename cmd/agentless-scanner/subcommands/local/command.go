// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package local implements the agentless-scanner local subcommand
package local

import (
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/common"
	"github.com/DataDog/datadog-agent/pkg/agentless/runner"
	"github.com/DataDog/datadog-agent/pkg/agentless/types"

	complog "github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/spf13/cobra"
	"go.uber.org/fx"
)

type localScanParams struct {
	targetName string
	resourceID string
}

// Commands returns the local commands
func Commands(globalParams *common.GlobalParams) []*cobra.Command {
	parent := &cobra.Command{
		Use:          "local",
		Short:        "Datadog Agentless Scanner at your service.",
		Long:         `Datadog Agentless Scanner scans your cloud environment for vulnerabilities, compliance and security issues.`,
		SilenceUsage: true,
	}

	{
		var params localScanParams
		cmd := &cobra.Command{
			Use:   "scan <path>",
			Short: "Performs a scan on the given path",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return fxutil.OneShot(
					localScanCmd,
					common.Bundle(globalParams),
					fx.Provide(func() (*localScanParams, error) {
						params.resourceID = args[0]
						return &params, nil
					}),
				)
			},
		}

		cmd.Flags().StringVar(&params.targetName, "target-name", "unknown", "scan target name")
		parent.AddCommand(cmd)
	}

	return []*cobra.Command{parent}
}

func localScanCmd(_ complog.Component, sc *types.ScannerConfig, params *localScanParams) error {
	ctx := common.CtxTerminated()
	statsd := common.InitStatsd(*sc)

	hostname := common.TryGetHostname(ctx)
	resourceID, err := types.HumanParseCloudID(params.resourceID, types.CloudProviderNone, "", "")
	if err != nil {
		return err
	}

	taskType, err := types.DefaultTaskType(resourceID)
	if err != nil {
		return err
	}
	scannerID := types.NewScannerID(types.CloudProviderNone, hostname)
	task, err := types.NewScanTask(
		taskType,
		resourceID.AsText(),
		scannerID,
		params.targetName,
		nil,
		sc.DefaultActions,
		sc.DefaultRolesMapping,
		sc.DiskMode)
	if err != nil {
		return err
	}

	scanner, err := runner.New(*sc, runner.Options{
		ScannerID:    scannerID,
		DdEnv:        sc.Env,
		Workers:      1,
		ScannersMax:  8,
		PrintResults: true,
		Statsd:       statsd,
	})
	if err != nil {
		return fmt.Errorf("could not initialize agentless-scanner: %w", err)
	}
	go func() {
		scanner.PushConfig(ctx, &types.ScanConfig{
			Type:  types.ConfigTypeAWS,
			Tasks: []*types.ScanTask{task},
		})
		scanner.Stop()
	}()
	scanner.Start(ctx)
	return nil
}
