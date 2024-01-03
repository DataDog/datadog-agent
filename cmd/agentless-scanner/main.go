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

	"golang.org/x/exp/slices"
	"golang.org/x/time/rate"

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
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/ebs"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"

	// DataDog agent: SBOM + proto stuffs
	sbommodel "github.com/DataDog/agent-payload/v5/sbom"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	// DataDog agent: RC stuffs
	"github.com/DataDog/datadog-agent/pkg/config/remote"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"

	// DataDog agent: logs stuffs
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	// DataDog agent: metrics Statsd
	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/spf13/cobra"
)

const (
	maxSnapshotRetries = 3
	maxAttachRetries   = 10

	defaultWorkersCount = 40

	defaultAWSRate          rate.Limit = 20.0
	defaultEC2Rate          rate.Limit = 20.0
	defaultEBSListBlockRate rate.Limit = 20.0
	defaultEBSGetBlockRate  rate.Limit = 400.0

	defaultSelfRegion      = "us-east-1"
	defaultSnapshotsMaxTTL = 24 * time.Hour
)

var statsd *ddgostatsd.Client

var (
	globalParams struct {
		configFilePath string
		diskMode       diskMode
	}

	cleanupMaxDuration = 1 * time.Minute

	awsConfigs   = make(map[string]*aws.Config)
	awsConfigsMu sync.Mutex
)

type configType string

const (
	awsScan configType = "aws-scan"
)

type scanType string

const (
	hostScanType   scanType = "host-scan"
	ebsScanType    scanType = "ebs-volume"
	lambdaScanType scanType = "lambda"
)

type scanAction string

const (
	malware         scanAction = "malware"
	vulnsHost       scanAction = "vulns"
	vulnsContainers scanAction = "vulnscontainers"
)

type diskMode string

const (
	volumeAttach diskMode = "volume-attach"
	nbdAttach    diskMode = "nbd-attach"
	noAttach     diskMode = "no-attach"
)

type resourceType string

const (
	resourceTypeVolume   = "volume"
	resourceTypeSnapshot = "snapshot"
	resourceTypeFunction = "function"
	resourceTypeRole     = "role"
)

var defaultActions = []string{
	string(vulnsHost),
	string(vulnsContainers),
}

type (
	rolesMapping map[string]*arn.ARN

	finding struct {
		AgentVersion string      `json:"agent_version,omitempty"`
		RuleID       string      `json:"agent_rule_id,omitempty"`
		RuleVersion  int         `json:"agent_rule_version,omitempty"`
		FrameworkID  string      `json:"agent_framework_id,omitempty"`
		Evaluator    string      `json:"evaluator,omitempty"`
		ExpireAt     *time.Time  `json:"expire_at,omitempty"`
		Result       string      `json:"result,omitempty"`
		ResourceType string      `json:"resource_type,omitempty"`
		ResourceID   string      `json:"resource_id,omitempty"`
		Tags         []string    `json:"tags"`
		Data         interface{} `json:"data"`
	}

	scanConfigRaw struct {
		Type  string `json:"type"`
		Tasks []struct {
			Type     string   `json:"type"`
			ARN      string   `json:"arn"`
			Hostname string   `json:"hostname"`
			Actions  []string `json:"actions,omitempty"`
		} `json:"tasks"`
		Roles    []string `json:"roles"`
		DiskMode string   `json:"disk_mode"`
	}

	scanConfig struct {
		Type     configType
		Tasks    []*scanTask
		Roles    rolesMapping
		DiskMode diskMode
	}

	scanTask struct {
		Type     scanType
		ARN      arn.ARN
		Hostname string
		Actions  []scanAction
		Roles    rolesMapping
		DiskMode diskMode
	}

	scanResult struct {
		scan     *scanTask
		err      error
		sbom     *sbommodel.SBOMEntity
		duration time.Duration
		findings []*finding
	}

	ebsVolume struct {
		Hostname string
		ARN      arn.ARN
	}
)

func (s scanTask) String() string {
	return fmt.Sprintf("%s=%q hostname=%q", s.Type, s.ARN, s.Hostname)
}

func main() {
	flavor.SetFlavor(flavor.AgentlessScanner)
	cmd := rootCommand()
	cmd.SilenceErrors = true
	err := cmd.Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
		os.Exit(-1)
	}
	os.Exit(1)
}

func rootCommand() *cobra.Command {
	var diskModeStr string

	sideScannerCmd := &cobra.Command{
		Use:          "agentless-scanner [command]",
		Short:        "Datadog Agentless Scanner at your service.",
		Long:         `Datadog Agentless Scanner scans your cloud environment for vulnerabilities, compliance and security issues.`,
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			var err error
			globalParams.diskMode, err = parseDiskMode(diskModeStr)
			if err != nil {
				return err
			}
			initStatsdClient()
			return nil
		},
	}

	pflags := sideScannerCmd.PersistentFlags()
	pflags.StringVarP(&globalParams.configFilePath, "config-path", "c", path.Join(commonpath.DefaultConfPath, "datadog.yaml"), "specify the path to agentless-scanner configuration yaml file")
	pflags.StringVar(&diskModeStr, "disk-mode", "no-attach", fmt.Sprintf("disk mode used for scanning EBS volumes: %s, %s or %s", volumeAttach, nbdAttach, noAttach))
	sideScannerCmd.AddCommand(runCommand())
	sideScannerCmd.AddCommand(scanCommand())
	sideScannerCmd.AddCommand(offlineCommand())
	sideScannerCmd.AddCommand(attachCommand())
	sideScannerCmd.AddCommand(nbdCommand())
	sideScannerCmd.AddCommand(cleanupCommand())

	return sideScannerCmd
}

func runCommand() *cobra.Command {
	var runParams struct {
		pidfilePath      string
		poolSize         int
		allowedScanTypes []string
	}

	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Runs the agentless-scanner",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(
				func(_ complog.Component, _ compconfig.Component) error {
					return runCmd(runParams.pidfilePath, runParams.poolSize, runParams.allowedScanTypes)
				},
				fx.Supply(compconfig.NewAgentParamsWithSecrets(globalParams.configFilePath)),
				fx.Supply(complog.ForDaemon("AGENTLESSSCANER", "log_file", pkgconfig.DefaultAgentlessScannerLogFile)),
				complog.Module,
				compconfig.Module,
			)
		},
	}
	runCmd.Flags().StringVarP(&runParams.pidfilePath, "pidfile", "p", "", "path to the pidfile")
	runCmd.Flags().IntVar(&runParams.poolSize, "workers", defaultWorkersCount, "number of scans running in parallel")
	runCmd.Flags().StringSliceVar(&runParams.allowedScanTypes, "allowed-scans-type", nil, "lists of allowed scan types (ebs-volume, lambda)")
	return runCmd
}

func scanCommand() *cobra.Command {
	var cliArgs struct {
		ScanType string
		ARN      string
		Hostname string
		SendData bool
		RawScan  string
	}
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "execute a scan",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(
				func(_ complog.Component, _ compconfig.Component) error {
					var config *scanConfig
					var err error
					if len(cliArgs.RawScan) > 0 {
						config, err = unmarshalConfig([]byte(cliArgs.RawScan))
					} else {
						roles := getDefaultRolesMapping()
						task, err := newScanTask(
							cliArgs.ScanType,
							cliArgs.ARN,
							cliArgs.Hostname,
							nil,
							roles,
							globalParams.diskMode)
						if err != nil {
							return err
						}
						config = &scanConfig{
							Type:  awsScan,
							Tasks: []*scanTask{task},
						}
					}
					if err != nil {
						return err
					}
					return scanCmd(*config)
				},
				fx.Supply(compconfig.NewAgentParamsWithSecrets(globalParams.configFilePath)),
				fx.Supply(complog.ForDaemon("AGENTLESSSCANER", "log_file", pkgconfig.DefaultAgentlessScannerLogFile)),
				complog.Module,
				compconfig.Module,
			)
		},
	}

	cmd.Flags().StringVar(&cliArgs.RawScan, "raw-config-data", "", "scan config data in JSON")
	cmd.Flags().StringVar(&cliArgs.ScanType, "scan-type", "", "scan type")
	cmd.Flags().StringVar(&cliArgs.ARN, "arn", "", "arn to scan")
	cmd.Flags().StringVar(&cliArgs.Hostname, "hostname", "", "scan hostname")
	cmd.MarkFlagsRequiredTogether("arn", "scan-type", "hostname")

	return cmd
}

func offlineCommand() *cobra.Command {
	var cliArgs struct {
		poolSize int
		regions  []string
		maxScans int
	}
	cmd := &cobra.Command{
		Use:   "offline",
		Short: "Runs the agentless-scanner in offline mode (server-less mode)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(
				func(_ complog.Component, _ compconfig.Component) error {
					return offlineCmd(cliArgs.poolSize, cliArgs.regions, cliArgs.maxScans)
				},
				fx.Supply(compconfig.NewAgentParamsWithSecrets(globalParams.configFilePath)),
				fx.Supply(complog.ForDaemon("AGENTLESSSCANER", "log_file", pkgconfig.DefaultAgentlessScannerLogFile)),
				complog.Module,
				compconfig.Module,
			)
		},
	}

	cmd.Flags().IntVarP(&cliArgs.poolSize, "workers", "", defaultWorkersCount, "number of scans running in parallel")
	cmd.Flags().StringSliceVarP(&cliArgs.regions, "regions", "", nil, "list of regions to scan (default to all regions)")
	cmd.Flags().IntVarP(&cliArgs.maxScans, "max-scans", "", 0, "maximum number of scans to perform")

	return cmd
}

func attachCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "attach",
		Short: "Attach a list of ARNs given in stdin into volumes to the EC2 instance using a dedicated EBS volume",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(
				func(_ complog.Component, _ compconfig.Component) error {
					return attachCmd()
				},
				fx.Supply(compconfig.NewAgentParamsWithSecrets(globalParams.configFilePath)),
				fx.Supply(complog.ForDaemon("AGENTLESSSCANER", "log_file", pkgconfig.DefaultAgentlessScannerLogFile)),
				complog.Module,
				compconfig.Module,
			)
		},
	}

	return cmd
}

func nbdCommand() *cobra.Command {
	var cliArgs struct {
		mount     bool
		runClient bool
	}

	cmd := &cobra.Command{
		Use:   "nbd <snapshot-arn>",
		Short: "Mount the given snapshot into /snapshots/<snapshot-id>/<part> using a network block device",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(
				func(_ complog.Component, _ compconfig.Component) error {
					snapshotARN, err := parseARN(args[0], resourceTypeSnapshot)
					if err != nil {
						return err
					}
					return nbdMountCmd(snapshotARN, cliArgs.mount, cliArgs.runClient)
				},
				fx.Supply(compconfig.NewAgentParamsWithSecrets(globalParams.configFilePath)),
				fx.Supply(complog.ForDaemon("AGENTLESSSCANER", "log_file", pkgconfig.DefaultAgentlessScannerLogFile)),
				complog.Module,
				compconfig.Module,
			)
		},
	}

	cmd.Flags().BoolVar(&cliArgs.runClient, "run-client", false, "start the nbd client")
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
			return fxutil.OneShot(
				func(_ complog.Component, _ compconfig.Component) error {
					return cleanupCmd(cliArgs.region, cliArgs.dryRun, cliArgs.delay)
				},
				fx.Supply(compconfig.NewAgentParamsWithSecrets(globalParams.configFilePath)),
				fx.Supply(complog.ForDaemon("AGENTLESSSCANER", "log_file", pkgconfig.DefaultAgentlessScannerLogFile)),
				complog.Module,
				compconfig.Module,
			)
		},
	}
	cmd.Flags().StringVarP(&cliArgs.region, "region", "", "us-east-1", "AWS region")
	cmd.Flags().BoolVarP(&cliArgs.dryRun, "dry-run", "", false, "dry run")
	cmd.Flags().DurationVarP(&cliArgs.delay, "delay", "", 0, "delete snapshot older than delay")
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

func runCmd(pidfilePath string, poolSize int, allowedScanTypes []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if pidfilePath != "" {
		err := pidfile.WritePID(pidfilePath)
		if err != nil {
			return log.Errorf("Error while writing PID file, exiting: %v", err)
		}
		defer os.Remove(pidfilePath)
		log.Infof("pid '%d' written to pid file '%s'", os.Getpid(), pidfilePath)
	}

	hostname, err := utils.GetHostnameWithContext(ctx)
	if err != nil {
		return fmt.Errorf("could not fetch hostname: %w", err)
	}

	limits := newAWSLimits(defaultEC2Rate, defaultEBSListBlockRate, defaultEBSGetBlockRate)

	scanner, err := newSideScanner(hostname, limits, poolSize, allowedScanTypes)
	if err != nil {
		return fmt.Errorf("could not initialize agentless-scanner: %w", err)
	}
	if err := scanner.subscribeRemoteConfig(ctx); err != nil {
		return fmt.Errorf("could not accept configs from Remote Config: %w", err)
	}
	scanner.start(ctx)
	return nil
}

func parseDiskMode(diskModeStr string) (diskMode, error) {
	switch diskModeStr {
	case string(volumeAttach):
		return volumeAttach, nil
	case string(nbdAttach):
		return nbdAttach, nil
	case string(noAttach), "":
		return noAttach, nil
	default:
		return "", fmt.Errorf("invalid flag \"disk-mode\": expecting either %s, %s or %s", volumeAttach, nbdAttach, noAttach)
	}
}

func parseRolesMapping(roles []string) rolesMapping {
	if len(roles) == 0 {
		return nil
	}
	rolesMapping := make(rolesMapping, len(roles))
	for _, role := range roles {
		roleARN, err := parseARN(role, resourceTypeRole)
		if err != nil {
			log.Warnf("role-mapping: bad role %q: %v", role, err)
			continue
		}
		rolesMapping[roleARN.AccountID] = &roleARN
	}
	return rolesMapping
}

func getDefaultRolesMapping() rolesMapping {
	roles := pkgconfig.Datadog.GetStringSlice("agentless_scanner.default_roles")
	return parseRolesMapping(roles)
}

func scanCmd(config scanConfig) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	ctxhostname, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	hostname, err := utils.GetHostnameWithContext(ctxhostname)
	if err != nil {
		hostname = "unknown"
	}

	limits := newAWSLimits(defaultEC2Rate, defaultEBSListBlockRate, defaultEBSGetBlockRate)
	sidescanner, err := newSideScanner(hostname, limits, 1, nil)
	if err != nil {
		return fmt.Errorf("could not initialize agentless-scanner: %w", err)
	}
	sidescanner.printResults = true
	go func() {
		sidescanner.configsCh <- &config
		close(sidescanner.configsCh)
	}()
	sidescanner.start(ctx)
	return nil
}

func offlineCmd(poolSize int, regions []string, maxScans int) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
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

	limits := newAWSLimits(defaultEC2Rate, defaultEBSListBlockRate, defaultEBSGetBlockRate)
	sidescanner, err := newSideScanner(hostname, limits, poolSize, nil)
	if err != nil {
		return fmt.Errorf("could not initialize agentless-scanner: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	go func() {
		scans := make([]*scanTask, 0)

		for _, regionName := range allRegions {
			if ctx.Err() != nil {
				return
			}
			if regionName == "auto" {
				regionName = selfRegion
			}
			volumesForRegion, err := listEBSVolumesForRegion(ctx, *identity.Account, regionName, roles)
			if err != nil {
				log.Errorf("could not scan region %q: %v", regionName, err)
				cancel()
				return
			}
			for _, volume := range volumesForRegion {
				scan, err := newScanTask(string(ebsScanType), volume.ARN.String(), volume.Hostname, defaultActions, roles, globalParams.diskMode)
				if err != nil {
					log.Warnf("could not create scan for volume %s: %v", volume.ARN, err)
				} else {
					scans = append(scans, scan)
				}
			}
		}

		if maxScans > 0 && len(scans) > maxScans {
			scans = scans[:maxScans]
		}

		sidescanner.configsCh <- &scanConfig{Type: awsScan, Tasks: scans, Roles: roles}
		close(sidescanner.configsCh)
	}()

	sidescanner.start(ctx)
	return nil
}

func listEBSVolumesForRegion(ctx context.Context, accountID, regionName string, roles rolesMapping) (volumes []ebsVolume, err error) {
	cfg, err := newAWSConfig(ctx, regionName, roles[accountID])
	if err != nil {
		return nil, err
	}

	ec2client := ec2.NewFromConfig(cfg)
	if err != nil {
		return nil, err
	}

	describeInstancesInput := &ec2.DescribeInstancesInput{}

	for {
		describeInstancesOutput, err := ec2client.DescribeInstances(ctx, describeInstancesInput)
		if err != nil {
			return nil, err
		}

		for _, reservation := range describeInstancesOutput.Reservations {
			for _, instance := range reservation.Instances {
				if instance.InstanceId == nil {
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
					volumeARN := ec2ARN(regionName, accountID, resourceTypeVolume, *blockDeviceMapping.Ebs.VolumeId)
					log.Debugf("%s %s %s %s %s", regionName, *instance.InstanceId, volumeARN, *blockDeviceMapping.DeviceName, *instance.PlatformDetails)
					volumes = append(volumes, ebsVolume{Hostname: *instance.InstanceId, ARN: volumeARN})
				}
			}
		}

		if describeInstancesOutput.NextToken == nil {
			break
		}

		describeInstancesInput.NextToken = describeInstancesOutput.NextToken
	}

	return
}

func cleanupCmd(region string, dryRun bool, delay time.Duration) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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
	toBeDeleted, err := listResourcesForCleanup(ctx, ec2client, delay)
	if err != nil {
		return err
	}
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
	toBeDeleted, err := listResourcesForCleanup(ctx, ec2client, maxTTL)
	if err != nil {
		return err
	}

	if len(toBeDeleted) == 0 {
		return nil
	}

	cloudResourcesCleanup(ctx, ec2client, toBeDeleted)
	return nil
}

func (s *sideScanner) cleanupProcess(ctx context.Context) {
	log.Infof("Starting cleanup process")

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}

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

//nolint:unused
func downloadSnapshot(ctx context.Context, w io.Writer, snapshotARN arn.ARN) error {
	defaultCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(snapshotARN.Region))
	if err != nil {
		return err
	}
	stsclient := sts.NewFromConfig(defaultCfg)
	identity, err := stsclient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return err
	}

	// Retrieve instance’s region.
	imdsclient := imds.NewFromConfig(defaultCfg)
	regionOutput, err := imdsclient.GetRegion(ctx, &imds.GetRegionInput{})
	selfRegion := defaultSelfRegion
	if err != nil {
		log.Errorf("could not retrieve region from ec2 instance - using default %q: %v", selfRegion, err)
	} else {
		selfRegion = regionOutput.Region
	}

	roles := getDefaultRolesMapping()
	cfg, err := newAWSConfig(ctx, selfRegion, roles[*identity.Account])
	if err != nil {
		return err
	}

	_, snapshotID, _ := getARNResource(snapshotARN)

	ebsclient := ebs.NewFromConfig(cfg)
	listSnapshotsInput := &ebs.ListSnapshotBlocksInput{
		SnapshotId: &snapshotID,
		NextToken:  nil,
	}
	var n int64
	var size int64
	var blockIndex int32

	nullBuffer := make([]byte, 512*1024)
	for {
		fmt.Printf("listing blocks for %s\n", snapshotID)
		blocks, err := ebsclient.ListSnapshotBlocks(ctx, listSnapshotsInput)
		if err != nil {
			return err
		}
		size = *blocks.VolumeSize << 30
		for _, block := range blocks.Blocks {
			for i := blockIndex; i < *block.BlockIndex; i++ {
				_, err := io.Copy(w, bytes.NewReader(nullBuffer))
				if err != nil {
					return err
				}
			}
			blockOutput, err := ebsclient.GetSnapshotBlock(ctx, &ebs.GetSnapshotBlockInput{
				BlockIndex: block.BlockIndex,
				BlockToken: block.BlockToken,
				SnapshotId: &snapshotID,
			})
			if err != nil {
				return err
			}
			copied, err := io.Copy(w, blockOutput.BlockData)
			if err != nil {
				blockOutput.BlockData.Close()
				return err
			}
			blockOutput.BlockData.Close()
			n += copied
			blockIndex = *block.BlockIndex + 1
		}
		listSnapshotsInput.NextToken = blocks.NextToken
		if listSnapshotsInput.NextToken == nil {
			break
		}
	}
	for n < size {
		w, err := io.Copy(w, bytes.NewReader(nullBuffer))
		if err != nil {
			return err
		}
		n += w
	}
	return nil
}

func attachCmd() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	stdin := os.Stdin

	var arns []arn.ARN
	lineNumber := 0
	scanner := bufio.NewScanner(stdin)
	for scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return err
		}
		lineNumber++
		line := scanner.Text()
		fmt.Println(lineNumber, line)
		arn, err := parseARN(line, resourceTypeSnapshot, resourceTypeVolume)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s (line %d)\n", err, lineNumber)
		} else {
			arns = append(arns, arn)
		}
	}
	if len(arns) == 0 {
		return fmt.Errorf("provided an empty list of ARNs in stdin to be mounted")
	}

	var cleanups []func(context.Context)
	defer func() {
		cleanupctx, cancel := context.WithTimeout(context.Background(), cleanupMaxDuration)
		defer cancel()
		for _, cleanup := range cleanups {
			cleanup(cleanupctx)
		}
	}()

	roles := getDefaultRolesMapping()
	for _, resourceARN := range arns {
		cfg, err := newAWSConfig(ctx, resourceARN.Region, roles[resourceARN.AccountID])
		if err != nil {
			return err
		}

		ec2client := ec2.NewFromConfig(cfg)
		hostname := ""
		scan, err := newScanTask(
			string(ebsScanType),
			resourceARN.String(),
			hostname,
			nil,
			roles,
			globalParams.diskMode,
		)
		if err != nil {
			return err
		}

		resourceType, resourceID, err := getARNResource(resourceARN)
		if err != nil {
			return err
		}
		var snapshotARN arn.ARN
		switch resourceType {
		case resourceTypeVolume:
			var cleanupSnapshot func(context.Context)
			snapshotARN, cleanupSnapshot, err = createSnapshot(ctx, scan, ec2client, resourceARN)
			cleanups = append(cleanups, cleanupSnapshot)
			if err != nil {
				return err
			}
		case resourceTypeSnapshot:
			snapshotARN = resourceARN
		default:
			return fmt.Errorf("unsupport resource type %q", resourceType)
		}

		device, volumeARN, localSnapshotARN, cleanupVolume, err := attachSnapshotWithVolume(ctx, scan, snapshotARN)
		cleanups = append(cleanups, cleanupVolume)
		if err != nil {
			return err
		}
		partitions, err := listDevicePartitions(ctx, device, &volumeARN)
		if err != nil {
			return err
		}
		mountPoints, cleanupMount, err := mountDevice(ctx, localSnapshotARN, partitions)
		cleanups = append(cleanups, cleanupMount)
		if err != nil {
			return err
		}
		fmt.Printf("%s mount directories:\n", resourceID)
		for _, mountPoint := range mountPoints {
			fmt.Printf("  - %s\n", mountPoint)
		}
	}

	<-ctx.Done()

	return nil
}

func nbdMountCmd(snapshotARN arn.ARN, mount, runClient bool) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := newAWSConfig(ctx, snapshotARN.Region, nil)
	if err != nil {
		return err
	}
	ebsclient := ebs.NewFromConfig(cfg)
	device := nextNBDDevice()
	ebsnbd := NewEBSBlockDevice(EBSBlockDeviceOptions{
		EBSClient:   ebsclient,
		SnapshotARN: snapshotARN,
		DeviceName:  device,
		RunClient:   runClient,
	})
	if err := ebsnbd.Start(ctx); err != nil {
		return err
	}

	if runClient {
		partitions, err := listDevicePartitions(ctx, device, nil)
		if err != nil {
			log.Errorf("error could list paritions (device is still available on %q): %v", device, err)
		} else if mount {
			mountPoints, cleanupMount, err := mountDevice(ctx, snapshotARN, partitions)
			defer func() {
				cleanupctx, cancel := context.WithTimeout(context.Background(), cleanupMaxDuration)
				defer cancel()
				cleanupMount(cleanupctx)
			}()
			if err != nil {
				log.Errorf("error could not mount (device is still available on %q): %v", device, err)
			} else {
				for _, mountPoint := range mountPoints {
					fmt.Println(mountPoint)
				}
			}
		}
	}

	<-ctx.Done()
	ebsnbd.WaitCleanup()
	return nil
}

func newScanTask(t string, resourceARN, hostname string, actions []string, roles rolesMapping, mode diskMode) (*scanTask, error) {
	var scan scanTask
	var err error
	if t == string(hostScanType) {
		scan.Type = hostScanType
	} else {
		scan.ARN, err = parseARN(resourceARN, resourceTypeSnapshot, resourceTypeVolume, resourceTypeFunction)
		if err != nil {
			return nil, err
		}
		resourceType, _, err := getARNResource(scan.ARN)
		if err != nil {
			return nil, err
		}
		switch t {
		case string(ebsScanType):
			if resourceType != resourceTypeSnapshot && resourceType != resourceTypeVolume {
				return nil, fmt.Errorf("malformed scan task: unexpected type %q", resourceType)
			}
			scan.Type = ebsScanType
		case string(lambdaScanType):
			if resourceType != resourceTypeFunction {
				return nil, fmt.Errorf("malformed scan task: unexpected type %q", resourceType)
			}
			scan.Type = lambdaScanType
		default:
			return nil, fmt.Errorf("unknown scan type %q", t)
		}
	}
	scan.Hostname = hostname
	scan.Roles = roles
	scan.DiskMode = mode
	if actions == nil {
		actions = defaultActions
	}
	for _, actionRaw := range actions {
		switch actionRaw {
		case string(vulnsHost):
			scan.Actions = append(scan.Actions, vulnsHost)
		case string(vulnsContainers):
			scan.Actions = append(scan.Actions, vulnsContainers)
		case string(malware):
			scan.Actions = append(scan.Actions, malware)
		default:
			log.Warnf("unknown action type %q", actionRaw)
		}
	}
	return &scan, nil
}

func unmarshalConfig(b []byte) (*scanConfig, error) {
	var configRaw scanConfigRaw
	err := json.Unmarshal(b, &configRaw)
	if err != nil {
		return nil, err
	}
	var config scanConfig

	switch configRaw.Type {
	case string(awsScan):
		config.Type = awsScan
	default:
		return nil, fmt.Errorf("unexpected config type %q", config.Type)
	}

	if len(configRaw.Roles) > 0 {
		config.Roles = parseRolesMapping(configRaw.Roles)
	} else {
		config.Roles = getDefaultRolesMapping()
	}

	config.DiskMode, err = parseDiskMode(configRaw.DiskMode)
	if err != nil {
		return nil, err
	}

	config.Tasks = make([]*scanTask, 0, len(configRaw.Tasks))
	for _, rawScan := range configRaw.Tasks {
		task, err := newScanTask(rawScan.Type, rawScan.ARN, rawScan.Hostname, rawScan.Actions, config.Roles, config.DiskMode)
		if err != nil {
			log.Warnf("dropping malformed task: %v", err)
			continue
		}
		config.Tasks = append(config.Tasks, task)
	}
	return &config, nil
}

func parseARN(s string, expectedTypes ...resourceType) (arn.ARN, error) {
	a, err := arn.Parse(s)
	if err != nil {
		return arn.ARN{}, err
	}
	resType, _, err := getARNResource(a)
	if err != nil {
		return arn.ARN{}, err
	}
	if len(expectedTypes) > 0 && !slices.Contains(expectedTypes, resType) {
		return arn.ARN{}, fmt.Errorf("bad arn: expecting on of these resource types %v", expectedTypes)
	}
	return a, nil
}

var (
	partitionReg  = regexp.MustCompile("^aws[a-zA-Z-]*$")
	regionReg     = regexp.MustCompile("^[a-z]{2}((-gov)|(-iso(b?)))?-[a-z]+-[0-9]{1}$")
	accountIDReg  = regexp.MustCompile("^[0-9]{12}$")
	resourceIDReg = regexp.MustCompile("^[a-f0-9]+$")
	roleNameReg   = regexp.MustCompile("^[a-zA-Z0-9_+=,.@-]{1,64}$")
	functionReg   = regexp.MustCompile(`^([a-zA-Z0-9-_.]+)(:(\$LATEST|[a-zA-Z0-9-_]+))?$`)
)

func getARNResource(arn arn.ARN) (resourceType resourceType, resourceID string, err error) {
	if !partitionReg.MatchString(arn.Partition) {
		err = fmt.Errorf("bad arn %q: unexpected partition", arn)
		return
	}
	if arn.Region != "" && !regionReg.MatchString(arn.Region) {
		err = fmt.Errorf("bad arn %q: unexpected region (should be empty or match %s)", arn, regionReg)
		return
	}
	if arn.AccountID != "" && !accountIDReg.MatchString(arn.AccountID) {
		err = fmt.Errorf("bad arn %q: unexpected account ID (should match %s)", arn, accountIDReg)
		return
	}
	switch {
	case arn.Service == "ec2" && strings.HasPrefix(arn.Resource, "volume/"):
		resourceType, resourceID = resourceTypeVolume, strings.TrimPrefix(arn.Resource, "volume/")
		if !strings.HasPrefix(resourceID, "vol-") {
			err = fmt.Errorf("bad arn %q: resource ID has wrong prefix", arn)
			return
		}
		if !resourceIDReg.MatchString(strings.TrimPrefix(resourceID, "vol-")) {
			err = fmt.Errorf("bad arn %q: resource ID has wrong format (should match %s)", arn, resourceIDReg)
			return
		}
	case arn.Service == "ec2" && strings.HasPrefix(arn.Resource, "snapshot/"):
		resourceType, resourceID = resourceTypeSnapshot, strings.TrimPrefix(arn.Resource, "snapshot/")
		if !strings.HasPrefix(resourceID, "snap-") {
			err = fmt.Errorf("bad arn %q: resource ID has wrong prefix", arn)
			return
		}
		if !resourceIDReg.MatchString(strings.TrimPrefix(resourceID, "snap-")) {
			err = fmt.Errorf("bad arn %q: resource ID has wrong format (should match %s)", arn, resourceIDReg)
			return
		}
	case arn.Service == "lambda" && strings.HasPrefix(arn.Resource, "function:"):
		resourceType, resourceID = resourceTypeFunction, strings.TrimPrefix(arn.Resource, "function:")
		if sep := strings.Index(resourceID, ":"); sep > 0 {
			resourceID = resourceID[:sep]
		}
		if !functionReg.MatchString(resourceID) {
			err = fmt.Errorf("bad arn %q: function name has wrong format (should match %s)", arn, functionReg)
		}
	case arn.Service == "iam" && strings.HasPrefix(arn.Resource, "role/"):
		resourceType, resourceID = resourceTypeRole, strings.TrimPrefix(arn.Resource, "role/")
		if !roleNameReg.MatchString(resourceID) {
			err = fmt.Errorf("bad arn %q: role name has wrong format (should match %s)", arn, roleNameReg)
			return
		}
	default:
		err = fmt.Errorf("bad arn %q: unexpected resource type", arn)
		return
	}
	return
}

type sideScanner struct {
	hostname         string
	poolSize         int
	eventForwarder   epforwarder.EventPlatformForwarder
	findingsReporter *LogReporter
	rcClient         *remote.Client
	allowedScanTypes []string
	limits           *awsLimits
	printResults     bool

	regionsCleanupMu sync.Mutex
	regionsCleanup   map[string]*arn.ARN

	scansInProgress   map[arn.ARN]struct{}
	scansInProgressMu sync.RWMutex

	configsCh chan *scanConfig
	scansCh   chan *scanTask
	resultsCh chan scanResult
}

func newSideScanner(hostname string, limits *awsLimits, poolSize int, allowedScanTypes []string) (*sideScanner, error) {
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
		poolSize:         poolSize,
		eventForwarder:   eventForwarder,
		findingsReporter: findingsReporter,
		rcClient:         rcClient,
		allowedScanTypes: allowedScanTypes,
		limits:           limits,

		scansInProgress: make(map[arn.ARN]struct{}),

		configsCh: make(chan *scanConfig),
		scansCh:   make(chan *scanTask),
		resultsCh: make(chan scanResult),
	}, nil
}

func (s *sideScanner) subscribeRemoteConfig(ctx context.Context) error {
	log.Infof("subscribing to remote-config")
	s.rcClient.Subscribe(state.ProductCSMSideScanning, func(update map[string]state.RawConfig, _ func(string, state.ApplyStatus)) {
		log.Debugf("received %d remote config config updates", len(update))
		for _, rawConfig := range update {
			log.Debugf("received new config %q from remote-config of size %d", rawConfig.Metadata.ID, len(rawConfig.Config))
			config, err := unmarshalConfig(rawConfig.Config)
			if err != nil {
				log.Errorf("could not parse agentless-scanner task: %v", err)
				return
			}
			select {
			case <-ctx.Done():
				return
			case s.configsCh <- config:
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
	return srv.ListenAndServe()
}

func (s *sideScanner) start(ctx context.Context) {
	log.Infof("starting agentless-scanner main loop with %d scan workers", s.poolSize)
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
			if result.err != nil {
				if err := statsd.Count("datadog.agentless_scanner.scans.finished", 1.0, tagFailure(result.scan, result.err), 1.0); err != nil {
					log.Warnf("failed to send metric: %v", err)
				}
			} else {
				if result.sbom != nil {
					if hasResults(result.sbom) {
						log.Debugf("scan %s finished successfully (took %s)", result.scan, result.duration)
						if err := statsd.Count("datadog.agentless_scanner.scans.finished", 1.0, tagSuccess(result.scan), 1.0); err != nil {
							log.Warnf("failed to send metric: %v", err)
						}
					} else {
						log.Debugf("scan %s finished successfully without results (took %s)", result.scan, result.duration)
						if err := statsd.Count("datadog.agentless_scanner.scans.finished", 1.0, tagNoResult(result.scan), 1.0); err != nil {
							log.Warnf("failed to send metric: %v", err)
						}
					}
					if err := s.sendSBOM(result.sbom); err != nil {
						log.Errorf("failed to send SBOM: %v", err)
					}
					if s.printResults {
						fmt.Printf("scanning SBOM result %s (took %s): %s\n", result.scan, result.duration, prototext.Format(result.sbom))
					}
				}
				if result.findings != nil {
					log.Debugf("sending findings for scan %s", result.scan)
					if err := statsd.Count("datadog.agentless_scanner.scans.finished", 1.0, tagSuccess(result.scan), 1.0); err != nil {
						log.Warnf("failed to send metric: %v", err)
					}
					s.sendFindings(result.findings)
					if s.printResults {
						b, _ := json.MarshalIndent(result.findings, "", "  ")
						fmt.Printf("scanning findings result %s (took %s): %s\n", result.scan, result.duration, string(b))
					}
				}
			}
		}
	}()

	for i := 0; i < s.poolSize; i++ {
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
					log.Errorf("error scanning task %s: %s", scan, err)
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
					if len(s.allowedScanTypes) > 0 && !slices.Contains(s.allowedScanTypes, string(scan.Type)) {
						continue
					}
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

	for i := 0; i < s.poolSize; i++ {
		<-done
	}
	close(s.resultsCh)
	<-done // waiting for done in range resultsCh goroutine
	log.Flush()
}

func (s *sideScanner) launchScan(ctx context.Context, scan *scanTask) (err error) {
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

	ctx = withAWSLimits(ctx, s.limits)
	switch scan.Type {
	case hostScanType:
		return scanRoots(ctx, scan, []string{"/"}, s.resultsCh)
	case ebsScanType:
		return scanEBS(ctx, scan, s.resultsCh)
	case lambdaScanType:
		return scanLambda(ctx, scan, s.resultsCh)
	default:
		return fmt.Errorf("unknown scan type: %s", scan.Type)
	}
}

func (s *sideScanner) sendSBOM(entity *sbommodel.SBOMEntity) error {
	sourceAgent := "agentless-scanner"
	envVarEnv := pkgconfig.Datadog.GetString("env")

	entity.DdTags = append(entity.DdTags, "agentless_scanner_host", s.hostname)

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

func (s *sideScanner) sendFindings(findings []*finding) {
	var tags []string // TODO: tags
	expireAt := time.Now().Add(24 * time.Hour)
	for _, finding := range findings {
		finding.ExpireAt = &expireAt
		finding.AgentVersion = version.AgentVersion
		s.findingsReporter.ReportEvent(finding, tags...)
	}
}

func cloudResourceTagSpec(resourceType resourceType) []ec2types.TagSpecification {
	return []ec2types.TagSpecification{
		{
			ResourceType: ec2types.ResourceType(resourceType),
			Tags: []ec2types.Tag{
				{Key: aws.String("DatadogAgentlessScanner"), Value: aws.String("true")},
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

func listResourcesForCleanup(ctx context.Context, ec2client *ec2.Client, maxTTL time.Duration) (map[resourceType][]string, error) {
	toBeDeleted := make(map[resourceType][]string)
	var nextToken *string
	for {
		volumes, err := ec2client.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{
			NextToken: nextToken,
			Filters:   cloudResourceTagFilters(),
		})
		if err != nil {
			return nil, fmt.Errorf("could not list volumes created by agentless-scanner: %w", err)
		}
		for i := range volumes.Volumes {
			if volumes.Volumes[i].State == ec2types.VolumeStateAvailable {
				volumeID := *volumes.Volumes[i].VolumeId
				toBeDeleted[resourceTypeVolume] = append(toBeDeleted[resourceTypeVolume], volumeID)
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
			return nil, fmt.Errorf("could not list snapshots created by agentless-scanner: %w", err)
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
			toBeDeleted[resourceTypeSnapshot] = append(toBeDeleted[resourceTypeSnapshot], snapshotID)
		}
		nextToken = snapshots.NextToken
		if nextToken == nil {
			break
		}
	}
	return toBeDeleted, nil
}

func cloudResourcesCleanup(ctx context.Context, ec2client *ec2.Client, toBeDeleted map[resourceType][]string) {
	for resourceType, resources := range toBeDeleted {
		for _, resourceID := range resources {
			if err := ctx.Err(); err != nil {
				return
			}
			log.Infof("cleaning up resource %s/%s", resourceType, resourceID)
			var err error
			switch resourceType {
			case resourceTypeSnapshot:
				_, err = ec2client.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{
					SnapshotId: aws.String(resourceID),
				})
			case resourceTypeVolume:
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

func statsResourceTTL(resourceType resourceType, scan *scanTask, createTime time.Time) {
	ttl := time.Since(createTime)
	tags := tagScan(scan)
	tags = append(tags, fmt.Sprintf("aws_resource_type:%s", string(resourceType)))
	if err := statsd.Histogram("datadog.agentless_scanner.aws.resources_ttl", float64(ttl.Milliseconds()), tags, 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
}

func createSnapshot(ctx context.Context, scan *scanTask, ec2client *ec2.Client, volumeARN arn.ARN) (snapshotARN arn.ARN, cleanupSnapshot func(context.Context), err error) {
	var createSnapshotOutput *ec2.CreateSnapshotOutput

	cleanupSnapshot = func(ctx context.Context) {
		if createSnapshotOutput != nil {
			log.Debugf("deleting snapshot %q for volume %q", snapshotARN, volumeARN)
			statsResourceTTL(resourceTypeSnapshot, scan, *createSnapshotOutput.StartTime)
			if _, err := ec2client.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{
				SnapshotId: createSnapshotOutput.SnapshotId,
			}); err != nil {
				log.Warnf("could not delete snapshot %s: %v", *createSnapshotOutput.SnapshotId, err)
			} else {
				log.Debugf("snapshot deleted %s", *createSnapshotOutput.SnapshotId)
			}
		}
	}

	snapshotStartedAt := time.Now()
	if err := statsd.Count("datadog.agentless_scanner.snapshots.started", 1.0, tagScan(scan), 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
	log.Debugf("starting volume snapshotting %q", volumeARN)

	retries := 0
retry:
	_, volumeID, _ := getARNResource(volumeARN)
	createSnapshotOutput, err = ec2client.CreateSnapshot(ctx, &ec2.CreateSnapshotInput{
		VolumeId:          aws.String(volumeID),
		TagSpecifications: cloudResourceTagSpec(resourceTypeSnapshot),
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
				log.Debugf("snapshot creation rate exceeded for volume %q; retrying after %v (%d/%d)", volumeARN, d, retries, maxSnapshotRetries)
				sleepCtx(ctx, d)
				goto retry
			}
		}
		if isRateExceededError {
			log.Debugf("snapshot creation rate exceeded for volume %q; skipping)", volumeARN)
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
		return
	}

	snapshotID := *createSnapshotOutput.SnapshotId
	snapshotARN = ec2ARN(volumeARN.Region, volumeARN.AccountID, resourceTypeSnapshot, snapshotID)

	waiter := ec2.NewSnapshotCompletedWaiter(ec2client, func(scwo *ec2.SnapshotCompletedWaiterOptions) {
		scwo.MinDelay = 1 * time.Second
	})
	err = waiter.Wait(ctx, &ec2.DescribeSnapshotsInput{SnapshotIds: []string{snapshotID}}, 10*time.Minute)

	if err == nil {
		snapshotDuration := time.Since(snapshotStartedAt)
		log.Debugf("volume snapshotting of %q finished successfully %q (took %s)", volumeARN, snapshotID, snapshotDuration)
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

	return
}

func tagScan(scan *scanTask, rest ...string) []string {
	return append([]string{
		fmt.Sprintf("agent_version:%s", version.AgentVersion),
		fmt.Sprintf("region:%s", scan.ARN.Region),
		fmt.Sprintf("type:%s", scan.Type),
	}, rest...)
}

func tagNoResult(scan *scanTask) []string {
	return tagScan(scan, "status:noresult")
}

func tagNotFound(scan *scanTask) []string {
	return tagScan(scan, "status:notfound")
}

func tagFailure(scan *scanTask, err error) []string {
	if errors.Is(err, context.Canceled) {
		return tagScan(scan, "status:canceled")
	}
	return tagScan(scan, "status:failure")
}

func tagSuccess(scan *scanTask) []string {
	return append(tagScan(scan), "status:success")
}

type awsRoundtripStats struct {
	transport *http.Transport
	region    string
	limits    *awsLimits
	role      arn.ARN
}

func newHTTPClientWithAWSStats(ctx context.Context, region string, assumedRole *arn.ARN) *http.Client {
	rt := &awsRoundtripStats{
		region: region,
		limits: getAWSLimit(ctx),
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
		Timeout:   10 * time.Second,
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
			sleepCtx(req.Context(), delay)
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
		if err := statsd.Histogram("datadog.agentless_scanner.aws.responses", duration, tags, 1.0); err != nil {
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
		if err := statsd.Histogram("datadog.agentless_scanner.responses.size", float64(contentLength), tags, 1.0); err != nil {
			log.Warnf("failed to send metric: %v", err)
		}
	}
	return resp, nil
}

func newAWSConfig(ctx context.Context, region string, assumedRole *arn.ARN) (aws.Config, error) {
	awsConfigsMu.Lock()
	defer awsConfigsMu.Unlock()

	key := region
	if assumedRole != nil {
		key += assumedRole.String()
	}
	if cfg, ok := awsConfigs[key]; ok {
		return *cfg, nil
	}

	httpClient := newHTTPClientWithAWSStats(ctx, region, assumedRole)
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithHTTPClient(httpClient),
	)
	if err != nil {
		return aws.Config{}, err
	}
	if assumedRole != nil {
		stsclient := sts.NewFromConfig(cfg)
		stsassume := stscreds.NewAssumeRoleProvider(stsclient, assumedRole.String())
		cfg.Credentials = aws.NewCredentialsCache(stsassume)
		stsclient = sts.NewFromConfig(cfg)
		result, err := stsclient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
		if err != nil {
			return aws.Config{}, fmt.Errorf("awsconfig: could not assumerole %q: %w", assumedRole, err)
		}
		log.Debugf("aws config: assuming role with arn=%q", *result.Arn)
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

func scanEBS(ctx context.Context, scan *scanTask, resultsCh chan scanResult) error {
	resourceType, _, err := getARNResource(scan.ARN)
	if err != nil {
		return err
	}
	if scan.Hostname == "" {
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
	case resourceTypeVolume:
		var cleanupSnapshot func(context.Context)
		snapshotARN, cleanupSnapshot, err = createSnapshot(ctx, scan, ec2client, scan.ARN)
		defer func() {
			cleanupctx, cancel := context.WithTimeout(context.Background(), cleanupMaxDuration)
			defer cancel()
			cleanupSnapshot(cleanupctx)
		}()
		if err != nil {
			return err
		}
	case resourceTypeSnapshot:
		snapshotARN = scan.ARN
	default:
		return fmt.Errorf("ebs-volume: bad arn %q", scan.ARN)
	}

	if snapshotARN.Resource == "" {
		return fmt.Errorf("ebs-volume: missing snapshot ID")
	}

	log.Infof("start EBS scanning %s", scan)

	// In noAttach mode we are only able to do host vuln scanning.
	// TODO: remove this mode
	if scan.DiskMode == noAttach {
		// Only vulns scanning works without a proper mount point (for now)
		for _, action := range scan.Actions {
			if action != vulnsHost {
				return fmt.Errorf("we can only perform vulns scanning of %q without volume attach", scan)
			}
		}
		ebsclient := ebs.NewFromConfig(cfg)
		scanStartedAt := time.Now()
		sbom, err := launchScannerTrivyVM(ctx, scan, ebsclient, snapshotARN)
		resultsCh <- scanResult{err: err, scan: scan, sbom: sbom, duration: time.Since(scanStartedAt)}
		scanDuration := time.Since(scanStartedAt)
		if err := statsd.Histogram("datadog.agentless_scanner.scans.duration", float64(scanDuration.Milliseconds()), tagScan(scan), 1.0); err != nil {
			log.Warnf("failed to send metric: %v", err)
		}
		return nil
	}

	var device string
	var attachedVolumeARN *arn.ARN
	var localSnapshotARN arn.ARN
	var cleanupAttach func(context.Context)
	switch scan.DiskMode {
	case volumeAttach:
		var volumeARN arn.ARN
		device, volumeARN, localSnapshotARN, cleanupAttach, err = attachSnapshotWithVolume(ctx, scan, snapshotARN)
		defer func() {
			cleanupctx, cancel := context.WithTimeout(context.Background(), cleanupMaxDuration)
			defer cancel()
			cleanupAttach(cleanupctx)
		}()
		if err != nil {
			return err
		}
		attachedVolumeARN = &volumeARN
	case nbdAttach:
		ebsclient := ebs.NewFromConfig(cfg)
		device, cleanupAttach, err = attachSnapshotWithNBD(ctx, snapshotARN, ebsclient)
		defer func() {
			cleanupctx, cancel := context.WithTimeout(context.Background(), cleanupMaxDuration)
			defer cancel()
			cleanupAttach(cleanupctx)
		}()
		if err != nil {
			return err
		}
		localSnapshotARN = snapshotARN
	default:
		panic("unreachable")
	}

	partitions, err := listDevicePartitions(ctx, device, attachedVolumeARN)
	if err != nil {
		return err
	}

	mountPoints, cleanupMount, err := mountDevice(ctx, localSnapshotARN, partitions)
	defer func() {
		cleanupctx, cancel := context.WithTimeout(context.Background(), cleanupMaxDuration)
		defer cancel()
		cleanupMount(cleanupctx)
	}()
	if err != nil {
		return err
	}

	return scanRoots(ctx, scan, mountPoints, resultsCh)
}

func scanRoots(ctx context.Context, scan *scanTask, roots []string, resultsCh chan scanResult) error {
	scanStartedAt := time.Now()

	for _, root := range roots {
		for _, action := range scan.Actions {
			switch action {
			case vulnsHost:
				start := time.Now()
				sbom, err := launchScannerTrivyLocal(ctx, scan, root, sbommodel.SBOMSourceType_HOST_FILE_SYSTEM, scan.Hostname, nil)
				resultsCh <- scanResult{err: err, scan: scan, sbom: sbom, duration: time.Since(start)}
			case vulnsContainers:
				start := time.Now()
				ctrMountPoints, cleanupMount, err := mountContainers(ctx, scan, root)
				defer func() {
					cleanupctx, cancel := context.WithTimeout(context.Background(), cleanupMaxDuration)
					defer cancel()
					cleanupMount(cleanupctx)
				}()
				if err != nil {
					resultsCh <- scanResult{err: err, scan: scan, duration: time.Since(start)}
				} else {
					for _, ctrMnt := range ctrMountPoints {
						entityID, entityTags := containerTags(ctrMnt)
						sbom, err := launchScannerTrivyLocal(ctx, scan, ctrMnt.Path, sbommodel.SBOMSourceType_CONTAINER_FILE_SYSTEM, entityID, entityTags)
						resultsCh <- scanResult{err: err, scan: scan, sbom: sbom, duration: time.Since(start)}
					}
				}
			case malware:
				start := time.Now()
				findings, err := launchScannerMalwareLocal(ctx, scan, root)
				resultsCh <- scanResult{err: err, scan: scan, findings: findings, duration: time.Since(start)}
			}
		}
	}

	scanDuration := time.Since(scanStartedAt)
	if err := statsd.Histogram("datadog.agentless_scanner.scans.duration", float64(scanDuration.Milliseconds()), tagScan(scan), 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
	return nil
}

func attachSnapshotWithNBD(ctx context.Context, snapshotARN arn.ARN, ebsclient *ebs.Client) (string, func(context.Context), error) {
	ctx, cancel := context.WithCancel(ctx)
	device := nextNBDDevice()
	ebsnbd := NewEBSBlockDevice(EBSBlockDeviceOptions{
		EBSClient:   ebsclient,
		DeviceName:  device,
		SnapshotARN: snapshotARN,
	})
	cleanupAttach := func(ctx context.Context) {
		cancel()
		ebsnbd.WaitCleanup()
	}
	if err := ebsnbd.Start(ctx); err != nil {
		return "", cleanupAttach, err
	}
	return device, cleanupAttach, nil
}

// reference: https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/device_naming.html
var deviceName struct {
	mu   sync.Mutex
	name string
}

func nextDeviceName() string {
	deviceName.mu.Lock()
	defer deviceName.mu.Unlock()
	// loops from "aa" to "zz"
	if deviceName.name == "" || deviceName.name == "zz" {
		deviceName.name = "aa"
	} else if strings.HasSuffix(deviceName.name, "z") {
		deviceName.name = fmt.Sprintf("%ca", deviceName.name[0]+1)
	} else {
		deviceName.name = fmt.Sprintf("%c%c", deviceName.name[0], deviceName.name[1]+1)
	}
	return fmt.Sprintf("/dev/xvd%s", deviceName.name)
}

var nbdDeviceName struct {
	mu    sync.Mutex
	count int
}

func nextNBDDevice() string {
	const nbdsMax = 1024
	nbdDeviceName.mu.Lock()
	defer nbdDeviceName.mu.Unlock()
	count := nbdDeviceName.count
	nbdDeviceName.count += 1 % nbdsMax
	return fmt.Sprintf("/dev/nbd%d", count)
}

func scanLambda(ctx context.Context, scan *scanTask, resultsCh chan scanResult) error {
	defer statsd.Flush()

	tempDir, err := os.MkdirTemp("", "aws-lambda")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	codePath, err := downloadLambda(ctx, scan, tempDir)
	if err != nil {
		return err
	}

	scanStartedAt := time.Now()
	sbom, err := launchScannerTrivyLambda(ctx, scan, codePath)
	resultsCh <- scanResult{err: err, scan: scan, sbom: sbom, duration: time.Since(scanStartedAt)}

	scanDuration := time.Since(scanStartedAt)
	if err := statsd.Histogram("datadog.agentless_scanner.scans.duration", float64(scanDuration.Milliseconds()), tagScan(scan), 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
	return nil
}

func downloadLambda(ctx context.Context, scan *scanTask, tempDir string) (codePath string, err error) {
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

	functionStartedAt := time.Now()

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

	archivePath := filepath.Join(tempDir, "code.zip")
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

	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("lambda: bad status: %s", resp.Status)
	}

	_, err = io.Copy(archiveFile, resp.Body)
	if err != nil {
		return "", err
	}

	codePath = filepath.Join(tempDir, "code")
	err = os.Mkdir(codePath, 0700)
	if err != nil {
		return "", err
	}

	err = extractZip(ctx, archivePath, codePath)
	if err != nil {
		return "", err
	}

	functionDuration := time.Since(functionStartedAt)
	log.Debugf("function retrieved successfully %q (took %s)", scan.ARN, functionDuration)
	if err := statsd.Histogram("datadog.agentless_scanner.functions.duration", float64(functionDuration.Milliseconds()), tagScan(scan), 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
	return codePath, nil
}

func extractZip(ctx context.Context, zipPath, destinationPath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("extractZip: openreader: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		dest := filepath.Join(destinationPath, f.Name)
		destDir := filepath.Dir(dest)
		if err := os.MkdirAll(destDir, 0700); err != nil {
			return err
		}
		if strings.HasSuffix(f.Name, "/") {
			if err := os.Mkdir(dest, 0700); err != nil {
				return err
			}
		} else {
			reader, err := f.Open()
			if err != nil {
				return fmt.Errorf("extractZip: open: %w", err)
			}
			defer reader.Close()
			writer, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
			if err != nil {
				return fmt.Errorf("extractZip: write: %w", err)
			}
			defer writer.Close()
			_, err = io.Copy(writer, reader)
			if err != nil {
				return fmt.Errorf("extractZip: copy: %w", err)
			}
		}
	}
	return nil
}

func attachSnapshotWithVolume(ctx context.Context, scan *scanTask, snapshotARN arn.ARN) (device string, volumeARN arn.ARN, localSnapshotARN arn.ARN, cleanupVolume func(context.Context), err error) {
	var cleanups []func(context.Context)
	pushCleanup := func(cleanup func(context.Context)) {
		cleanups = append(cleanups, cleanup)
	}
	cleanupVolume = func(ctx context.Context) {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i](ctx)
		}
	}

	resourceType, snapshotID, err := getARNResource(snapshotARN)
	if err != nil {
		return
	}
	if resourceType != resourceTypeSnapshot {
		err = fmt.Errorf("expected ARN for a snapshot: %s", snapshotARN.String())
		return
	}

	self, err := getSelfEC2InstanceIndentity(ctx)
	if err != nil {
		err = fmt.Errorf("could not get EC2 instance identity: using attach volumes cannot work outside an EC2 instance: %w", err)
		return
	}

	remoteAssumedRole := scan.Roles[snapshotARN.AccountID]
	remoteAWSCfg, err := newAWSConfig(ctx, self.Region, remoteAssumedRole)
	if err != nil {
		err = fmt.Errorf("could not create local aws config: %w", err)
		return
	}
	remoteEC2Client := ec2.NewFromConfig(remoteAWSCfg)

	if snapshotARN.Region != self.Region {
		log.Debugf("copying snapshot %q into %q", snapshotARN, self.Region)
		var copySnapshot *ec2.CopySnapshotOutput
		copySnapshotStartTime := time.Now()
		copySnapshot, err = remoteEC2Client.CopySnapshot(ctx, &ec2.CopySnapshotInput{
			SourceRegion: aws.String(snapshotARN.Region),
			// DestinationRegion: aws.String(self.Region): automatically filled by SDK
			SourceSnapshotId:  aws.String(snapshotID),
			TagSpecifications: cloudResourceTagSpec(resourceTypeSnapshot),
		})
		if err != nil {
			err = fmt.Errorf("could not copy snapshot %q to %q: %w", snapshotARN, self.Region, err)
			return
		}
		pushCleanup(func(ctx context.Context) {
			log.Debugf("deleting snapshot %q", *copySnapshot.SnapshotId)
			if _, err := remoteEC2Client.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{
				SnapshotId: copySnapshot.SnapshotId,
			}); err != nil {
				log.Warnf("could not delete snapshot %s: %v", *copySnapshot.SnapshotId, err)
			} else {
				log.Debugf("snapshot deleted %s", *copySnapshot.SnapshotId)
				statsResourceTTL(resourceTypeSnapshot, scan, copySnapshotStartTime)
			}
		})
		log.Debugf("waiting for copy of snapshot %q into %q as %q", snapshotARN, self.Region, *copySnapshot.SnapshotId)
		waiter := ec2.NewSnapshotCompletedWaiter(remoteEC2Client, func(scwo *ec2.SnapshotCompletedWaiterOptions) {
			scwo.MinDelay = 1 * time.Second
		})
		err = waiter.Wait(ctx, &ec2.DescribeSnapshotsInput{SnapshotIds: []string{*copySnapshot.SnapshotId}}, 10*time.Minute)
		if err != nil {
			err = fmt.Errorf("could not finish copying %q to %q as %q: %w", snapshotARN, self.Region, *copySnapshot.SnapshotId, err)
			return
		}
		log.Debugf("successfully copied snapshot %q into %q: %q", snapshotARN, self.Region, *copySnapshot.SnapshotId)
		localSnapshotARN = ec2ARN(self.Region, snapshotARN.AccountID, resourceTypeSnapshot, *copySnapshot.SnapshotId)
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
			err = fmt.Errorf("could not modify snapshot attributes %q for sharing with account ID %q: %w", localSnapshotARN, self.AccountID, err)
			return
		}
	}

	localAssumedRole := scan.Roles[self.AccountID]
	localAWSCfg, err := newAWSConfig(ctx, self.Region, localAssumedRole)
	if err != nil {
		err = fmt.Errorf("could not create local aws config: %w", err)
		return
	}
	locaEC2Client := ec2.NewFromConfig(localAWSCfg)

	log.Debugf("creating new volume for snapshot %q in az %q", localSnapshotARN, self.AvailabilityZone)
	_, localSnapshotID, _ := getARNResource(localSnapshotARN)
	volume, err := locaEC2Client.CreateVolume(ctx, &ec2.CreateVolumeInput{
		VolumeType:        ec2types.VolumeTypeGp2,
		AvailabilityZone:  aws.String(self.AvailabilityZone),
		SnapshotId:        aws.String(localSnapshotID),
		TagSpecifications: cloudResourceTagSpec(resourceTypeVolume),
	})
	if err != nil {
		err = fmt.Errorf("could not create volume from snapshot: %s", err)
		return
	}
	pushCleanup(func(ctx context.Context) {
		log.Debugf("detaching volume %q", *volume.VolumeId)
		if _, err := locaEC2Client.DetachVolume(ctx, &ec2.DetachVolumeInput{
			Force:    aws.Bool(true),
			VolumeId: volume.VolumeId,
		}); err != nil {
			log.Warnf("could not detach volume %s: %v", *volume.VolumeId, err)
		}
		var errd error
		for i := 0; i < 50; i++ {
			if !sleepCtx(ctx, 1*time.Second) {
				break
			}
			_, errd = locaEC2Client.DeleteVolume(ctx, &ec2.DeleteVolumeInput{
				VolumeId: volume.VolumeId,
			})
			if errd == nil {
				log.Debugf("volume deleted %q", *volume.VolumeId)
				break
			}
		}
		if errd != nil {
			log.Warnf("could not delete volume %q: %v", *volume.VolumeId, errd)
		} else {
			statsResourceTTL(resourceTypeVolume, scan, *volume.CreateTime)
		}
	})

	device = nextDeviceName()
	log.Debugf("attaching volume %q into device %q", *volume.VolumeId, device)
	var errAttach error
	for i := 0; i < maxAttachRetries; i++ {
		_, errAttach = locaEC2Client.AttachVolume(ctx, &ec2.AttachVolumeInput{
			InstanceId: aws.String(self.InstanceID),
			VolumeId:   volume.VolumeId,
			Device:     aws.String(device),
		})
		if errAttach == nil {
			log.Debugf("volume attached successfully %q device=%s", *volume.VolumeId, device)
			break
		}
		d := 1 * time.Second
		log.Debugf("couldn't attach volume %q into device %q; retrying after %v (%d/%d)", *volume.VolumeId, device, d, i+1, maxAttachRetries)
		if !sleepCtx(ctx, d) {
			break
		}
	}
	if errAttach != nil {
		err = fmt.Errorf("could not attach volume %q into device %q: %w", *volume.VolumeId, device, errAttach)
		return
	}

	volumeARN = ec2ARN(localSnapshotARN.Region, localSnapshotARN.AccountID, resourceTypeVolume, *volume.VolumeId)
	return
}

type devicePartition struct {
	devicePath string
	fsType     string
}

func listDevicePartitions(ctx context.Context, device string, volumeARN *arn.ARN) ([]devicePartition, error) {
	log.Debugf("listing partition from device %q (volume = %q)", device, volumeARN)

	// NOTE(jinroh): we identified that on some Linux kernel the device path
	// may not be the expected one (passed to AttachVolume). The kernel may
	// map the block device to another path. However, the serial number
	// associated with the volume is always of the form volXXX (not vol-XXX).
	// So we use both the expected device path AND the serial number to find
	// the actual block device path.
	var serialNumber *string
	if volumeARN != nil {
		_, volumeID, _ := getARNResource(*volumeARN)
		sn := strings.Replace(volumeID, "-", "", 1) // vol-XXX => volXXX
		serialNumber = &sn
	}

	type blockDevice struct {
		Name     string `json:"name"`
		Serial   string `json:"serial"`
		Path     string `json:"path"`
		Type     string `json:"type"`
		FsType   string `json:"fstype"`
		Children []struct {
			Name   string `json:"name"`
			Path   string `json:"path"`
			Type   string `json:"type"`
			FsType string `json:"fstype"`
		} `json:"children"`
	}

	var blockDevices struct {
		BlockDevices []blockDevice `json:"blockdevices"`
	}

	var foundBlockDevice *blockDevice
	for i := 0; i < 120; i++ {
		if !sleepCtx(ctx, 500*time.Millisecond) {
			break
		}
		lsblkJSON, err := exec.CommandContext(ctx, "lsblk", "--json", "--bytes", "--output", "NAME,SERIAL,PATH,TYPE,FSTYPE").Output()
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(lsblkJSON, &blockDevices); err != nil {
			log.Warnf("lsblk parsing error: %v", err)
			continue
		}
		for _, bd := range blockDevices.BlockDevices {
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
		return nil, fmt.Errorf("could not find the block device for volume %q", volumeARN)
	}

	var partitions []devicePartition
	for i := 0; i < 5; i++ {
		_, _ = exec.CommandContext(ctx, "udevadm", "settle", "--timeout=1").CombinedOutput()
		lsblkJSON, err := exec.CommandContext(ctx, "lsblk", foundBlockDevice.Path, "--json", "--bytes", "--output", "NAME,SERIAL,PATH,TYPE,FSTYPE").Output()
		if err != nil {
			return nil, err
		}
		log.Tracef("lsblkd %q: %s", foundBlockDevice.Path, lsblkJSON)
		if err := json.Unmarshal(lsblkJSON, &blockDevices); err != nil {
			log.Warnf("lsblk parsing error: %v", err)
			continue
		}
		if len(blockDevices.BlockDevices) != 1 {
			continue
		}
		for _, part := range blockDevices.BlockDevices[0].Children {
			if part.FsType == "ext2" || part.FsType == "ext3" || part.FsType == "ext4" || part.FsType == "xfs" {
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
			break
		}
	}
	if len(partitions) == 0 {
		return nil, fmt.Errorf("could not find any ext2, ext3, ext4 or xfs partition in the snapshot %q", volumeARN)
	}

	log.Debugf("found %d compatible partitions for device %q", len(partitions), device)
	return partitions, nil
}

func mountDevice(ctx context.Context, snapshotARN arn.ARN, partitions []devicePartition) (mountPoints []string, cleanupMount func(context.Context), err error) {
	var cleanups []func(context.Context)
	pushCleanup := func(cleanup func(context.Context)) {
		cleanups = append(cleanups, cleanup)
	}
	cleanupMount = func(ctx context.Context) {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i](ctx)
		}
	}

	_, snapshotID, err := getARNResource(snapshotARN)
	if err != nil {
		return
	}

	pushCleanup(func(_ context.Context) {
		baseMountTarget := fmt.Sprintf("/snapshots/%s", snapshotID)
		log.Debugf("unlink directory %q", baseMountTarget)
		os.Remove(baseMountTarget)
	})

	for _, mp := range partitions {
		mountPoint := fmt.Sprintf("/snapshots/%s/%s", snapshotID, path.Base(mp.devicePath))
		err = os.MkdirAll(mountPoint, 0700)
		if err != nil {
			err = fmt.Errorf("could not create mountPoint directory %q: %w", mountPoint, err)
			return
		}
		pushCleanup(func(_ context.Context) {
			log.Debugf("unlink directory %q", mountPoint)
			os.Remove(mountPoint)
		})

		fsOptions := "ro,noauto,nodev,noexec,nosuid," // these are generic options supported for all filesystems
		switch mp.fsType {
		case "ext2", "ext3", "ext4":
			// noload means we do not try to load the journal
			fsOptions += "noload"
		case "xfs":
			// norecovery means we do not try to recover the FS
			fsOptions += "norecovery,nouuid"
		default:
			panic(fmt.Errorf("unsupported filesystem type %s", mp.fsType))
		}

		var mountOutput []byte
		for i := 0; i < 50; i++ {
			log.Debugf("execing mount -o %s -t %s --source %s --target %q", fsOptions, mp.fsType, mp.devicePath, mountPoint)
			mountOutput, err = exec.CommandContext(ctx, "mount", "-o", fsOptions, "-t", mp.fsType, "--source", mp.devicePath, "--target", mountPoint).CombinedOutput()
			if err == nil {
				break
			}
			if !sleepCtx(ctx, 200*time.Millisecond) {
				log.Debugf("mount error %#v: %v", mp, err)
				break
			}
		}
		if err != nil {
			err = fmt.Errorf("could not mount into target=%q device=%q output=%q: %w", mountPoint, mp.devicePath, string(mountOutput), err)
			return
		}
		pushCleanup(func(ctx context.Context) {
			log.Debugf("un-mounting %s", mountPoint)
			for i := 0; i < 10; i++ {
				umountOuput, err := exec.CommandContext(ctx, "umount", "-f", mountPoint).CombinedOutput()
				if err == nil {
					break
				}
				log.Warnf("could not umount %s: %s: %s", mountPoint, err, string(umountOuput))
				if !sleepCtx(ctx, 3*time.Second) {
					break
				}
			}
		})
		mountPoints = append(mountPoints, mountPoint)
	}

	return
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	select {
	case <-time.After(d):
		return true
	case <-ctx.Done():
		return false
	}
}

func ec2ARN(region, accountID string, resourceType resourceType, resourceID string) arn.ARN {
	return arn.ARN{
		Partition: "aws",
		Service:   "ec2",
		Region:    region,
		AccountID: accountID,
		Resource:  fmt.Sprintf("%s/%s", resourceType, resourceID),
	}
}

func hasResults(results *sbommodel.SBOMEntity) bool {
	bom := results.GetCyclonedx()
	// We can't use Dependencies > 0, since len(Dependencies) == 1 when there are no components.
	// See https://github.com/aquasecurity/trivy/blob/main/pkg/sbom/cyclonedx/core/cyclonedx.go
	return len(bom.Components) > 0
}

type awsLimits struct {
	ec2Rate          rate.Limit
	ebsListBlockRate rate.Limit
	ebsGetBlockRate  rate.Limit
	limitersMu       sync.Mutex
	limiters         map[string]*rate.Limiter
}

func newAWSLimits(ec2Rate, ebsListBlockRate, ebsGetBlockRate rate.Limit) *awsLimits {
	return &awsLimits{
		ec2Rate:          ec2Rate,
		ebsListBlockRate: ebsListBlockRate,
		ebsGetBlockRate:  ebsGetBlockRate,
		limiters:         make(map[string]*rate.Limiter),
	}
}

var keyRateLimits = struct{}{}

func withAWSLimits(ctx context.Context, limits *awsLimits) context.Context {
	if limits != nil {
		return context.WithValue(ctx, keyRateLimits, limits)
	}
	return ctx
}

func getAWSLimit(ctx context.Context) *awsLimits {
	limits := ctx.Value(keyRateLimits)
	if limits == nil {
		return newAWSLimits(rate.Inf, rate.Inf, rate.Inf)
	}
	return limits.(*awsLimits)
}

func (l *awsLimits) getLimiter(accountID, region, service, action string) *rate.Limiter {
	var limit rate.Limit
	switch service {
	case "ec2":
		switch {
		// reference: https://docs.aws.amazon.com/AWSEC2/latest/APIReference/throttling.html#throttling-limits
		case strings.HasPrefix(action, "Describe"), strings.HasPrefix(action, "Get"):
			limit = l.ec2Rate
		default:
			limit = l.ec2Rate / 4.0
		}
	case "ebs":
		switch action {
		case "getblock":
			limit = l.ebsGetBlockRate
		case "listblocks", "changedblocks":
			limit = l.ebsListBlockRate
		}
	case "s3", "imds":
		limit = 0.0 // no rate limiting
	default:
		limit = defaultAWSRate
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
