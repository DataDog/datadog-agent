// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package main implements the agentless-scanner command.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/awsutils"
	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/devices"
	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/runner"
	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/types"

	// DataDog agent: config stuffs
	commonpath "github.com/DataDog/datadog-agent/cmd/agent/common/path"
	compconfig "github.com/DataDog/datadog-agent/comp/core/config"
	complog "github.com/DataDog/datadog-agent/comp/core/log"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/version"
	"go.uber.org/fx"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/ebs"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	ddogstatsd "github.com/DataDog/datadog-go/v5/statsd"

	_ "crypto/sha256"
	_ "crypto/sha512"
	_ "modernc.org/sqlite" // sqlite driver, used by github.com/knqyf263/go-rpmdb

	"github.com/spf13/cobra"
)

const (
	defaultWorkersCount = 15
	defaultScannersMax  = 8 // max number of child-process scanners spawned by a worker in parallel

	defaultSelfRegion = "us-east-1"
)

var statsd *ddogstatsd.Client

var (
	globalParams struct {
		configFilePath string
		diskMode       types.DiskMode
		defaultActions []types.ScanAction
		noForkScanners bool
	}
)

func main() {
	flavor.SetFlavor(flavor.AgentlessScanner)

	signal.Ignore(syscall.SIGPIPE)

	cmd := rootCommand()
	cmd.SilenceErrors = true
	err := cmd.Execute()

	if err != nil {
		log.Flush()
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
		os.Exit(-1)
	}
	log.Flush()
	os.Exit(0)
}

func rootCommand() *cobra.Command {
	var diskModeStr string
	var defaultActionsStr []string

	cmd := &cobra.Command{
		Use:          "agentless-scanner [command]",
		Short:        "Datadog Agentless Scanner at your service.",
		Long:         `Datadog Agentless Scanner scans your cloud environment for vulnerabilities, compliance and security issues.`,
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			var err error

			initStatsdClient()

			globalParams.diskMode, err = types.ParseDiskMode(diskModeStr)
			if err != nil {
				return err
			}

			globalParams.defaultActions, err = types.ParseScanActions(defaultActionsStr)
			if err != nil {
				return err
			}

			awsutils.InitConfig(statsd, getAWSLimitsOptions(), []string{
				fmt.Sprintf("agent_version:%s", version.AgentVersion),
			})
			return nil
		},
	}

	pflags := cmd.PersistentFlags()
	pflags.StringVarP(&globalParams.configFilePath, "config-path", "c", path.Join(commonpath.DefaultConfPath, "datadog.yaml"), "specify the path to agentless-scanner configuration yaml file")
	pflags.StringVar(&diskModeStr, "disk-mode", string(types.NBDAttach), fmt.Sprintf("disk mode used for scanning EBS volumes: %s or %s", types.VolumeAttach, types.NBDAttach))
	pflags.BoolVar(&globalParams.noForkScanners, "no-fork-scanners", false, "disable spawning a dedicated process for launching scanners")
	pflags.StringSliceVar(&defaultActionsStr, "actions", []string{string(types.VulnsHost)}, "disable spawning a dedicated process for launching scanners")
	cmd.AddCommand(runCommand())
	cmd.AddCommand(runScannerCommand())
	cmd.AddCommand(scanCommand())
	cmd.AddCommand(snapshotCommand())
	cmd.AddCommand(offlineCommand())
	cmd.AddCommand(attachCommand())
	cmd.AddCommand(cleanupCommand())

	return cmd
}

func runWithModules(run func(cmd *cobra.Command, args []string) error) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		return fxutil.OneShot(
			func(_ complog.Component, _ compconfig.Component) error {
				return run(cmd, args)
			},
			fx.Supply(compconfig.NewAgentParamsWithSecrets(globalParams.configFilePath)),
			fx.Supply(complog.ForDaemon(runner.LoggerName, "log_file", pkgconfig.DefaultAgentlessScannerLogFile)),
			complog.Module,
			compconfig.Module,
		)
	}
}

func runCommand() *cobra.Command {
	var runParams struct {
		pidfilePath string
		workers     int
		scannersMax int
	}

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Runs the agentless-scanner",
		RunE: runWithModules(func(cmd *cobra.Command, args []string) error {
			return runCmd(runParams.pidfilePath, runParams.workers, runParams.scannersMax)
		}),
	}
	cmd.Flags().StringVarP(&runParams.pidfilePath, "pidfile", "p", "", "path to the pidfile")
	cmd.Flags().IntVar(&runParams.workers, "workers", defaultWorkersCount, "number of snapshots running in parallel")
	cmd.Flags().IntVar(&runParams.scannersMax, "scannersMax", defaultScannersMax, "maximum number of scanner processes in parallel")
	return cmd
}

func runScannerCommand() *cobra.Command {
	var sock string
	cmd := &cobra.Command{
		Use:   "run-scanner",
		Short: "Runs a scanner (fork/exec model)",
		RunE: runWithModules(func(cmd *cobra.Command, args []string) error {
			return runScannerCmd(sock)
		}),
	}
	cmd.Flags().StringVar(&sock, "sock", "", "path to unix socket for IPC")
	_ = cmd.MarkFlagRequired("sock")
	return cmd
}

func scanCommand() *cobra.Command {
	var flags struct {
		ARN      string
		Hostname string
	}
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "execute a scan",
		RunE: runWithModules(func(cmd *cobra.Command, args []string) error {
			resourceARN, err := humanParseARN(flags.ARN)
			if err != nil {
				return err
			}
			return scanCmd(resourceARN, flags.Hostname, globalParams.defaultActions)
		}),
	}

	cmd.Flags().StringVar(&flags.ARN, "arn", "", "arn to scan")
	cmd.Flags().StringVar(&flags.Hostname, "hostname", "unknown", "scan hostname")
	_ = cmd.MarkFlagRequired("arn")
	return cmd
}

func snapshotCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Create a snapshot of the given (server-less mode)",
		Args:  cobra.ExactArgs(1),
		RunE: runWithModules(func(cmd *cobra.Command, args []string) error {
			ctx := ctxTerminated()
			volumeARN, err := humanParseARN(args[0], types.ResourceTypeVolume)
			if err != nil {
				return err
			}
			scan, err := types.NewScanTask(volumeARN.String(), "unknown", "unknown", globalParams.defaultActions, nil, globalParams.diskMode)
			if err != nil {
				return err
			}
			cfg, err := awsutils.GetConfig(ctx, scan.ARN.Region, nil)
			if err != nil {
				return err
			}
			var waiter awsutils.SnapshotWaiter
			ec2client := ec2.NewFromConfig(cfg)
			snapshotARN, err := awsutils.CreateSnapshot(ctx, scan, &waiter, ec2client, scan.ARN)
			if err != nil {
				return err
			}
			fmt.Println(snapshotARN)
			return nil
		}),
	}
	return cmd
}

func offlineCommand() *cobra.Command {
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
		RunE: runWithModules(func(cmd *cobra.Command, args []string) error {
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
			return offlineCmd(cliArgs.workers, types.ScanType(cliArgs.scanType), cliArgs.regions, cliArgs.maxScans, cliArgs.printResults, globalParams.defaultActions, filters)
		}),
	}

	cmd.Flags().IntVar(&cliArgs.workers, "workers", defaultWorkersCount, "number of scans running in parallel")
	cmd.Flags().StringSliceVar(&cliArgs.regions, "regions", []string{"auto"}, "list of regions to scan (default to all regions)")
	cmd.Flags().StringVar(&cliArgs.filters, "filters", "", "list of filters to filter the resources (format: Name=string,Values=string,string)")
	cmd.Flags().StringVar(&cliArgs.scanType, "scan-type", string(types.EBSScanType), "scan type (ebs-volume or lambda)")
	cmd.Flags().IntVar(&cliArgs.maxScans, "max-scans", 0, "maximum number of scans to perform")
	cmd.Flags().BoolVar(&cliArgs.printResults, "print-results", false, "print scan results to stdout")
	return cmd
}

func attachCommand() *cobra.Command {
	var cliArgs struct {
		mount bool
	}

	cmd := &cobra.Command{
		Use:   "attach <snapshot-arn>",
		Short: "Mount the given snapshot into /snapshots/<snapshot-id>/<part> using a network block device",
		Args:  cobra.ExactArgs(1),
		RunE: runWithModules(func(cmd *cobra.Command, args []string) error {
			resourceARN, err := humanParseARN(args[0], types.ResourceTypeSnapshot, types.ResourceTypeVolume)
			if err != nil {
				return err
			}
			return attachCmd(resourceARN, globalParams.diskMode, cliArgs.mount)
		}),
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
		RunE: runWithModules(func(cmd *cobra.Command, args []string) error {
			return cleanupCmd(cliArgs.region, cliArgs.dryRun, cliArgs.delay)
		}),
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

func runCmd(pidfilePath string, workers, scannersMax int) error {
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
		NoFork:         globalParams.noForkScanners,
		DefaultActions: globalParams.defaultActions,
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

func runScannerCmd(sock string) error {
	ctx := ctxTerminated()

	var opts types.ScannerOptions

	conn, err := net.Dial("unix", sock)
	if err != nil {
		return err
	}
	defer conn.Close()

	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)
	_ = conn.SetReadDeadline(time.Now().Add(4 * time.Second))
	if err := dec.Decode(&opts); err != nil {
		if errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}

	result := runner.LaunchScannerInSameProcess(ctx, opts)
	_ = conn.SetWriteDeadline(time.Now().Add(4 * time.Second))
	if err := enc.Encode(result); err != nil {
		return err
	}
	return nil
}

func getDefaultRolesMapping() types.RolesMapping {
	roles := pkgconfig.Datadog.GetStringSlice("agentless_scanner.default_roles")
	return types.ParseRolesMapping(roles)
}

func scanCmd(resourceARN arn.ARN, targetHostname string, actions []types.ScanAction) error {
	ctx := ctxTerminated()

	ctxhostname, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	hostname, err := utils.GetHostnameWithContext(ctxhostname)
	if err != nil {
		hostname = "unknown"
	}

	roles := getDefaultRolesMapping()
	task, err := types.NewScanTask(resourceARN.String(), hostname, targetHostname, actions, roles, globalParams.diskMode)
	if err != nil {
		return err
	}

	scanner, err := runner.New(runner.Options{
		Hostname:       hostname,
		DdEnv:          pkgconfig.Datadog.GetString("env"),
		Workers:        1,
		ScannersMax:    defaultScannersMax,
		PrintResults:   true,
		NoFork:         globalParams.noForkScanners,
		DefaultActions: actions,
		DefaultRoles:   roles,
		Statsd:         statsd,
	})
	if err != nil {
		return fmt.Errorf("could not initialize agentless-scanner: %w", err)
	}
	go func() {
		scanner.PushConfig(ctx, &types.ScanConfig{
			Type:  types.AWSScan,
			Tasks: []*types.ScanTask{task},
		})
		scanner.Stop()
	}()
	scanner.Start(ctx)
	return nil
}

func offlineCmd(workers int, scanType types.ScanType, regions []string, maxScans int, printResults bool, actions []types.ScanAction, filters []ec2types.Filter) error {
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
		NoFork:         globalParams.noForkScanners,
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
							volumeARN := awsutils.EC2ARN(regionName, *identity.Account, types.ResourceTypeVolume, *blockDeviceMapping.Ebs.VolumeId)
							log.Debugf("%s %s %s %s %s", regionName, *instance.InstanceId, volumeARN, *blockDeviceMapping.DeviceName, *instance.PlatformDetails)
							scan, err := types.NewScanTask(volumeARN.String(), hostname, *instance.InstanceId, actions, roles, globalParams.diskMode)
							if err != nil {
								return err
							}

							config := &types.ScanConfig{Type: types.AWSScan, Tasks: []*types.ScanTask{scan}, Roles: roles}
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
				return fmt.Errorf("could not scan region %q for EBS volumes: %w", regionName, err)
			}
			lambdaclient := lambda.NewFromConfig(cfg)
			var marker *string
			for {
				functions, err := lambdaclient.ListFunctions(ctx, &lambda.ListFunctionsInput{
					Marker: marker,
				})
				if err != nil {
					return fmt.Errorf("could not scan region %q for EBS volumes: %w", regionName, err)
				}
				for _, function := range functions.Functions {
					scan, err := types.NewScanTask(*function.FunctionArn, hostname, "", actions, roles, globalParams.diskMode)
					if err != nil {
						return fmt.Errorf("could not create scan for lambda %s: %w", *function.FunctionArn, err)
					}
					config := &types.ScanConfig{Type: types.AWSScan, Tasks: []*types.ScanTask{scan}, Roles: roles}
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
		if scanType == types.EBSScanType {
			err = pushEBSVolumes()
		} else if scanType == types.LambdaScanType {
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
		for _, resourceID := range resources {
			fmt.Printf("    - %s\n", resourceID)
		}
	}
	if !dryRun {
		awsutils.ResourcesCleanup(ctx, toBeDeleted, region, assumedRole)
	}
	return nil
}

func attachCmd(resourceARN arn.ARN, mode types.DiskMode, mount bool) error {
	ctx := ctxTerminated()

	cfg, err := awsutils.GetConfig(ctx, resourceARN.Region, nil)
	if err != nil {
		return err
	}

	ctxhostname, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	hostname, err := utils.GetHostnameWithContext(ctxhostname)
	if err != nil {
		hostname = "unknown"
	}

	scan, err := types.NewScanTask(resourceARN.String(), hostname, resourceARN.Resource, globalParams.defaultActions, nil, mode)
	if err != nil {
		return err
	}
	defer awsutils.CleanupScan(scan)

	var waiter awsutils.SnapshotWaiter

	resourceType, _, _ := types.GetARNResource(resourceARN)
	var snapshotARN arn.ARN
	switch resourceType {
	case types.ResourceTypeVolume:
		ec2client := ec2.NewFromConfig(cfg)
		snapshotARN, err = awsutils.CreateSnapshot(ctx, scan, &waiter, ec2client, resourceARN)
		if err != nil {
			return err
		}
	case types.ResourceTypeSnapshot:
		snapshotARN = resourceARN
	default:
		panic("unreachable")
	}

	switch mode {
	case types.VolumeAttach:
		if err := awsutils.AttachSnapshotWithVolume(ctx, scan, &waiter, snapshotARN); err != nil {
			return err
		}
	case types.NBDAttach:
		ebsclient := ebs.NewFromConfig(cfg)
		if err := awsutils.AttachSnapshotWithNBD(ctx, scan, snapshotARN, ebsclient); err != nil {
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

func humanParseARN(s string, expectedTypes ...types.ResourceType) (arn.ARN, error) {
	if strings.HasPrefix(s, "arn:") {
		return types.ParseARN(s, expectedTypes...)
	}
	self, err := awsutils.GetSelfEC2InstanceIndentity(context.TODO())
	if err != nil {
		return arn.ARN{}, err
	}
	a := arn.ARN{
		Partition: "aws",
		Region:    self.Region,
		AccountID: self.AccountID,
		Resource:  s,
	}
	if strings.HasPrefix(s, "/") && (len(s) == 1 || fs.ValidPath(s[1:])) {
		a.Partition = "localhost"
	} else if strings.HasPrefix(s, "vol-") {
		a.Service = "ec2"
		a.Resource = "volume/" + a.Resource
	} else if strings.HasPrefix(s, "snap-") {
		a.Service = "ec2"
		a.Resource = "snapshot/" + a.Resource
	} else if strings.HasPrefix(s, "function:") {
		a.Service = "lambda"
	} else {
		return arn.ARN{}, fmt.Errorf("unable to parse resource: expecting an ARN for %v", expectedTypes)
	}
	return types.ParseARN(a.String(), expectedTypes...)
}
