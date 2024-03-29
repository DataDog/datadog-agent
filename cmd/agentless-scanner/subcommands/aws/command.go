// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package aws implements the agentless-scanner aws subcommand
package aws

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/common"
	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/flags"

	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/agentless/awsbackend"
	"github.com/DataDog/datadog-agent/pkg/agentless/devices"
	"github.com/DataDog/datadog-agent/pkg/agentless/runner"
	"github.com/DataDog/datadog-agent/pkg/agentless/types"

	"github.com/DataDog/datadog-agent/pkg/security/utils"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/lambda"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	ddogstatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/spf13/cobra"
)

var awsFlags struct {
	region  string
	account string
}

// GroupCommand returns the AWS commands
func GroupCommand(parent *cobra.Command, statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, evp *eventplatform.Component) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "aws",
		Short:             "Datadog Agentless Scanner at your service.",
		Long:              `Datadog Agentless Scanner scans your cloud environment for vulnerabilities, compliance and security issues.`,
		SilenceUsage:      true,
		PersistentPreRunE: parent.PersistentPreRunE,
	}
	pflags := cmd.PersistentFlags()
	pflags.StringVar(&awsFlags.region, "region", "", "AWS region")
	pflags.StringVar(&awsFlags.account, "account-id", "", "AWS account ID")
	cmd.AddCommand(awsScanCommand(statsd, sc, evp))
	cmd.AddCommand(awsSnapshotCommand(statsd, sc))
	cmd.AddCommand(awsOfflineCommand(statsd, sc, evp))
	cmd.AddCommand(awsAttachCommand(statsd, sc))
	cmd.AddCommand(awsCleanupCommand(statsd, sc))
	return cmd
}

func awsScanCommand(statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, evp *eventplatform.Component) *cobra.Command {
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
			self, err := probeAWSEnv(ctx, sc)
			if err != nil {
				return err
			}
			resourceID, err := types.HumanParseCloudID(args[0], types.CloudProviderAWS, self.Region, self.AccountID)
			if err != nil {
				return err
			}
			return awsScanCmd(ctx, statsd, sc, evp, resourceID, localFlags.Hostname, flags.GlobalFlags.DefaultActions, flags.GlobalFlags.DiskMode, flags.GlobalFlags.NoForkScanners)
		},
	}

	cmd.Flags().StringVar(&localFlags.Hostname, "hostname", "unknown", "scan hostname")
	return cmd
}

func awsSnapshotCommand(statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Create a snapshot of the given (server-less mode)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := common.CtxTerminated()
			hostname := common.TryGetHostname(ctx)
			scannerID := types.NewScannerID(types.CloudProviderAWS, hostname)
			self, err := probeAWSEnv(ctx, sc)
			if err != nil {
				return err
			}
			volumeID, err := types.HumanParseCloudID(args[0], types.CloudProviderAWS, self.Region, self.AccountID)
			if err != nil {
				return err
			}
			roles := common.GetDefaultRolesMapping(sc, types.CloudProviderAWS)
			scan, err := types.NewScanTask(
				types.TaskTypeEBS,
				volumeID.AsText(),
				scannerID,
				"unknown",
				nil,
				flags.GlobalFlags.DefaultActions,
				roles,
				flags.GlobalFlags.DiskMode)
			if err != nil {
				return err
			}
			cfg := awsbackend.GetConfigFromCloudID(ctx, statsd, sc, roles, scan.TargetID)
			var waiter awsbackend.ResourceWaiter
			ec2client := ec2.NewFromConfig(cfg)
			snapshotID, err := awsbackend.CreateSnapshot(ctx, statsd, scan, &waiter, ec2client, scan.TargetID)
			if err != nil {
				return err
			}
			fmt.Println(snapshotID)
			return nil
		},
	}
	return cmd
}

func awsOfflineCommand(statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, evp *eventplatform.Component) *cobra.Command {
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

	var localFlags struct {
		workers      int
		filters      string
		taskType     string
		maxScans     int
		printResults bool
	}
	cmd := &cobra.Command{
		Use:   "offline",
		Short: "Runs the agentless-scanner in offline mode (server-less mode)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := common.CtxTerminated()
			self, err := probeAWSEnv(ctx, sc)
			if err != nil {
				return err
			}
			if localFlags.workers <= 0 {
				return fmt.Errorf("workers must be greater than 0")
			}
			filters, err := parseFilters(localFlags.filters)
			if err != nil {
				return err
			}
			taskType, err := types.ParseTaskType(localFlags.taskType)
			if err != nil {
				return err
			}
			return awsOfflineCmd(
				ctx,
				statsd,
				sc,
				evp,
				localFlags.workers,
				taskType,
				self.AccountID,
				self.Region,
				localFlags.maxScans,
				localFlags.printResults,
				filters,
				flags.GlobalFlags.DiskMode,
				flags.GlobalFlags.DefaultActions,
				flags.GlobalFlags.NoForkScanners)
		},
	}

	cmd.Flags().IntVar(&localFlags.workers, "workers", 40, "number of scans running in parallel")
	cmd.Flags().StringVar(&localFlags.filters, "filters", "", "list of filters to filter the resources (format: Name=string,Values=string,string)")
	cmd.Flags().StringVar(&localFlags.taskType, "task-type", string(types.TaskTypeEBS), fmt.Sprintf("scan type (%s, %s, %s or %s)", types.TaskTypeEBS, types.TaskTypeLambda, types.TaskTypeAMI, types.TaskTypeHost))
	cmd.Flags().IntVar(&localFlags.maxScans, "max-scans", 0, "maximum number of scans to perform")
	cmd.Flags().BoolVar(&localFlags.printResults, "print-results", false, "print scan results to stdout")
	return cmd
}

func awsAttachCommand(statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig) *cobra.Command {
	var localFlags struct {
		noMount bool
	}
	cmd := &cobra.Command{
		Use:   "attach <snapshot|volume>",
		Short: "Attaches a snapshot or volume to the current instance",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := common.CtxTerminated()
			self, err := probeAWSEnv(ctx, sc)
			if err != nil {
				return err
			}
			resourceID, err := types.HumanParseCloudID(args[0], types.CloudProviderAWS, self.Region, self.AccountID)
			if err != nil {
				return err
			}
			return awsAttachCmd(ctx, statsd, sc, resourceID, !localFlags.noMount, flags.GlobalFlags.DiskMode, flags.GlobalFlags.DefaultActions)
		},
	}
	cmd.Flags().BoolVar(&localFlags.noMount, "no-mount", false, "mount the device")
	return cmd
}

func awsCleanupCommand(statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig) *cobra.Command {
	var flags struct {
		dryRun bool
		delay  time.Duration
	}
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Cleanup resources created by the agentless-scanner",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := common.CtxTerminated()
			self, err := probeAWSEnv(ctx, sc)
			if err != nil {
				return err
			}
			return awsCleanupCmd(ctx, statsd, sc, self.Region, self.AccountID, flags.dryRun, flags.delay)
		},
	}
	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", false, "dry run")
	cmd.Flags().DurationVar(&flags.delay, "delay", 0, "delete snapshot older than delay")
	return cmd
}

func awsScanCmd(ctx context.Context, statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, evp *eventplatform.Component, resourceID types.CloudID, targetName string, actions []types.ScanAction, diskMode types.DiskMode, noForkScanners bool) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	hostname := common.TryGetHostname(ctx)
	scannerID := types.NewScannerID(types.CloudProviderAWS, hostname)
	taskType, err := types.DefaultTaskType(resourceID)
	if err != nil {
		return err
	}
	roles := common.GetDefaultRolesMapping(sc, types.CloudProviderAWS)
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
			Type:  types.ConfigTypeAWS,
			Tasks: []*types.ScanTask{task},
		})
		cancel()
	}()
	scanner.Start(ctx, statsd, sc)
	return nil
}

func awsOfflineCmd(ctx context.Context, statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, evp *eventplatform.Component, workers int, taskType types.TaskType, accountID, regionName string, maxScans int, printResults bool, filters []ec2types.Filter, diskMode types.DiskMode, actions []types.ScanAction, noForkScanners bool) error {
	defer statsd.Flush()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	hostname, err := utils.GetHostnameWithContext(ctx)
	if err != nil {
		return fmt.Errorf("could not fetch hostname: %w", err)
	}

	scannerID := types.NewScannerID(types.CloudProviderAWS, hostname)
	roles := common.GetDefaultRolesMapping(sc, types.CloudProviderAWS)
	scanner, err := runner.New(runner.Options{
		ScannerID:      scannerID,
		DdEnv:          sc.Env,
		Workers:        workers,
		ScannersMax:    8,
		PrintResults:   printResults,
		NoFork:         noForkScanners,
		DefaultActions: actions,
		DefaultRoles:   roles,
		Statsd:         statsd,
		EventForwarder: *evp,
	})
	if err != nil {
		return fmt.Errorf("could not initialize agentless-scanner: %w", err)
	}
	if err := scanner.CleanSlate(statsd, sc); err != nil {
		log.Error(err)
	}

	pushEBSVolumes := func() error {
		ec2client := ec2.NewFromConfig(awsbackend.GetConfig(ctx, statsd, sc, regionName, roles.GetRole(accountID)))
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
						volumeID, err := types.AWSCloudID(regionName, accountID, types.ResourceTypeVolume, *blockDeviceMapping.Ebs.VolumeId)
						if err != nil {
							return err
						}
						log.Debugf("%s %s %s %s %s", regionName, *instance.InstanceId, volumeID, *blockDeviceMapping.DeviceName, *instance.PlatformDetails)
						scan, err := types.NewScanTask(
							types.TaskTypeEBS,
							volumeID.AsText(),
							scannerID,
							*instance.InstanceId,
							ec2TagsToStringTags(instance.Tags),
							actions,
							roles,
							diskMode)
						if err != nil {
							return err
						}

						if !scanner.PushConfig(ctx, &types.ScanConfig{
							Type:  types.ConfigTypeAWS,
							Tasks: []*types.ScanTask{scan},
							Roles: roles,
						}) {
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
		ec2client := ec2.NewFromConfig(awsbackend.GetConfig(ctx, statsd, sc, regionName, roles.GetRole(accountID)))
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
				return fmt.Errorf("could not scan region %q for instances: %w", regionName, err)
			}
			for _, reservation := range instances.Reservations {
				var imageIDS []string
				for _, instance := range reservation.Instances {
					if instance.InstanceId == nil {
						continue
					}
					if instance.State.Name != ec2types.InstanceStateNameRunning {
						continue
					}
					if imageID := instance.ImageId; imageID != nil {
						imageIDS = append(imageIDS, *imageID)
					}
				}
				if len(imageIDS) > 0 {
					images, err := ec2client.DescribeImages(ctx, &ec2.DescribeImagesInput{
						ImageIds: imageIDS,
						Owners:   []string{"self"},
					})
					if err != nil {
						return fmt.Errorf("could not scan region %q for AMIs: %w", regionName, err)
					}
					for _, image := range images.Images {
						if image.ImageId == nil {
							continue
						}
						imageID, err := types.AWSCloudID(regionName, accountID, types.ResourceTypeHostImage, *image.ImageId)
						if err != nil {
							return err
						}
						log.Debugf("%s %s %s %s", regionName, *image.ImageId, imageID, *image.OwnerId)
						scan, err := types.NewScanTask(
							types.TaskTypeAMI,
							imageID.AsText(),
							scannerID,
							*image.ImageId,
							ec2TagsToStringTags(image.Tags),
							actions,
							roles,
							diskMode)
						if err != nil {
							return err
						}
						if !scanner.PushConfig(ctx, &types.ScanConfig{
							Type:  types.ConfigTypeAWS,
							Tasks: []*types.ScanTask{scan},
							Roles: roles,
						}) {
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

	pushLambdaFunctions := func() error {
		lambdaclient := lambda.NewFromConfig(awsbackend.GetConfig(ctx, statsd, sc, regionName, roles.GetRole(accountID)))
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
				fn, err := lambdaclient.GetFunction(ctx, &lambda.GetFunctionInput{
					FunctionName: function.FunctionName,
				})
				if err != nil {
					return fmt.Errorf("could not get lambda function %s: %w", *function.FunctionName, err)
				}
				var functionTags []string
				for k, v := range fn.Tags {
					functionTags = append(functionTags, fmt.Sprintf("%s:%s", k, v))
				}
				scan, err := types.NewScanTask(
					types.TaskTypeLambda,
					*function.FunctionArn,
					scannerID,
					*fn.Configuration.Version,
					functionTags,
					actions,
					roles,
					diskMode)
				if err != nil {
					return fmt.Errorf("could not create scan for lambda %s: %w", *function.FunctionArn, err)
				}
				if !scanner.PushConfig(ctx, &types.ScanConfig{
					Type:  types.ConfigTypeAWS,
					Tasks: []*types.ScanTask{scan},
					Roles: roles,
				}) {
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
		defer cancel()
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

	scanner.Start(ctx, statsd, sc)
	return nil
}

func awsCleanupCmd(ctx context.Context, statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, region, account string, dryRun bool, delay time.Duration) error {
	assumedRole := common.GetDefaultRolesMapping(sc, types.CloudProviderAWS).GetRole(account)
	toBeDeleted, err := awsbackend.ListResourcesForCleanup(ctx, statsd, sc, delay, region, assumedRole)
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
		awsbackend.ResourcesCleanup(ctx, statsd, sc, toBeDeleted, region, assumedRole)
	}
	return nil
}

func awsAttachCmd(ctx context.Context, statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, resourceID types.CloudID, mount bool, diskMode types.DiskMode, defaultActions []types.ScanAction) error {
	hostname := common.TryGetHostname(ctx)
	scannerID := types.NewScannerID(types.CloudProviderAWS, hostname)
	roles := common.GetDefaultRolesMapping(sc, types.CloudProviderAWS)
	scan, err := types.NewScanTask(
		types.TaskTypeEBS,
		resourceID.AsText(),
		scannerID,
		"unknown",
		nil,
		defaultActions,
		roles,
		diskMode)
	if err != nil {
		return err
	}

	defer func() {
		ctxcleanup, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		runner.CleanupScanDir(ctxcleanup, scan)
		for resourceID := range scan.CreatedResources {
			if err := awsbackend.CleanupScanEBS(ctxcleanup, statsd, sc, scan, resourceID); err != nil {
				log.Errorf("%s: could not cleanup resource %q: %v", scan, resourceID, err)
			}
		}
	}()

	var waiter awsbackend.ResourceWaiter
	snapshotID, err := awsbackend.SetupEBSSnapshot(ctx, statsd, sc, scan, &waiter)
	if err != nil {
		return err
	}

	err = awsbackend.SetupEBSVolume(ctx, statsd, sc, scan, &waiter, snapshotID)
	if err != nil {
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
				fmt.Printf("mountpoint\t%s", mountpoint)
			}
		}
	} else {
		fmt.Printf("no compatible partition found on %s\n", *scan.AttachedDeviceName)
	}

	fmt.Println("Ctrl+C to detach the device")
	<-ctx.Done()
	return nil
}

func ec2TagsToStringTags(tags []ec2types.Tag) []string {
	var tgs []string
	for _, tag := range tags {
		if tag.Key == nil {
			continue
		}
		if tag.Value == nil {
			tgs = append(tgs, *tag.Key)
		} else {
			tgs = append(tgs, fmt.Sprintf("%s:%s", *tag.Key, *tag.Value))
		}
	}
	return tgs
}

type awsEnv struct {
	Region    string
	AccountID string
}

func probeAWSEnv(ctx context.Context, sc *types.ScannerConfig) (*awsEnv, error) {
	region, account := awsFlags.region, awsFlags.account
	if region == "" || account == "" {
		cfg, err := config.LoadDefaultConfig(ctx)
		if err != nil {
			return nil, err
		}
		imdsclient := imds.NewFromConfig(cfg)
		id, err := imdsclient.GetInstanceIdentityDocument(ctx, &imds.GetInstanceIdentityDocumentInput{})
		if err == nil {
			if region == "" {
				region = id.Region
			}
			if account == "" {
				account = id.AccountID
			}
		} else if errors.Is(err, syscall.ECONNREFUSED) {
			// Probably not running from an EC2 instance
			if region == "" {
				region = sc.AWSRegion
			}
			if region == "" {
				region = awsbackend.DefaultSelfRegion
			}
			if account == "" {
				id2, err := awsbackend.GetIdentity(ctx)
				if err != nil {
					return nil, fmt.Errorf("could not get self identity: %w", err)
				}
				account = *id2.Account
			}
		} else if err != nil {
			return nil, fmt.Errorf("could not get self identity: %w", err)
		}
	}
	return &awsEnv{
		Region:    region,
		AccountID: account,
	}, nil
}
