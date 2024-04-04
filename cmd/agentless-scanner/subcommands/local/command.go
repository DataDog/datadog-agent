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
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/spf13/cobra"
	"go.uber.org/fx"
)

type localScanParams struct {
	path       string
	targetName string
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
			Use:   "scan",
			Short: "Performs a scan on the given path",
			RunE: func(cmd *cobra.Command, args []string) error {
				return fxutil.OneShot(
					localScanCmd,
					common.Bundle(globalParams),
					fx.Supply(&params),
				)
			},
		}

		cmd.Flags().StringVar(&params.path, "path", "", "attch target id")
		cmd.Flags().StringVar(&params.targetName, "target-name", "unknown", "scan target name")
		_ = cmd.MarkFlagRequired("path")

		parent.AddCommand(cmd)
	}

	return []*cobra.Command{parent}
}

func localScanCmd(_ complog.Component, sc *types.ScannerConfig, evp eventplatform.Component, params *localScanParams) error {
	ctx := common.CtxTerminated()
	statsd := common.InitStatsd(*sc)

	hostname := common.TryGetHostname(ctx)
	resourceID, err := types.HumanParseCloudID(params.path, types.CloudProviderNone, "", "")
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
		ScannerID:      scannerID,
		Workers:        1,
		ScannersMax:    8,
		PrintResults:   true,
		Statsd:         statsd,
		EventForwarder: evp,
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
