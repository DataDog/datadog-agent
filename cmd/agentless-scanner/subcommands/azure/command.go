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

	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/agentless/azurebackend"
	"github.com/DataDog/datadog-agent/pkg/agentless/devices"
	"github.com/DataDog/datadog-agent/pkg/agentless/runner"
	"github.com/DataDog/datadog-agent/pkg/agentless/types"

	"github.com/DataDog/datadog-agent/pkg/security/utils"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v4"

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
	cmd.AddCommand(azureOfflineCommand(statsd, sc, evp))

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
			return azureAttachCmd(ctx, sc, resourceID, !localFlags.noMount)
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
			return azureScanCmd(ctx, statsd, sc, evp, resourceID, localFlags.Hostname)
		},
	}
	cmd.Flags().StringVar(&localFlags.Hostname, "hostname", "unknown", "scan hostname")

	return cmd
}

func azureOfflineCommand(statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, evp *eventplatform.Component) *cobra.Command {
	var localFlags struct {
		workers       int
		subscription  string
		resourceGroup string
		taskType      string
		maxScans      int
		printResults  bool
	}
	cmd := &cobra.Command{
		Use:   "offline",
		Short: "Runs the agentless-scanner in offline mode (server-less mode)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := common.CtxTerminated()
			if localFlags.workers <= 0 {
				return fmt.Errorf("workers must be greater than 0")
			}
			taskType, err := types.ParseTaskType(localFlags.taskType)
			if err != nil {
				return err
			}
			return azureOfflineCmd(
				ctx,
				statsd,
				sc,
				evp,
				localFlags.workers,
				taskType,
				localFlags.maxScans,
				localFlags.printResults,
				localFlags.subscription,
				localFlags.resourceGroup)
		},
	}

	cmd.Flags().IntVar(&localFlags.workers, "workers", 40, "number of scans running in parallel")
	cmd.Flags().StringVar(&localFlags.subscription, "subscription", "", "only scan resources in the specified subscription")
	cmd.Flags().StringVar(&localFlags.resourceGroup, "resource-group", "", "only scan resources in the specified resource group")
	cmd.Flags().StringVar(&localFlags.taskType, "task-type", string(types.TaskTypeAzureDisk), fmt.Sprintf("scan type (%s or %s)", types.TaskTypeAzureDisk, types.TaskTypeHost))
	cmd.Flags().IntVar(&localFlags.maxScans, "max-scans", 0, "maximum number of scans to perform")
	cmd.Flags().BoolVar(&localFlags.printResults, "print-results", false, "print scan results to stdout")

	// TODO support scanning all RGs in a subscription
	_ = cmd.MarkFlagRequired("resource-group")
	return cmd
}

func azureAttachCmd(ctx context.Context, sc *types.ScannerConfig, resourceID types.CloudID, mount bool) error {
	scannerHostname := common.TryGetHostname(ctx)
	scannerID := types.ScannerID{Hostname: scannerHostname, Provider: types.CloudProviderAzure}

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

	var waiter azurebackend.ResourceWaiter
	if err := azurebackend.SetupDisk(ctx, cfg, scan, &waiter); err != nil {
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

		if mount {
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

func azureScanCmd(ctx context.Context, statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, evp *eventplatform.Component, resourceID types.CloudID, targetName string) error {
	hostname := common.TryGetHostname(ctx)
	scannerID := types.NewScannerID(types.CloudProviderAzure, hostname)
	taskType, err := types.DefaultTaskType(resourceID)
	if err != nil {
		return err
	}
	task, err := types.NewScanTask(
		taskType,
		resourceID.AsText(),
		scannerID,
		targetName,
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
	scanner.Start(ctx)
	return nil
}

func azureOfflineCmd(ctx context.Context, statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, evp *eventplatform.Component, workers int, taskType types.TaskType, maxScans int, printResults bool, subscription, resourceGroup string) error {
	defer statsd.Flush()

	hostname, err := utils.GetHostnameWithContext(ctx)
	if err != nil {
		return fmt.Errorf("could not fetch hostname: %w", err)
	}

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
		Workers:        workers,
		ScannersMax:    8,
		PrintResults:   printResults,
		Statsd:         statsd,
		EventForwarder: *evp,
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

		pager := vmClient.NewListPager(resourceGroup, nil)
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
				if maxScans > 0 && count >= maxScans {
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
