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
	"github.com/DataDog/datadog-agent/pkg/agentless/azurebackend"
	"github.com/DataDog/datadog-agent/pkg/agentless/devices"
	"github.com/DataDog/datadog-agent/pkg/agentless/runner"
	"github.com/DataDog/datadog-agent/pkg/agentless/types"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v4"

	complog "github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/spf13/cobra"
	"go.uber.org/fx"
)

type azureAttachParams struct {
	resourceID string
	noMount    bool
}

type azureScanParams struct {
	resourceID string
	targetName string
}

type azureOfflineParams struct {
	workers       int
	subscription  string
	resourceGroup string
	taskType      string
	maxScans      int
	printResults  bool
}

// Commands returns the Azure commands
func Commands() []*cobra.Command {
	parent := &cobra.Command{
		Use:          "azure",
		Short:        "Datadog Agentless Scanner at your service.",
		Long:         `Datadog Agentless Scanner scans your cloud environment for vulnerabilities, compliance and security issues.`,
		SilenceUsage: true,
	}

	{
		var params azureAttachParams
		cmd := &cobra.Command{
			Use:   "attach <snapshot|disk>",
			Short: "Attaches a snapshot or disk to the current instance",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return fxutil.OneShot(
					azureAttachCmd,
					fx.Provide(func() *azureAttachParams {
						params.resourceID = args[0]
						return &params
					}), common.Bundle())
			},
		}
		cmd.Flags().BoolVar(&params.noMount, "no-mount", false, "mount the device")
		parent.AddCommand(cmd)
	}

	{
		var params azureScanParams
		cmd := &cobra.Command{
			Use:   "scan <snapshot|disk>",
			Short: "Performs a scan on the given resource",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return fxutil.OneShot(
					azureScanCmd,
					common.Bundle(),
					fx.Provide(func() *azureScanParams {
						params.resourceID = args[0]
						return &params
					}),
				)
			},
		}
		cmd.Flags().StringVar(&params.targetName, "target-name", "unknown", "scan target name")

		parent.AddCommand(cmd)
	}

	{
		var params azureOfflineParams
		cmd := &cobra.Command{
			Use:   "offline",
			Short: "Runs the agentless-scanner in offline mode (server-less mode)",
			RunE: func(cmd *cobra.Command, args []string) error {
				return fxutil.OneShot(
					azureOfflineCmd,
					fx.Supply(&params),
					common.Bundle())
			},
		}

		cmd.Flags().IntVar(&params.workers, "workers", 40, "number of scans running in parallel")
		cmd.Flags().StringVar(&params.subscription, "subscription", "", "only scan resources in the specified subscription")
		cmd.Flags().StringVar(&params.resourceGroup, "resource-group", "", "only scan resources in the specified resource group")
		cmd.Flags().StringVar(&params.taskType, "task-type", string(types.TaskTypeAzureDisk), fmt.Sprintf("scan type (%s or %s)", types.TaskTypeAzureDisk, types.TaskTypeHost))
		cmd.Flags().IntVar(&params.maxScans, "max-scans", 0, "maximum number of scans to perform")
		cmd.Flags().BoolVar(&params.printResults, "print-results", false, "print scan results to stdout")

		// TODO support scanning all RGs in a subscription
		_ = cmd.MarkFlagRequired("resource-group")
		parent.AddCommand(cmd)
	}

	return []*cobra.Command{parent}
}

func azureAttachCmd(_ complog.Component, sc *types.ScannerConfig, params *azureAttachParams) error {
	ctx := common.CtxTerminated()
	scannerHostname := common.TryGetHostname(ctx)
	scannerID := types.ScannerID{Hostname: scannerHostname, Provider: types.CloudProviderAzure}

	self, err := azurebackend.GetInstanceMetadata(ctx)
	if err != nil {
		return err
	}
	resourceID, err := types.HumanParseCloudID(params.resourceID, types.CloudProviderAzure, self.Compute.Location, self.Compute.SubscriptionID)
	if err != nil {
		return err
	}
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
		sc.DefaultActions,
		sc.DefaultRolesMapping,
		sc.DiskMode)
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

	if err := azurebackend.SetupDisk(ctx, cfg, scan); err != nil {
		return err
	}

	partitions, err := devices.ListPartitions(ctx, scan, *scan.AttachedDeviceName)
	if err != nil {
		return err
	}

	if len(partitions) > 0 {
		for _, part := range partitions {
			fmt.Printf("partition\t%s\t%s\n", part.DevicePath, part.FSType)
		}

		if !params.noMount {
			mountpoints, err := devices.Mount(ctx, scan, partitions)
			if err != nil {
				return err
			}
			for _, mountpoint := range mountpoints {
				fmt.Printf("mountpoint\t%s\n", mountpoint)
			}
		}
	} else {
		fmt.Printf("no compatible partition found on %s\n", *scan.AttachedDeviceName)
	}

	fmt.Println("Ctrl+C to detach the device")
	<-ctx.Done()
	return nil
}

func azureScanCmd(_ complog.Component, sc *types.ScannerConfig, evp eventplatform.Component, params *azureScanParams) error {
	ctx := common.CtxTerminated()
	statsd := common.InitStatsd(*sc)
	hostname := common.TryGetHostname(ctx)
	scannerID := types.NewScannerID(types.CloudProviderAzure, hostname)
	self, err := azurebackend.GetInstanceMetadata(ctx)
	if err != nil {
		return err
	}
	resourceID, err := types.HumanParseCloudID(params.resourceID, types.CloudProviderAzure, self.Compute.Location, self.Compute.SubscriptionID)
	if err != nil {
		return err
	}
	taskType, err := types.DefaultTaskType(resourceID)
	if err != nil {
		return err
	}
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
		DdEnv:          sc.Env,
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
			Type:  types.ConfigTypeAzure,
			Tasks: []*types.ScanTask{task},
		})
		scanner.Stop()
	}()
	scanner.Start(ctx)
	return nil
}

func azureOfflineCmd(_ complog.Component, sc *types.ScannerConfig, evp eventplatform.Component, params *azureOfflineParams) error {
	ctx := common.CtxTerminated()
	statsd := common.InitStatsd(*sc)
	hostname := common.TryGetHostname(ctx)
	defer statsd.Flush()

	taskType, err := types.ParseTaskType(params.taskType)
	if err != nil {
		return err
	}

	subscription := params.subscription
	if len(subscription) == 0 {
		self, err := azurebackend.GetInstanceMetadata(ctx)
		if err != nil {
			return err
		}
		subscription = self.Compute.SubscriptionID
	}

	scannerID := types.NewScannerID(types.CloudProviderAzure, hostname)
	scanner, err := runner.New(*sc, runner.Options{
		ScannerID:      scannerID,
		DdEnv:          sc.Env,
		Workers:        params.workers,
		ScannersMax:    8,
		PrintResults:   params.printResults,
		Statsd:         statsd,
		EventForwarder: evp,
	})
	if err != nil {
		return fmt.Errorf("could not initialize agentless-scanner: %w", err)
	}
	if err := scanner.CleanSlate(); err != nil {
		log.Error(err)
	}

	pushDisks := func() error {
		config, err := azurebackend.GetConfig(ctx, subscription)
		if err != nil {
			return err
		}
		vmClient := config.ComputeClientFactory.NewVirtualMachinesClient()

		pager := vmClient.NewListPager(params.resourceGroup, nil)
		count := 0
		for pager.More() {
			nextResult, err := pager.NextPage(ctx)
			if err != nil {
				return fmt.Errorf("could not scan subscription %q for disks: %w", subscription, err)
			}

			if nextResult.Value == nil {
				continue
			}
			for _, vm := range nextResult.Value {
				// TODO use InstanceView to check if VM is running
				//if instance.State.Name != ec2types.InstanceStateNameRunning { continue }

				if *vm.Properties.StorageProfile.OSDisk.OSType != armcompute.OperatingSystemTypesLinux {
					log.Debugf("Skipping %s VM: %s", *vm.Properties.StorageProfile.OSDisk.OSType, *vm.ID)
					continue
				}

				diskID, err := types.ParseAzureResourceID(*vm.Properties.StorageProfile.OSDisk.ManagedDisk.ID)
				if err != nil {
					return err
				}
				log.Debugf("%v %s %s", *vm.Location, *vm.Name, diskID)
				scan, err := types.NewScanTask(
					types.TaskTypeAzureDisk,
					diskID.AsText(),
					scannerID,
					*vm.ID,
					nil, //ec2TagsToStringTags(instance.Tags),
					sc.DefaultActions,
					sc.DefaultRolesMapping,
					sc.DiskMode)
				if err != nil {
					return err
				}

				if !scanner.PushConfig(ctx, &types.ScanConfig{
					Type:  types.ConfigTypeAzure,
					Tasks: []*types.ScanTask{scan},
					Roles: sc.DefaultRolesMapping,
				}) {
					return nil
				}
				count++
				if params.maxScans > 0 && count >= params.maxScans {
					return nil
				}
			}
		}
		return nil
	}

	go func() {
		defer scanner.Stop()
		var err error
		switch taskType {
		case types.TaskTypeAzureDisk:
			err = pushDisks()
		default:
			panic("unreachable")
		}
		if err != nil {
			log.Error(err)
		}
	}()

	scanner.Start(ctx)
	return nil
}
