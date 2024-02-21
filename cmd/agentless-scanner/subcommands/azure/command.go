// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package azure implements the agentless-scanner azure subcommand
package azure

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/common"
	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/flags"

	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/agentless/azurebackend"
	"github.com/DataDog/datadog-agent/pkg/agentless/runner"
	"github.com/DataDog/datadog-agent/pkg/agentless/types"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	ddogstatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/spf13/cobra"
)

// GroupCommand returns the Azure commands
func GroupCommand(parent *cobra.Command, statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, evp *eventplatform.Component) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "azure",
		Short:             "Datadog Agentless Scanner at your service.",
		Long:              `Datadog Agentless Scanner scans your cloud environment for vulnerabilities, compliance and security issues.`,
		SilenceUsage:      true,
		PersistentPreRunE: parent.PersistentPreRunE,
	}
	cmd.AddCommand(azureAttachCommand(sc))
	cmd.AddCommand(azureScanCommand(statsd, sc, evp))

	return cmd
}

func azureAttachCommand(sc *types.ScannerConfig) *cobra.Command {
	var localFlags struct {
		noMount bool
	}
	cmd := &cobra.Command{
		Use:   "attach <snapshot|volume>",
		Short: "Attaches a snapshot or volume to the current instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := common.CtxTerminated()
			self, err := azurebackend.GetInstanceMetadata(context.Background())
			if err != nil {
				return err
			}
			resourceID, err := types.HumanParseCloudID(args[0], types.CloudProviderAzure, self.Compute.Location, self.Compute.SubscriptionID)
			if err != nil {
				return err
			}
			return azureAttachCmd(ctx, sc, resourceID, flags.GlobalFlags.DiskMode, flags.GlobalFlags.DefaultActions)
		},
	}
	cmd.Flags().BoolVar(&localFlags.noMount, "no-mount", false, "mount the device")
	return cmd
}

func azureScanCommand(statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, evp *eventplatform.Component) *cobra.Command {
	var localFlags struct {
		Hostname string
		Region   string
	}
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Executes a scan",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := common.CtxTerminated()
			self, err := azurebackend.GetInstanceMetadata(context.Background())
			if err != nil {
				return err
			}
			resourceID, err := types.HumanParseCloudID(args[0], types.CloudProviderAzure, self.Compute.Location, self.Compute.SubscriptionID)
			if err != nil {
				return err
			}
			return azureScanCmd(ctx, statsd, sc, evp, resourceID, localFlags.Hostname, flags.GlobalFlags.DefaultActions, flags.GlobalFlags.DiskMode, flags.GlobalFlags.NoForkScanners)
		},
	}
	cmd.Flags().StringVar(&localFlags.Hostname, "hostname", "unknown", "scan hostname")

	return cmd
}

func azureAttachCmd(ctx context.Context, sc *types.ScannerConfig, resourceID types.CloudID, diskMode types.DiskMode, defaultActions []types.ScanAction) error {
	scannerHostname := common.TryGetHostname(ctx)
	scannerID := types.ScannerID{Hostname: scannerHostname, Provider: types.CloudProviderAzure}

	roles := common.GetDefaultRolesMapping(sc, types.CloudProviderAzure)
	cfg, err := azurebackend.GetConfigFromCloudID(ctx, resourceID)
	if err != nil {
		return err
	}
	scan, err := types.NewScanTask(
		types.TaskTypeAzureDisk,
		resourceID.AsText(),
		scannerID,
		resourceID.ResourceName(),
		nil,
		defaultActions,
		roles,
		diskMode)
	if err != nil {
		return err
	}

	defer func() {
		ctxCleanup, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		runner.CleanupScanDir(ctxCleanup, scan)
		for resourceID := range scan.CreatedResources {
			log.Debugf("Cleaning up resource %q\n", resourceID)
			if err := azurebackend.CleanupScan(ctxCleanup, cfg, scan, resourceID); err != nil {
				log.Errorf("%s: could not cleanup resource %q: %v", scan, resourceID, err)
			}
		}
	}()

	log.Infof("Setting up disk %s\n", scan.TargetID)

	var waiter azurebackend.ResourceWaiter
	if err := azurebackend.SetupDisk(ctx, cfg, scan, &waiter); err != nil {
		return err
	}

	log.Infof("Set up disk on NBD device %v\n", *scan.AttachedDeviceName)

	<-ctx.Done()

	return nil
}

func azureScanCmd(ctx context.Context, statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, evp *eventplatform.Component, resourceID types.CloudID, targetName string, actions []types.ScanAction, diskMode types.DiskMode, noForkScanners bool) error {
	hostname := common.TryGetHostname(ctx)
	scannerID := types.NewScannerID(types.CloudProviderAzure, hostname)
	taskType, err := types.DefaultTaskType(resourceID)
	if err != nil {
		return err
	}
	roles := common.GetDefaultRolesMapping(sc, types.CloudProviderAzure)
	task, err := types.NewScanTask(
		taskType,
		resourceID.AsText(),
		scannerID,
		targetName,
		nil,
		actions,
		roles,
		diskMode)
	if err != nil {
		return err
	}

	scanner, err := runner.New(runner.Options{
		ScannerID:      scannerID,
		DdEnv:          sc.Env,
		Workers:        1,
		ScannersMax:    8,
		PrintResults:   true,
		NoFork:         noForkScanners,
		DefaultActions: actions,
		DefaultRoles:   roles,
		Statsd:         statsd,
		EventForwarder: *evp,
	})
	if err != nil {
		return fmt.Errorf("could not initialize agentless-scanner: %w", err)
	}
	go func() {
		scanner.PushConfig(ctx, &types.ScanConfig{
			Type:  types.ConfigTypeAzure,
			Tasks: []*types.ScanTask{task},
		})
		scanner.Stop()
	}()
	scanner.Start(ctx, statsd, sc)
	return nil
}
