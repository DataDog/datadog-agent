// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/awsutils"
	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/runner"
	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/types"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"

	"github.com/DataDog/datadog-agent/pkg/security/utils"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/spf13/cobra"
)

const (
	defaultSelfRegion = "us-east-1"
)

func awsGroupCommand(parent *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "aws",
		Short:             "Datadog Agentless Scanner at your service.",
		Long:              `Datadog Agentless Scanner scans your cloud environment for vulnerabilities, compliance and security issues.`,
		SilenceUsage:      true,
		PersistentPreRunE: parent.PersistentPreRunE,
	}
	cmd.AddCommand(awsScanCommand())
	cmd.AddCommand(awsSnapshotCommand())
	cmd.AddCommand(awsOfflineCommand())
	cmd.AddCommand(awsAttachCommand())
	cmd.AddCommand(awsCleanupCommand())
	return cmd
}

func awsScanCommand() *cobra.Command {
	var flags struct {
		Hostname string
	}
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Executes a scan",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			self, err := awsutils.GetSelfEC2InstanceIndentity(context.Background())
			if err != nil {
				return err
			}
			resourceID, err := types.HumanParseCloudID(args[0], types.CloudProviderAWS, self.Region, self.AccountID)
			if err != nil {
				return err
			}
			return awsScanCmd(resourceID, flags.Hostname, globalFlags.defaultActions, globalFlags.diskMode, globalFlags.noForkScanners)
		},
	}

	cmd.Flags().StringVar(&flags.Hostname, "hostname", "unknown", "scan hostname")
	return cmd
}

func awsSnapshotCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Create a snapshot of the given (server-less mode)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := ctxTerminated()
			self, err := awsutils.GetSelfEC2InstanceIndentity(context.Background())
			if err != nil {
				return err
			}
			volumeID, err := types.HumanParseCloudID(args[0], types.CloudProviderAWS, self.Region, self.AccountID)
			if err != nil {
				return err
			}
			roles := getDefaultRolesMapping(types.CloudProviderAWS)
			scan, err := types.NewScanTask(types.TaskTypeEBS, volumeID.AsText(), "unknown", "unknown", globalFlags.defaultActions, roles, globalFlags.diskMode)
			if err != nil {
				return err
			}
			cfg := awsutils.GetConfigFromCloudID(ctx, roles, scan.CloudID)
			var waiter awsutils.SnapshotWaiter
			ec2client := ec2.NewFromConfig(cfg)
			snapshotID, err := awsutils.CreateSnapshot(ctx, scan, &waiter, ec2client, scan.CloudID)
			if err != nil {
				return err
			}
			fmt.Println(snapshotID)
			return nil
		},
	}
	return cmd
}

func awsOfflineCommand() *cobra.Command {
	parseFilters := func(filters string) ([]ec2types.Filter, error) {
		var fs []ec2types.Filter
		if filter := filters; filter != "" {
			if !strings.HasPrefix(filter, "Name=") {
				return nil, fmt.Errorf("bad format for filters: expecting Name=string,Values=string,string")
			}
			filter = filter[len("Name="):]
			split := strings.SplitN(filter, ",", 2)
			if len(split) != 2 {
				return nil, fmt.Errorf("bad format for filters: expecting Name=string,Values=string,string")
			}
			name := split[0]
			filter = split[1]
			if !strings.HasPrefix(filter, "Values=") {
				return nil, fmt.Errorf("bad format for filters: expecting Name=string,Values=string,string")
			}
			filter = filter[len("Values="):]
			values := strings.Split(filter, ",")
			fs = append(fs, ec2types.Filter{
				Name:   aws.String(name),
				Values: values,
			})
		}
		return fs, nil
	}

	var flags struct {
		workers      int
		account      string
		region       string
		filters      string
		taskType     string
		maxScans     int
		printResults bool
	}
	cmd := &cobra.Command{
		Use:   "offline",
		Short: "Runs the agentless-scanner in offline mode (server-less mode)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.workers <= 0 {
				return fmt.Errorf("workers must be greater than 0")
			}
			filters, err := parseFilters(flags.filters)
			if err != nil {
				return err
			}
			taskType, err := types.ParseTaskType(flags.taskType)
			if err != nil {
				return err
			}
			return awsOfflineCmd(
				flags.workers,
				taskType,
				flags.account,
				flags.region,
				flags.maxScans,
				flags.printResults,
				filters,
				globalFlags.diskMode,
				globalFlags.defaultActions,
				globalFlags.noForkScanners)
		},
	}

	cmd.Flags().IntVar(&flags.workers, "workers", 40, "number of scans running in parallel")
	cmd.Flags().StringVar(&flags.account, "account", "auto", "account id (default to the current account)")
	cmd.Flags().StringVar(&flags.region, "regions", "auto", "list of regions to scan (default to the current region)")
	cmd.Flags().StringVar(&flags.filters, "filters", "", "list of filters to filter the resources (format: Name=string,Values=string,string)")
	cmd.Flags().StringVar(&flags.taskType, "task-type", string(types.TaskTypeEBS), fmt.Sprintf("scan type (%s, %s, %s or %s)", types.TaskTypeEBS, types.TaskTypeLambda, types.TaskTypeAMI, types.TaskTypeHost))
	cmd.Flags().IntVar(&flags.maxScans, "max-scans", 0, "maximum number of scans to perform")
	cmd.Flags().BoolVar(&flags.printResults, "print-results", false, "print scan results to stdout")
	return cmd
}

func awsAttachCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "attach <snapshot|volume>",
		Short: "Attaches a snapshot or volume to the current instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			self, err := awsutils.GetSelfEC2InstanceIndentity(context.Background())
			if err != nil {
				return err
			}
			resourceID, err := types.HumanParseCloudID(args[0], types.CloudProviderAWS, self.Region, self.AccountID)
			if err != nil {
				return err
			}
			return awsAttachCmd(resourceID, globalFlags.diskMode, globalFlags.defaultActions)
		},
	}

	return cmd
}

func awsCleanupCommand() *cobra.Command {
	var flags struct {
		region string
		dryRun bool
		delay  time.Duration
	}
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Cleanup resources created by the agentless-scanner",
		RunE: func(cmd *cobra.Command, args []string) error {
			return awsCleanupCmd(flags.region, flags.dryRun, flags.delay)
		},
	}
	cmd.Flags().StringVar(&flags.region, "region", defaultSelfRegion, "AWS region")
	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", false, "dry run")
	cmd.Flags().DurationVar(&flags.delay, "delay", 0, "delete snapshot older than delay")
	return cmd
}

func awsScanCmd(resourceID types.CloudID, targetHostname string, actions []types.ScanAction, diskMode types.DiskMode, noForkScanners bool) error {
	ctx := ctxTerminated()

	ctxhostname, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	hostname, err := utils.GetHostnameWithContext(ctxhostname)
	if err != nil {
		hostname = "unknown"
	}

	taskType, err := types.DefaultTaskType(resourceID)
	if err != nil {
		return err
	}
	roles := getDefaultRolesMapping(types.CloudProviderAWS)
	task, err := types.NewScanTask(taskType, resourceID.AsText(), hostname, targetHostname, actions, roles, diskMode)
	if err != nil {
		return err
	}

	scanner, err := runner.New(runner.Options{
		Hostname:       hostname,
		CloudProvider:  types.CloudProviderAWS,
		DdEnv:          pkgconfig.Datadog.GetString("env"),
		Workers:        1,
		ScannersMax:    8,
		PrintResults:   true,
		NoFork:         noForkScanners,
		DefaultActions: actions,
		DefaultRoles:   roles,
		Statsd:         statsd,
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

func awsOfflineCmd(workers int, taskType types.TaskType, accountID, regionName string, maxScans int, printResults bool, filters []ec2types.Filter, diskMode types.DiskMode, actions []types.ScanAction, noForkScanners bool) error {
	ctx := ctxTerminated()
	defer statsd.Flush()

	hostname, err := utils.GetHostnameWithContext(ctx)
	if err != nil {
		return fmt.Errorf("could not fetch hostname: %w", err)
	}

	self, err := awsutils.GetSelfEC2InstanceIndentity(ctx)
	if err != nil {
		return err
	}

	roles := getDefaultRolesMapping(types.CloudProviderAWS)
	if regionName == "auto" {
		regionName = self.Region
	}
	if accountID == "auto" {
		accountID = self.AccountID
	}

	scanner, err := runner.New(runner.Options{
		Hostname:       hostname,
		CloudProvider:  types.CloudProviderAWS,
		DdEnv:          pkgconfig.Datadog.GetString("env"),
		Workers:        workers,
		ScannersMax:    8,
		PrintResults:   printResults,
		NoFork:         noForkScanners,
		DefaultActions: actions,
		DefaultRoles:   roles,
		Statsd:         statsd,
	})
	if err != nil {
		return fmt.Errorf("could not initialize agentless-scanner: %w", err)
	}
	if err := scanner.CleanSlate(); err != nil {
		log.Error(err)
	}

	pushEBSVolumes := func() error {
		ec2client := ec2.NewFromConfig(awsutils.GetConfig(ctx, regionName, roles.GetRole(accountID)))
		if err != nil {
			return err
		}
		describeInstancesInput := &ec2.DescribeInstancesInput{
			Filters: append([]ec2types.Filter{
				{
					Name:   aws.String("instance-state-name"),
					Values: []string{string(ec2types.InstanceStateNameRunning)},
				},
			}, filters...),
		}
		count := 0
		for {
			instances, err := ec2client.DescribeInstances(ctx, describeInstancesInput)
			if err != nil {
				return fmt.Errorf("could not scan region %q for EBS volumes: %w", regionName, err)
			}
			for _, reservation := range instances.Reservations {
				for _, instance := range reservation.Instances {
					if instance.InstanceId == nil {
						continue
					}
					if instance.State.Name != ec2types.InstanceStateNameRunning {
						continue
					}
					for _, blockDeviceMapping := range instance.BlockDeviceMappings {
						if blockDeviceMapping.DeviceName == nil {
							continue
						}
						if blockDeviceMapping.Ebs == nil {
							continue
						}
						if *blockDeviceMapping.DeviceName != *instance.RootDeviceName {
							continue
						}
						if instance.Architecture == ec2types.ArchitectureValuesX8664Mac || instance.Architecture == ec2types.ArchitectureValuesArm64Mac {
							// Exclude macOS.
							continue
						}
						if instance.Platform == "windows" {
							// ec2types.PlatformValuesWindows incorrectly spells "Windows".
							// Exclude Windows.
							continue
						}
						if instance.PlatformDetails != nil && strings.HasPrefix(*instance.PlatformDetails, "Windows") {
							// Exclude Windows.
							continue
						}
						volumeID, err := types.AWSCloudID("ec2", regionName, accountID, types.ResourceTypeVolume, *blockDeviceMapping.Ebs.VolumeId)
						if err != nil {
							return err
						}
						log.Debugf("%s %s %s %s %s", regionName, *instance.InstanceId, volumeID, *blockDeviceMapping.DeviceName, *instance.PlatformDetails)
						scan, err := types.NewScanTask(types.TaskTypeEBS, volumeID.AsText(), hostname, *instance.InstanceId, actions, roles, diskMode)
						if err != nil {
							return err
						}

						config := &types.ScanConfig{Type: types.ConfigTypeAWS, Tasks: []*types.ScanTask{scan}, Roles: roles}
						if !scanner.PushConfig(ctx, config) {
							return nil
						}
						count++
						if maxScans > 0 && count >= maxScans {
							return nil
						}
					}
				}
			}
			if instances.NextToken == nil {
				break
			}
			describeInstancesInput.NextToken = instances.NextToken
		}
		return nil
	}

	pushAMI := func() error {
		count := 0
		ec2client := ec2.NewFromConfig(awsutils.GetConfig(ctx, regionName, roles.GetRole(accountID)))
		if err != nil {
			return err
		}
		images, err := ec2client.DescribeImages(ctx, &ec2.DescribeImagesInput{
			Owners:  []string{"self"},
			Filters: filters,
		})
		if err != nil {
			return fmt.Errorf("could not scan region %q for AMIs: %w", regionName, err)
		}
		for _, image := range images.Images {
			if image.ImageId == nil {
				continue
			}
			for _, blockDeviceMapping := range image.BlockDeviceMappings {
				if blockDeviceMapping.DeviceName == nil {
					continue
				}
				if blockDeviceMapping.Ebs == nil {
					continue
				}
				if *blockDeviceMapping.DeviceName != *image.RootDeviceName {
					continue
				}
				snapshotID, err := types.AWSCloudID("ec2", regionName, accountID, types.ResourceTypeSnapshot, *blockDeviceMapping.Ebs.SnapshotId)
				if err != nil {
					return err
				}
				log.Debugf("%s %s %s %s", regionName, *image.ImageId, snapshotID, *blockDeviceMapping.DeviceName)
				scan, err := types.NewScanTask(types.TaskTypeAMI, snapshotID.AsText(), hostname, *image.ImageId, actions, roles, diskMode)
				if err != nil {
					return err
				}
				config := &types.ScanConfig{Type: types.ConfigTypeAWS, Tasks: []*types.ScanTask{scan}, Roles: roles}
				if !scanner.PushConfig(ctx, config) {
					return nil
				}
				count++
				if maxScans > 0 && count >= maxScans {
					return nil
				}
				break
			}
		}
		return nil
	}

	pushLambdaFunctions := func() error {
		lambdaclient := lambda.NewFromConfig(awsutils.GetConfig(ctx, regionName, roles.GetRole(accountID)))
		var marker *string
		count := 0
		for {
			functions, err := lambdaclient.ListFunctions(ctx, &lambda.ListFunctionsInput{
				Marker: marker,
			})
			if err != nil {
				return fmt.Errorf("could not scan region %q for Lambda functions: %w", regionName, err)
			}
			for _, function := range functions.Functions {
				scan, err := types.NewScanTask(types.TaskTypeLambda, *function.FunctionArn, hostname, "", actions, roles, diskMode)
				if err != nil {
					return fmt.Errorf("could not create scan for lambda %s: %w", *function.FunctionArn, err)
				}
				config := &types.ScanConfig{Type: types.ConfigTypeAWS, Tasks: []*types.ScanTask{scan}, Roles: roles}
				if !scanner.PushConfig(ctx, config) {
					return nil
				}
				count++
				if maxScans > 0 && count >= maxScans {
					return nil
				}
			}
			marker = functions.NextMarker
			if marker == nil {
				break
			}
		}
		return nil
	}

	go func() {
		defer scanner.Stop()
		var err error
		switch taskType {
		case types.TaskTypeEBS:
			err = pushEBSVolumes()
		case types.TaskTypeAMI:
			err = pushAMI()
		case types.TaskTypeLambda:
			err = pushLambdaFunctions()
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

func awsCleanupCmd(region string, dryRun bool, delay time.Duration) error {
	ctx := ctxTerminated()

	defaultCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return err
	}
	stsclient := sts.NewFromConfig(defaultCfg)
	identity, err := stsclient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return err
	}

	assumedRole := getDefaultRolesMapping(types.CloudProviderAWS).GetRole(*identity.Account)
	toBeDeleted, err := awsutils.ListResourcesForCleanup(ctx, delay, region, assumedRole)
	if err != nil {
		return err
	}
	if len(toBeDeleted) == 0 {
		fmt.Printf("no resources found to cleanup\n")
		return nil
	}
	fmt.Printf("cleaning up these resources:\n")
	for _, resourceID := range toBeDeleted {
		fmt.Printf(" - %s\n", resourceID)
	}
	if !dryRun {
		awsutils.ResourcesCleanup(ctx, toBeDeleted, region, assumedRole)
	}
	return nil
}

func awsAttachCmd(resourceID types.CloudID, mode types.DiskMode, defaultActions []types.ScanAction) error {
	ctx := ctxTerminated()

	ctxhostname, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	hostname, err := utils.GetHostnameWithContext(ctxhostname)
	if err != nil {
		hostname = "unknown"
	}

	roles := getDefaultRolesMapping(types.CloudProviderAWS)
	scan, err := types.NewScanTask(types.TaskTypeEBS, resourceID.AsText(), hostname, resourceID.ResourceName(), defaultActions, roles, mode)
	if err != nil {
		return err
	}
	defer func() {
		ctxcleanup, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		for resourceID := range scan.CreatedResources {
			if err := awsutils.CleanupScanEBS(ctxcleanup, scan, resourceID); err != nil {
				log.Errorf("%s: could not cleanup resource %q: %v", scan, resourceID, err)
			}
		}
	}()

	var waiter awsutils.SnapshotWaiter
	mountpoints, err := awsutils.SetupEBS(ctx, scan, &waiter)
	if err != nil {
		return err
	}

	for _, mountpoint := range mountpoints {
		fmt.Println(mountpoint)
	}
	<-ctx.Done()
	return nil
}

func ctxTerminated() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case sig := <-ch:
			fmt.Fprintf(os.Stderr, "received %s signal\n", sig)
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx
}
