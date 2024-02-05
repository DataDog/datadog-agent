// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package awscmd exports the root subcommand for AWS Agentless Scanner.
package awscmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/awsutils"
	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/devices"
	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/runner"
	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/types"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"

	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/version"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/ebs"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	ddogstatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/spf13/cobra"
)

var statsd *ddogstatsd.Client

const (
	defaultWorkersCount = 15
	defaultScannersMax  = 8 // max number of child-process scanners spawned by a worker in parallel

	defaultSelfRegion = "us-east-1"
)

// RootCommand returns the AWS sub-command for the agentless-scanner.
func RootCommand(diskMode *types.DiskMode, defaultActions *[]types.ScanAction, noForkScanners *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "aws",
		Short:        "Datadog Agentless Scanner at your service.",
		Long:         `Datadog Agentless Scanner scans your cloud environment for vulnerabilities, compliance and security issues.`,
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := cmd.Parent().PersistentPreRunE(cmd.Parent(), args); err != nil {
				return err
			}
			initStatsdClient()
			awsutils.InitConfig(statsd, getAWSLimitsOptions(), []string{
				fmt.Sprintf("agent_version:%s", version.AgentVersion),
			})
			return nil
		},
	}
	cmd.AddCommand(runCommand(defaultActions, noForkScanners))
	cmd.AddCommand(scanCommand(diskMode, defaultActions, noForkScanners))
	cmd.AddCommand(snapshotCommand(diskMode, defaultActions))
	cmd.AddCommand(offlineCommand(diskMode, defaultActions, noForkScanners))
	cmd.AddCommand(attachCommand(diskMode, defaultActions))
	cmd.AddCommand(cleanupCommand())
	return cmd
}

func runCommand(defaultActions *[]types.ScanAction, noForkScanners *bool) *cobra.Command {
	var runParams struct {
		pidfilePath string
		workers     int
		scannersMax int
	}

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Runs the agentless-scanner",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCmd(runParams.pidfilePath, runParams.workers, runParams.scannersMax, *defaultActions, *noForkScanners)
		},
	}
	cmd.Flags().StringVarP(&runParams.pidfilePath, "pidfile", "p", "", "path to the pidfile")
	cmd.Flags().IntVar(&runParams.workers, "workers", defaultWorkersCount, "number of snapshots running in parallel")
	cmd.Flags().IntVar(&runParams.scannersMax, "scannersMax", defaultScannersMax, "maximum number of scanner processes in parallel")
	return cmd
}

func scanCommand(diskMode *types.DiskMode, defaultActions *[]types.ScanAction, noForkScanners *bool) *cobra.Command {
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
			return scanCmd(resourceID, flags.Hostname, *defaultActions, *diskMode, *noForkScanners)
		},
	}

	cmd.Flags().StringVar(&flags.Hostname, "hostname", "unknown", "scan hostname")
	return cmd
}

func snapshotCommand(diskMode *types.DiskMode, defaultActions *[]types.ScanAction) *cobra.Command {
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
			scan, err := types.NewScanTask(volumeID.String(), "unknown", "unknown", *defaultActions, nil, *diskMode)
			if err != nil {
				return err
			}
			cfg, err := awsutils.GetConfig(ctx, scan.CloudID.Region, nil)
			if err != nil {
				return err
			}
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

func offlineCommand(diskMode *types.DiskMode, defaultActions *[]types.ScanAction, noForkScanners *bool) *cobra.Command {
	var cliArgs struct {
		workers      int
		regions      []string
		filters      string
		scanType     string
		maxScans     int
		printResults bool
	}
	cmd := &cobra.Command{
		Use:   "offline",
		Short: "Runs the agentless-scanner in offline mode (server-less mode)",
		RunE: func(cmd *cobra.Command, args []string) error {
			var filters []ec2types.Filter
			if filter := cliArgs.filters; filter != "" {
				if !strings.HasPrefix(filter, "Name=") {
					return fmt.Errorf("bad format for filters: expecting Name=string,Values=string,string")
				}
				filter = filter[len("Name="):]
				split := strings.SplitN(filter, ",", 2)
				if len(split) != 2 {
					return fmt.Errorf("bad format for filters: expecting Name=string,Values=string,string")
				}
				name := split[0]
				filter = split[1]
				if !strings.HasPrefix(filter, "Values=") {
					return fmt.Errorf("bad format for filters: expecting Name=string,Values=string,string")
				}
				filter = filter[len("Values="):]
				values := strings.Split(filter, ",")
				filters = append(filters, ec2types.Filter{
					Name:   aws.String(name),
					Values: values,
				})
			}
			return offlineCmd(cliArgs.workers, types.ScanType(cliArgs.scanType), cliArgs.regions, cliArgs.maxScans, cliArgs.printResults, filters, *diskMode, *defaultActions, *noForkScanners)
		},
	}

	cmd.Flags().IntVar(&cliArgs.workers, "workers", defaultWorkersCount, "number of scans running in parallel")
	cmd.Flags().StringSliceVar(&cliArgs.regions, "regions", []string{"auto"}, "list of regions to scan (default to all regions)")
	cmd.Flags().StringVar(&cliArgs.filters, "filters", "", "list of filters to filter the resources (format: Name=string,Values=string,string)")
	cmd.Flags().StringVar(&cliArgs.scanType, "scan-type", string(types.ScanTypeEBS), "scan type (ebs-volume or lambda)")
	cmd.Flags().IntVar(&cliArgs.maxScans, "max-scans", 0, "maximum number of scans to perform")
	cmd.Flags().BoolVar(&cliArgs.printResults, "print-results", false, "print scan results to stdout")
	return cmd
}

func attachCommand(diskMode *types.DiskMode, defaultActions *[]types.ScanAction) *cobra.Command {
	var cliArgs struct {
		mount bool
	}

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
			return attachCmd(resourceID, *diskMode, cliArgs.mount, *defaultActions)
		},
	}

	cmd.Flags().BoolVar(&cliArgs.mount, "mount", false, "mount the nbd device")

	return cmd
}

func cleanupCommand() *cobra.Command {
	var cliArgs struct {
		region string
		dryRun bool
		delay  time.Duration
	}
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Cleanup resources created by the agentless-scanner",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cleanupCmd(cliArgs.region, cliArgs.dryRun, cliArgs.delay)
		},
	}
	cmd.Flags().StringVar(&cliArgs.region, "region", "us-east-1", "AWS region")
	cmd.Flags().BoolVar(&cliArgs.dryRun, "dry-run", false, "dry run")
	cmd.Flags().DurationVar(&cliArgs.delay, "delay", 0, "delete snapshot older than delay")
	return cmd
}

func initStatsdClient() {
	statsdHost := pkgconfig.GetBindHost()
	statsdPort := pkgconfig.Datadog.GetInt("dogstatsd_port")
	statsdAddr := fmt.Sprintf("%s:%d", statsdHost, statsdPort)
	var err error
	statsd, err = ddogstatsd.New(statsdAddr)
	if err != nil {
		log.Warnf("could not init dogstatsd client: %s", err)
	}
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

func runCmd(pidfilePath string, workers, scannersMax int, defaultActions []types.ScanAction, noForkScanners bool) error {
	ctx := ctxTerminated()

	if pidfilePath != "" {
		err := pidfile.WritePID(pidfilePath)
		if err != nil {
			return fmt.Errorf("could not write PID file, exiting: %w", err)
		}
		defer os.Remove(pidfilePath)
		log.Infof("pid '%d' written to pid file '%s'", os.Getpid(), pidfilePath)
	}

	hostname, err := utils.GetHostnameWithContext(ctx)
	if err != nil {
		return fmt.Errorf("could not fetch hostname: %w", err)
	}

	scanner, err := runner.New(runner.Options{
		Hostname:       hostname,
		DdEnv:          pkgconfig.Datadog.GetString("env"),
		Workers:        workers,
		ScannersMax:    scannersMax,
		PrintResults:   false,
		NoFork:         noForkScanners,
		DefaultActions: defaultActions,
		DefaultRoles:   getDefaultRolesMapping(),
		Statsd:         statsd,
	})
	if err != nil {
		return fmt.Errorf("could not initialize agentless-scanner: %w", err)
	}
	if err := scanner.CleanSlate(); err != nil {
		log.Error(err)
	}
	if err := scanner.SubscribeRemoteConfig(ctx); err != nil {
		return fmt.Errorf("could not accept configs from Remote Config: %w", err)
	}
	scanner.Start(ctx)
	return nil
}

func getDefaultRolesMapping() types.RolesMapping {
	roles := pkgconfig.Datadog.GetStringSlice("agentless_scanner.default_roles")
	return types.ParseRolesMapping(roles)
}

func scanCmd(resourceID types.CloudID, targetHostname string, actions []types.ScanAction, diskMode types.DiskMode, noForkScanners bool) error {
	ctx := ctxTerminated()

	ctxhostname, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	hostname, err := utils.GetHostnameWithContext(ctxhostname)
	if err != nil {
		hostname = "unknown"
	}

	roles := getDefaultRolesMapping()
	task, err := types.NewScanTask(resourceID.String(), hostname, targetHostname, actions, roles, diskMode)
	if err != nil {
		return err
	}

	scanner, err := runner.New(runner.Options{
		Hostname:       hostname,
		DdEnv:          pkgconfig.Datadog.GetString("env"),
		Workers:        1,
		ScannersMax:    defaultScannersMax,
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

func offlineCmd(workers int, scanType types.ScanType, regions []string, maxScans int, printResults bool, filters []ec2types.Filter, diskMode types.DiskMode, actions []types.ScanAction, noForkScanners bool) error {
	ctx := ctxTerminated()
	defer statsd.Flush()

	hostname, err := utils.GetHostnameWithContext(ctx)
	if err != nil {
		return fmt.Errorf("could not fetch hostname: %w", err)
	}

	// Retrieve instance’s region.
	defaultCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return err
	}
	imdsclient := imds.NewFromConfig(defaultCfg)
	regionOutput, err := imdsclient.GetRegion(ctx, &imds.GetRegionInput{})
	selfRegion := defaultSelfRegion
	if err != nil {
		log.Errorf("could not retrieve region from ec2 instance - using default %q: %v", selfRegion, err)
	} else {
		selfRegion = regionOutput.Region
	}

	var identity *sts.GetCallerIdentityOutput
	{
		cfg, err := awsutils.GetConfig(ctx, selfRegion, nil)
		if err != nil {
			return err
		}
		stsclient := sts.NewFromConfig(cfg)
		identity, err = stsclient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
		if err != nil {
			return err
		}
	}

	roles := getDefaultRolesMapping()

	cfg, err := awsutils.GetConfig(ctx, selfRegion, roles[*identity.Account])
	if err != nil {
		return err
	}

	ec2client := ec2.NewFromConfig(cfg)
	if err != nil {
		return err
	}

	var allRegions []string
	if len(regions) > 0 {
		allRegions = regions
	} else {
		describeRegionsInput := &ec2.DescribeRegionsInput{}
		describeRegionsOutput, err := ec2client.DescribeRegions(ctx, describeRegionsInput)
		if err != nil {
			return err
		}
		for _, region := range describeRegionsOutput.Regions {
			if region.RegionName == nil {
				continue
			}
			allRegions = append(allRegions, *region.RegionName)
		}
	}

	scanner, err := runner.New(runner.Options{
		Hostname:       hostname,
		DdEnv:          pkgconfig.Datadog.GetString("env"),
		Workers:        workers,
		ScannersMax:    defaultScannersMax,
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
		count := 0
		for _, regionName := range allRegions {
			if ctx.Err() != nil {
				return nil
			}
			if regionName == "auto" {
				regionName = selfRegion
			}
			cfg, err := awsutils.GetConfig(ctx, regionName, roles[*identity.Account])
			if err != nil {
				if err != nil {
					return err
				}
			}
			ec2client := ec2.NewFromConfig(cfg)
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
							volumeID, err := types.AWSCloudID("ec2", regionName, *identity.Account, types.ResourceTypeVolume, *blockDeviceMapping.Ebs.VolumeId)
							if err != nil {
								return err
							}
							log.Debugf("%s %s %s %s %s", regionName, *instance.InstanceId, volumeID, *blockDeviceMapping.DeviceName, *instance.PlatformDetails)
							scan, err := types.NewScanTask(volumeID.String(), hostname, *instance.InstanceId, actions, roles, diskMode)
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
		}
		return nil
	}

	pushLambdaFunctions := func() error {
		count := 0
		for _, regionName := range regions {
			if ctx.Err() != nil {
				return nil
			}
			if regionName == "auto" {
				regionName = selfRegion
			}
			cfg, err := awsutils.GetConfig(ctx, regionName, roles[*identity.Account])
			if err != nil {
				return fmt.Errorf("could not scan region %q for Lambda functions: %w", regionName, err)
			}
			lambdaclient := lambda.NewFromConfig(cfg)
			var marker *string
			for {
				functions, err := lambdaclient.ListFunctions(ctx, &lambda.ListFunctionsInput{
					Marker: marker,
				})
				if err != nil {
					return fmt.Errorf("could not scan region %q for Lambda functions: %w", regionName, err)
				}
				for _, function := range functions.Functions {
					scan, err := types.NewScanTask(*function.FunctionArn, hostname, "", actions, roles, diskMode)
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
		}
		return nil
	}

	go func() {
		defer scanner.Stop()
		var err error
		if scanType == types.ScanTypeEBS {
			err = pushEBSVolumes()
		} else if scanType == types.ScanTypeLambda {
			err = pushLambdaFunctions()
		} else {
			panic("unreachable")
		}
		if err != nil {
			log.Error(err)
		}
	}()

	scanner.Start(ctx)
	return nil
}

func cleanupCmd(region string, dryRun bool, delay time.Duration) error {
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

	assumedRole := getDefaultRolesMapping()[*identity.Account]
	toBeDeleted := awsutils.ListResourcesForCleanup(ctx, delay, region, assumedRole)
	if len(toBeDeleted) == 0 {
		fmt.Printf("no resources found to cleanup\n")
		return nil
	}
	fmt.Printf("cleaning up these resources:\n")
	for resourceType, resources := range toBeDeleted {
		fmt.Printf("  - %s:\n", resourceType)
		for _, resourceName := range resources {
			fmt.Printf("    - %s\n", resourceName)
		}
	}
	if !dryRun {
		awsutils.ResourcesCleanup(ctx, toBeDeleted, region, assumedRole)
	}
	return nil
}

func attachCmd(resourceID types.CloudID, mode types.DiskMode, mount bool, defaultActions []types.ScanAction) error {
	ctx := ctxTerminated()

	cfg, err := awsutils.GetConfig(ctx, resourceID.Region, nil)
	if err != nil {
		return err
	}

	ctxhostname, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	hostname, err := utils.GetHostnameWithContext(ctxhostname)
	if err != nil {
		hostname = "unknown"
	}

	scan, err := types.NewScanTask(resourceID.String(), hostname, resourceID.ResourceName(), defaultActions, nil, mode)
	if err != nil {
		return err
	}
	defer func() {
		ctxcleanup, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		for resourceID := range scan.CreatedResources {
			if err := awsutils.CleanupScanEBS(ctxcleanup, scan, resourceID); err != nil {
				log.Errorf("%s: could not cleanup resource %q: %v", scan, resourceID, err)
			}
		}
	}()

	var waiter awsutils.SnapshotWaiter

	var snapshotID types.CloudID
	switch resourceID.ResourceType() {
	case types.ResourceTypeVolume:
		ec2client := ec2.NewFromConfig(cfg)
		snapshotID, err = awsutils.CreateSnapshot(ctx, scan, &waiter, ec2client, resourceID)
		if err != nil {
			return err
		}
	case types.ResourceTypeSnapshot:
		snapshotID = resourceID
	default:
		panic("unreachable")
	}

	switch mode {
	case types.DiskModeVolumeAttach:
		if err := awsutils.AttachSnapshotWithVolume(ctx, scan, &waiter, snapshotID); err != nil {
			return err
		}
	case types.DiskModeNBDAttach:
		ebsclient := ebs.NewFromConfig(cfg)
		if err := awsutils.AttachSnapshotWithNBD(ctx, scan, snapshotID, ebsclient); err != nil {
			return err
		}
	default:
		panic("unreachable")
	}

	partitions, err := devices.ListPartitions(ctx, scan, *scan.AttachedDeviceName)
	if err != nil {
		log.Errorf("could not list partitions (device is still available on %q): %v", *scan.AttachedDeviceName, err)
	} else {
		for _, part := range partitions {
			fmt.Println(part.DevicePath, part.FSType)
		}
		if mount {
			mountPoints, err := devices.Mount(ctx, scan, partitions)
			if err != nil {
				log.Errorf("could not mount (device is still available on %q): %v", *scan.AttachedDeviceName, err)
			} else {
				fmt.Println()
				for _, mountPoint := range mountPoints {
					fmt.Println(mountPoint)
				}
			}
		}
	}

	<-ctx.Done()
	return nil
}

func getAWSLimitsOptions() awsutils.LimiterOptions {
	return awsutils.LimiterOptions{
		EC2Rate:          rate.Limit(pkgconfig.Datadog.GetFloat64("agentless_scanner.limits.aws_ec2_rate")),
		EBSListBlockRate: rate.Limit(pkgconfig.Datadog.GetFloat64("agentless_scanner.limits.aws_ebs_list_block_rate")),
		EBSGetBlockRate:  rate.Limit(pkgconfig.Datadog.GetFloat64("agentless_scanner.limits.aws_ebs_get_block_rate")),
		DefaultRate:      rate.Limit(pkgconfig.Datadog.GetFloat64("agentless_scanner.limits.aws_default_rate")),
	}
}
