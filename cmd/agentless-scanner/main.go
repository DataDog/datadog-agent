// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package main implements the agentless-scanner command.
package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/nbd"
	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/scanners"
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
	"github.com/docker/distribution/reference"
	"go.uber.org/fx"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/ebs"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"

	// DataDog agent: SBOM + proto stuffs
	cdx "github.com/CycloneDX/cyclonedx-go"
	sbommodel "github.com/DataDog/agent-payload/v5/sbom"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	// DataDog agent: RC stuffs
	"github.com/DataDog/datadog-agent/pkg/config/remote"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"

	// DataDog agent: logs stuffs
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	// DataDog agent: metrics Statsd
	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	// sqlite driver, used by github.com/knqyf263/go-rpmdb
	_ "modernc.org/sqlite"

	"github.com/spf13/cobra"
)

const (
	loggerName = "AGENTLESSSCANER"

	maxSnapshotRetries = 3
	maxAttachRetries   = 15

	maxLambdaUncompressed = 256 * 1024 * 1024

	defaultWorkersCount = 15
	defaultScannersMax  = 8 // max number of child-process scanners spawned by a worker in parallel

	defaultSelfRegion      = "us-east-1"
	defaultSnapshotsMaxTTL = 24 * time.Hour
)

var statsd *ddgostatsd.Client

var (
	globalParams struct {
		configFilePath string
		diskMode       types.DiskMode
		defaultActions []types.ScanAction
		noForkScanners bool
	}

	cleanupMaxDuration = 2 * time.Minute

	awsConfigs   = make(map[awsConfigKey]*aws.Config)
	awsConfigsMu sync.Mutex
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
			mode, err := types.ParseDiskMode(diskModeStr)
			if err != nil {
				return err
			}
			globalParams.defaultActions, err = types.ParseScanActions(defaultActionsStr)
			if err != nil {
				return err
			}
			globalParams.diskMode = mode
			initStatsdClient()
			return nil
		},
	}

	pflags := cmd.PersistentFlags()
	pflags.StringVarP(&globalParams.configFilePath, "config-path", "c", path.Join(commonpath.DefaultConfPath, "datadog.yaml"), "specify the path to agentless-scanner configuration yaml file")
	pflags.StringVar(&diskModeStr, "disk-mode", string(types.NoAttach), fmt.Sprintf("disk mode used for scanning EBS volumes: %s, %s or %s", types.VolumeAttach, types.NBDAttach, types.NoAttach))
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
			fx.Supply(complog.ForDaemon(loggerName, "log_file", pkgconfig.DefaultAgentlessScannerLogFile)),
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
			cfg, err := newAWSConfig(ctx, scan.ARN.Region, nil)
			if err != nil {
				return err
			}
			ec2client := ec2.NewFromConfig(cfg)
			snapshotARN, err := createSnapshot(ctx, scan, &awsWaiter{}, ec2client, scan.ARN)
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
			for _, action := range globalParams.defaultActions {
				if action == types.VulnsContainers && globalParams.diskMode == types.NoAttach {
					globalParams.diskMode = types.VolumeAttach
				}
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
	statsd, err = ddgostatsd.New(statsdAddr)
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

	scanner, err := newSideScanner(hostname, workers, scannersMax)
	if err != nil {
		return fmt.Errorf("could not initialize agentless-scanner: %w", err)
	}
	if err := scanner.cleanSlate(); err != nil {
		log.Error(err)
	}
	if err := scanner.subscribeRemoteConfig(ctx); err != nil {
		return fmt.Errorf("could not accept configs from Remote Config: %w", err)
	}
	scanner.start(ctx)
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

	result := launchScannerInSameProcess(ctx, opts)
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

	scanner, err := newSideScanner(hostname, 1, defaultScannersMax)
	if err != nil {
		return fmt.Errorf("could not initialize agentless-scanner: %w", err)
	}
	scanner.printResults = true
	go func() {
		scanner.pushConfig(ctx, &types.ScanConfig{
			Type:  types.AWSScan,
			Tasks: []*types.ScanTask{task},
		})
		scanner.stop()
	}()
	scanner.start(ctx)
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
		cfg, err := newAWSConfig(ctx, selfRegion, nil)
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

	cfg, err := newAWSConfig(ctx, selfRegion, roles[*identity.Account])
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

	scanner, err := newSideScanner(hostname, workers, defaultScannersMax)
	if err != nil {
		return fmt.Errorf("could not initialize agentless-scanner: %w", err)
	}
	if err := scanner.cleanSlate(); err != nil {
		log.Error(err)
	}
	scanner.printResults = printResults

	pushEBSVolumes := func() error {
		count := 0
		for _, regionName := range allRegions {
			if ctx.Err() != nil {
				return nil
			}
			if regionName == "auto" {
				regionName = selfRegion
			}
			cfg, err := newAWSConfig(ctx, regionName, roles[*identity.Account])
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
							volumeARN := ec2ARN(regionName, *identity.Account, types.ResourceTypeVolume, *blockDeviceMapping.Ebs.VolumeId)
							log.Debugf("%s %s %s %s %s", regionName, *instance.InstanceId, volumeARN, *blockDeviceMapping.DeviceName, *instance.PlatformDetails)
							scan, err := types.NewScanTask(volumeARN.String(), hostname, *instance.InstanceId, actions, roles, globalParams.diskMode)
							if err != nil {
								return err
							}

							config := &types.ScanConfig{Type: types.AWSScan, Tasks: []*types.ScanTask{scan}, Roles: roles}
							if !scanner.pushConfig(ctx, config) {
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
			cfg, err := newAWSConfig(ctx, regionName, roles[*identity.Account])
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
					if !scanner.pushConfig(ctx, config) {
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
		defer scanner.stop()
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

	scanner.start(ctx)
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

	roles := getDefaultRolesMapping()
	cfg, err := newAWSConfig(ctx, region, roles[*identity.Account])
	if err != nil {
		return err
	}
	ec2client := ec2.NewFromConfig(cfg)
	toBeDeleted := listResourcesForCleanup(ctx, ec2client, delay)
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
		cloudResourcesCleanup(ctx, ec2client, toBeDeleted)
	}
	return nil
}

func (s *sideScanner) cleanup(ctx context.Context, maxTTL time.Duration, region string, assumedRole *arn.ARN) error {
	cfg, err := newAWSConfig(ctx, region, assumedRole)
	if err != nil {
		return err
	}

	ec2client := ec2.NewFromConfig(cfg)
	toBeDeleted := listResourcesForCleanup(ctx, ec2client, maxTTL)
	cloudResourcesCleanup(ctx, ec2client, toBeDeleted)
	return nil
}

func (s *sideScanner) cleanupProcess(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}

		log.Infof("starting cleanup process")
		s.regionsCleanupMu.Lock()
		regionsCleanup := make(map[string]*arn.ARN, len(s.regionsCleanup))
		for region, role := range s.regionsCleanup {
			regionsCleanup[region] = role
		}
		s.regionsCleanup = nil
		s.regionsCleanupMu.Unlock()

		if len(regionsCleanup) > 0 {
			for region, role := range regionsCleanup {
				if err := s.cleanup(ctx, defaultSnapshotsMaxTTL, region, role); err != nil {
					log.Warnf("cleanupProcess failed on region %q with role %q: %v", region, role, err)
				}
			}
		}
	}
}

func attachCmd(resourceARN arn.ARN, mode types.DiskMode, mount bool) error {
	ctx := ctxTerminated()

	cfg, err := newAWSConfig(ctx, resourceARN.Region, nil)
	if err != nil {
		return err
	}

	if mode == types.NoAttach {
		mode = types.NBDAttach
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
	defer cleanupScan(scan)

	waiter := &awsWaiter{}

	resourceType, _, _ := types.GetARNResource(resourceARN)
	var snapshotARN arn.ARN
	switch resourceType {
	case types.ResourceTypeVolume:
		ec2client := ec2.NewFromConfig(cfg)
		snapshotARN, err = createSnapshot(ctx, scan, waiter, ec2client, resourceARN)
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
		if err := attachSnapshotWithVolume(ctx, scan, waiter, snapshotARN); err != nil {
			return err
		}
	case types.NBDAttach:
		ebsclient := ebs.NewFromConfig(cfg)
		if err := attachSnapshotWithNBD(ctx, scan, snapshotARN, ebsclient); err != nil {
			return err
		}
	default:
		panic("unreachable")
	}

	partitions, err := listDevicePartitions(ctx, scan)
	if err != nil {
		log.Errorf("could not list partitions (device is still available on %q): %v", *scan.AttachedDeviceName, err)
	} else {
		for _, part := range partitions {
			fmt.Println(part.devicePath, part.fsType)
		}
		if mount {
			mountPoints, err := mountDevice(ctx, scan, partitions)
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

type sideScanner struct {
	hostname         string
	workers          int
	scannersMax      int
	eventForwarder   epforwarder.EventPlatformForwarder
	findingsReporter *LogReporter
	rcClient         *remote.Client
	waiter           *awsWaiter
	printResults     bool

	regionsCleanupMu sync.Mutex
	regionsCleanup   map[string]*arn.ARN

	scansInProgress   map[arn.ARN]struct{}
	scansInProgressMu sync.RWMutex

	configsCh chan *types.ScanConfig
	scansCh   chan *types.ScanTask
	resultsCh chan types.ScanResult
}

func newSideScanner(hostname string, workers, scannersMax int) (*sideScanner, error) {
	eventForwarder := epforwarder.NewEventPlatformForwarder()
	findingsReporter, err := newFindingsReporter()
	if err != nil {
		return nil, err
	}
	rcClient, err := remote.NewUnverifiedGRPCClient("sidescanner", version.AgentVersion, nil, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("could not init Remote Config client: %w", err)
	}
	return &sideScanner{
		hostname:         hostname,
		workers:          workers,
		scannersMax:      scannersMax,
		eventForwarder:   eventForwarder,
		findingsReporter: findingsReporter,
		rcClient:         rcClient,
		waiter:           &awsWaiter{},

		scansInProgress: make(map[arn.ARN]struct{}),

		configsCh: make(chan *types.ScanConfig),
		scansCh:   make(chan *types.ScanTask),
		resultsCh: make(chan types.ScanResult),
	}, nil
}

func (s *sideScanner) subscribeRemoteConfig(ctx context.Context) error {
	log.Infof("subscribing to remote-config")
	defaultRolesMapping := getDefaultRolesMapping()
	s.rcClient.Subscribe(state.ProductCSMSideScanning, func(update map[string]state.RawConfig, _ func(string, state.ApplyStatus)) {
		log.Debugf("received %d remote config config updates", len(update))
		for _, rawConfig := range update {
			log.Debugf("received new config %q from remote-config of size %d", rawConfig.Metadata.ID, len(rawConfig.Config))
			config, err := types.UnmarshalConfig(rawConfig.Config, s.hostname, globalParams.defaultActions, defaultRolesMapping)
			if err != nil {
				log.Errorf("could not parse agentless-scanner task: %v", err)
				return
			}
			if !s.pushConfig(ctx, config) {
				return
			}
		}
	})
	return nil
}

func (s *sideScanner) healthServer(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	addr := "127.0.0.1:6253"
	srv := &http.Server{Addr: "127.0.0.1:6253"}
	srv.Handler = mux

	go func() {
		<-ctx.Done()
		err := srv.Shutdown(context.TODO())
		if err != nil {
			log.Warnf("error shutting down: %v", err)
		}
	}()

	log.Infof("Starting health-check server for agentless-scanner on address %q", addr)
	err := srv.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *sideScanner) cleanSlate() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	scansDir, err := os.Open(types.ScansRootDir)
	if os.IsNotExist(err) {
		if err := os.Mkdir(types.ScansRootDir, 0700); err != nil {
			return fmt.Errorf("clean slate: could not create directory %q: %w", types.ScansRootDir, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("clean slate: could not open %q: %w", types.ScansRootDir, err)
	}
	scanDirInfo, err := scansDir.Stat()
	if err != nil {
		return fmt.Errorf("clean slate: could not stat %q: %w", types.ScansRootDir, err)
	}
	if !scanDirInfo.IsDir() {
		return fmt.Errorf("clean slate: %q already exists and is not a directory: %w", types.ScansRootDir, os.ErrExist)
	}
	if scanDirInfo.Mode() != 0700 {
		if err := os.Chmod(types.ScansRootDir, 0700); err != nil {
			return fmt.Errorf("clean slate: could not chmod %q: %w", types.ScansRootDir, err)
		}
	}
	scanDirs, err := scansDir.ReadDir(0)
	if err != nil {
		return err
	}

	var ebsMountPoints []string
	var ctrMountPoints []string
	for _, scanDir := range scanDirs {
		name := filepath.Join(types.ScansRootDir, scanDir.Name())
		if !scanDir.IsDir() {
			if err := os.Remove(name); err != nil {
				log.Warnf("clean slate: could not remove file %q", name)
			}
		} else {
			switch {
			case strings.HasPrefix(scanDir.Name(), string(types.LambdaScanType)+"-"):
				if err := os.RemoveAll(name); err != nil {
					log.Warnf("clean slate: could not remove directory %q", name)
				}
			case strings.HasPrefix(scanDir.Name(), string(types.EBSScanType)):
				scanDirname := filepath.Join(types.ScansRootDir, scanDir.Name())
				scanEntries, err := os.ReadDir(scanDirname)
				if err != nil {
					log.Errorf("clean slate: %v", err)
				} else {
					for _, scanEntry := range scanEntries {
						switch {
						case strings.HasPrefix(scanEntry.Name(), types.EBSMountPrefix):
							ebsMountPoints = append(ebsMountPoints, filepath.Join(scanDirname, scanEntry.Name()))
						case strings.HasPrefix(scanEntry.Name(), types.ContainerdMountPrefix) || strings.HasPrefix(scanEntry.Name(), types.DockerMountPrefix):
							ctrMountPoints = append(ctrMountPoints, filepath.Join(scanDirname, scanEntry.Name()))
						}
					}
				}
			}
		}
	}

	for _, mountPoint := range ctrMountPoints {
		log.Warnf("clean slate: unmounting %q", mountPoint)
		cleanupScanUmount(ctx, nil, mountPoint)
	}
	// unmount "ebs-*" entrypoint last as the other mountpoint may depend on it
	for _, mountPoint := range ebsMountPoints {
		log.Warnf("clean slate: unmounting %q", mountPoint)
		cleanupScanUmount(ctx, nil, mountPoint)
	}

	for _, scanDir := range scanDirs {
		scanDirname := filepath.Join(types.ScansRootDir, scanDir.Name())
		log.Warnf("clean slate: removing directory %q", scanDirname)
		if err := os.RemoveAll(scanDirname); err != nil {
			log.Errorf("clean slate: could not remove directory %q", scanDirname)
		}
	}

	var attachedVolumes []string
	if blockDevices, err := listBlockDevices(ctx); err == nil {
		for _, bd := range blockDevices {
			if strings.HasPrefix(bd.Name, "nbd") || strings.HasPrefix(bd.Serial, "vol") {
				for _, child := range bd.getChildrenType("lvm") {
					log.Warnf("clean slate: detaching volume group %q for block device %q", child.Path, bd.Name)
					if err := exec.Command("dmsetup", "remove", child.Path).Run(); err != nil {
						log.Errorf("clean slate: could not detach virtual group %q on block device %q from dev mapper: %v", child.Path, bd.Name, err)
					}
				}
			}
			if strings.HasPrefix(bd.Name, "nbd") && len(bd.Children) > 0 {
				log.Warnf("clean slate: detaching nbd device %q", bd.Name)
				if err := exec.CommandContext(ctx, "nbd-client", "-d", path.Join("/dev", bd.Name)).Run(); err != nil {
					log.Errorf("clean slate: could not detach nbd device %q: %v", bd.Name, err)
				}
			}
			if strings.HasPrefix(bd.Serial, "vol") && len(bd.Children) > 0 {
				isScan := false
				noMount := 0
				// TODO: we could maybe rely on the output of lsblk to do our cleanup instead
				for _, child := range bd.Children {
					if len(child.Mountpoints) == 0 {
						noMount++
					} else {
						for _, mountpoint := range child.Mountpoints {
							if strings.HasPrefix(mountpoint, types.ScansRootDir+"/") {
								isScan = true
							}
						}
					}
				}
				if isScan || len(bd.Children) == noMount {
					volumeID := "vol-" + strings.TrimPrefix(bd.Serial, "vol")
					attachedVolumes = append(attachedVolumes, volumeID)
				}
			}
		}
	}

	if self, err := getSelfEC2InstanceIndentity(ctx); err == nil {
		for _, volumeID := range attachedVolumes {
			volumeARN := ec2ARN(self.Region, self.AccountID, types.ResourceTypeVolume, volumeID)
			if errd := cleanupScanDetach(ctx, nil, volumeARN, getDefaultRolesMapping()); err != nil {
				log.Warnf("clean slate: %v", errd)
			}
		}
	}

	return nil
}

func (s *sideScanner) start(ctx context.Context) {
	log.Infof("starting agentless-scanner main loop with %d scan workers", s.workers)
	defer log.Infof("stopped agentless-scanner main loop")

	s.eventForwarder.Start()
	defer s.eventForwarder.Stop()

	s.rcClient.Start()

	go func() {
		err := s.healthServer(ctx)
		if err != nil {
			log.Warnf("healthServer: %v", err)
		}
	}()

	go s.cleanupProcess(ctx)

	done := make(chan struct{})
	go func() {
		defer func() { done <- struct{}{} }()
		for result := range s.resultsCh {
			if result.Err != nil {
				if !errors.Is(result.Err, context.Canceled) {
					log.Errorf("%s: %s scanner reported a failure: %v", result.Scan, result.Scanner, result.Err)
				}
				if err := statsd.Count("datadog.agentless_scanner.scans.finished", 1.0, tagFailure(result.Scan, result.Err), 1.0); err != nil {
					log.Warnf("failed to send metric: %v", err)
				}
			} else {
				log.Infof("%s: scanner %s finished successfully (waited %s | took %s)", result.Scan, result.Scanner, result.StartedAt.Sub(result.CreatedAt), time.Since(result.StartedAt))
				if vulns := result.Vulns; vulns != nil {
					if hasResults(vulns.BOM) {
						if err := statsd.Count("datadog.agentless_scanner.scans.finished", 1.0, tagSuccess(result.Scan), 1.0); err != nil {
							log.Warnf("failed to send metric: %v", err)
						}
					} else {
						log.Debugf("%s: scanner %s finished successfully without results", result.Scan, result.Scanner)
						if err := statsd.Count("datadog.agentless_scanner.scans.finished", 1.0, tagNoResult(result.Scan), 1.0); err != nil {
							log.Warnf("failed to send metric: %v", err)
						}
					}
					if err := s.sendSBOM(result); err != nil {
						log.Errorf("%s: failed to send SBOM: %v", result.Scan, err)
					}
					if s.printResults {
						if bomRaw, err := json.MarshalIndent(vulns.BOM, "  ", "  "); err == nil {
							fmt.Printf("scanning SBOM result %s (took %s):\n", result.Scan, time.Since(result.StartedAt))
							fmt.Printf("ID: %s\n", vulns.ID)
							fmt.Printf("SourceType: %s\n", vulns.SourceType.String())
							fmt.Printf("Tags: %+q\n", vulns.Tags)
							fmt.Printf("%s\n", bomRaw)
						}
					}
				}
				if malware := result.Malware; malware != nil {
					if err := statsd.Count("datadog.agentless_scanner.scans.finished", 1.0, tagSuccess(result.Scan), 1.0); err != nil {
						log.Warnf("failed to send metric: %v", err)
					}
					log.Debugf("%s: sending findings", result.Scan)
					s.sendFindings(malware.Findings)
					if s.printResults {
						b, _ := json.MarshalIndent(malware.Findings, "", "  ")
						fmt.Printf("scanning types.Malware result %s (took %s): %s\n", result.Scan, time.Since(result.StartedAt), string(b))
					}
				}
			}
		}
	}()

	for i := 0; i < s.workers; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for scan := range s.scansCh {
				// Gather the  scanned roles / accounts as we go. We only ever
				// need to store one role associated with one region. They
				// will be used for cleanup process.
				s.regionsCleanupMu.Lock()
				if s.regionsCleanup == nil {
					s.regionsCleanup = make(map[string]*arn.ARN)
				}
				s.regionsCleanup[scan.ARN.Region] = scan.Roles[scan.ARN.Region]
				s.regionsCleanupMu.Unlock()

				// Avoid pushing a scan that we are already performing.
				// TODO: this guardrail could be avoided with a smarter scheduling.
				s.scansInProgressMu.Lock()
				if _, ok := s.scansInProgress[scan.ARN]; ok {
					s.scansInProgressMu.Unlock()
					continue
				}
				s.scansInProgress[scan.ARN] = struct{}{}
				s.scansInProgressMu.Unlock()

				if err := s.launchScan(ctx, scan); err != nil {
					if !errors.Is(err, context.Canceled) {
						log.Errorf("%s: could not be setup properly: %v", scan, err)
					}
				}

				s.scansInProgressMu.Lock()
				delete(s.scansInProgress, scan.ARN)
				s.scansInProgressMu.Unlock()
			}
		}()
	}

	go func() {
		defer close(s.scansCh)
		defer s.rcClient.Close()
		for {
			select {
			case config, ok := <-s.configsCh:
				if !ok {
					return
				}

				for _, scan := range config.Tasks {
					select {
					case <-ctx.Done():
						return
					case s.scansCh <- scan:
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	for i := 0; i < s.workers; i++ {
		<-done
	}
	close(s.resultsCh)
	<-done // waiting for done in range resultsCh goroutine
}

func (s *sideScanner) stop() {
	close(s.configsCh)
}

func (s *sideScanner) pushConfig(ctx context.Context, config *types.ScanConfig) bool {
	select {
	case s.configsCh <- config:
		return true
	case <-ctx.Done():
		return false
	}
}

func (s *sideScanner) launchScan(ctx context.Context, scan *types.ScanTask) (err error) {
	if err := statsd.Count("datadog.agentless_scanner.scans.started", 1.0, tagScan(scan), 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
	defer func() {
		if err != nil {
			if err := statsd.Count("datadog.agentless_scanner.scans.finished", 1.0, tagFailure(scan, err), 1.0); err != nil {
				log.Warnf("failed to send metric: %v", err)
			}
		}
	}()

	if err := os.MkdirAll(scan.Path(), 0700); err != nil {
		return err
	}

	pool := newScannersPool(s.scannersMax)
	scan.StartedAt = time.Now()
	defer cleanupScan(scan)
	switch scan.Type {
	case types.HostScanType:
		return scanRootFilesystems(ctx, scan, []string{scan.ARN.Resource}, pool, s.resultsCh)
	case types.EBSScanType:
		return scanEBS(ctx, scan, s.waiter, pool, s.resultsCh)
	case types.LambdaScanType:
		return scanLambda(ctx, scan, pool, s.resultsCh)
	default:
		return fmt.Errorf("unknown scan type: %s", scan.Type)
	}
}

func (s *sideScanner) sendSBOM(result types.ScanResult) error {
	vulns := result.Vulns
	sourceAgent := "agentless-scanner"
	envVarEnv := pkgconfig.Datadog.GetString("env")

	entity := &sbommodel.SBOMEntity{
		Status: sbommodel.SBOMStatus_SUCCESS,
		Type:   vulns.SourceType,
		Id:     vulns.ID,
		InUse:  true,
		DdTags: append([]string{
			"agentless_scanner_host:" + s.hostname,
			"region:" + result.Scan.ARN.Region,
			"account_id:" + result.Scan.ARN.AccountID,
		}, vulns.Tags...),
		GeneratedAt:        timestamppb.New(result.StartedAt),
		GenerationDuration: convertDuration(time.Since(result.StartedAt)),
		Hash:               "",
		Sbom: &sbommodel.SBOMEntity_Cyclonedx{
			Cyclonedx: convertBOM(vulns.BOM),
		},
	}
	rawEvent, err := proto.Marshal(&sbommodel.SBOMPayload{
		Version:  1,
		Source:   &sourceAgent,
		Entities: []*sbommodel.SBOMEntity{entity},
		DdEnv:    &envVarEnv,
	})
	if err != nil {
		return fmt.Errorf("unable to proto marhsal sbom: %w", err)
	}

	m := message.NewMessage(rawEvent, nil, "", 0)
	return s.eventForwarder.SendEventPlatformEvent(m, epforwarder.EventTypeContainerSBOM)
}

func (s *sideScanner) sendFindings(findings []*types.ScanFinding) {
	var tags []string // TODO: tags
	expireAt := time.Now().Add(24 * time.Hour)
	for _, finding := range findings {
		finding.ExpireAt = &expireAt
		finding.AgentVersion = version.AgentVersion
		s.findingsReporter.ReportEvent(finding, tags...)
	}
}

func cloudResourceTagSpec(resourceType types.ResourceType, scannerHostname string) []ec2types.TagSpecification {
	return []ec2types.TagSpecification{
		{
			ResourceType: ec2types.ResourceType(resourceType),
			Tags: []ec2types.Tag{
				{Key: aws.String("DatadogAgentlessScanner"), Value: aws.String("true")},
				{Key: aws.String("DatadogAgentlessScannerHostOrigin"), Value: aws.String(scannerHostname)},
				// TODO: add origin account and instance ID
			},
		},
	}
}

func cloudResourceTagFilters() []ec2types.Filter {
	return []ec2types.Filter{
		{
			Name: aws.String("tag:DatadogAgentlessScanner"),
			Values: []string{
				"true",
			},
		},
	}
}

func listResourcesForCleanup(ctx context.Context, ec2client *ec2.Client, maxTTL time.Duration) map[types.ResourceType][]string {
	toBeDeleted := make(map[types.ResourceType][]string)
	var nextToken *string

	for {
		volumes, err := ec2client.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{
			NextToken: nextToken,
			Filters:   cloudResourceTagFilters(),
		})
		if err != nil {
			log.Warnf("could not list volumes created by agentless-scanner: %v", err)
			break
		}
		for i := range volumes.Volumes {
			if volumes.Volumes[i].State == ec2types.VolumeStateAvailable {
				volumeID := *volumes.Volumes[i].VolumeId
				toBeDeleted[types.ResourceTypeVolume] = append(toBeDeleted[types.ResourceTypeVolume], volumeID)
			}
		}
		nextToken = volumes.NextToken
		if nextToken == nil {
			break
		}
	}

	for {
		snapshots, err := ec2client.DescribeSnapshots(ctx, &ec2.DescribeSnapshotsInput{
			NextToken: nextToken,
			Filters:   cloudResourceTagFilters(),
		})
		if err != nil {
			log.Warnf("could not list snapshots created by agentless-scanner: %v", err)
			break
		}
		for i := range snapshots.Snapshots {
			if snapshots.Snapshots[i].State != ec2types.SnapshotStateCompleted {
				continue
			}
			since := time.Now().Add(-maxTTL)
			if snapshots.Snapshots[i].StartTime != nil && snapshots.Snapshots[i].StartTime.After(since) {
				continue
			}
			snapshotID := *snapshots.Snapshots[i].SnapshotId
			toBeDeleted[types.ResourceTypeSnapshot] = append(toBeDeleted[types.ResourceTypeSnapshot], snapshotID)
		}
		nextToken = snapshots.NextToken
		if nextToken == nil {
			break
		}
	}
	return toBeDeleted
}

func cloudResourcesCleanup(ctx context.Context, ec2client *ec2.Client, toBeDeleted map[types.ResourceType][]string) {
	for resourceType, resources := range toBeDeleted {
		for _, resourceID := range resources {
			if err := ctx.Err(); err != nil {
				return
			}
			log.Infof("cleaning up resource %s/%s", resourceType, resourceID)
			var err error
			switch resourceType {
			case types.ResourceTypeSnapshot:
				_, err = ec2client.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{
					SnapshotId: aws.String(resourceID),
				})
			case types.ResourceTypeVolume:
				_, err = ec2client.DeleteVolume(ctx, &ec2.DeleteVolumeInput{
					VolumeId: aws.String(resourceID),
				})
			}
			if err != nil {
				log.Errorf("could not delete resource %s/%s: %s", resourceType, resourceID, err)
			}
		}
	}
}

func statsResourceTTL(resourceType types.ResourceType, scan *types.ScanTask, createTime time.Time) {
	ttl := time.Since(createTime)
	tags := tagScan(scan)
	tags = append(tags, fmt.Sprintf("aws_resource_type:%s", string(resourceType)))
	if err := statsd.Histogram("datadog.agentless_scanner.aws.resources_ttl", float64(ttl.Milliseconds()), tags, 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
}

func createSnapshot(ctx context.Context, scan *types.ScanTask, waiter *awsWaiter, ec2client *ec2.Client, volumeARN arn.ARN) (arn.ARN, error) {
	snapshotCreatedAt := time.Now()
	if err := statsd.Count("datadog.agentless_scanner.snapshots.started", 1.0, tagScan(scan), 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
	log.Debugf("%s: starting volume snapshotting %q", scan, volumeARN)

	retries := 0
retry:
	resourceType, volumeID, err := types.GetARNResource(volumeARN)
	if err != nil {
		return arn.ARN{}, err
	}
	if resourceType != types.ResourceTypeVolume {
		return arn.ARN{}, fmt.Errorf("bad volume ARN %q: expecting a volume ARN", volumeARN)
	}
	createSnapshotOutput, err := ec2client.CreateSnapshot(ctx, &ec2.CreateSnapshotInput{
		VolumeId:          aws.String(volumeID),
		TagSpecifications: cloudResourceTagSpec(types.ResourceTypeSnapshot, scan.ScannerHostname),
	})
	if err != nil {
		var aerr smithy.APIError
		var isRateExceededError bool
		// TODO: if we reach this error, we maybe could reuse a pending or
		// very recent snapshot that was created by the scanner.
		if errors.As(err, &aerr) && aerr.ErrorCode() == "SnapshotCreationPerVolumeRateExceeded" {
			isRateExceededError = true
		}
		if retries <= maxSnapshotRetries {
			retries++
			if isRateExceededError {
				// https://docs.aws.amazon.com/AWSEC2/latest/APIReference/errors-overview.html
				// Wait at least 15 seconds between concurrent volume snapshots.
				d := 15 * time.Second
				log.Debugf("%s: snapshot creation rate exceeded for volume %q; retrying after %v (%d/%d)", scan, volumeARN, d, retries, maxSnapshotRetries)
				if !sleepCtx(ctx, d) {
					return arn.ARN{}, ctx.Err()
				}
				goto retry
			}
		}
		if isRateExceededError {
			log.Debugf("%s: snapshot creation rate exceeded for volume %q; skipping)", scan, volumeARN)
		}
	}
	if err != nil {
		var isVolumeNotFoundError bool
		var aerr smithy.APIError
		if errors.As(err, &aerr) && aerr.ErrorCode() == "InvalidVolume.NotFound" {
			isVolumeNotFoundError = true
		}
		var tags []string
		if isVolumeNotFoundError {
			tags = tagNotFound(scan)
		} else {
			tags = tagFailure(scan, err)
		}
		if err := statsd.Count("datadog.agentless_scanner.snapshots.finished", 1.0, tags, 1.0); err != nil {
			log.Warnf("failed to send metric: %v", err)
		}
		return arn.ARN{}, err
	}

	snapshotID := *createSnapshotOutput.SnapshotId
	snapshotARN := ec2ARN(volumeARN.Region, volumeARN.AccountID, types.ResourceTypeSnapshot, snapshotID)
	scan.CreatedSnapshots[snapshotARN.String()] = &snapshotCreatedAt

	err = <-waiter.wait(ctx, snapshotARN, ec2client)
	if err == nil {
		snapshotDuration := time.Since(snapshotCreatedAt)
		log.Debugf("%s: volume snapshotting of %q finished successfully %q (took %s)", scan, volumeARN, snapshotID, snapshotDuration)
		if err := statsd.Histogram("datadog.agentless_scanner.snapshots.duration", float64(snapshotDuration.Milliseconds()), tagScan(scan), 1.0); err != nil {
			log.Warnf("failed to send metric: %v", err)
		}
		if err := statsd.Histogram("datadog.agentless_scanner.snapshots.size", float64(*createSnapshotOutput.VolumeSize), tagFailure(scan, err), 1.0); err != nil {
			log.Warnf("failed to send metric: %v", err)
		}
		if err := statsd.Count("datadog.agentless_scanner.snapshots.finished", 1.0, tagSuccess(scan), 1.0); err != nil {
			log.Warnf("failed to send metric: %v", err)
		}
	} else {
		if err := statsd.Count("datadog.agentless_scanner.snapshots.finished", 1.0, tagFailure(scan, err), 1.0); err != nil {
			log.Warnf("failed to send metric: %v", err)
		}
	}
	return snapshotARN, err
}

func tagScan(scan *types.ScanTask, rest ...string) []string {
	return append([]string{
		fmt.Sprintf("agent_version:%s", version.AgentVersion),
		fmt.Sprintf("region:%s", scan.ARN.Region),
		fmt.Sprintf("type:%s", scan.Type),
	}, rest...)
}

func tagNoResult(scan *types.ScanTask) []string {
	return tagScan(scan, "status:noresult")
}

func tagNotFound(scan *types.ScanTask) []string {
	return tagScan(scan, "status:notfound")
}

func tagFailure(scan *types.ScanTask, err error) []string {
	if errors.Is(err, context.Canceled) {
		return tagScan(scan, "status:canceled")
	}
	return tagScan(scan, "status:failure")
}

func tagSuccess(scan *types.ScanTask) []string {
	return append(tagScan(scan), "status:success")
}

type awsRoundtripStats struct {
	transport *http.Transport
	region    string
	limits    *awsLimits
	role      arn.ARN
}

func newHTTPClientWithAWSStats(region string, assumedRole *arn.ARN, limits *awsLimits) *http.Client {
	rt := &awsRoundtripStats{
		region: region,
		limits: limits,
		transport: &http.Transport{
			DisableKeepAlives:   false,
			IdleConnTimeout:     10 * time.Second,
			MaxIdleConns:        500,
			MaxConnsPerHost:     500,
			MaxIdleConnsPerHost: 500,
			TLSHandshakeTimeout: 5 * time.Second,
		},
	}
	if assumedRole != nil {
		rt.role = *assumedRole
	}
	return &http.Client{
		Timeout:   10 * time.Minute,
		Transport: rt,
	}
}

var (
	ebsGetBlockReg      = regexp.MustCompile("^/snapshots/(snap-[a-f0-9]+)/blocks/([0-9]+)$")
	ebsListBlocksReg    = regexp.MustCompile("^/snapshots/(snap-[a-f0-9]+)/blocks$")
	ebsChangedBlocksReg = regexp.MustCompile("^/snapshots/(snap-[a-f0-9]+)/changedblocks$")
)

func (rt *awsRoundtripStats) getAction(req *http.Request) (service, action string, error error) {
	host := req.URL.Host
	if strings.HasSuffix(host, ".amazonaws.com") {
		switch {
		// STS (sts.(region.)?amazonaws.com)
		case strings.HasPrefix(host, "sts."):
			return "sts", "getcalleridentity", nil

		// Lambda (lambda.(region.)?amazonaws.com)
		case strings.HasPrefix(host, "lambda."):
			return "lambda", "getfunction", nil

		case strings.HasPrefix(host, "ebs."):
			if req.Method == http.MethodGet && ebsGetBlockReg.MatchString(req.URL.Path) {
				return "ebs", "getblock", nil
			}
			if req.Method == http.MethodGet && ebsListBlocksReg.MatchString(req.URL.Path) {
				return "ebs", "listblocks", nil
			}
			if req.Method == http.MethodGet && ebsChangedBlocksReg.MatchString(req.URL.Path) {
				return "ebs", "changedblocks", nil
			}
			return "ebs", "unknown", nil

		// EC2 (ec2.(region.)?amazonaws.com): https://docs.aws.amazon.com/AWSEC2/latest/APIReference/Using_Endpoints.html
		case strings.HasPrefix(host, "ec2."):
			if req.Method == http.MethodPost && req.Body != nil {
				defer req.Body.Close()
				body, err := io.ReadAll(req.Body)
				if err != nil {
					return
				}
				req.Body = io.NopCloser(bytes.NewReader(body))
				form, err := url.ParseQuery(string(body))
				if err == nil {
					if action := form.Get("Action"); action != "" {
						return "ec2", strings.ToLower(action), nil
					}
					return "ec2", "unknown", nil
				}
			} else {
				form := req.URL.Query()
				if action := form.Get("Action"); action != "" {
					return "ec2", strings.ToLower(action), nil
				}
				return "ec2", "unknown", nil
			}
		case strings.Contains(host, ".s3.") || strings.Contains(host, ".s3-"):
			return "s3", "unknown", nil
		}
	} else if host == "169.254.169.254" {
		return "imds", "unknown", nil
	}
	return "unknown", "unknown", nil
}

func (rt *awsRoundtripStats) RoundTrip(req *http.Request) (*http.Response, error) {
	startTime := time.Now()
	service, action, err := rt.getAction(req)
	if err != nil {
		return nil, err
	}
	limiter := rt.limits.getLimiter(rt.role.AccountID, rt.region, service, action)
	throttled100 := false
	throttled1000 := false
	throttled5000 := false
	if limiter != nil {
		r := limiter.Reserve()
		if !r.OK() {
			panic("unexpected limiter with a zero burst")
		}
		if delay := r.Delay(); delay > 0 {
			throttled100 = delay > 100*time.Millisecond
			throttled1000 = delay > 1000*time.Millisecond
			throttled5000 = delay > 5000*time.Millisecond
			if !sleepCtx(req.Context(), delay) {
				return nil, req.Context().Err()
			}
		}
	}
	tags := []string{
		fmt.Sprintf("agent_version:%s", version.AgentVersion),
		fmt.Sprintf("aws_region:%s", rt.region),
		fmt.Sprintf("aws_assumed_role:%s", rt.role.Resource),
		fmt.Sprintf("aws_account_id:%s", rt.role.AccountID),
		fmt.Sprintf("aws_service:%s", service),
		fmt.Sprintf("aws_action:%s_%s", service, action),
		fmt.Sprintf("aws_throttled_100:%t", throttled100),
		fmt.Sprintf("aws_throttled_1000:%t", throttled1000),
		fmt.Sprintf("aws_throttled_5000:%t", throttled5000),
	}
	if err := statsd.Incr("datadog.agentless_scanner.aws.requests", tags, 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
	resp, err := rt.transport.RoundTrip(req)
	duration := float64(time.Since(startTime).Milliseconds())
	defer func() {
		if err := statsd.Histogram("datadog.agentless_scanner.aws.responses", duration, tags, 0.2); err != nil {
			log.Warnf("failed to send metric: %v", err)
		}
	}()
	if err != nil {
		if err == context.Canceled {
			tags = append(tags, "aws_statuscode:ctx_canceled")
		} else if err == context.DeadlineExceeded {
			tags = append(tags, "aws_statuscode:ctx_deadline_exceeded")
		} else {
			tags = append(tags, "aws_statuscode:unknown_error")
		}
		return nil, err
	}

	tags = append(tags, fmt.Sprintf("aws_statuscode:%d", resp.StatusCode))
	if resp.StatusCode >= 400 {
		switch {
		case service == "ec2" && resp.Header.Get("Content-Type") == "text/xml;charset=UTF-8":
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return resp, err
			}
			resp.Body = io.NopCloser(bytes.NewReader(body))
			var ec2Error struct {
				XMLName   xml.Name `xml:"Response"`
				RequestID string   `xml:"RequestID"`
				Errors    []struct {
					Code    string `xml:"Code"`
					Message string `xml:"Message"`
				} `xml:"Errors>Error"`
			}
			if errx := xml.Unmarshal(body, &ec2Error); errx == nil {
				for _, errv := range ec2Error.Errors {
					tags = append(tags, fmt.Sprintf("aws_ec2_errorcode:%s", strings.ToLower(errv.Code)))
				}
			}
		case service == "ebs" && resp.Header.Get("Content-Type") == "application/json":
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return resp, err
			}
			resp.Body = io.NopCloser(bytes.NewReader(body))
			// {"Message":"The snapshot 'snap-00000' does not exist.","Reason":"SNAPSHOT_NOT_FOUND"}
			var ebsError struct {
				Reason string `json:"Reason"`
			}
			if errx := json.Unmarshal(body, &ebsError); errx == nil {
				tags = append(tags, fmt.Sprintf("aws_ebs_errorcode:%s", strings.ToLower(ebsError.Reason)))
			}
		}
	}
	if contentLength, err := strconv.Atoi(resp.Header.Get("Content-Length")); err == nil {
		if err := statsd.Histogram("datadog.agentless_scanner.responses.size", float64(contentLength), tags, 0.2); err != nil {
			log.Warnf("failed to send metric: %v", err)
		}
	}
	return resp, nil
}

type awsConfigKey struct {
	role   arn.ARN
	region string
}

func newAWSConfig(ctx context.Context, region string, assumedRole *arn.ARN) (aws.Config, error) {
	awsConfigsMu.Lock()
	defer awsConfigsMu.Unlock()

	key := awsConfigKey{
		region: region,
	}
	if assumedRole != nil {
		key.role = *assumedRole
	}
	if cfg, ok := awsConfigs[key]; ok {
		return *cfg, nil
	}

	limits := newAWSLimits(getAWSLimitsOptions())
	httpClient := newHTTPClientWithAWSStats(region, assumedRole, limits)
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithHTTPClient(httpClient),
	)
	if err != nil {
		return aws.Config{}, fmt.Errorf("awsconfig: could not load default config: %w", err)
	}

	stsclient := sts.NewFromConfig(cfg)
	if assumedRole != nil {
		stsassume := stscreds.NewAssumeRoleProvider(stsclient, assumedRole.String())
		cfg.Credentials = aws.NewCredentialsCache(stsassume)
	}

	identity, err := stsclient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return aws.Config{}, fmt.Errorf("awsconfig: could not assumerole %q: %w", assumedRole, err)
	}
	log.Tracef("aws config: assuming role with arn=%q", *identity.Arn)

	if assumedRole == nil {
		roleARN, err := arn.Parse(*identity.Arn)
		if err != nil {
			return aws.Config{}, fmt.Errorf("awsconfig: could not parse caller identity arn: %w", err)
		}
		cfg.HTTPClient = newHTTPClientWithAWSStats(region, &roleARN, limits)
	}

	awsConfigs[key] = &cfg
	return cfg, nil
}

func getSelfEC2InstanceIndentity(ctx context.Context) (*imds.GetInstanceIdentityDocumentOutput, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	imdsclient := imds.NewFromConfig(cfg)
	return imdsclient.GetInstanceIdentityDocument(ctx, &imds.GetInstanceIdentityDocumentInput{})
}

func scanEBS(ctx context.Context, scan *types.ScanTask, waiter *awsWaiter, pool *scannersPool, resultsCh chan types.ScanResult) error {
	resourceType, _, err := types.GetARNResource(scan.ARN)
	if err != nil {
		return err
	}
	if scan.TargetHostname == "" {
		return fmt.Errorf("ebs-volume: missing hostname")
	}

	defer statsd.Flush()

	assumedRole := scan.Roles[scan.ARN.AccountID]
	cfg, err := newAWSConfig(ctx, scan.ARN.Region, assumedRole)
	if err != nil {
		return err
	}

	ec2client := ec2.NewFromConfig(cfg)
	if err != nil {
		return err
	}

	var snapshotARN arn.ARN
	switch resourceType {
	case types.ResourceTypeVolume:
		snapshotARN, err = createSnapshot(ctx, scan, waiter, ec2client, scan.ARN)
		if err != nil {
			return err
		}
	case types.ResourceTypeSnapshot:
		snapshotARN = scan.ARN
	default:
		return fmt.Errorf("ebs-volume: bad arn %q", scan.ARN)
	}

	if snapshotARN.Resource == "" {
		return fmt.Errorf("ebs-volume: missing snapshot ID")
	}

	log.Infof("%s: start EBS scanning", scan)

	// In types.NoAttach mode we are only able to do host vuln scanning.
	// TODO: remove this mode
	if scan.DiskMode == types.NoAttach {
		// Only vulns scanning works without a proper mount point (for now)
		for _, action := range scan.Actions {
			if action != types.VulnsHost {
				return fmt.Errorf("we can only perform vulns scanning of %q without volume attach", scan)
			}
		}
		result := pool.launchScanner(ctx, types.ScannerOptions{
			Scanner:     types.ScannerNameHostVulnsEBS,
			Scan:        scan,
			SnapshotARN: &snapshotARN,
			CreatedAt:   time.Now(),
		})
		if result.Vulns != nil {
			result.Vulns.SourceType = sbommodel.SBOMSourceType_HOST_FILE_SYSTEM
			result.Vulns.ID = scan.TargetHostname
			result.Vulns.Tags = nil
		}
		resultsCh <- result
		if err := statsd.Histogram("datadog.agentless_scanner.scans.duration", float64(time.Since(result.StartedAt).Milliseconds()), tagScan(scan), 1.0); err != nil {
			log.Warnf("failed to send metric: %v", err)
		}
		return nil
	}

	switch scan.DiskMode {
	case types.VolumeAttach:
		if err := attachSnapshotWithVolume(ctx, scan, waiter, snapshotARN); err != nil {
			return err
		}
	case types.NBDAttach:
		ebsclient := ebs.NewFromConfig(cfg)
		if err := attachSnapshotWithNBD(ctx, scan, snapshotARN, ebsclient); err != nil {
			return err
		}
	default:
		panic("unreachable")
	}

	partitions, err := listDevicePartitions(ctx, scan)
	if err != nil {
		return err
	}

	mountPoints, err := mountDevice(ctx, scan, partitions)
	if err != nil {
		return err
	}

	return scanRootFilesystems(ctx, scan, mountPoints, pool, resultsCh)
}

func scanRootFilesystems(ctx context.Context, scan *types.ScanTask, roots []string, pool *scannersPool, resultsCh chan types.ScanResult) error {
	var wg sync.WaitGroup

	scanRoot := func(root string, action types.ScanAction) {
		defer wg.Done()

		switch action {
		case types.VulnsHost:
			result := pool.launchScanner(ctx, types.ScannerOptions{
				Scanner:   types.ScannerNameHostVulns,
				Scan:      scan,
				Root:      root,
				CreatedAt: time.Now(),
			})
			if result.Vulns != nil {
				result.Vulns.SourceType = sbommodel.SBOMSourceType_HOST_FILE_SYSTEM
				result.Vulns.ID = scan.TargetHostname
				result.Vulns.Tags = nil
			}
			resultsCh <- result
		case types.VulnsContainers:
			ctrResult := pool.launchScanner(ctx, types.ScannerOptions{
				Scanner:   types.ScannerNameContainers,
				Scan:      scan,
				Root:      root,
				CreatedAt: time.Now(),
			})
			if ctrResult.Err != nil {
				resultsCh <- ctrResult
			} else if len(ctrResult.Containers.Containers) > 0 {
				log.Infof("%s: found %d containers on %q", scan, len(ctrResult.Containers.Containers), root)
				runtimes := make(map[string]int64)
				for _, ctr := range ctrResult.Containers.Containers {
					runtimes[ctr.Runtime]++
				}
				for runtime, count := range runtimes {
					tags := tagScan(scan, fmt.Sprintf("container_runtime:%s", runtime))
					if err := statsd.Count("datadog.agentless_scanner.containers.count", count, tags, 1.0); err != nil {
						log.Warnf("failed to send metric: %v", err)
					}
				}
				for _, ctr := range ctrResult.Containers.Containers {
					wg.Add(1)
					go func(ctr types.Container) {
						defer wg.Done()
						resultsCh <- pool.launchScanner(ctx, types.ScannerOptions{
							Scanner:   types.ScannerNameContainerVulns,
							Scan:      scan,
							Root:      root,
							Container: &ctr,
							CreatedAt: time.Now(),
						})
					}(*ctr)
				}
			}
		case types.Malware:
			resultsCh <- pool.launchScanner(ctx, types.ScannerOptions{
				Scanner:   types.ScannerNameMalware,
				Scan:      scan,
				Root:      root,
				CreatedAt: time.Now(),
			})
		}
	}

	for _, root := range roots {
		for _, action := range scan.Actions {
			wg.Add(1)
			go scanRoot(root, action)
		}
	}
	wg.Wait()

	if err := statsd.Histogram("datadog.agentless_scanner.scans.duration", float64(time.Since(scan.StartedAt).Milliseconds()), tagScan(scan), 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
	return nil
}

type scannersPool struct {
	sem chan struct{}
}

func newScannersPool(size int) *scannersPool {
	return &scannersPool{make(chan struct{}, size)}
}

func (p *scannersPool) launchScanner(ctx context.Context, opts types.ScannerOptions) types.ScanResult {
	select {
	case p.sem <- struct{}{}:
	case <-ctx.Done():
		return opts.ErrResult(ctx.Err())
	}

	opts.StartedAt = time.Now()
	ch := make(chan types.ScanResult, 1)
	go func() {
		var result types.ScanResult
		if globalParams.noForkScanners {
			result = launchScannerInSameProcess(ctx, opts)
		} else {
			result = launchScannerInChildProcess(ctx, opts)
		}
		<-p.sem
		ch <- result
	}()

	select {
	case result := <-ch:
		return result
	case <-ctx.Done():
		return opts.ErrResult(ctx.Err())
	}
}

func launchScannerInSameProcess(ctx context.Context, opts types.ScannerOptions) types.ScanResult {
	switch opts.Scanner {
	case types.ScannerNameHostVulns:
		bom, err := scanners.LaunchTrivyHost(ctx, opts)
		if err != nil {
			return opts.ErrResult(err)
		}
		return types.ScanResult{ScannerOptions: opts, Vulns: &types.ScanVulnsResult{BOM: bom}}

	case types.ScannerNameHostVulnsEBS:
		assumedRole := opts.Scan.Roles[opts.Scan.ARN.AccountID]
		cfg, err := newAWSConfig(ctx, opts.Scan.ARN.Region, assumedRole)
		if err != nil {
			return opts.ErrResult(err)
		}

		ebsclient := ebs.NewFromConfig(cfg)
		bom, err := scanners.LaunchTrivyHostVM(ctx, opts, ebsclient)
		if err != nil {
			return opts.ErrResult(err)
		}
		return types.ScanResult{ScannerOptions: opts, Vulns: &types.ScanVulnsResult{BOM: bom}}

	case types.ScannerNameContainerVulns:
		ctr := *opts.Container
		mountPoint, err := scanners.MountContainer(ctx, opts.Scan, ctr)
		if err != nil {
			return opts.ErrResult(err)
		}
		var bom *cdx.BOM
		{
			opts := opts
			opts.Root = mountPoint
			bom, err = scanners.LaunchTrivyHost(ctx, opts)
			if err != nil {
				return opts.ErrResult(err)
			}
		}

		// We cleanup overlays as we go instead of acumulating them. However
		// the cleanupScan routine also cleans up any leftover. We do not rely
		// on the parent ctx as we still want to clean these mounts even for a
		// canceled/timeouted context.
		cleanupctx, abort := context.WithTimeout(context.Background(), 5*time.Second)
		cleanupScanUmount(cleanupctx, opts.Scan, mountPoint)
		abort()

		refTag := ctr.ImageRefTagged.Reference().(reference.NamedTagged)
		refCan := ctr.ImageRefCanonical.Reference().(reference.Canonical)
		// Tracking some examples as reference:
		//     Name:  public.ecr.aws/datadog/agent
		//      Tag:  3
		//      Domain:  public.ecr.aws
		//      Path:  datadog/agent
		//      FamiliarName:  public.ecr.aws/datadog/agent
		//      FamiliarString:  public.ecr.aws/datadog/agent:3
		//     Name:  docker.io/library/python
		//      Tag:  3
		//      Domain:  docker.io
		//      Path:  library/python
		//      FamiliarName:  python
		//      FamiliarString:  python:3
		tags := []string{
			"image_id:" + refCan.String(),                      // public.ecr.aws/datadog/agent@sha256:052f1fdf4f9a7117d36a1838ab60782829947683007c34b69d4991576375c409
			"image_name:" + refTag.Name(),                      // public.ecr.aws/datadog/agent
			"image_registry:" + reference.Domain(refTag),       // public.ecr.aws
			"image_repository:" + reference.Path(refTag),       // datadog/agent
			"short_image:" + path.Base(reference.Path(refTag)), // agent
			"repo_digest:" + refCan.Digest().String(),          // sha256:052f1fdf4f9a7117d36a1838ab60782829947683007c34b69d4991576375c409
			"image_tag:" + refTag.Tag(),                        // 7-rc
			"container_name:" + ctr.ContainerName,
		}
		// TODO: remove this when we backport
		// https://github.com/DataDog/datadog-agent/pull/22161
		appendSBOMRepoMetadata(bom, refTag, refCan)

		vulns := &types.ScanVulnsResult{
			BOM:        bom,
			SourceType: sbommodel.SBOMSourceType_CONTAINER_IMAGE_LAYERS, // TODO: sbommodel.SBOMSourceType_CONTAINER_FILE_SYSTEM
			ID:         ctr.ImageRefCanonical.Reference().String(),
			Tags:       tags,
		}
		return types.ScanResult{ScannerOptions: opts, Vulns: vulns}

	case types.ScannerNameAppVulns:
		bom, err := scanners.LaunchTrivyApp(ctx, opts)
		if err != nil {
			return opts.ErrResult(err)
		}
		return types.ScanResult{ScannerOptions: opts, Vulns: &types.ScanVulnsResult{BOM: bom}}

	case types.ScannerNameContainers:
		containers, err := scanners.LaunchContainers(ctx, opts)
		if err != nil {
			return opts.ErrResult(err)
		}
		return types.ScanResult{ScannerOptions: opts, Containers: &containers}

	case types.ScannerNameMalware:
		result, err := scanners.LaunchMalware(ctx, opts)
		if err != nil {
			return opts.ErrResult(err)
		}
		return types.ScanResult{ScannerOptions: opts, Malware: &result}
	default:
		panic("unreachable")
	}
}

func launchScannerInChildProcess(ctx context.Context, opts types.ScannerOptions) types.ScanResult {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	exe, err := os.Executable()
	if err != nil {
		return opts.ErrResult(err)
	}

	sockName := filepath.Join(opts.Scan.Path(opts.ID() + ".sock"))
	l, err := net.Listen("unix", sockName)
	if err != nil {
		return opts.ErrResult(err)
	}
	defer l.Close()

	remoteCall := func() types.ScanResult {
		var result types.ScanResult

		conn, err := l.Accept()
		if err != nil {
			return opts.ErrResult(err)
		}
		defer conn.Close()

		deadline, ok := ctx.Deadline()
		if ok {
			_ = conn.SetDeadline(deadline)
		}

		enc := json.NewEncoder(conn)
		dec := json.NewDecoder(conn)
		if err := enc.Encode(opts); err != nil {
			return opts.ErrResult(err)
		}
		if err := dec.Decode(&result); err != nil {
			return opts.ErrResult(err)
		}
		return result
	}

	resultsCh := make(chan types.ScanResult, 1)
	go func() {
		resultsCh <- remoteCall()
	}()

	stderr := &truncatedWriter{max: 512 * 1024}
	cmd := exec.CommandContext(ctx, exe, "run-scanner", "--sock", sockName)
	cmd.Env = []string{
		"GOMAXPROCS=1",
		"DD_LOG_FILE=" + opts.Scan.Path(fmt.Sprintf("scanner-%s.log", opts.ID())),
		"PATH=" + os.Getenv("PATH"),
	}
	cmd.Dir = opts.Scan.Path()
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return opts.ErrResult(err)
	}

	if err := cmd.Start(); err != nil {
		if ctx.Err() != nil {
			return opts.ErrResult(ctx.Err())
		}
		return opts.ErrResult(err)
	}

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			// remove the info log at startup from log component spamming "0 Features detected from environment"
			if len(line) < 256 && strings.Contains(line, "Features detected from environment") {
				continue
			}
			// should start with "XXXX-XX-XX XX:XX:XX UTC | AGENTLESSSCANER |"
			if len(line) > 24 && strings.HasPrefix(line[24:], "| "+loggerName+" |") {
				fmt.Println(line)
			} else {
				log.Warnf("%s: scanner %q malformed stdout: %s", opts.Scan, opts.ID(), line)
			}
		}
	}()

	pid := cmd.Process.Pid
	if err := os.WriteFile(opts.Scan.Path(opts.ID()+".pid"), []byte(strconv.Itoa(pid)), 0600); err != nil {
		log.Warnf("%s: could not write pid file %d: %v", opts.Scan, cmd.Process.Pid, err)
	}

	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return opts.ErrResult(ctx.Err())
		}
		var errx *exec.ExitError
		if errors.As(err, &errx) {
			stderrx := strings.ReplaceAll(stderr.String(), "\n", "\\n")
			log.Errorf("%s: execed scanner %q with pid=%d: %v: with output:%s", opts.Scan, opts.Scanner, cmd.Process.Pid, errx, stderrx)
		} else {
			log.Errorf("%s: execed scanner %q: %v", opts.Scan, opts.Scanner, err)
		}
		return opts.ErrResult(err)
	}

	return <-resultsCh
}

type truncatedWriter struct {
	max int
	buf bytes.Buffer
}

func (w *truncatedWriter) String() string {
	return w.buf.String()
}

func (w *truncatedWriter) Write(b []byte) (n int, err error) {
	remaining := w.max - len(w.buf.Bytes())
	if remaining > 0 {
		if remaining <= len(b) {
			w.buf.Write(b[:remaining])
			w.buf.WriteString("... truncated")
		} else {
			w.buf.Write(b)
		}
	}
	return len(b), nil
}

func attachSnapshotWithNBD(_ context.Context, scan *types.ScanTask, snapshotARN arn.ARN, ebsclient *ebs.Client) error {
	device, ok := nextNBDDevice()
	if !ok {
		return fmt.Errorf("could not find non busy NBD block device")
	}
	backend, err := nbd.NewEBSBackend(ebsclient, snapshotARN)
	if err != nil {
		return err
	}
	if err := nbd.StartNBDBlockDevice(scan.ID, device, backend); err != nil {
		return err
	}
	scan.AttachedDeviceName = &device
	return nil
}

// reference: https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/device_naming.html
var xenDeviceName struct {
	sync.Mutex
	count int
}

var nbdDeviceName struct {
	sync.Mutex
	count   int
	nbdsMax *int
}

func nextXenDevice() (string, bool) {
	xenDeviceName.Lock()
	defer xenDeviceName.Unlock()

	// loops from "xvdaa" to "xvddx"
	// we found out that xvddy and xvddz are problematic for some undocumented reason
	const xenMax = ('d'-'a'+1)*26 - 2
	count := xenDeviceName.count % xenMax
	dev := 'a' + uint8(count/26)
	rst := 'a' + uint8(count%26)
	bdPath := fmt.Sprintf("/dev/xvd%c%c", dev, rst)
	// TODO: just like for NBD devices, we should ensure that the
	// associated device is not already busy. However on ubuntu AMIs there
	// is no udev rule making the proper symlink from /dev/xvdxx device to
	// the /dev/nvmex created block device on volume attach.
	xenDeviceName.count = (count + 1) % xenMax
	return bdPath, true
}

func nextNBDDevice() (string, bool) {
	nbdDeviceName.Lock()
	defer nbdDeviceName.Unlock()

	// Init phase: counting the number of nbd devices created.
	if nbdDeviceName.nbdsMax == nil {
		bds, _ := filepath.Glob("/dev/nbd*")
		bdsCount := len(bds)
		nbdDeviceName.nbdsMax = &bdsCount
	}

	nbdsMax := *nbdDeviceName.nbdsMax
	if nbdsMax == 0 {
		log.Error("could not locate any NBD block device in /dev")
		return "", false
	}

	for i := 0; i < nbdsMax; i++ {
		count := (nbdDeviceName.count + i) % nbdsMax
		// From man 2 open: O_EXCL: ... on Linux 2.6 and later, O_EXCL can be
		// used without  O_CREAT  if pathname refers to  a block device.  If
		// the block device is in use by the system (e.g., mounted), open()
		// fails with the error EBUSY.
		bdPath := fmt.Sprintf("/dev/nbd%d", count)
		f, err := os.OpenFile(bdPath, os.O_RDONLY|os.O_EXCL, 0600)
		if err == nil {
			f.Close()
			nbdDeviceName.count = (count + 1) % nbdsMax
			return bdPath, true
		}
	}
	return "", false
}

func scanLambda(ctx context.Context, scan *types.ScanTask, pool *scannersPool, resultsCh chan types.ScanResult) error {
	defer statsd.Flush()

	lambdaDir := scan.Path()
	if err := os.MkdirAll(lambdaDir, 0700); err != nil {
		return err
	}

	codePath, err := downloadAndUnzipLambda(ctx, scan, lambdaDir)
	if err != nil {
		return err
	}

	result := pool.launchScanner(ctx, types.ScannerOptions{
		Scanner:   types.ScannerNameAppVulns,
		Scan:      scan,
		Root:      codePath,
		CreatedAt: time.Now(),
	})
	if result.Vulns != nil {
		result.Vulns.SourceType = sbommodel.SBOMSourceType_CI_PIPELINE // TODO: SBOMSourceType_LAMBDA
		result.Vulns.ID = scan.ARN.String()
		result.Vulns.Tags = []string{
			"runtime_id:" + scan.ARN.String(),
			"service_version:TODO", // XXX
		}
	}
	resultsCh <- result

	if err := statsd.Histogram("datadog.agentless_scanner.scans.duration", float64(time.Since(scan.StartedAt).Milliseconds()), tagScan(scan), 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
	return nil
}

func downloadAndUnzipLambda(ctx context.Context, scan *types.ScanTask, lambdaDir string) (codePath string, err error) {
	if err := statsd.Count("datadog.agentless_scanner.functions.started", 1.0, tagScan(scan), 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
	defer func() {
		if err != nil {
			var isResourceNotFoundError bool
			var aerr smithy.APIError
			if errors.As(err, &aerr) && aerr.ErrorCode() == "ResourceNotFoundException" {
				isResourceNotFoundError = true
			}
			var tags []string
			if isResourceNotFoundError {
				tags = tagNotFound(scan)
			} else {
				tags = tagFailure(scan, err)
			}
			if err := statsd.Count("datadog.agentless_scanner.functions.finished", 1.0, tags, 1.0); err != nil {
				log.Warnf("failed to send metric: %v", err)
			}
		} else {
			if err := statsd.Count("datadog.agentless_scanner.functions.finished", 1.0, tagSuccess(scan), 1.0); err != nil {
				log.Warnf("failed to send metric: %v", err)
			}
		}
	}()

	assumedRole := scan.Roles[scan.ARN.AccountID]
	cfg, err := newAWSConfig(ctx, scan.ARN.Region, assumedRole)
	if err != nil {
		return "", err
	}

	lambdaclient := lambda.NewFromConfig(cfg)
	if err != nil {
		return "", err
	}

	lambdaFunc, err := lambdaclient.GetFunction(ctx, &lambda.GetFunctionInput{
		FunctionName: aws.String(scan.ARN.String()),
	})
	if err != nil {
		return "", err
	}

	if lambdaFunc.Code.ImageUri != nil {
		return "", fmt.Errorf("lambda: OCI images are not supported")
	}
	if lambdaFunc.Code.Location == nil {
		return "", fmt.Errorf("lambda: no code location")
	}

	archivePath := filepath.Join(lambdaDir, "code.zip")
	log.Tracef("%s: creating file %q", scan, archivePath)
	archiveFile, err := os.OpenFile(archivePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	defer archiveFile.Close()

	lambdaURL := *lambdaFunc.Code.Location
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, lambdaURL, nil)
	if err != nil {
		return "", err
	}

	log.Tracef("%s: downloading code", scan)
	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("lambda: bad status: %s", resp.Status)
	}

	log.Tracef("%s: copying code archive to %q", scan, archivePath)
	compressedSize, err := io.Copy(archiveFile, resp.Body)
	if err != nil {
		return "", err
	}

	codePath = filepath.Join(lambdaDir, "code")
	err = os.Mkdir(codePath, 0700)
	if err != nil {
		return "", err
	}

	log.Tracef("%s: extracting code in %q", scan, codePath)
	uncompressedSize, err := extractLambdaZip(ctx, archivePath, codePath)
	if err != nil {
		return "", err
	}

	log.Debugf("%s: function retrieved successfully (took %s)", scan, time.Since(scan.StartedAt))
	if err := statsd.Histogram("datadog.agentless_scanner.functions.duration", float64(time.Since(scan.StartedAt).Milliseconds()), tagScan(scan), 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
	if err := statsd.Histogram("datadog.agentless_scanner.functions.size_compressed", float64(compressedSize), tagScan(scan), 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
	if err := statsd.Histogram("datadog.agentless_scanner.functions.size_uncompressed", float64(uncompressedSize), tagScan(scan), 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}

	log.Debugf("%s: downloaded and extracted code ; compressed_size=%d uncompressed_size=%d", scan, compressedSize, uncompressedSize)
	return codePath, nil
}

func extractLambdaZip(ctx context.Context, zipPath, destinationPath string) (uint64, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return 0, fmt.Errorf("extractLambdaZip: openreader: %w", err)
	}
	defer r.Close()

	var uncompressed uint64
	for _, f := range r.File {
		if ctx.Err() != nil {
			return uncompressed, ctx.Err()
		}
		name := filepath.Join("/", f.Name)[1:]
		dest := filepath.Join(destinationPath, name)
		destDir := filepath.Dir(dest)
		if err := os.MkdirAll(destDir, 0700); err != nil {
			return uncompressed, err
		}
		if strings.HasSuffix(f.Name, "/") {
			if err := os.Mkdir(dest, 0700); err != nil {
				return uncompressed, err
			}
		} else {
			reader, err := f.Open()
			if err != nil {
				return uncompressed, fmt.Errorf("extractLambdaZip: open: %w", err)
			}
			defer reader.Close()
			writer, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
			if err != nil {
				return uncompressed, fmt.Errorf("extractLambdaZip: write: %w", err)
			}
			defer writer.Close()
			if uncompressed+f.UncompressedSize64 > maxLambdaUncompressed {
				return uncompressed, fmt.Errorf("extractLambdaZip: uncompressed size is too big")
			}
			n, err := io.Copy(writer, reader)
			uncompressed += uint64(n)
			if err != nil {
				return uncompressed, fmt.Errorf("extractLambdaZip: copy: %w", err)
			}
		}
	}
	return uncompressed, nil
}

func attachSnapshotWithVolume(ctx context.Context, scan *types.ScanTask, waiter *awsWaiter, snapshotARN arn.ARN) error {
	resourceType, snapshotID, err := types.GetARNResource(snapshotARN)
	if err != nil {
		return err
	}
	if resourceType != types.ResourceTypeSnapshot {
		return fmt.Errorf("expected ARN for a snapshot: %s", snapshotARN.String())
	}

	self, err := getSelfEC2InstanceIndentity(ctx)
	if err != nil {
		return fmt.Errorf("could not get EC2 instance identity: using attach volumes cannot work outside an EC2 instance: %w", err)
	}

	remoteAssumedRole := scan.Roles[snapshotARN.AccountID]
	remoteAWSCfg, err := newAWSConfig(ctx, snapshotARN.Region, remoteAssumedRole)
	if err != nil {
		return err
	}
	remoteEC2Client := ec2.NewFromConfig(remoteAWSCfg)

	var localSnapshotARN arn.ARN
	if snapshotARN.Region != self.Region {
		log.Debugf("%s: copying snapshot %q into %q", scan, snapshotARN, self.Region)
		copySnapshotCreatedAt := time.Now()
		copySnapshot, err := remoteEC2Client.CopySnapshot(ctx, &ec2.CopySnapshotInput{
			SourceRegion: aws.String(snapshotARN.Region),
			// DestinationRegion: aws.String(self.Region): automatically filled by SDK
			SourceSnapshotId:  aws.String(snapshotID),
			TagSpecifications: cloudResourceTagSpec(types.ResourceTypeSnapshot, scan.ScannerHostname),
		})
		if err != nil {
			return fmt.Errorf("could not copy snapshot %q to %q: %w", snapshotARN, self.Region, err)
		}
		localSnapshotARN = ec2ARN(self.Region, snapshotARN.AccountID, types.ResourceTypeSnapshot, *copySnapshot.SnapshotId)
		log.Debugf("%s: waiting for copy of snapshot %q into %q as %q", scan, snapshotARN, self.Region, *copySnapshot.SnapshotId)
		err = <-waiter.wait(ctx, localSnapshotARN, remoteEC2Client)
		if err != nil {
			return fmt.Errorf("could not finish copying %q to %q as %q: %w", snapshotARN, self.Region, *copySnapshot.SnapshotId, err)
		}
		log.Debugf("%s: successfully copied snapshot %q into %q: %q", scan, snapshotARN, self.Region, *copySnapshot.SnapshotId)
		scan.CreatedSnapshots[localSnapshotARN.String()] = &copySnapshotCreatedAt
	} else {
		localSnapshotARN = snapshotARN
	}

	if localSnapshotARN.AccountID != "" && localSnapshotARN.AccountID != self.AccountID {
		_, err = remoteEC2Client.ModifySnapshotAttribute(ctx, &ec2.ModifySnapshotAttributeInput{
			SnapshotId:    aws.String(snapshotID),
			Attribute:     ec2types.SnapshotAttributeNameCreateVolumePermission,
			OperationType: ec2types.OperationTypeAdd,
			UserIds:       []string{self.AccountID},
		})
		if err != nil {
			return fmt.Errorf("could not modify snapshot attributes %q for sharing with account ID %q: %w", localSnapshotARN, self.AccountID, err)
		}
	}

	localAssumedRole := scan.Roles[self.AccountID]
	localAWSCfg, err := newAWSConfig(ctx, self.Region, localAssumedRole)
	if err != nil {
		return err
	}
	locaEC2Client := ec2.NewFromConfig(localAWSCfg)

	log.Debugf("%s: creating new volume for snapshot %q in az %q", scan, localSnapshotARN, self.AvailabilityZone)
	_, localSnapshotID, _ := types.GetARNResource(localSnapshotARN)
	volume, err := locaEC2Client.CreateVolume(ctx, &ec2.CreateVolumeInput{
		VolumeType:        ec2types.VolumeTypeGp3,
		AvailabilityZone:  aws.String(self.AvailabilityZone),
		SnapshotId:        aws.String(localSnapshotID),
		TagSpecifications: cloudResourceTagSpec(types.ResourceTypeVolume, scan.ScannerHostname),
	})
	if err != nil {
		return fmt.Errorf("could not create volume from snapshot: %w", err)
	}

	volumeARN := ec2ARN(localSnapshotARN.Region, localSnapshotARN.AccountID, types.ResourceTypeVolume, *volume.VolumeId)
	scan.AttachedVolumeARN = &volumeARN
	scan.AttachedVolumeCreatedAt = volume.CreateTime

	device, ok := nextXenDevice()
	if !ok {
		return fmt.Errorf("could not find non busy XEN block device")
	}
	scan.AttachedDeviceName = &device

	log.Debugf("%s: attaching volume %q into device %q", scan, *volume.VolumeId, device)
	var errAttach error
	for i := 0; i < maxAttachRetries; i++ {
		sleep := 2 * time.Second
		if !sleepCtx(ctx, sleep) {
			return ctx.Err()
		}
		_, errAttach = locaEC2Client.AttachVolume(ctx, &ec2.AttachVolumeInput{
			InstanceId: aws.String(self.InstanceID),
			VolumeId:   volume.VolumeId,
			Device:     aws.String(device),
		})
		if errAttach == nil {
			log.Debugf("%s: volume attached successfully %q device=%s", scan, *volume.VolumeId, device)
			break
		}
		var aerr smithy.APIError
		// NOTE(jinroh): we're trying to attach a volume in not yet in an
		// 'available' state. Continue.
		if errors.As(errAttach, &aerr) && aerr.ErrorCode() == "IncorrectState" {
			log.Tracef("%s: couldn't attach volume %q into device %q; retrying after %v (%d/%d)", scan, *volume.VolumeId, device, sleep, i+1, maxAttachRetries)
		} else {
			break
		}
	}
	if errAttach != nil {
		return fmt.Errorf("could not attach volume %q into device %q: %w", *volume.VolumeId, device, errAttach)
	}

	return nil
}

type devicePartition struct {
	devicePath string
	fsType     string
}

type blockDevice struct {
	Name        string         `json:"name"`
	Serial      string         `json:"serial"`
	Path        string         `json:"path"`
	Type        string         `json:"type"`
	FsType      string         `json:"fstype"`
	Mountpoints []string       `json:"mountpoints"`
	Children    []*blockDevice `json:"children"`
}

func (bd blockDevice) getChildrenType(t string) []blockDevice {
	var bds []blockDevice
	bd.recurse(func(child blockDevice) {
		if child.Type == t {
			for _, b := range bds {
				if b.Path == child.Path {
					return
				}
			}
			bds = append(bds, child)
		}
	})
	return bds
}

func (bd blockDevice) recurse(cb func(blockDevice)) {
	for _, child := range bd.Children {
		child.recurse(cb)
	}
	cb(bd)
}

func listBlockDevices(ctx context.Context, deviceName ...string) ([]blockDevice, error) {
	var blockDevices struct {
		BlockDevices []blockDevice `json:"blockdevices"`
	}
	_, _ = exec.Command("udevadm", "settle", "--timeout=1").CombinedOutput()
	lsblkArgs := []string{"--json", "--bytes", "--output", "NAME,SERIAL,PATH,TYPE,FSTYPE,MOUNTPOINTS"}
	lsblkArgs = append(lsblkArgs, deviceName...)
	lsblkJSON, err := exec.CommandContext(ctx, "lsblk", lsblkArgs...).Output()
	if err != nil {
		var errx *exec.ExitError
		if errors.As(err, &errx) && errx.ExitCode() == 32 { // none of specified devices found
			return nil, nil
		}
		if !errors.Is(err, context.Canceled) {
			log.Warnf("lsblk exited with error: %v", err)
		}
		return nil, fmt.Errorf("lsblk exited with error: %w", err)
	}
	if err := json.Unmarshal(lsblkJSON, &blockDevices); err != nil {
		return nil, fmt.Errorf("lsblk output parsing error: %w", err)
	}
	// lsblk can return [null] as mountpoints list. We need to clean this up.
	for _, bd := range blockDevices.BlockDevices {
		for _, child := range bd.Children {
			mountpoints := child.Mountpoints
			child.Mountpoints = make([]string, 0, len(mountpoints))
			for _, mp := range mountpoints {
				if mp != "" {
					child.Mountpoints = append(child.Mountpoints, mp)
				}
			}
		}
	}
	return blockDevices.BlockDevices, nil
}

func listDevicePartitions(ctx context.Context, scan *types.ScanTask) ([]devicePartition, error) {
	device, volumeARN := *scan.AttachedDeviceName, scan.AttachedVolumeARN
	log.Debugf("%s: listing partitions from device %q (volume = %q)", scan, device, volumeARN)

	// NOTE(jinroh): we identified that on some Linux kernel the device path
	// may not be the expected one (passed to AttachVolume). The kernel may
	// map the block device to another path. However, the serial number
	// associated with the volume is always of the form volXXX (not vol-XXX).
	// So we use both the expected device path AND the serial number to find
	// the actual block device path.
	var serialNumber *string
	if volumeARN != nil {
		_, volumeID, _ := types.GetARNResource(*volumeARN)
		sn := "vol" + strings.TrimPrefix(volumeID, "vol-") // vol-XXX => volXXX
		serialNumber = &sn
	}

	var foundBlockDevice *blockDevice
	for i := 0; i < 120; i++ {
		if !sleepCtx(ctx, 500*time.Millisecond) {
			return nil, ctx.Err()
		}
		blockDevices, err := listBlockDevices(ctx)
		if err != nil {
			continue
		}
		for _, bd := range blockDevices {
			if serialNumber != nil && bd.Serial != "" {
				if bd.Serial == *serialNumber {
					foundBlockDevice = &bd
					break
				}
			} else if bd.Path == device {
				foundBlockDevice = &bd
				break
			}
		}

		if foundBlockDevice != nil {
			break
		}
	}
	if foundBlockDevice == nil {
		return nil, fmt.Errorf("could not find the block device %s for (volume=%q)", device, volumeARN)
	}

	// The attached device name may not be the one we expect. We update it.
	if scan.AttachedDeviceName == nil || foundBlockDevice.Path != *scan.AttachedDeviceName {
		scan.AttachedDeviceName = &foundBlockDevice.Path
	}

	var partitions []devicePartition
	for i := 0; i < 5; i++ {
		blockDevices, err := listBlockDevices(ctx, foundBlockDevice.Path)
		if err != nil {
			continue
		}
		if len(blockDevices) != 1 {
			continue
		}
		for _, part := range blockDevices[0].Children {
			if part.FsType == "btrfs" || part.FsType == "ext2" || part.FsType == "ext3" || part.FsType == "ext4" || part.FsType == "xfs" {
				partitions = append(partitions, devicePartition{
					devicePath: part.Path,
					fsType:     part.FsType,
				})
			}
		}
		if len(partitions) > 0 {
			break
		}
		if !sleepCtx(ctx, 100*time.Millisecond) {
			return nil, ctx.Err()
		}
	}
	if len(partitions) == 0 {
		return nil, fmt.Errorf("could not find any btrfs, ext2, ext3, ext4 or xfs partition in %s (volume = %q)", device, volumeARN)
	}

	log.Debugf("%s: found %d compatible partitions for device %q", scan, len(partitions), device)
	return partitions, nil
}

func mountDevice(ctx context.Context, scan *types.ScanTask, partitions []devicePartition) ([]string, error) {
	var mountPoints []string
	for _, mp := range partitions {
		mountPoint := scan.Path(types.EBSMountPrefix + path.Base(mp.devicePath))
		if err := os.MkdirAll(mountPoint, 0700); err != nil {
			return nil, fmt.Errorf("could not create mountPoint directory %q: %w", mountPoint, err)
		}

		fsOptions := "ro,noauto,nodev,noexec,nosuid," // these are generic options supported for all filesystems
		switch mp.fsType {
		case "btrfs":
			// TODO: we could implement support for multiple BTRFS subvolumes in the future.
			fsOptions += "subvol=/root"
		case "ext2":
			// nothing
		case "ext3", "ext4":
			// noload means we do not try to load the journal
			fsOptions += "noload"
		case "xfs":
			// norecovery means we do not try to recover the FS
			fsOptions += "norecovery,nouuid"
		default:
			panic(fmt.Errorf("unsupported filesystem type %s", mp.fsType))
		}

		if mp.fsType == "btrfs" {
			// Replace fsid of btrfs partition with randomly generated UUID.
			log.Debugf("%s: execing btrfstune -f -u %s", scan, mp.devicePath)
			_, err := exec.CommandContext(ctx, "btrfstune", "-f", "-u", mp.devicePath).CombinedOutput()
			if err != nil {
				return nil, err
			}

			// Clear the tree log, to prevent "failed to read log tree" warning, which leads to "open_ctree failed" error.
			log.Debugf("%s: execing btrfs rescue zero-log %s", scan, mp.devicePath)
			_, err = exec.CommandContext(ctx, "btrfs", "rescue", "zero-log", mp.devicePath).CombinedOutput()
			if err != nil {
				return nil, err
			}
		}

		mountCmd := []string{"-o", fsOptions, "-t", mp.fsType, "--source", mp.devicePath, "--target", mountPoint}
		log.Debugf("%s: execing mount %s", scan, mountCmd)

		var mountOutput []byte
		var errm error
		for i := 0; i < 50; i++ {
			// using context.Background() here as we do not want to sigkill
			// the "mount" command during work.
			mountOutput, errm = exec.CommandContext(context.Background(), "mount", mountCmd...).CombinedOutput()
			if errm == nil {
				break
			}
			if !sleepCtx(ctx, 200*time.Millisecond) {
				errm = ctx.Err()
				break
			}
		}
		if errm != nil {
			return nil, fmt.Errorf("could not mount into target=%q device=%q output=%q: %w", mountPoint, mp.devicePath, string(mountOutput), errm)
		}
		mountPoints = append(mountPoints, mountPoint)
	}
	return mountPoints, nil
}

func cleanupScanDetach(ctx context.Context, maybeScan *types.ScanTask, volumeARN arn.ARN, roles types.RolesMapping) error {
	_, volumeID, _ := types.GetARNResource(volumeARN)
	cfg, err := newAWSConfig(ctx, volumeARN.Region, roles[volumeARN.AccountID])
	if err != nil {
		return err
	}

	ec2client := ec2.NewFromConfig(cfg)

	volumeNotFound := false
	volumeDetached := false
	log.Debugf("%s: detaching volume %q", maybeScan, volumeID)
	for i := 0; i < 5; i++ {
		if _, err := ec2client.DetachVolume(ctx, &ec2.DetachVolumeInput{
			Force:    aws.Bool(true),
			VolumeId: aws.String(volumeID),
		}); err != nil {
			var aerr smithy.APIError
			// NOTE(jinroh): we're trying to detach a volume in an 'available'
			// state for instance. Just bail.
			if errors.As(err, &aerr) {
				if aerr.ErrorCode() == "IncorrectState" {
					volumeDetached = true
					break
				}
				if aerr.ErrorCode() == "InvalidVolume.NotFound" {
					volumeNotFound = true
					break
				}
			}
			log.Warnf("%s: could not detach volume %s: %v", maybeScan, volumeID, err)
		} else {
			volumeDetached = true
			break
		}
		if !sleepCtx(ctx, 10*time.Second) {
			return fmt.Errorf("could not detach volume: %w", ctx.Err())
		}
	}

	if volumeDetached && maybeScan != nil && maybeScan.AttachedDeviceName != nil {
		for i := 0; i < 30; i++ {
			if !sleepCtx(ctx, 1*time.Second) {
				return ctx.Err()
			}
			devices, err := listBlockDevices(ctx, *maybeScan.AttachedDeviceName)
			if err != nil || len(devices) == 0 {
				break
			}
		}
	}

	var errd error
	for i := 0; i < 10; i++ {
		if volumeNotFound {
			break
		}
		_, errd = ec2client.DeleteVolume(ctx, &ec2.DeleteVolumeInput{
			VolumeId: aws.String(volumeID),
		})
		if errd != nil {
			var aerr smithy.APIError
			if errors.As(err, &aerr) && aerr.ErrorCode() == "InvalidVolume.NotFound" {
				errd = nil
				break
			}
		} else {
			log.Debugf("%s: volume deleted %q", maybeScan, volumeID)
			break
		}
		if !sleepCtx(ctx, 10*time.Second) {
			errd = ctx.Err()
			break
		}
	}
	if errd != nil {
		return fmt.Errorf("could not delete volume %q: %w", volumeID, errd)
	}
	return nil
}

func cleanupScanUmount(ctx context.Context, maybeScan *types.ScanTask, mountPoint string) {
	log.Debugf("%s: un-mounting %q", maybeScan, mountPoint)
	var umountOutput []byte
	var erru error
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(mountPoint); os.IsNotExist(err) {
			return
		}
		umountCmd := exec.CommandContext(ctx, "umount", mountPoint)
		if umountOutput, erru = umountCmd.CombinedOutput(); erru != nil {
			// Check for "not mounted" errors that we ignore
			const MntExFail = 32 // MNT_EX_FAIL
			if exiterr, ok := erru.(*exec.ExitError); ok && exiterr.ExitCode() == MntExFail && bytes.Contains(umountOutput, []byte("not mounted")) {
				return
			}
			log.Warnf("%s: could not umount %s: %s: %s", maybeScan, mountPoint, erru, string(umountOutput))
			if !sleepCtx(ctx, 3*time.Second) {
				return
			}
			continue
		}
		if err := os.Remove(mountPoint); err != nil {
			log.Warnf("could not remove mount point %q: %v", mountPoint, err)
		}
		return
	}
	log.Errorf("could not umount %s: %s: %s", mountPoint, erru, string(umountOutput))
}

func cleanupScan(scan *types.ScanTask) {
	ctx, cancel := context.WithTimeout(context.Background(), cleanupMaxDuration)
	defer cancel()

	scanRoot := scan.Path()

	log.Debugf("%s: cleaning up scan data on filesystem", scan)

	for snapshotARNString, snapshotCreatedAt := range scan.CreatedSnapshots {
		snapshotARN, err := types.ParseARN(snapshotARNString, types.ResourceTypeSnapshot)
		if err != nil {
			continue
		}
		_, snapshotID, _ := types.GetARNResource(snapshotARN)
		cfg, err := newAWSConfig(ctx, snapshotARN.Region, scan.Roles[snapshotARN.AccountID])
		if err != nil {
			log.Errorf("%s: %v", scan, err)
		} else {
			ec2client := ec2.NewFromConfig(cfg)
			log.Debugf("%s: deleting snapshot %q", scan, snapshotID)
			if _, err := ec2client.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{
				SnapshotId: aws.String(snapshotID),
			}); err != nil {
				log.Warnf("%s: could not delete snapshot %s: %v", scan, snapshotID, err)
			} else {
				log.Debugf("%s: snapshot deleted %s", scan, snapshotID)
				statsResourceTTL(types.ResourceTypeSnapshot, scan, *snapshotCreatedAt)
			}
		}
	}

	entries, err := os.ReadDir(scanRoot)
	if err == nil {
		var wg sync.WaitGroup

		umount := func(mountPoint string) {
			defer wg.Done()
			cleanupScanUmount(ctx, scan, mountPoint)
		}

		var ebsMountPoints []fs.DirEntry
		var ctrMountPoints []fs.DirEntry
		var pidFiles []fs.DirEntry

		for _, entry := range entries {
			if entry.IsDir() {
				if strings.HasPrefix(entry.Name(), types.EBSMountPrefix) {
					ebsMountPoints = append(ebsMountPoints, entry)
				}
				if strings.HasPrefix(entry.Name(), types.ContainerdMountPrefix) || strings.HasPrefix(entry.Name(), types.DockerMountPrefix) {
					ctrMountPoints = append(ctrMountPoints, entry)
				}
				if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".pid") {
					pidFiles = append(pidFiles, entry)
				}
			}
		}
		for _, entry := range ctrMountPoints {
			wg.Add(1)
			go umount(filepath.Join(scanRoot, entry.Name()))
		}
		wg.Wait()
		// unmount "ebs-*" entrypoint last as the other mountpoint may depend on it
		for _, entry := range ebsMountPoints {
			wg.Add(1)
			go umount(filepath.Join(scanRoot, entry.Name()))
		}
		wg.Wait()

		for _, entry := range pidFiles {
			pidFile, err := os.Open(filepath.Join(scanRoot, entry.Name()))
			if err != nil {
				continue
			}
			pidRaw, err := io.ReadAll(io.LimitReader(pidFile, 32))
			if err != nil {
				pidFile.Close()
				continue
			}
			pidFile.Close()
			if pid, err := strconv.Atoi(strings.TrimSpace(string(pidRaw))); err == nil {
				if proc, err := os.FindProcess(pid); err == nil {
					log.Debugf("%s: killing remaining scanner process with pid %d", scan, pid)
					_ = proc.Kill()
				}
			}
		}
	}

	log.Debugf("%s: removing folder %q", scan, scanRoot)
	if err := os.RemoveAll(scanRoot); err != nil {
		log.Errorf("%s: could not cleanup mount root %q: %v", scan, scanRoot, err)
	}

	if scan.AttachedDeviceName != nil {
		blockDevices, err := listBlockDevices(ctx, *scan.AttachedDeviceName)
		if err == nil && len(blockDevices) == 1 {
			for _, child := range blockDevices[0].getChildrenType("lvm") {
				if err := exec.Command("dmsetup", "remove", child.Path).Run(); err != nil {
					log.Errorf("%s: could not remove logical device %q from block device %q: %v", scan, child.Path, child.Name, err)
				}
			}
		}
	}

	switch scan.DiskMode {
	case types.VolumeAttach:
		if volumeARN := scan.AttachedVolumeARN; volumeARN != nil {
			if errd := cleanupScanDetach(ctx, scan, *volumeARN, scan.Roles); errd != nil {
				log.Warnf("%s: could not delete volume %q: %v", scan, volumeARN, errd)
			} else {
				statsResourceTTL(types.ResourceTypeVolume, scan, *scan.AttachedVolumeCreatedAt)
			}
		}
	case types.NBDAttach:
		if diskDeviceName := scan.AttachedDeviceName; diskDeviceName != nil {
			nbd.StopNBDBlockDevice(ctx, *diskDeviceName)
		}
	case types.NoAttach:
		// do nothing
	default:
		panic("unreachable")
	}
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	select {
	case <-time.After(d):
		return true
	case <-ctx.Done():
		return false
	}
}

func ec2ARN(region, accountID string, resourceType types.ResourceType, resourceID string) arn.ARN {
	return arn.ARN{
		Partition: "aws",
		Service:   "ec2",
		Region:    region,
		AccountID: accountID,
		Resource:  fmt.Sprintf("%s/%s", resourceType, resourceID),
	}
}

func hasResults(bom *cdx.BOM) bool {
	// We can't use Dependencies > 0, since len(Dependencies) == 1 when there are no components.
	// See https://github.com/aquasecurity/trivy/blob/main/pkg/sbom/cyclonedx/core/cyclonedx.go
	return bom.Components != nil && len(*bom.Components) > 0
}

type awsLimitsOptions struct {
	EC2Rate          rate.Limit
	EBSListBlockRate rate.Limit
	EBSGetBlockRate  rate.Limit
	DefaultRate      rate.Limit
}

func getAWSLimitsOptions() awsLimitsOptions {
	return awsLimitsOptions{
		EC2Rate:          rate.Limit(pkgconfig.Datadog.GetFloat64("agentless_scanner.limits.aws_ec2_rate")),
		EBSListBlockRate: rate.Limit(pkgconfig.Datadog.GetFloat64("agentless_scanner.limits.aws_ebs_list_block_rate")),
		EBSGetBlockRate:  rate.Limit(pkgconfig.Datadog.GetFloat64("agentless_scanner.limits.aws_ebs_get_block_rate")),
		DefaultRate:      rate.Limit(pkgconfig.Datadog.GetFloat64("agentless_scanner.limits.aws_default_rate")),
	}
}

type awsWaiter struct {
	sync.Mutex
	subs map[string]map[string][]chan error
}

func (w *awsWaiter) wait(ctx context.Context, snapshotARN arn.ARN, ec2client *ec2.Client) <-chan error {
	w.Lock()
	defer w.Unlock()
	region := snapshotARN.Region
	if w.subs == nil {
		w.subs = make(map[string]map[string][]chan error)
	}
	if w.subs[region] == nil {
		w.subs[region] = make(map[string][]chan error)
	}
	_, resourceID, _ := types.GetARNResource(snapshotARN)
	ch := make(chan error, 1)
	subs := w.subs[region]
	subs[resourceID] = append(subs[resourceID], ch)
	if len(subs) == 1 {
		go w.loop(ctx, region, ec2client)
	}
	return ch
}

func (w *awsWaiter) abort(region string, err error) {
	w.Lock()
	defer w.Unlock()
	for _, chs := range w.subs[region] {
		for _, ch := range chs {
			ch <- err
		}
	}
	w.subs[region] = nil
}

func (w *awsWaiter) loop(ctx context.Context, region string, ec2client *ec2.Client) {
	const (
		tickerInterval  = 5 * time.Second
		snapshotTimeout = 15 * time.Minute
	)

	ticker := time.NewTicker(tickerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			w.abort(region, ctx.Err())
			return
		}

		w.Lock()
		snapshotIDs := make([]string, 0, len(w.subs[region]))
		for snapshotID := range w.subs[region] {
			snapshotIDs = append(snapshotIDs, snapshotID)
		}
		w.Unlock()

		if len(snapshotIDs) == 0 {
			return
		}

		// TODO: could we rely on ListSnapshotBlocks instead of
		// DescribeSnapshots as a "fast path" to not consume precious quotas ?
		output, err := ec2client.DescribeSnapshots(context.TODO(), &ec2.DescribeSnapshotsInput{
			SnapshotIds: snapshotIDs,
		})
		if err != nil {
			w.abort(region, err)
			return
		}

		snapshots := make(map[string]ec2types.Snapshot, len(output.Snapshots))
		for _, snap := range output.Snapshots {
			snapshots[*snap.SnapshotId] = snap
		}

		w.Lock()
		subs := w.subs[region]
		noError := errors.New("")
		for _, snapshotID := range snapshotIDs {
			var errp error
			snap, ok := snapshots[snapshotID]
			if !ok {
				errp = fmt.Errorf("snapshot %q does not exist", *snap.SnapshotId)
			} else {
				switch snap.State {
				case ec2types.SnapshotStatePending:
					if elapsed := time.Since(*snap.StartTime); elapsed > snapshotTimeout {
						errp = fmt.Errorf("snapshot %q creation timed out (started at %s)", *snap.SnapshotId, *snap.StartTime)
					}
				case ec2types.SnapshotStateRecoverable:
					errp = fmt.Errorf("snapshot %q in recoverable state", *snap.SnapshotId)
				case ec2types.SnapshotStateRecovering:
					errp = fmt.Errorf("snapshot %q in recovering state", *snap.SnapshotId)
				case ec2types.SnapshotStateError:
					msg := fmt.Sprintf("snapshot %q failed", *snap.SnapshotId)
					if snap.StateMessage != nil {
						msg += ": " + *snap.StateMessage
					}
					errp = fmt.Errorf(msg)
				case ec2types.SnapshotStateCompleted:
					errp = noError
				}
			}
			if errp != nil {
				for _, ch := range subs[*snap.SnapshotId] {
					if errp == noError {
						ch <- nil
					} else {
						ch <- errp
					}
				}
				delete(subs, *snap.SnapshotId)
			}
		}
		w.Unlock()
	}
}

type awsLimits struct {
	limitersMu sync.Mutex
	limiters   map[string]*rate.Limiter
	opts       awsLimitsOptions
}

func newAWSLimits(opts awsLimitsOptions) *awsLimits {
	return &awsLimits{
		limiters: make(map[string]*rate.Limiter),
		opts:     opts,
	}
}

func (l *awsLimits) getLimiter(accountID, region, service, action string) *rate.Limiter {
	var limit rate.Limit
	switch service {
	case "ec2":
		switch {
		// reference: https://docs.aws.amazon.com/AWSEC2/latest/APIReference/throttling.html#throttling-limits
		case strings.HasPrefix(action, "Describe"), strings.HasPrefix(action, "Get"):
			limit = l.opts.EC2Rate
		default:
			limit = l.opts.EC2Rate / 4.0
		}
	case "ebs":
		switch action {
		case "getblock":
			limit = l.opts.EBSGetBlockRate
		case "listblocks", "changedblocks":
			limit = l.opts.EBSListBlockRate
		}
	case "s3", "imds":
		limit = 0.0 // no rate limiting
	default:
		limit = l.opts.DefaultRate
	}
	if limit == 0.0 {
		return nil // no rate limiting
	}
	key := accountID + region + service + action
	l.limitersMu.Lock()
	ll, ok := l.limiters[key]
	if !ok {
		ll = rate.NewLimiter(limit, 1)
		l.limiters[key] = ll
	}
	l.limitersMu.Unlock()
	return ll
}

func humanParseARN(s string, expectedTypes ...types.ResourceType) (arn.ARN, error) {
	if strings.HasPrefix(s, "arn:") {
		return types.ParseARN(s, expectedTypes...)
	}
	self, err := getSelfEC2InstanceIndentity(context.TODO())
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
