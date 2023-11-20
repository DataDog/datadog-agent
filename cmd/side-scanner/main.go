package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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
	"slices"
	"strings"
	"sync"
	"time"

	// DataDog agent: config stuffs
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	commonpath "github.com/DataDog/datadog-agent/cmd/agent/common/path"
	compconfig "github.com/DataDog/datadog-agent/comp/core/config"
	complog "github.com/DataDog/datadog-agent/comp/core/log"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
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

	// Trivy stuffs
	"github.com/aquasecurity/trivy-db/pkg/db"
	"github.com/aquasecurity/trivy/pkg/detector/ospkg"
	"github.com/aquasecurity/trivy/pkg/fanal/analyzer"
	"github.com/aquasecurity/trivy/pkg/fanal/applier"
	"github.com/aquasecurity/trivy/pkg/fanal/artifact"
	trivyartifactlocal "github.com/aquasecurity/trivy/pkg/fanal/artifact/local"
	"github.com/aquasecurity/trivy/pkg/fanal/artifact/vm"
	ftypes "github.com/aquasecurity/trivy/pkg/fanal/types"
	"github.com/aquasecurity/trivy/pkg/sbom/cyclonedx"
	"github.com/aquasecurity/trivy/pkg/scanner"
	"github.com/aquasecurity/trivy/pkg/scanner/local"
	"github.com/aquasecurity/trivy/pkg/types"
	"github.com/aquasecurity/trivy/pkg/vulnerability"

	"github.com/spf13/cobra"
)

const (
	maxSnapshotRetries = 3
)

var statsd *ddgostatsd.Client

var (
	globalParams struct {
		configFilePath string
		assumedRole    string
		attachVolumes  bool
	}

	defaultHTTPClient = &http.Client{
		Timeout: 10 * time.Second,
	}

	cleanupMaxDuration = 1 * time.Minute
)

type scanType string

const (
	ebsScanType    scanType = "ebs-scan"
	lambdaScanType          = "lambda-scan"
)

func main() {
	flavor.SetFlavor(flavor.SideScannerAgent)
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
	sideScannerCmd := &cobra.Command{
		Use:          "side-scanner [command]",
		Short:        "Datadog Side Scanner at your service.",
		Long:         `Datadog Side Scanner scans your cloud environment for vulnerabilities, compliance and security issues.`,
		SilenceUsage: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			initStatsdClient()
		},
	}

	sideScannerCmd.PersistentFlags().StringVarP(&globalParams.configFilePath, "config-path", "c", path.Join(commonpath.DefaultConfPath, "datadog.yaml"), "specify the path to side-scanner configuration yaml file")
	sideScannerCmd.PersistentFlags().BoolVarP(&globalParams.attachVolumes, "attach-volumes", "", false, "scan EBS snapshots by creating a dedicated volume")
	sideScannerCmd.PersistentFlags().StringVarP(&globalParams.assumedRole, "assumed-role", "", "", "force an AWS role to perform the scan")
	sideScannerCmd.AddCommand(runCommand())
	sideScannerCmd.AddCommand(scanCommand())
	sideScannerCmd.AddCommand(offlineCommand())
	sideScannerCmd.AddCommand(mountCommand())
	sideScannerCmd.AddCommand(cleanupCommand())

	return sideScannerCmd
}

func runCommand() *cobra.Command {
	var runParams struct {
		poolSize         int
		allowedScanTypes []string
	}

	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Runs the side-scanner",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(
				func(_ complog.Component, _ compconfig.Component) error {
					return runCmd(runParams.poolSize, runParams.allowedScanTypes)
				},
				fx.Supply(compconfig.NewAgentParamsWithSecrets(globalParams.configFilePath)),
				fx.Supply(complog.ForDaemon("SIDESCANNER", "log_file", pkgconfig.DefaultSideScannerLogFile)),
				complog.Module,
				compconfig.Module,
			)
		},
	}
	runCmd.Flags().IntVarP(&runParams.poolSize, "workers", "", 40, "number of scans running in parallel")
	runCmd.Flags().StringSliceVarP(&runParams.allowedScanTypes, "allowed-scans-type", "", nil, "lists of allowed scan types (ebs-scan, lambda-scan)")
	return runCmd
}

func scanCommand() *cobra.Command {
	var cliArgs struct {
		RawScan string
	}
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "execute a scan",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(
				func(_ complog.Component, _ compconfig.Component) error {
					return scanCmd([]byte(cliArgs.RawScan))
				},
				fx.Supply(compconfig.NewAgentParamsWithSecrets(globalParams.configFilePath)),
				fx.Supply(complog.ForDaemon("SIDESCANNER", "log_file", pkgconfig.DefaultSideScannerLogFile)),
				complog.Module,
				compconfig.Module,
			)
		},
	}

	cmd.Flags().StringVarP(&cliArgs.RawScan, "raw-scan-data", "", "", "scan data in JSON")

	cmd.MarkFlagRequired("scan-type")
	cmd.MarkFlagRequired("raw-scan-data")
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
		Short: "Runs the side-scanner in offline mode (server-less mode)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(
				func(_ complog.Component, _ compconfig.Component) error {
					return offlineCmd(cliArgs.poolSize, cliArgs.regions, cliArgs.maxScans)
				},
				fx.Supply(compconfig.NewAgentParamsWithSecrets(globalParams.configFilePath)),
				fx.Supply(complog.ForDaemon("SIDESCANNER", "log_file", pkgconfig.DefaultSideScannerLogFile)),
				complog.Module,
				compconfig.Module,
			)
		},
	}

	cmd.Flags().IntVarP(&cliArgs.poolSize, "workers", "", 40, "number of scans running in parallel")
	cmd.Flags().StringSliceVarP(&cliArgs.regions, "regions", "", nil, "list of regions to scan (default to all regions)")
	cmd.Flags().IntVarP(&cliArgs.maxScans, "max-scans", "", 0, "maximum number of scans to perform")

	return cmd
}

func mountCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mount",
		Short: "Runs the side-scanner in offline mode (server-less mode)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(
				func(_ complog.Component, _ compconfig.Component) error {
					return mountCmd(globalParams.assumedRole)
				},
				fx.Supply(compconfig.NewAgentParamsWithSecrets(globalParams.configFilePath)),
				fx.Supply(complog.ForDaemon("SIDESCANNER", "log_file", pkgconfig.DefaultSideScannerLogFile)),
				complog.Module,
				compconfig.Module,
			)
		},
	}

	return cmd
}

func cleanupCommand() *cobra.Command {
	var cliArgs struct {
		region string
		dryRun bool
	}
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Cleanup resources created by the side-scanner",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(
				func(_ complog.Component, _ compconfig.Component) error {
					return cleanupCmd(cliArgs.region, globalParams.assumedRole, cliArgs.dryRun)
				},
				fx.Supply(compconfig.NewAgentParamsWithSecrets(globalParams.configFilePath)),
				fx.Supply(complog.ForDaemon("SIDESCANNER", "log_file", pkgconfig.DefaultSideScannerLogFile)),
				complog.Module,
				compconfig.Module,
			)
		},
	}
	cmd.Flags().StringVarP(&cliArgs.region, "region", "", "us-east-1", "AWS region")
	cmd.Flags().BoolVarP(&cliArgs.dryRun, "dry-run", "", false, "dry run")
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

func runCmd(poolSize int, allowedScanTypes []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	common.SetupInternalProfiling(pkgconfig.Datadog, "")

	hostname, err := utils.GetHostnameWithContext(ctx)
	if err != nil {
		return fmt.Errorf("could not fetch hostname: %w", err)
	}

	rcClient, err := remote.NewUnverifiedGRPCClient("sidescanner", version.AgentVersion, nil, 100*time.Millisecond)
	if err != nil {
		return fmt.Errorf("could not init Remote Config client: %w", err)
	}

	eventForwarder := epforwarder.NewEventPlatformForwarder()

	scanner := newSideScanner(hostname, rcClient, eventForwarder, poolSize, allowedScanTypes)
	scanner.start(ctx)
	return nil
}

func scanCmd(rawScan []byte) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	common.SetupInternalProfiling(pkgconfig.Datadog, "")

	scans, err := unmarshalScanTasks(rawScan)
	if err != nil {
		return err
	}

	for _, scan := range scans {
		entity, err := launchScan(ctx, scan)
		if err != nil {
			log.Errorf("error scanning task %s: %s", scan, err)
		} else {
			fmt.Printf("scanning result %s: %s\n", scan, prototext.Format(entity))
		}
	}
	return nil
}

func offlineCmd(poolSize int, regions []string, maxScans int) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	defer statsd.Flush()

	common.SetupInternalProfiling(pkgconfig.Datadog, "")

	cfg, awsstats, err := newAWSConfig(ctx, "us-east-1", globalParams.assumedRole)
	if err != nil {
		return err
	}
	defer awsstats.SendStats()

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

	scansCh := make(chan scanTask)

	go func() {
		defer close(scansCh)
		scans := make([]scanTask, 0)

		for _, regionName := range allRegions {
			if ctx.Err() != nil {
				return
			}
			scansForRegion, err := listEBSScansForRegion(ctx, regionName)
			if err != nil {
				log.Errorf("could not scan region %q: %v", regionName, err)
			}
			scans = append(scans, scansForRegion...)
		}

		if maxScans > 0 && len(scans) > maxScans {
			scans = scans[:maxScans]
		}

		for _, scan := range scans {
			scansCh <- scan
		}
	}()

	done := make(chan struct{})
	for i := 0; i < poolSize; i++ {
		go func() {
			defer func() {
				done <- struct{}{}
			}()
			for {
				select {
				case <-ctx.Done():
					return
				case scan, ok := <-scansCh:
					if !ok {
						return
					}
					_, err := launchScan(ctx, scan)
					if err != nil {
						log.Errorf("error scanning task %s: %s", scan, err)
					}
				}
			}
		}()
	}
	for i := 0; i < poolSize; i++ {
		<-done
	}
	return nil
}

func listEBSScansForRegion(ctx context.Context, regionName string) (scans []scanTask, err error) {
	cfg, awsstats, err := newAWSConfig(ctx, regionName, globalParams.assumedRole)
	if err != nil {
		return nil, err
	}
	defer awsstats.SendStats()

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
					fmt.Println(regionName, *instance.InstanceId, *blockDeviceMapping.DeviceName, *blockDeviceMapping.Ebs.VolumeId)
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
					accountID := "" // TODO
					scan := ebsScan{
						ARN:      ec2ARN(regionName, accountID, ec2types.ResourceTypeVolume, *blockDeviceMapping.Ebs.VolumeId).String(),
						Hostname: *instance.InstanceId,
					}
					scans = append(scans, scanTask{
						Type: ebsScanType,
						Scan: scan,
					})

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

func cleanupCmd(region string, assumedRole string, dryRun bool) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	cfg, _, err := newAWSConfig(ctx, region, assumedRole)
	if err != nil {
		return err
	}
	ec2client := ec2.NewFromConfig(cfg)
	toBeDeleted, err := listResourcesForCleanup(ctx, ec2client)
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

func downloadSnapshot(ctx context.Context, w io.Writer, snapshotID string) error {
	cfg, awsstats, err := newAWSConfig(ctx, "us-east-1", "")
	if err != nil {
		return err
	}
	defer awsstats.SendStats()

	ebsclient := ebs.NewFromConfig(cfg)
	listSnapshotsInput := &ebs.ListSnapshotBlocksInput{
		SnapshotId: &snapshotID,
		NextToken:  nil,
	}
	var n int64
	for {
		fmt.Printf("listing blocks for %s\n", snapshotID)
		blocks, err := ebsclient.ListSnapshotBlocks(ctx, listSnapshotsInput)
		if err != nil {
			return err
		}
		for _, block := range blocks.Blocks {
			fmt.Printf("getting block %d\n", *block.BlockIndex)
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
			fmt.Printf("copied %d\n", n)
		}
		listSnapshotsInput.NextToken = blocks.NextToken
		if listSnapshotsInput.NextToken == nil {
			return nil
		}
	}
}

func mountCmd(assumedRole string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
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
		arn, err := arn.Parse(line)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bad arn resource %q on line %d\n", line, lineNumber)
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

	for _, resourceARN := range arns {
		cfg, awsstats, err := newAWSConfig(ctx, resourceARN.Region, assumedRole)
		if err != nil {
			return err
		}
		defer awsstats.SendStats()

		ec2client := ec2.NewFromConfig(cfg)

		resourceType, resourceID, ok := getARNResource(resourceARN)
		if !ok {
			return fmt.Errorf("bad arn resource %q", resourceARN.String())
		}
		var snapshotARN arn.ARN
		switch resourceType {
		case ec2types.ResourceTypeVolume:
			sID, cleanupSnapshot, err := createSnapshot(ctx, ec2client, nil, resourceARN)
			cleanups = append(cleanups, cleanupSnapshot)
			if err != nil {
				return err
			}
			snapshotARN = ec2ARN(resourceARN.Region, resourceARN.AccountID, ec2types.ResourceTypeSnapshot, sID)
		case ec2types.ResourceTypeSnapshot:
			snapshotARN = resourceARN
		default:
			return fmt.Errorf("unsupport resource type %q", resourceType)
		}

		localAssumedRole := "" // TODO
		mountTargets, cleanupVolume, err := attachAndMountVolume(ctx, localAssumedRole, nil, snapshotARN)
		cleanups = append(cleanups, cleanupVolume)
		if err != nil {
			return err
		}
		fmt.Printf("%s mount directories:\n", resourceID)
		for _, mountTarget := range mountTargets {
			fmt.Printf("  - %s\n", mountTarget)
		}
	}

	<-ctx.Done()

	return nil
}

type scansTask struct {
	Type     scanType          `json:"type"`
	RawScans []json.RawMessage `json:"scans"`
}

type scanTask struct {
	Type scanType
	Scan interface{}
}

func unmarshalScanTasks(b []byte) ([]scanTask, error) {
	var task scansTask
	if err := json.Unmarshal(b, &task); err != nil {
		return nil, err
	}
	tasks := make([]scanTask, 0, len(task.RawScans))
	for _, rawScan := range task.RawScans {
		switch task.Type {
		case ebsScanType:
			var scan ebsScan
			if err := json.Unmarshal(rawScan, &scan); err != nil {
				return nil, err
			}
			tasks = append(tasks, scanTask{
				Type: task.Type,
				Scan: scan,
			})
		case lambdaScanType:
			var scan lambdaScan
			if err := json.Unmarshal(rawScan, &scan); err != nil {
				return nil, err
			}
			tasks = append(tasks, scanTask{
				Type: task.Type,
				Scan: scan,
			})
		}
	}
	return tasks, nil
}

type ebsScan struct {
	ARN          string `json:"arn"`
	Hostname     string `json:"hostname"`
	AttachVolume bool   `json:"attachVolume,omitempty"`
	AssumedRole  string `json:"assumedRole,omitempty"`
}

func (s ebsScan) Region() string {
	arn, _ := arn.Parse(s.ARN)
	return arn.Region
}

func (s ebsScan) String() string {
	return fmt.Sprintf("ebs_scan=%s hostname=%q", s.ARN, s.Hostname)
}

func getARNResource(arn arn.ARN) (resourceType ec2types.ResourceType, resourceID string, ok bool) {
	switch {
	case strings.HasPrefix(arn.Resource, "volume/"):
		resourceType, resourceID = ec2types.ResourceTypeVolume, strings.TrimPrefix(arn.Resource, "volume/")
	case strings.HasPrefix(arn.Resource, "snapshot/"):
		resourceType, resourceID = ec2types.ResourceTypeSnapshot, strings.TrimPrefix(arn.Resource, "snapshot/")
	}
	ok = resourceType != "" && resourceID != ""
	return
}

type lambdaScan struct {
	ARN         string `json:"arn"`
	AssumedRole string `json:"assumedRole,omitempty"`
}

func (s lambdaScan) Region() string {
	arn, _ := arn.Parse(s.ARN)
	return arn.Region
}

func (s lambdaScan) String() string {
	return fmt.Sprintf("lambda_scan=%s", s.ARN)
}

type sideScanner struct {
	hostname         string
	rcClient         *remote.Client
	poolSize         int
	eventForwarder   epforwarder.EventPlatformForwarder
	allowedScanTypes []string

	scansCh           chan scanTask
	scansInProgress   map[interface{}]struct{}
	scansInProgressMu sync.RWMutex
}

func newSideScanner(hostname string, rcClient *remote.Client, eventForwarder epforwarder.EventPlatformForwarder, poolSize int, allowedScanTypes []string) *sideScanner {
	return &sideScanner{
		hostname:         hostname,
		rcClient:         rcClient,
		eventForwarder:   eventForwarder,
		poolSize:         poolSize,
		scansCh:          make(chan scanTask),
		scansInProgress:  make(map[interface{}]struct{}),
		allowedScanTypes: allowedScanTypes,
	}
}

func (s *sideScanner) start(ctx context.Context) {
	log.Infof("starting side-scanner main loop with %d scan workers", s.poolSize)
	defer log.Infof("stopped side-scanner main loop")

	s.eventForwarder.Start()
	defer s.eventForwarder.Stop()

	s.rcClient.Start()
	defer s.rcClient.Close()

	log.Infof("subscribing to remote-config")
	s.rcClient.Subscribe(state.ProductDebug, func(update map[string]state.RawConfig, _ func(string, state.ApplyStatus)) {
		log.Debugf("received %d remote config config updates", len(update))
		for _, rawConfig := range update {
			s.pushOrSkipScan(ctx, rawConfig)
		}
	})

	done := make(chan struct{})
	for i := 0; i < s.poolSize; i++ {
		go func() {
			for {
				select {
				case <-ctx.Done():
					done <- struct{}{}
					return
				case scan := <-s.scansCh:
					if err := s.launchScanAndSendResult(ctx, scan); err != nil {
						log.Errorf("error scanning task %s: %s", scan, err)
					}
				}
			}
		}()
	}
	for i := 0; i < s.poolSize; i++ {
		<-done
	}
	<-ctx.Done()

}

func (s *sideScanner) pushOrSkipScan(ctx context.Context, rawConfig state.RawConfig) {
	log.Debugf("received new config %q from remote-config of size %d", rawConfig.Metadata.ID, len(rawConfig.Config))
	scans, err := unmarshalScanTasks(rawConfig.Config)
	if err != nil {
		log.Errorf("could not parse side-scanner task: %w", err)
		return
	}
	for _, scan := range scans {
		if len(s.allowedScanTypes) > 0 && !slices.Contains(s.allowedScanTypes, string(scan.Type)) {
			continue
		}
		s.scansInProgressMu.RLock()
		if _, ok := s.scansInProgress[scan]; ok {
			log.Debugf("scan in progress %s; skipping", scan)
			s.scansInProgressMu.RUnlock()
		} else {
			s.scansInProgressMu.RUnlock()
			select {
			case <-ctx.Done():
				return
			case s.scansCh <- scan:
			}
		}
	}
}

func (s *sideScanner) launchScanAndSendResult(ctx context.Context, scan scanTask) error {
	s.scansInProgressMu.Lock()
	s.scansInProgress[scan] = struct{}{}
	s.scansInProgressMu.Unlock()

	defer func() {
		s.scansInProgressMu.Lock()
		delete(s.scansInProgress, scan)
		s.scansInProgressMu.Unlock()
	}()

	entity, err := launchScan(ctx, scan)
	if err != nil {
		return err
	}
	if entity == nil {
		return nil
	}
	sourceAgent := "agent"
	if scan.Type == "lambda-scan" {
		// FIXME: hack
		sourceAgent = "CI"
	}
	return s.sendSBOM(sourceAgent, entity)
}

func (s *sideScanner) sendSBOM(sourceAgent string, entity *sbommodel.SBOMEntity) error {
	envVarEnv := pkgconfig.Datadog.GetString("env")

	entity.DdTags = append(entity.DdTags, "sidescanner_host", s.hostname)

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

func launchScan(ctx context.Context, scan scanTask) (*sbommodel.SBOMEntity, error) {
	switch scan.Type {
	case ebsScanType:
		return scanEBS(ctx, scan.Scan.(ebsScan))
	case lambdaScanType:
		return scanLambda(ctx, scan.Scan.(lambdaScan))
	default:
		return nil, fmt.Errorf("unknown scan type: %s", scan.Type)
	}
}

func cloudResourceTagSpec(resourceType ec2types.ResourceType) []ec2types.TagSpecification {
	return []ec2types.TagSpecification{
		ec2types.TagSpecification{
			ResourceType: resourceType,
			Tags: []ec2types.Tag{
				{Key: aws.String("ddsource"), Value: aws.String("datadog-side-scanner")},
			},
		},
	}
}

func cloudResourceTagFilters() []ec2types.Filter {
	return []ec2types.Filter{
		{
			Name: aws.String("tag:ddsource"),
			Values: []string{
				"datadog-side-scanner",
			},
		},
	}
}

func listResourcesForCleanup(ctx context.Context, ec2client *ec2.Client) (map[ec2types.ResourceType][]string, error) {
	toBeDeleted := make(map[ec2types.ResourceType][]string)
	var nextToken *string
	for {
		volumes, err := ec2client.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{
			NextToken: nextToken,
			Filters:   cloudResourceTagFilters(),
		})
		if err != nil {
			return nil, fmt.Errorf("could not list volumes created by side-scanner: %w", err)
		}
		for i := range volumes.Volumes {
			if volumes.Volumes[i].State == ec2types.VolumeStateAvailable {
				volumeID := *volumes.Volumes[i].VolumeId
				toBeDeleted[ec2types.ResourceTypeVolume] = append(toBeDeleted[ec2types.ResourceTypeVolume], volumeID)
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
			return nil, fmt.Errorf("could not list snapshots created by side-scanner: %w", err)
		}
		for i := range snapshots.Snapshots {
			if snapshots.Snapshots[i].State == ec2types.SnapshotStateCompleted {
				snapshotID := *snapshots.Snapshots[i].SnapshotId
				toBeDeleted[ec2types.ResourceTypeSnapshot] = append(toBeDeleted[ec2types.ResourceTypeSnapshot], snapshotID)
			}
		}
		nextToken = snapshots.NextToken
		if nextToken == nil {
			break
		}
	}
	return toBeDeleted, nil
}

func cloudResourcesCleanup(ctx context.Context, ec2client *ec2.Client, toBeDeleted map[ec2types.ResourceType][]string) {
	for resourceType, resources := range toBeDeleted {
		for _, resourceID := range resources {
			if err := ctx.Err(); err != nil {
				return
			}
			log.Infof("cleaning up resource %s/%s", resourceType, resourceID)
			var err error
			switch resourceType {
			case ec2types.ResourceTypeSnapshot:
				_, err = ec2client.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{
					SnapshotId: aws.String(resourceID),
				})
			case ec2types.ResourceTypeVolume:
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

func createSnapshot(ctx context.Context, ec2client *ec2.Client, metrictags []string, volumeARN arn.ARN) (snapshotID string, cleanupSnapshot func(context.Context), err error) {
	cleanupSnapshot = func(ctx context.Context) {
		if snapshotID != "" {
			log.Debugf("deleting snapshot %q", snapshotID)
			// do not use context here: we want to force snapshot deletion
			ec2client.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{
				SnapshotId: &snapshotID,
			})
		}
	}

	snapshotStartedAt := time.Now()
	statsd.Count("datadog.sidescanner.snapshots.started", 1.0, metrictags, 1.0)
	log.Debugf("starting volume snapshotting %q", volumeARN)

	retries := 0
retry:
	_, volumeID, _ := getARNResource(volumeARN)
	result, err := ec2client.CreateSnapshot(ctx, &ec2.CreateSnapshotInput{
		VolumeId:          aws.String(volumeID),
		TagSpecifications: cloudResourceTagSpec(ec2types.ResourceTypeSnapshot),
	})
	if err != nil {
		var aerr smithy.APIError
		var isRateExceededError bool
		if errors.As(err, &aerr) && aerr.ErrorCode() == "SnapshotCreationPerVolumeRateExceeded" {
			isRateExceededError = true
		}
		if retries <= maxSnapshotRetries {
			retries++
			if isRateExceededError {
				// https://docs.aws.amazon.com/AWSEC2/latest/APIReference/errors-overview.html
				// Wait at least 15 seconds between concurrent volume snapshots.
				d := 15 * time.Second
				log.Debugf("snapshot creation rate exceeded for volume %s; retrying after %v (%d/%d)", volumeARN, d, retries, maxSnapshotRetries)
				sleepCtx(ctx, d)
				goto retry
			}
		}
		if isRateExceededError {
			log.Debugf("snapshot creation rate exceeded for volume %s; skipping)", volumeARN)
		}
	}
	if err != nil {
		var isVolumeNotFoundError bool
		var aerr smithy.APIError
		if errors.As(err, &aerr) && aerr.ErrorCode() == "InvalidVolume.NotFound" {
			isVolumeNotFoundError = true
		}
		if isVolumeNotFoundError {
			metrictags = tagNotFound(metrictags)
		} else {
			metrictags = tagFailure(metrictags)
		}
		statsd.Count("datadog.sidescanner.snapshots.finished", 1.0, metrictags, 1.0)
		return
	}

	snapshotID = *result.SnapshotId

	waiter := ec2.NewSnapshotCompletedWaiter(ec2client, func(scwo *ec2.SnapshotCompletedWaiterOptions) {
		scwo.MinDelay = 1 * time.Second
	})
	err = waiter.Wait(ctx, &ec2.DescribeSnapshotsInput{SnapshotIds: []string{snapshotID}}, 10*time.Minute)

	if err == nil {
		snapshotDuration := time.Since(snapshotStartedAt)
		log.Debugf("volume snapshotting finished sucessfully %q (took %s)", snapshotID, snapshotDuration)
		statsd.Histogram("datadog.sidescanner.snapshots.duration", float64(snapshotDuration.Milliseconds()), metrictags, 1.0)
		statsd.Count("datadog.sidescanner.snapshots.finished", 1.0, tagSuccess(metrictags), 1.0)
	} else {
		statsd.Count("datadog.sidescanner.snapshots.finished", 1.0, tagFailure(metrictags), 1.0)
	}
	return
}

func tagNoResult(s []string) []string {
	return append(s, fmt.Sprint("status:noresult"))
}

func tagNotFound(s []string) []string {
	return append(s, fmt.Sprint("status:notfound"))
}

func tagFailure(s []string) []string {
	return append(s, fmt.Sprint("status:failure"))
}

func tagSuccess(s []string) []string {
	return append(s, fmt.Sprint("status:success"))
}

type awsClientStats struct {
	transport *http.Transport
	statsMu   sync.Mutex
	ec2stats  map[string]float64
	ebsstats  map[string]float64
}

func (rt *awsClientStats) SendStats() {
	rt.statsMu.Lock()
	defer rt.statsMu.Unlock()

	for action, value := range rt.ec2stats {
		statsd.Histogram("datadog.sidescanner.awsstats.actions", value, rt.tags("ec2", action), 1.0)
	}
	for action, value := range rt.ebsstats {
		statsd.Histogram("datadog.sidescanner.awsstats.actions", value, rt.tags("ebs", action), 1.0)
	}
	statsd.Count("datadog.sidescanner.awsstats.total_requests", int64(len(rt.ec2stats)), rt.tags("ec2"), 1.0)
	statsd.Count("datadog.sidescanner.awsstats.total_requests", int64(len(rt.ebsstats)), rt.tags("ebs"), 1.0)

	rt.ec2stats = nil
	rt.ebsstats = nil
}

func (ty *awsClientStats) tags(serviceName string, actions ...string) []string {
	tags := []string{
		fmt.Sprintf("aws_service:", serviceName),
	}
	for _, action := range actions {
		tags = append(tags, fmt.Sprintf("aws_action:%s_%s", serviceName, action))
	}
	return tags
}

var (
	ebsGetBlockReg = regexp.MustCompile("/snapshots/(snap-[a-z0-9]+)/blocks/([0-9]+)")
)

func (rt *awsClientStats) recordStats(req *http.Request) error {
	rt.statsMu.Lock()
	defer rt.statsMu.Unlock()

	if rt.ec2stats == nil {
		rt.ec2stats = make(map[string]float64, 0)
	}
	if rt.ebsstats == nil {
		rt.ebsstats = make(map[string]float64, 0)
	}

	switch {
	// EBS
	case strings.HasPrefix(req.URL.Host, "ebs."):
		if ebsGetBlockReg.MatchString(req.URL.Path) {
			// https://ebs.us-east-1.amazonaws.com/snapshots/snap-0d136ea9e1e8767ea/blocks/X/
			rt.ebsstats["getblock"] += 1
		}

	// EC2
	case req.URL.Host == "ec2.amazonaws.com":
		if req.Method == http.MethodPost && req.Body != nil {
			body, err := io.ReadAll(req.Body)
			if err != nil {
				return err
			}
			form, err := url.ParseQuery(string(body))
			if err == nil {
				if action := form.Get("Action"); action != "" {
					rt.ec2stats[action] += 1.0
				}
			}
			req.Body = io.NopCloser(bytes.NewReader(body))
		} else if req.Method == http.MethodGet {
			form := req.URL.Query()
			if action := form.Get("Action"); action != "" {
				rt.ec2stats[action] += 1.0
			}
		}
	}

	return nil
}

func (rt *awsClientStats) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := rt.recordStats(req); err != nil {
		return nil, err
	}
	resp, err := rt.transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func newAWSConfig(ctx context.Context, region string, assumedRoleARN string) (aws.Config, *awsClientStats, error) {
	awsstats := &awsClientStats{
		transport: &http.Transport{
			IdleConnTimeout: 10 * time.Second,
			MaxIdleConns:    10,
		},
	}

	httpClient := *defaultHTTPClient
	httpClient.Transport = awsstats

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithHTTPClient(&httpClient),
	)
	if err != nil {
		return aws.Config{}, nil, err
	}
	if assumedRoleARN != "" {
		stsclient := sts.NewFromConfig(cfg)
		stsassume := stscreds.NewAssumeRoleProvider(stsclient, assumedRoleARN)
		cfg.Credentials = aws.NewCredentialsCache(stsassume)

		// TODO(jinroh): we may want to omit this check. This is mostly to
		// make sure that the configuration is effective.
		stsclient = sts.NewFromConfig(cfg)
		result, err := stsclient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
		if err != nil {
			return aws.Config{}, nil, fmt.Errorf("awsconfig: could not assumerole %q: %w", assumedRoleARN, err)
		}
		log.Debugf("aws config: assuming role with arn=%q", *result.Arn)
	}

	return cfg, awsstats, nil
}

func getSelfEC2InstanceIndentity(ctx context.Context) (*imds.GetInstanceIdentityDocumentOutput, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	imdsclient := imds.NewFromConfig(cfg)
	return imdsclient.GetInstanceIdentityDocument(ctx, &imds.GetInstanceIdentityDocumentInput{})
}

func scanEBS(ctx context.Context, scan ebsScan) (entity *sbommodel.SBOMEntity, err error) {
	if scan.AssumedRole == "" {
		scan.AssumedRole = globalParams.assumedRole // TODO(pierre): remove this HACK
	}
	resourceARN, err := arn.Parse(scan.ARN)
	if err != nil {
		return
	}
	resourceType, _, ok := getARNResource(resourceARN)
	if !ok {
		return nil, fmt.Errorf("ebs-scan: bad or missing ARN: %w", err)
	}
	if scan.Hostname == "" {
		return nil, fmt.Errorf("ebs-scan: missing hostname")
	}

	defer statsd.Flush()

	tags := []string{
		fmt.Sprintf("region:%s", scan.Region),
		fmt.Sprintf("type:%s", ebsScanType),
		fmt.Sprintf("scan_host:%s", scan.Hostname),
	}

	if err != nil {
		return nil, err
	}

	cfg, awsstats, err := newAWSConfig(ctx, scan.Region(), scan.AssumedRole)
	if err != nil {
		return nil, err
	}
	defer awsstats.SendStats()

	ec2client := ec2.NewFromConfig(cfg)
	if err != nil {
		return nil, err
	}

	var snapshotARN arn.ARN
	switch resourceType {
	case ec2types.ResourceTypeVolume:
		sID, cleanupSnapshot, err := createSnapshot(ctx, ec2client, tags, resourceARN)
		defer func() {
			cleanupctx, cancel := context.WithTimeout(context.Background(), cleanupMaxDuration)
			defer cancel()
			cleanupSnapshot(cleanupctx)
		}()
		if err != nil {
			return nil, err
		}
		snapshotARN = ec2ARN(resourceARN.Region, resourceARN.AccountID, ec2types.ResourceTypeSnapshot, sID)
	case ec2types.ResourceTypeSnapshot:
		snapshotARN = resourceARN
	default:
		return nil, fmt.Errorf("ebs-scan: bad arn %q", resourceARN)
	}

	if snapshotARN.Resource == "" {
		return nil, fmt.Errorf("ebs-scan: missing snapshot ID")
	}

	log.Infof("start EBS scanning %s", scan)

	noResult := false
	statsd.Count("datadog.sidescanner.scans.started", 1.0, tags, 1.0)
	defer func() {
		if err != nil {
			statsd.Count("datadog.sidescanner.scans.finished", 1.0, tagFailure(tags), 1.0)
		} else if noResult {
			statsd.Count("datadog.sidescanner.scans.finished", 1.0, tagNoResult(tags), 1.0)
		} else {
			statsd.Count("datadog.sidescanner.scans.finished", 1.0, tagSuccess(tags), 1.0)
		}
	}()

	trivyCache := newMemoryCache()
	trivyDisabledAnalyzers := []analyzer.Type{analyzer.TypeSecret, analyzer.TypeLicenseFile}
	trivyDisabledAnalyzers = append(trivyDisabledAnalyzers, analyzer.TypeConfigFiles...)
	trivyDisabledAnalyzers = append(trivyDisabledAnalyzers, analyzer.TypeLanguages...)
	var trivyArtifact artifact.Artifact

	if scan.AttachVolume || globalParams.attachVolumes {
		localAssumedRole := ""
		mountTargets, cleanupVolume, err := attachAndMountVolume(ctx, localAssumedRole, tags, snapshotARN)
		defer func() {
			cleanupctx, cancel := context.WithTimeout(context.Background(), cleanupMaxDuration)
			defer cancel()
			cleanupVolume(cleanupctx)
		}()
		if err != nil {
			return nil, err
		}
		// TODO(jinroh): support multiple partition scanning
		mountTarget := mountTargets[0]
		trivyArtifact, err = trivyartifactlocal.NewArtifact(mountTarget, trivyCache, artifact.Option{
			Offline:           true,
			NoProgress:        true,
			DisabledAnalyzers: trivyDisabledAnalyzers,
			Slow:              false,
			SBOMSources:       []string{},
			DisabledHandlers:  []ftypes.HandlerType{ftypes.UnpackagedPostHandler},
			OnlyDirs: []string{
				filepath.Join(mountTarget, "etc"),
				filepath.Join(mountTarget, "var/lib/dpkg"),
				filepath.Join(mountTarget, "var/lib/rpm"),
				filepath.Join(mountTarget, "lib/apk"),
			},
			AWSRegion: scan.Region(),
		})
		if err != nil {
			return nil, fmt.Errorf("could not create local trivy artifact: %w", err)
		}
	} else {
		_, snapshotID, _ := getARNResource(snapshotARN)
		target := "ebs:" + snapshotID
		trivyArtifact, err = vm.NewArtifact(target, trivyCache, artifact.Option{
			Offline:           true,
			NoProgress:        true,
			DisabledAnalyzers: trivyDisabledAnalyzers,
			Slow:              false,
			SBOMSources:       []string{},
			DisabledHandlers:  []ftypes.HandlerType{ftypes.UnpackagedPostHandler},
			OnlyDirs:          []string{"etc", "var/lib/dpkg", "var/lib/rpm", "lib/apk"},
			AWSRegion:         scan.Region(),
		})
		ebsclient := ebs.NewFromConfig(cfg)
		if err != nil {
			return nil, fmt.Errorf("could not create EBS client: %w", err)
		}
		trivyArtifactEBS := trivyArtifact.(*vm.EBS)
		trivyArtifactEBS.SetEBS(EBSClientWithWalk{ebsclient})
	}

	scanStartedAt := time.Now()

	trivyDetector := ospkg.Detector{}
	trivyVulnClient := vulnerability.NewClient(db.Config{})
	trivyApplier := applier.NewApplier(trivyCache)
	trivyLocalScanner := local.NewScanner(trivyApplier, trivyDetector, trivyVulnClient)
	trivyScanner := scanner.NewScanner(trivyLocalScanner, trivyArtifact)

	log.Debugf("starting scan of artifact")
	trivyReport, err := trivyScanner.ScanArtifact(ctx, types.ScanOptions{
		VulnType:            []string{},
		SecurityChecks:      []string{},
		ScanRemovedPackages: false,
		ListAllPackages:     true,
	})
	if err != nil {
		return nil, fmt.Errorf("trivy scan failed: %w", err)
	}

	noResult = !hasPackages(trivyReport.Results)
	scanDuration := time.Since(scanStartedAt)
	log.Debugf("ebs-scan: finished (took %s)", scanDuration)
	statsd.Histogram("datadog.sidescanner.scans.duration", float64(scanDuration.Milliseconds()), tags, 1.0)

	createdAt := time.Now()
	duration := time.Since(scanStartedAt)
	marshaler := cyclonedx.NewMarshaler("")
	bom, err := marshaler.Marshal(trivyReport)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal report to sbom format: %w", err)
	}

	entity = &sbommodel.SBOMEntity{
		Status:             sbommodel.SBOMStatus_SUCCESS,
		Type:               sbommodel.SBOMSourceType_HOST_FILE_SYSTEM, // TODO: SBOMSourceType_EBS
		Id:                 scan.Hostname,
		InUse:              true,
		GeneratedAt:        timestamppb.New(createdAt),
		GenerationDuration: convertDuration(duration),
		Hash:               "",
		Sbom: &sbommodel.SBOMEntity_Cyclonedx{
			Cyclonedx: convertBOM(bom),
		},
	}
	return
}

func hasPackages(results types.Results) bool {
	for _, r := range results {
		if len(r.Packages) > 0 {
			return true
		}
	}
	return false
}

// reference: https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/device_naming.html
var deviceName struct {
	mu     sync.Mutex
	letter byte
}

func nextDeviceName() string {
	deviceName.mu.Lock()
	defer deviceName.mu.Unlock()
	if deviceName.letter == 0 || deviceName.letter == 'p' {
		deviceName.letter = 'f'
	} else {
		deviceName.letter += 1
	}
	return fmt.Sprintf("/dev/sd%c", deviceName.letter)
}

func scanLambda(ctx context.Context, scan lambdaScan) (entity *sbommodel.SBOMEntity, err error) {
	if scan.AssumedRole == "" {
		scan.AssumedRole = globalParams.assumedRole // TODO(pierre): remove this HACK
	}
	if _, err := arn.Parse(scan.ARN); err != nil {
		return nil, fmt.Errorf("lambda-scan: bad or missing ARN: %w", err)
	}

	defer statsd.Flush()

	tags := []string{
		fmt.Sprintf("region:%s", scan.Region),
		fmt.Sprintf("type:%s", "lambda-scan"),
	}

	tempDir, err := os.MkdirTemp("", "aws-lambda")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)

	codePath, err := downloadLambda(ctx, scan, tempDir, tags)
	if err != nil {
		return
	}

	statsd.Count("datadog.sidescanner.scans.started", 1.0, tags, 1.0)
	defer func() {
		if err != nil {
			statsd.Count("datadog.sidescanner.scans.finished", 1.0, tagFailure(tags), 1.0)
		} else {
			statsd.Count("datadog.sidescanner.scans.finished", 1.0, tagSuccess(tags), 1.0)
		}
	}()

	scanStartedAt := time.Now()
	trivyCache := newMemoryCache()
	trivyFSArtifact, err := trivyartifactlocal.NewArtifact(codePath, trivyCache, artifact.Option{
		Offline:           true,
		NoProgress:        true,
		DisabledAnalyzers: []analyzer.Type{},
		Slow:              true,
		SBOMSources:       []string{},
		AWSRegion:         scan.Region(),
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create artifact from fs: %w", err)
	}

	trivyDetector := ospkg.Detector{}
	trivyVulnClient := vulnerability.NewClient(db.Config{})
	trivyApplier := applier.NewApplier(trivyCache)
	trivyLocalScanner := local.NewScanner(trivyApplier, trivyDetector, trivyVulnClient)
	trivyScanner := scanner.NewScanner(trivyLocalScanner, trivyFSArtifact)
	trivyReport, err := trivyScanner.ScanArtifact(ctx, types.ScanOptions{
		VulnType:            []string{},
		SecurityChecks:      []string{},
		ScanRemovedPackages: false,
		ListAllPackages:     true,
	})
	statsd.Histogram("datadog.sidescanner.scans.duration", float64(time.Since(scanStartedAt).Milliseconds()), tags, 1.0)
	if err != nil {
		return nil, fmt.Errorf("trivy scan failed: %w", err)
	}

	createdAt := time.Now()
	duration := time.Since(scanStartedAt)
	marshaler := cyclonedx.NewMarshaler("")
	bom, err := marshaler.Marshal(trivyReport)
	if err != nil {
		return nil, err
	}

	entity = &sbommodel.SBOMEntity{
		Status: sbommodel.SBOMStatus_SUCCESS,
		Type:   sbommodel.SBOMSourceType_HOST_FILE_SYSTEM, // TODO: SBOMSourceType_LAMBDA
		Id:     "",
		InUse:  true,
		DdTags: []string{
			"function:" + scan.ARN,
		},
		GeneratedAt:        timestamppb.New(createdAt),
		GenerationDuration: convertDuration(duration),
		Hash:               "",
		Sbom: &sbommodel.SBOMEntity_Cyclonedx{
			Cyclonedx: convertBOM(bom),
		},
	}
	return
}

func downloadLambda(ctx context.Context, scan lambdaScan, tempDir string, tags []string) (codePath string, err error) {
	statsd.Count("datadog.sidescanner.functions.started", 1.0, tags, 1.0)
	defer func() {
		if err != nil {
			var isResourceNotFoundError bool
			var aerr smithy.APIError
			if errors.As(err, &aerr) && aerr.ErrorCode() == "ResourceNotFoundException" {
				isResourceNotFoundError = true
			}
			if isResourceNotFoundError {
				tags = tagNotFound(tags)
			} else {
				tags = tagFailure(tags)
			}
			statsd.Count("datadog.sidescanner.functions.finished", 1.0, tagFailure(tags), 1.0)
		}
	}()

	functionStartedAt := time.Now()

	cfg, awsstats, err := newAWSConfig(ctx, scan.Region(), scan.AssumedRole)
	if err != nil {
		return "", err
	}
	defer awsstats.SendStats()

	lambdaclient := lambda.NewFromConfig(cfg)
	if err != nil {
		return "", err
	}

	lambdaFunc, err := lambdaclient.GetFunction(ctx, &lambda.GetFunctionInput{
		FunctionName: aws.String(scan.ARN),
	})
	if err != nil {
		return "", err
	}

	if lambdaFunc.Code.Location == nil {
		return "", fmt.Errorf("lambdaFunc.Code.Location is nil")
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

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bad status: %s", resp.Status)
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
	log.Debugf("function retrieved sucessfully %q (took %s)", scan.ARN, functionDuration)
	statsd.Count("datadog.sidescanner.functions.finished", 1.0, tagSuccess(tags), 1.0)
	statsd.Histogram("datadog.sidescanner.functions.duration", float64(functionDuration.Milliseconds()), tags, 1.0)
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

func attachAndMountVolume(ctx context.Context, localAssumedRole string, metrictags []string, snapshotARN arn.ARN) (mountTargets []string, cleanupVolume func(context.Context), err error) {
	var cleanups []func(context.Context)
	pushCleanup := func(cleanup func(context.Context)) {
		cleanups = append(cleanups, cleanup)
	}
	cleanupVolume = func(ctx context.Context) {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i](ctx)
		}
	}

	resourceType, _, ok := getARNResource(snapshotARN)
	if !ok || resourceType != ec2types.ResourceTypeSnapshot {
		err = fmt.Errorf("expected ARN for a snapshot: %s", snapshotARN.String())
		return
	}

	self, err := getSelfEC2InstanceIndentity(ctx)
	if err != nil {
		err = fmt.Errorf("could not get EC2 instance identity: using attach volumes cannot work outside an EC2 instance: %w", err)
		return
	}

	localAWSCfg, _, err := newAWSConfig(ctx, self.Region, localAssumedRole)
	if err != nil {
		err = fmt.Errorf("could not create local aws config: %w", err)
		return
	}
	ec2client := ec2.NewFromConfig(localAWSCfg)

	if snapshotARN.Region != self.Region {
		log.Debugf("copying snapshot %q into %q", snapshotARN, self.Region)
		_, snapshotID, _ := getARNResource(snapshotARN)
		var copySnapshot *ec2.CopySnapshotOutput
		copySnapshot, err = ec2client.CopySnapshot(ctx, &ec2.CopySnapshotInput{
			SourceRegion:      aws.String(snapshotARN.Region),
			SourceSnapshotId:  aws.String(snapshotID),
			TagSpecifications: cloudResourceTagSpec(ec2types.ResourceTypeSnapshot),
		})
		if err != nil {
			err = fmt.Errorf("could not copy snapshot %q to %q: %w", snapshotARN, self.Region, err)
			return
		}
		pushCleanup(func(ctx context.Context) {
			log.Debugf("deleting snapshot %q", *copySnapshot.SnapshotId)
			// do not use context here: we want to force snapshot deletion
			ec2client.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{
				SnapshotId: copySnapshot.SnapshotId,
			})
		})
		log.Debugf("waiting for copy of snapshot %q into %q as %q", snapshotARN, self.Region, *copySnapshot.SnapshotId)
		waiter := ec2.NewSnapshotCompletedWaiter(ec2client, func(scwo *ec2.SnapshotCompletedWaiterOptions) {
			scwo.MinDelay = 1 * time.Second
		})
		err = waiter.Wait(ctx, &ec2.DescribeSnapshotsInput{SnapshotIds: []string{*copySnapshot.SnapshotId}}, 10*time.Minute)
		if err != nil {
			err = fmt.Errorf("could not finish copying %q to %q as %q: %w", snapshotARN, self.Region, *copySnapshot.SnapshotId, err)
			return
		}
		log.Debugf("successfully copied snapshot %q into %q: %q", snapshotARN, self.Region, *copySnapshot.SnapshotId)
		snapshotARN = ec2ARN(self.Region, self.AccountID, ec2types.ResourceTypeSnapshot, *copySnapshot.SnapshotId)
	}

	log.Debugf("creating new volume for snapshot %q in az %q", snapshotARN, self.AvailabilityZone)
	_, localSnapshotID, _ := getARNResource(snapshotARN)
	volume, err := ec2client.CreateVolume(ctx, &ec2.CreateVolumeInput{
		VolumeType:        ec2types.VolumeTypeGp2,
		AvailabilityZone:  aws.String(self.AvailabilityZone),
		SnapshotId:        aws.String(localSnapshotID),
		TagSpecifications: cloudResourceTagSpec(ec2types.ResourceTypeVolume),
	})
	if err != nil {
		err = fmt.Errorf("could not create volume from snapshot: %s", err)
		return
	}
	pushCleanup(func(ctx context.Context) {
		// do not use context here: we want to force deletion
		log.Debugf("detaching volume %q", *volume.VolumeId)
		ec2client.DetachVolume(ctx, &ec2.DetachVolumeInput{
			Force:    aws.Bool(true),
			VolumeId: volume.VolumeId,
		})
		var errd error
		for i := 0; i < 10; i++ {
			_, errd = ec2client.DeleteVolume(ctx, &ec2.DeleteVolumeInput{
				VolumeId: volume.VolumeId,
			})
			if errd == nil {
				log.Debugf("volume deleted %q", *volume.VolumeId)
				break
			}
			if !sleepCtx(ctx, 1*time.Second) {
				break
			}
		}
		if errd != nil {
			log.Warnf("could not delete volume %q: %v", *volume.VolumeId, errd)
		}
	})

	device := nextDeviceName()
	log.Debugf("attaching volume %q into device %q", *volume.VolumeId, device)
	for i := 0; i < 10; i++ {
		_, err = ec2client.AttachVolume(ctx, &ec2.AttachVolumeInput{
			InstanceId: aws.String(self.InstanceID),
			VolumeId:   volume.VolumeId,
			Device:     aws.String(device),
		})
		if err == nil {
			break
		}
		if !sleepCtx(ctx, 1*time.Second) {
			break
		}
	}
	if err != nil {
		err = fmt.Errorf("could not attach volume %q into device %q: %w", *volume.VolumeId, device, err)
		return
	}

	type mountPoint struct {
		partDevice string
		partFsType string
	}

	var mountPoints []mountPoint
	for i := 0; i < 60; i++ {
		if !sleepCtx(ctx, 500*time.Millisecond) {
			break
		}
		lsblkJSON, err := exec.CommandContext(ctx, "lsblk", device, "--paths", "--json", "--bytes", "--fs", "--output", "NAME,PATH,TYPE,FSTYPE").Output()
		if err != nil {
			log.Warnf("lsblk error: %v", err)
			continue
		}
		log.Debugf("lsblk %q: %s", device, string(lsblkJSON))
		var blockDevices struct {
			BlockDevices []struct {
				Name     string `json:"name"`
				Path     string `json:"path"`
				Type     string `json:"type"`
				FsType   string `json:"fstype"`
				Children []struct {
					Name   string `json:"name"`
					Path   string `json:"path"`
					Type   string `json:"type"`
					FsType string `json:"fstype"`
				} `json:"children"`
			} `json:"blockdevices"`
		}
		if err := json.Unmarshal(lsblkJSON, &blockDevices); err != nil {
			log.Warnf("lsblk parsing error: %v", err)
			continue
		}
		if len(blockDevices.BlockDevices) == 0 {
			continue
		}
		blockDevice := blockDevices.BlockDevices[0]
		for _, child := range blockDevice.Children {
			if child.Type == "part" && (child.FsType == "ext4" || child.FsType == "xfs") {
				mountPoints = append(mountPoints, mountPoint{
					partDevice: child.Path,
					partFsType: child.FsType,
				})
			}
		}
		if len(mountPoints) > 0 {
			break
		}
	}

	if len(mountPoints) == 0 {
		err = fmt.Errorf("could not find any mountpoint in the snapshot %q", snapshotARN)
		return
	}

	pushCleanup(func(_ context.Context) {
		baseMountTarget := fmt.Sprintf("/data/%s", localSnapshotID)
		log.Debugf("unlink directory %q", baseMountTarget)
		os.Remove(baseMountTarget)
	})

	for _, mp := range mountPoints {
		mountTarget := fmt.Sprintf("/data/%s/%s", localSnapshotID, path.Base(mp.partDevice))
		err = os.MkdirAll(mountTarget, 0700)
		if err != nil {
			err = fmt.Errorf("could not create mountTarget directory %q: %w", mountTarget, err)
			return
		}
		pushCleanup(func(_ context.Context) {
			log.Debugf("unlink directory %q", mountTarget)
			os.Remove(mountTarget)
		})

		var mountOutput []byte
		for i := 0; i < 50; i++ {
			log.Debugf("execing mount %q %q", mp.partDevice, mountTarget)
			mountOutput, err = exec.CommandContext(ctx, "mount", "-t", mp.partFsType, "--source", mp.partDevice, "--target", mountTarget).CombinedOutput()
			if err == nil {
				break
			}
			if !sleepCtx(ctx, 200*time.Millisecond) {
				break
			}
		}
		if err != nil {
			err = fmt.Errorf("could not mount into target=%q device=%q output=%q: %w", mountTarget, mp.partDevice, string(mountOutput), err)
			return
		}
		pushCleanup(func(ctx context.Context) {
			log.Debugf("un-mounting %s", mountTarget)
			umountOuput, err := exec.CommandContext(ctx, "umount", "-f", mountTarget).CombinedOutput()
			if err != nil {
				log.Warnf("could not umount %s: %s:\n%s", mountTarget, err, string(umountOuput))
			}
		})
		mountTargets = append(mountTargets, mountTarget)
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

func ec2ARN(region, accountID string, resourceType ec2types.ResourceType, resourceID string) arn.ARN {
	return arn.ARN{
		Partition: "aws",
		Service:   "ec2",
		Region:    region,
		AccountID: accountID,
		Resource:  fmt.Sprintf("%s/%s", resourceType, resourceID),
	}
}
