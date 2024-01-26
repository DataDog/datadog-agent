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
	"crypto/sha256"
	"encoding/hex"
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

	"github.com/spf13/cobra"
)

const (
	loggerName = "AGENTLESSSCANER"

	maxSnapshotRetries = 3
	maxAttachRetries   = 15

	maxLambdaUncompressed = 256 * 1024 * 1024

	defaultWorkersCount = 40

	defaultSelfRegion      = "us-east-1"
	defaultSnapshotsMaxTTL = 24 * time.Hour
)

var statsd *ddgostatsd.Client

var (
	globalParams struct {
		configFilePath string
		diskMode       diskMode
		defaultActions []string
		noForkScanners bool
	}

	cleanupMaxDuration = 2 * time.Minute

	awsConfigs   = make(map[string]*aws.Config)
	awsConfigsMu sync.Mutex
)

const (
	scansRootDir = "/scans"

	ebsMountPrefix    = "ebs-"
	ctrdMountPrefix   = "ctrd-"
	dockerMountPrefix = "docker-"
	lambdaMountPrefix = "lambda-"
)

type configType string

const (
	awsScan configType = "aws-scan"
)

type scanType string

const (
	hostScanType   scanType = "localhost-scan"
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

type scannerName string

const (
	scannerNameHostVulns    scannerName = "hostvulns"
	scannerNameHostVulnsEBS scannerName = "hostvulns-ebs"
	scannerNameAppVulns     scannerName = "appvulns"
	scannerNameContainers   scannerName = "containers"
	scannerNameMalware      scannerName = "malware"
)

type resourceType string

const (
	resourceTypeLocalDir = "localdir"
	resourceTypeVolume   = "volume"
	resourceTypeSnapshot = "snapshot"
	resourceTypeFunction = "function"
	resourceTypeRole     = "role"
)

type (
	rolesMapping map[string]*arn.ARN

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
		ID        string       `json:"ID"`
		CreatedAt time.Time    `json:"CreatedAt"`
		StartedAt time.Time    `json:"StartedAt"`
		Type      scanType     `json:"Type"`
		ARN       arn.ARN      `json:"ARN"`
		Hostname  string       `json:"Hostname"`
		Actions   []scanAction `json:"Actions"`
		Roles     rolesMapping `json:"Roles"`
		DiskMode  diskMode     `json:"DiskMode"`

		// Lifecycle metadata of the task
		CreatedSnapshots        map[string]*time.Time `json:"CreatedSnapshots"`
		AttachedDeviceName      *string               `json:"AttachedDeviceName"`
		AttachedVolumeARN       *arn.ARN              `json:"AttachedVolumeARN"`
		AttachedVolumeCreatedAt *time.Time            `json:"AttachedVolumeCreatedAt"`
	}

	scanJSONError struct {
		err error
	}

	scanResult struct {
		scannerOptions

		Err *scanJSONError `json:"Err"`

		// Results union
		Vulns      *scanVulnsResult     `json:"Vulns"`
		Malware    *scanMalwareResult   `json:"Malware"`
		Containers *scanContainerResult `json:"Containers"`
	}

	scannerOptions struct {
		Scanner   scannerName `json:"Scanner"`
		Scan      *scanTask   `json:"Scan"`
		Root      string      `json:"Root"`
		StartedAt time.Time   `jons:"StartedAt"`

		// Vulns specific
		SnapshotARN *arn.ARN `json:"SnapshotARN"` // TODO: deprecate as we remove "vm" mode
	}

	scanVulnsResult struct {
		BOM        *cdx.BOM                 `json:"BOM"`
		SourceType sbommodel.SBOMSourceType `json:"SourceType"`
		ID         string                   `json:"ID"`
		Tags       []string                 `json:"Tags"`
	}

	scanContainerResult struct {
		Containers []*container `json:"MountPoints"`
	}

	scanMalwareResult struct {
		Findings []*scanFinding
	}

	scanFinding struct {
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
)

func (e *scanJSONError) Error() string {
	return e.err.Error()
}

func (e *scanJSONError) Unwrap() error {
	return e.err
}

func (e *scanJSONError) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.err.Error())
}

func (e *scanJSONError) UnmarshalJSON(data []byte) error {
	var msg string
	if err := json.Unmarshal(data, &msg); err != nil {
		return err
	}
	e.err = errors.New(msg)
	return nil
}

func (o scannerOptions) ErrResult(err error) scanResult {
	return scanResult{scannerOptions: o, Err: &scanJSONError{err}}
}

func (o scannerOptions) ID() string {
	h := sha256.New()
	h.Write([]byte(o.Scanner))
	h.Write([]byte(o.Root))
	h.Write([]byte(o.Scan.ID))
	if o.SnapshotARN != nil {
		h.Write([]byte(o.SnapshotARN.String()))
	}
	return string(o.Scanner) + "-" + hex.EncodeToString(h.Sum(nil)[:8])
}

func makeScanTaskID(s *scanTask) string {
	h := sha256.New()
	createdAt, _ := s.CreatedAt.MarshalBinary()
	h.Write(createdAt)
	h.Write([]byte(s.Type))
	h.Write([]byte(s.ARN.String()))
	h.Write([]byte(s.Hostname))
	h.Write([]byte(s.DiskMode))
	for _, action := range s.Actions {
		h.Write([]byte(action))
	}
	return string(s.Type) + "-" + hex.EncodeToString(h.Sum(nil)[:8])
}

func (s *scanTask) Path(names ...string) string {
	root := filepath.Join(scansRootDir, s.ID)
	for _, name := range names {
		name = strings.ToLower(name)
		name = regexp.MustCompile("[^a-z0-9_.-]").ReplaceAllString(name, "")
		root = filepath.Join(root, name)
	}
	return root
}

func (s *scanTask) String() string {
	if s == nil {
		return "<scan[nil]>"
	}
	return fmt.Sprintf("<%s[%s]>", s.ID, s.ARN)
}

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

	sideScannerCmd := &cobra.Command{
		Use:          "agentless-scanner [command]",
		Short:        "Datadog Agentless Scanner at your service.",
		Long:         `Datadog Agentless Scanner scans your cloud environment for vulnerabilities, compliance and security issues.`,
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			mode, err := parseDiskMode(diskModeStr)
			if err != nil {
				return err
			}
			if _, err := parseScanActions(globalParams.defaultActions); err != nil {
				return err
			}
			globalParams.diskMode = mode
			initStatsdClient()
			return nil
		},
	}

	pflags := sideScannerCmd.PersistentFlags()
	pflags.StringVarP(&globalParams.configFilePath, "config-path", "c", path.Join(commonpath.DefaultConfPath, "datadog.yaml"), "specify the path to agentless-scanner configuration yaml file")
	pflags.StringVar(&diskModeStr, "disk-mode", string(noAttach), fmt.Sprintf("disk mode used for scanning EBS volumes: %s, %s or %s", volumeAttach, nbdAttach, noAttach))
	pflags.BoolVar(&globalParams.noForkScanners, "no-fork-scanners", false, "disable spawning a dedicated process for launching scanners")
	pflags.StringSliceVar(&globalParams.defaultActions, "actions", []string{string(vulnsHost)}, "disable spawning a dedicated process for launching scanners")
	sideScannerCmd.AddCommand(runCommand())
	sideScannerCmd.AddCommand(runScannerCommand())
	sideScannerCmd.AddCommand(scanCommand())
	sideScannerCmd.AddCommand(offlineCommand())
	sideScannerCmd.AddCommand(attachCommand())
	sideScannerCmd.AddCommand(cleanupCommand())

	return sideScannerCmd
}

func runWithModules(run func(cmd *cobra.Command, args []string) error) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		return fxutil.OneShot(
			func(_ complog.Component, _ compconfig.Component) error {
				if lvl := pkgconfig.Datadog.GetString("log_level"); lvl == "info" {
					_ = pkgconfig.ChangeLogLevel("debug") // TODO(jinroh): remove this
				}
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
		poolSize    int
	}

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Runs the agentless-scanner",
		RunE: runWithModules(func(cmd *cobra.Command, args []string) error {
			return runCmd(runParams.pidfilePath, runParams.poolSize)
		}),
	}
	cmd.Flags().StringVarP(&runParams.pidfilePath, "pidfile", "p", "", "path to the pidfile")
	cmd.Flags().IntVar(&runParams.poolSize, "workers", defaultWorkersCount, "number of scans running in parallel")
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

func offlineCommand() *cobra.Command {
	var cliArgs struct {
		poolSize     int
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
			filter := cliArgs.filters
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
			return offlineCmd(cliArgs.poolSize, scanType(cliArgs.scanType), cliArgs.regions, cliArgs.maxScans, cliArgs.printResults, globalParams.defaultActions, filters)
		}),
	}

	cmd.Flags().IntVar(&cliArgs.poolSize, "workers", defaultWorkersCount, "number of scans running in parallel")
	cmd.Flags().StringSliceVar(&cliArgs.regions, "regions", []string{"auto"}, "list of regions to scan (default to all regions)")
	cmd.Flags().StringVar(&cliArgs.filters, "filters", "", "list of filters to filter the resources (format: Name=string,Values=string,string)")
	cmd.Flags().StringVar(&cliArgs.scanType, "scan-type", string(ebsScanType), "scan type (ebs-volume or lambda)")
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
			resourceARN, err := humanParseARN(args[0], resourceTypeSnapshot, resourceTypeVolume)
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

func runCmd(pidfilePath string, poolSize int) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if pidfilePath != "" {
		err := pidfile.WritePID(pidfilePath)
		if err != nil {
			return log.Errorf("could not write PID file, exiting: %v", err)
		}
		defer os.Remove(pidfilePath)
		log.Infof("pid '%d' written to pid file '%s'", os.Getpid(), pidfilePath)
	}

	hostname, err := utils.GetHostnameWithContext(ctx)
	if err != nil {
		return fmt.Errorf("could not fetch hostname: %w", err)
	}

	limits := newAWSLimits(getAWSLimitsOptions())

	scanner, err := newSideScanner(hostname, limits, poolSize)
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
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var opts scannerOptions

	conn, err := net.Dial("unix", sock)
	if err != nil {
		return err
	}
	defer conn.Close()

	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)
	for {
		_ = conn.SetReadDeadline(time.Now().Add(4 * time.Second))
		if err := dec.Decode(&opts); err != nil {
			if errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		result := launchScannerLocally(ctx, opts)
		_ = conn.SetWriteDeadline(time.Now().Add(4 * time.Second))
		if err := enc.Encode(result); err != nil {
			return err
		}
	}
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

func scanCmd(resourceARN arn.ARN, scannedHostname string, actions []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	ctxhostname, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	hostname, err := utils.GetHostnameWithContext(ctxhostname)
	if err != nil {
		hostname = "unknown"
	}

	roles := getDefaultRolesMapping()
	task, err := newScanTask(resourceARN.String(), scannedHostname, actions, roles, globalParams.diskMode)
	if err != nil {
		return err
	}

	limits := newAWSLimits(getAWSLimitsOptions())
	scanner, err := newSideScanner(hostname, limits, 1)
	if err != nil {
		return fmt.Errorf("could not initialize agentless-scanner: %w", err)
	}
	scanner.printResults = true
	go func() {
		scanner.pushConfig(ctx, &scanConfig{
			Type:  awsScan,
			Tasks: []*scanTask{task},
		})
		scanner.stop()
	}()
	scanner.start(ctx)
	return nil
}

func offlineCmd(poolSize int, scanType scanType, regions []string, maxScans int, printResults bool, actions []string, filters []ec2types.Filter) error {
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

	limits := newAWSLimits(getAWSLimitsOptions())
	scanner, err := newSideScanner(hostname, limits, poolSize)
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
							volumeARN := ec2ARN(regionName, *identity.Account, resourceTypeVolume, *blockDeviceMapping.Ebs.VolumeId)
							log.Debugf("%s %s %s %s %s", regionName, *instance.InstanceId, volumeARN, *blockDeviceMapping.DeviceName, *instance.PlatformDetails)
							scan, err := newScanTask(volumeARN.String(), *instance.InstanceId, actions, roles, globalParams.diskMode)
							if err != nil {
								return err
							}

							config := &scanConfig{Type: awsScan, Tasks: []*scanTask{scan}, Roles: roles}
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
					scan, err := newScanTask(*function.FunctionArn, "", actions, roles, globalParams.diskMode)
					if err != nil {
						return fmt.Errorf("could not create scan for lambda %s: %w", *function.FunctionArn, err)
					}
					config := &scanConfig{Type: awsScan, Tasks: []*scanTask{scan}, Roles: roles}
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
		if scanType == ebsScanType {
			err = pushEBSVolumes()
		} else if scanType == lambdaScanType {
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

func attachCmd(resourceARN arn.ARN, mode diskMode, mount bool) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := newAWSConfig(ctx, resourceARN.Region, nil)
	if err != nil {
		return err
	}

	if mode == noAttach {
		mode = nbdAttach
	}

	scan, err := newScanTask(resourceARN.String(), "unknown", nil, nil, mode)
	if err != nil {
		return err
	}
	defer cleanupScan(scan)

	resourceType, _, _ := getARNResource(resourceARN)
	var snapshotARN arn.ARN
	switch resourceType {
	case resourceTypeVolume:
		ec2client := ec2.NewFromConfig(cfg)
		snapshotARN, err = createSnapshot(ctx, scan, ec2client, resourceARN)
		if err != nil {
			return err
		}
	case resourceTypeSnapshot:
		snapshotARN = resourceARN
	default:
		panic("unreachable")
	}

	switch mode {
	case volumeAttach:
		if err := attachSnapshotWithVolume(ctx, scan, snapshotARN); err != nil {
			return err
		}
	case nbdAttach:
		ebsclient := ebs.NewFromConfig(cfg)
		if err := attachSnapshotWithNBD(ctx, scan, snapshotARN, ebsclient); err != nil {
			return err
		}
	default:
		panic("unreachable")
	}

	partitions, err := listDevicePartitions(ctx, scan)
	if err != nil {
		log.Errorf("could list paritions (device is still available on %q): %v", *scan.AttachedDeviceName, err)
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

func newScanTask(resourceARN, hostname string, actions []string, roles rolesMapping, mode diskMode) (*scanTask, error) {
	var scan scanTask
	var err error
	scan.ARN, err = parseARN(resourceARN, resourceTypeLocalDir, resourceTypeSnapshot, resourceTypeVolume, resourceTypeFunction)
	if err != nil {
		return nil, err
	}
	resourceType, _, err := getARNResource(scan.ARN)
	if err != nil {
		return nil, err
	}
	switch {
	case resourceType == resourceTypeLocalDir:
		scan.Type = hostScanType
	case resourceType == resourceTypeSnapshot || resourceType == resourceTypeVolume:
		scan.Type = ebsScanType
	case resourceType == resourceTypeFunction:
		scan.Type = lambdaScanType
	default:
		return nil, fmt.Errorf("unsupported resource type %q for scanning", resourceType)
	}
	scan.Hostname = hostname
	scan.Roles = roles
	scan.DiskMode = mode
	if actions == nil {
		actions = globalParams.defaultActions
	}
	scan.Actions, err = parseScanActions(actions)
	if err != nil {
		log.Warn(err)
	}
	scan.CreatedAt = time.Now()
	scan.ID = makeScanTaskID(&scan)
	scan.CreatedSnapshots = make(map[string]*time.Time)
	return &scan, nil
}

func parseScanActions(actions []string) ([]scanAction, error) {
	var scanActions []scanAction
	for _, actionRaw := range actions {
		switch actionRaw {
		case string(vulnsHost):
			scanActions = append(scanActions, vulnsHost)
		case string(vulnsContainers):
			scanActions = append(scanActions, vulnsContainers)
		case string(malware):
			scanActions = append(scanActions, malware)
		default:
			return nil, fmt.Errorf("unknown action type %q", actionRaw)
		}
	}
	return scanActions, nil
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
		task, err := newScanTask(rawScan.ARN, rawScan.Hostname, rawScan.Actions, config.Roles, config.DiskMode)
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
	isExpected := len(expectedTypes) == 0
	for _, t := range expectedTypes {
		if t == resType {
			isExpected = true
			break
		}
	}
	if !isExpected {
		return arn.ARN{}, fmt.Errorf("bad arn: expecting one of these resource types %v but got %s", expectedTypes, resType)
	}
	return a, nil
}

func humanParseARN(s string, expectedTypes ...resourceType) (arn.ARN, error) {
	if strings.HasPrefix(s, "arn:") {
		return parseARN(s, expectedTypes...)
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
	if strings.HasPrefix(s, "/") && fs.ValidPath(s[1:]) {
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
	return parseARN(a.String(), expectedTypes...)
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
	if arn.Partition == "localhost" {
		return resourceTypeLocalDir, filepath.Join("/", arn.Resource), nil
	}
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
	limits           *awsLimits
	waiter           *awsWaiter
	printResults     bool

	regionsCleanupMu sync.Mutex
	regionsCleanup   map[string]*arn.ARN

	scansInProgress   map[arn.ARN]struct{}
	scansInProgressMu sync.RWMutex

	configsCh chan *scanConfig
	scansCh   chan *scanTask
	resultsCh chan scanResult
}

func newSideScanner(hostname string, limits *awsLimits, poolSize int) (*sideScanner, error) {
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
		limits:           limits,
		waiter:           &awsWaiter{},

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

	scansDir, err := os.Open(scansRootDir)
	if os.IsNotExist(err) {
		if err := os.Mkdir(scansRootDir, 0700); err != nil {
			return fmt.Errorf("clean slate: could not create directory %q: %w", scansRootDir, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("clean slate: could not open %q: %w", scansRootDir, err)
	}
	scanDirInfo, err := scansDir.Stat()
	if err != nil {
		return fmt.Errorf("clean slate: could not stat %q: %w", scansRootDir, err)
	}
	if !scanDirInfo.IsDir() {
		return fmt.Errorf("clean slate: %q already exists and is not a directory: %w", scansRootDir, os.ErrExist)
	}
	if scanDirInfo.Mode() != 0700 {
		if err := os.Chmod(scansRootDir, 0700); err != nil {
			return fmt.Errorf("clean slate: could not chmod %q: %w", scansRootDir, err)
		}
	}
	scanDirs, err := scansDir.ReadDir(0)
	if err != nil {
		return err
	}

	var ebsMountPoints []string
	var ctrMountPoints []string
	for _, scanDir := range scanDirs {
		name := filepath.Join(scansRootDir, scanDir.Name())
		if !scanDir.IsDir() {
			if err := os.Remove(name); err != nil {
				log.Warnf("clean slate: could not remove file %q", name)
			}
		} else {
			switch {
			case strings.HasPrefix(scanDir.Name(), string(lambdaScanType)+"-"):
				if err := os.RemoveAll(name); err != nil {
					log.Warnf("clean slate: could not remove directory %q", name)
				}
			case strings.HasPrefix(scanDir.Name(), string(ebsScanType)):
				scanDirname := filepath.Join(scansRootDir, scanDir.Name())
				scanEntries, err := os.ReadDir(scanDirname)
				if err != nil {
					log.Errorf("clean slate: %v", err)
				} else {
					for _, scanEntry := range scanEntries {
						switch {
						case strings.HasPrefix(scanEntry.Name(), ebsMountPrefix):
							ebsMountPoints = append(ebsMountPoints, filepath.Join(scanDirname, scanEntry.Name()))
						case strings.HasPrefix(scanEntry.Name(), ctrdMountPrefix) || strings.HasPrefix(scanEntry.Name(), dockerMountPrefix):
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
		scanDirname := filepath.Join(scansRootDir, scanDir.Name())
		log.Warnf("clean slate: removing directory %q", scanDirname)
		if err := os.RemoveAll(scanDirname); err != nil {
			log.Errorf("clean slate: could not remove directory %q", scanDirname)

		}
	}

	var attachedVolumes []string
	if blockDevices, err := listBlockDevices(); err == nil {
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
							if strings.HasPrefix(mountpoint, scansRootDir+"/") {
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
			volumeARN := ec2ARN(self.Region, self.AccountID, resourceTypeVolume, volumeID)
			if errd := cleanupScanDetach(ctx, nil, volumeARN, getDefaultRolesMapping()); err != nil {
				log.Warnf("clean slate: %v", errd)
			}
		}
	}

	return nil
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
			if result.Err != nil {
				if !errors.Is(result.Err, context.Canceled) {
					log.Errorf("%s: %s scanner reported a failure: %v", result.Scan, result.Scanner, result.Err)
				}
				if err := statsd.Count("datadog.agentless_scanner.scans.finished", 1.0, tagFailure(result.Scan, result.Err), 1.0); err != nil {
					log.Warnf("failed to send metric: %v", err)
				}
			} else {
				log.Debugf("%s: scanner %s finished successfully (took %s)", result.Scan, result.Scanner, time.Since(result.StartedAt))
				if vulns := result.Vulns; vulns != nil {
					if hasResults(vulns.BOM) {
						if err := statsd.Count("datadog.agentless_scanner.scans.finished", 1.0, tagSuccess(result.Scan), 1.0); err != nil {
							log.Warnf("failed to send metric: %v", err)
						}
					} else {
						log.Debugf("%s: scanner %s finished successfully without results (took %s)", result.Scan, result.Scanner, time.Since(result.StartedAt))
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
						fmt.Printf("scanning malware result %s (took %s): %s\n", result.Scan, time.Since(result.StartedAt), string(b))
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

	for i := 0; i < s.poolSize; i++ {
		<-done
	}
	close(s.resultsCh)
	<-done // waiting for done in range resultsCh goroutine
}

func (s *sideScanner) stop() {
	close(s.configsCh)
}

func (s *sideScanner) pushConfig(ctx context.Context, config *scanConfig) bool {
	select {
	case s.configsCh <- config:
		return true
	case <-ctx.Done():
		return false
	}
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
	ctx = withAWSWaiter(ctx, s.waiter)

	if err := os.MkdirAll(scan.Path(), 0700); err != nil {
		return err
	}

	scan.StartedAt = time.Now()
	defer cleanupScan(scan)
	switch scan.Type {
	case hostScanType:
		return scanRoots(ctx, scan, []string{scan.ARN.Resource}, s.resultsCh)
	case ebsScanType:
		return scanEBS(ctx, scan, s.resultsCh)
	case lambdaScanType:
		return scanLambda(ctx, scan, s.resultsCh)
	default:
		return fmt.Errorf("unknown scan type: %s", scan.Type)
	}
}

func (s *sideScanner) sendSBOM(result scanResult) error {
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

func (s *sideScanner) sendFindings(findings []*scanFinding) {
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

func listResourcesForCleanup(ctx context.Context, ec2client *ec2.Client, maxTTL time.Duration) map[resourceType][]string {
	toBeDeleted := make(map[resourceType][]string)
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
			toBeDeleted[resourceTypeSnapshot] = append(toBeDeleted[resourceTypeSnapshot], snapshotID)
		}
		nextToken = snapshots.NextToken
		if nextToken == nil {
			break
		}
	}
	return toBeDeleted
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

func createSnapshot(ctx context.Context, scan *scanTask, ec2client *ec2.Client, volumeARN arn.ARN) (arn.ARN, error) {
	snapshotCreatedAt := time.Now()
	if err := statsd.Count("datadog.agentless_scanner.snapshots.started", 1.0, tagScan(scan), 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
	log.Debugf("%s: starting volume snapshotting %q", scan, volumeARN)

	retries := 0
retry:
	_, volumeID, _ := getARNResource(volumeARN)
	createSnapshotOutput, err := ec2client.CreateSnapshot(ctx, &ec2.CreateSnapshotInput{
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
	snapshotARN := ec2ARN(volumeARN.Region, volumeARN.AccountID, resourceTypeSnapshot, snapshotID)
	scan.CreatedSnapshots[snapshotARN.String()] = &snapshotCreatedAt

	waiter := getAWSWaiter(ctx)
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

	limits := getAWSLimit(ctx)
	httpClient := newHTTPClientWithAWSStats(region, assumedRole, limits)
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
		log.Tracef("aws config: assuming role with arn=%q", *result.Arn)
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
		snapshotARN, err = createSnapshot(ctx, scan, ec2client, scan.ARN)
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
		result := launchScanner(ctx, scannerOptions{
			Scanner:     scannerNameHostVulnsEBS,
			Scan:        scan,
			SnapshotARN: &snapshotARN,
			StartedAt:   time.Now(),
		})
		if result.Vulns != nil {
			result.Vulns.SourceType = sbommodel.SBOMSourceType_HOST_FILE_SYSTEM
			result.Vulns.ID = scan.Hostname
			result.Vulns.Tags = nil
		}
		resultsCh <- result
		if err := statsd.Histogram("datadog.agentless_scanner.scans.duration", float64(time.Since(result.StartedAt).Milliseconds()), tagScan(scan), 1.0); err != nil {
			log.Warnf("failed to send metric: %v", err)
		}
		return nil
	}

	switch scan.DiskMode {
	case volumeAttach:
		if err := attachSnapshotWithVolume(ctx, scan, snapshotARN); err != nil {
			return err
		}
	case nbdAttach:
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

	return scanRoots(ctx, scan, mountPoints, resultsCh)
}

func scanRoots(ctx context.Context, scan *scanTask, roots []string, resultsCh chan scanResult) error {
	for _, root := range roots {
		for _, action := range scan.Actions {
			switch action {
			case vulnsHost:
				result := launchScanner(ctx, scannerOptions{
					Scanner:   scannerNameHostVulns,
					Scan:      scan,
					Root:      root,
					StartedAt: time.Now(),
				})
				if result.Vulns != nil {
					result.Vulns.SourceType = sbommodel.SBOMSourceType_HOST_FILE_SYSTEM
					result.Vulns.ID = scan.Hostname
					result.Vulns.Tags = nil
				}
				resultsCh <- result
			case vulnsContainers:
				opts := scannerOptions{
					Scanner:   scannerNameContainers,
					Scan:      scan,
					Root:      root,
					StartedAt: time.Now(),
				}
				ctrResult := launchScanner(ctx, opts)
				if ctrResult.Err != nil {
					resultsCh <- ctrResult
				} else {
					for _, ctr := range ctrResult.Containers.Containers {
						mountPoint, err := mountContainer(ctx, scan, *ctr)
						if err != nil {
							resultsCh <- opts.ErrResult(err)
							continue
						}
						result := launchScanner(ctx, scannerOptions{
							Scanner:   scannerNameAppVulns,
							Scan:      scan,
							Root:      mountPoint,
							StartedAt: time.Now(),
						})
						if result.Vulns != nil {
							refTag := ctr.ImageRefTagged.Reference().(reference.NamedTagged)
							refCan := ctr.ImageRefCanonical.Reference().(reference.Canonical)
							result.Vulns.SourceType = sbommodel.SBOMSourceType_CONTAINER_IMAGE_LAYERS // TODO: sbommodel.SBOMSourceType_CONTAINER_FILE_SYSTEM
							result.Vulns.ID = ctr.ImageRefCanonical.Reference().String()
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
							result.Vulns.Tags = []string{
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
							appendSBOMRepoMetadata(result.Vulns.BOM, refTag, refCan)
						}

						// We cleanup overlays as we go instead of acumulating
						// them. However the cleanupScan routine also cleans
						// up any leftover. We do not rely on the parent ctx
						// as we still want to clean these mounts even for a
						// canceled/timeouted context.
						cleanupctx, abort := context.WithTimeout(context.Background(), 5*time.Second)
						cleanupScanUmount(cleanupctx, opts.Scan, mountPoint)
						abort()

						resultsCh <- result
					}
				}
			case malware:
				resultsCh <- launchScanner(ctx, scannerOptions{
					Scanner:   scannerNameMalware,
					Scan:      scan,
					Root:      root,
					StartedAt: time.Now(),
				})
			}
		}
	}

	if err := statsd.Histogram("datadog.agentless_scanner.scans.duration", float64(time.Since(scan.StartedAt).Milliseconds()), tagScan(scan), 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
	return nil
}

func launchScanner(ctx context.Context, opts scannerOptions) scanResult {
	if globalParams.noForkScanners {
		return launchScannerLocally(ctx, opts)
	}
	return launchScannerRemotely(ctx, opts)
}

func launchScannerRemotely(ctx context.Context, opts scannerOptions) scanResult {
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

	remoteCall := func() scanResult {
		var result scanResult

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

	resultsCh := make(chan scanResult, 1)
	go func() {
		resultsCh <- remoteCall()
	}()

	stderr := &truncatedWriter{max: 512 * 1024}
	cmd := exec.CommandContext(ctx, exe, "run-scanner", "--sock", sockName)
	cmd.Env = []string{
		"GOMAXPROCS=1",
		"DD_LOG_FILE=" + opts.Scan.Path(fmt.Sprintf("scanner-%s.log", opts.ID())),
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

func launchScannerLocally(ctx context.Context, opts scannerOptions) scanResult {
	switch opts.Scanner {
	case scannerNameHostVulns:
		bom, err := launchScannerTrivyLocal(ctx, opts)
		if err != nil {
			return opts.ErrResult(err)
		}
		return scanResult{scannerOptions: opts, Vulns: &scanVulnsResult{BOM: bom}}
	case scannerNameHostVulnsEBS:
		bom, err := launchScannerTrivyVM(ctx, opts)
		if err != nil {
			return opts.ErrResult(err)
		}
		return scanResult{scannerOptions: opts, Vulns: &scanVulnsResult{BOM: bom}}
	case scannerNameAppVulns:
		bom, err := launchScannerTrivyApp(ctx, opts)
		if err != nil {
			return opts.ErrResult(err)
		}
		return scanResult{scannerOptions: opts, Vulns: &scanVulnsResult{BOM: bom}}
	case scannerNameContainers:
		containers, err := launchScannerContainers(ctx, opts)
		if err != nil {
			return opts.ErrResult(err)
		}
		return scanResult{scannerOptions: opts, Containers: &scanContainerResult{Containers: containers}}
	case scannerNameMalware:
		result, err := launchScannerMalware(ctx, opts)
		if err != nil {
			return opts.ErrResult(err)
		}
		return scanResult{scannerOptions: opts, Malware: &result}
	default:
		panic("unreachable")
	}
}

func attachSnapshotWithNBD(_ context.Context, scan *scanTask, snapshotARN arn.ARN, ebsclient *ebs.Client) error {
	device, ok := nextNBDDevice()
	if !ok {
		return fmt.Errorf("could not find non busy NBD block device")
	}
	err := startEBSBlockDevice(scan.ID, ebsclient, device, snapshotARN)
	scan.AttachedDeviceName = &device
	return err
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

	// loops from "xvdaa" to "xvddz"
	const xenMax = ('d' - 'a' + 1) * 26
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

func scanLambda(ctx context.Context, scan *scanTask, resultsCh chan scanResult) error {
	defer statsd.Flush()

	lambdaDir := scan.Path()
	if err := os.MkdirAll(lambdaDir, 0700); err != nil {
		return err
	}

	codePath, err := downloadAndUnzipLambda(ctx, scan, lambdaDir)
	if err != nil {
		return err
	}

	result := launchScanner(ctx, scannerOptions{
		Scanner:   scannerNameAppVulns,
		Scan:      scan,
		Root:      codePath,
		StartedAt: time.Now(),
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

func downloadAndUnzipLambda(ctx context.Context, scan *scanTask, lambdaDir string) (codePath string, err error) {
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

func attachSnapshotWithVolume(ctx context.Context, scan *scanTask, snapshotARN arn.ARN) error {
	resourceType, snapshotID, err := getARNResource(snapshotARN)
	if err != nil {
		return err
	}
	if resourceType != resourceTypeSnapshot {
		return fmt.Errorf("expected ARN for a snapshot: %s", snapshotARN.String())
	}

	self, err := getSelfEC2InstanceIndentity(ctx)
	if err != nil {
		return fmt.Errorf("could not get EC2 instance identity: using attach volumes cannot work outside an EC2 instance: %w", err)
	}

	remoteAssumedRole := scan.Roles[snapshotARN.AccountID]
	remoteAWSCfg, err := newAWSConfig(ctx, snapshotARN.Region, remoteAssumedRole)
	if err != nil {
		return fmt.Errorf("could not create local aws config: %w", err)
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
			TagSpecifications: cloudResourceTagSpec(resourceTypeSnapshot),
		})
		if err != nil {
			return fmt.Errorf("could not copy snapshot %q to %q: %w", snapshotARN, self.Region, err)
		}
		localSnapshotARN = ec2ARN(self.Region, snapshotARN.AccountID, resourceTypeSnapshot, *copySnapshot.SnapshotId)
		log.Debugf("%s: waiting for copy of snapshot %q into %q as %q", scan, snapshotARN, self.Region, *copySnapshot.SnapshotId)
		waiter := getAWSWaiter(ctx)
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
		return fmt.Errorf("could not create local aws config: %w", err)
	}
	locaEC2Client := ec2.NewFromConfig(localAWSCfg)

	log.Debugf("%s: creating new volume for snapshot %q in az %q", scan, localSnapshotARN, self.AvailabilityZone)
	_, localSnapshotID, _ := getARNResource(localSnapshotARN)
	volume, err := locaEC2Client.CreateVolume(ctx, &ec2.CreateVolumeInput{
		VolumeType:        ec2types.VolumeTypeGp3,
		AvailabilityZone:  aws.String(self.AvailabilityZone),
		SnapshotId:        aws.String(localSnapshotID),
		TagSpecifications: cloudResourceTagSpec(resourceTypeVolume),
	})
	if err != nil {
		return fmt.Errorf("could not create volume from snapshot: %s", err)
	}

	volumeARN := ec2ARN(localSnapshotARN.Region, localSnapshotARN.AccountID, resourceTypeVolume, *volume.VolumeId)
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
		// NOTE(jinroh): we're trying to detach a volume in not in 'available'
		// state for instance. Continue.
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

func listBlockDevices(deviceName ...string) ([]blockDevice, error) {
	var blockDevices struct {
		BlockDevices []blockDevice `json:"blockdevices"`
	}
	_, _ = exec.Command("udevadm", "settle", "--timeout=1").CombinedOutput()
	lsblkArgs := []string{"--json", "--bytes", "--output", "NAME,SERIAL,PATH,TYPE,FSTYPE,MOUNTPOINTS"}
	lsblkArgs = append(lsblkArgs, deviceName...)
	lsblkJSON, err := exec.Command("lsblk", lsblkArgs...).Output()
	if err != nil {
		return nil, fmt.Errorf("lsblk exited with error: %w", err)
	}
	if err := json.Unmarshal(lsblkJSON, &blockDevices); err != nil {
		return nil, fmt.Errorf("lsblk output parsing error: %v", err)
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

func listDevicePartitions(ctx context.Context, scan *scanTask) ([]devicePartition, error) {
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
		_, volumeID, _ := getARNResource(*volumeARN)
		sn := "vol" + strings.TrimPrefix(volumeID, "vol-") // vol-XXX => volXXX
		serialNumber = &sn
	}

	var foundBlockDevice *blockDevice
	for i := 0; i < 120; i++ {
		if !sleepCtx(ctx, 500*time.Millisecond) {
			return nil, ctx.Err()
		}
		blockDevices, err := listBlockDevices()
		if err != nil {
			log.Warn(err)
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

	var partitions []devicePartition
	for i := 0; i < 5; i++ {
		blockDevices, err := listBlockDevices(foundBlockDevice.Path)
		if err != nil {
			log.Warn(err)
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

func mountDevice(ctx context.Context, scan *scanTask, partitions []devicePartition) ([]string, error) {
	var mountPoints []string
	for _, mp := range partitions {
		mountPoint := scan.Path(ebsMountPrefix + path.Base(mp.devicePath))
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

func cleanupScanDetach(ctx context.Context, maybeScan *scanTask, volumeARN arn.ARN, roles rolesMapping) error {
	_, volumeID, _ := getARNResource(volumeARN)
	cfg, err := newAWSConfig(ctx, volumeARN.Region, roles[volumeARN.AccountID])
	if err != nil {
		return err
	}

	ec2client := ec2.NewFromConfig(cfg)

	volumeNotFound := false
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
					break
				}
				if aerr.ErrorCode() == "InvalidVolume.NotFound" {
					volumeNotFound = true
					break
				}
			}
			log.Warnf("%s: could not detach volume %s: %v", maybeScan, volumeID, err)
		} else {
			break
		}
		if !sleepCtx(ctx, 10*time.Second) {
			return fmt.Errorf("could not detach volume: %w", ctx.Err())
		}
	}

	var errd error
	for i := 0; i < 25; i++ {
		if volumeNotFound {
			break
		}
		if !sleepCtx(ctx, 2*time.Second) {
			errd = ctx.Err()
			break
		}
		_, errd = ec2client.DeleteVolume(ctx, &ec2.DeleteVolumeInput{
			VolumeId: aws.String(volumeID),
		})
		if errd == nil {
			log.Debugf("%s: volume deleted %q", maybeScan, volumeID)
			break
		}
	}
	if errd != nil {
		return fmt.Errorf("could not delete volume %q: %v", volumeID, errd)
	}
	return nil
}

func cleanupScanUmount(ctx context.Context, maybeScan *scanTask, mountPoint string) {
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

func cleanupScan(scan *scanTask) {
	ctx, cancel := context.WithTimeout(context.Background(), cleanupMaxDuration)
	defer cancel()

	scanRoot := scan.Path()

	log.Debugf("%s: cleaning up scan data on filesystem", scan)

	for snapshotARNString, snapshotCreatedAt := range scan.CreatedSnapshots {
		snapshotARN, err := parseARN(snapshotARNString, resourceTypeSnapshot)
		if err != nil {
			continue
		}
		_, snapshotID, _ := getARNResource(snapshotARN)
		cfg, err := newAWSConfig(ctx, snapshotARN.Region, scan.Roles[snapshotARN.AccountID])
		if err != nil {
			log.Errorf("%s: could not create local aws config: %v", scan, err)
		} else {
			ec2client := ec2.NewFromConfig(cfg)
			log.Debugf("%s: deleting snapshot %q", scan, snapshotID)
			if _, err := ec2client.DeleteSnapshot(ctx, &ec2.DeleteSnapshotInput{
				SnapshotId: aws.String(snapshotID),
			}); err != nil {
				log.Warnf("%s: could not delete snapshot %s: %v", scan, snapshotID, err)
			} else {
				log.Debugf("%s: snapshot deleted %s", scan, snapshotID)
				statsResourceTTL(resourceTypeSnapshot, scan, *snapshotCreatedAt)
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
				if strings.HasPrefix(entry.Name(), ebsMountPrefix) {
					ebsMountPoints = append(ebsMountPoints, entry)
				}
				if strings.HasPrefix(entry.Name(), ctrdMountPrefix) || strings.HasPrefix(entry.Name(), dockerMountPrefix) {
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
		blockDevices, err := listBlockDevices(*scan.AttachedDeviceName)
		if err == nil && len(blockDevices) == 1 {
			for _, child := range blockDevices[0].getChildrenType("lvm") {
				if err := exec.Command("dmsetup", "remove", child.Path).Run(); err != nil {
					log.Errorf("%s: could not remove logical device %q from block device %q: %v", scan, child.Path, child.Name, err)
				}
			}
		}
	}

	switch scan.DiskMode {
	case volumeAttach:
		if volumeARN := scan.AttachedVolumeARN; volumeARN != nil {
			if err != nil {
				log.Errorf("%s: could not create local aws config: %v", scan, err)
			} else {
				if errd := cleanupScanDetach(ctx, scan, *volumeARN, scan.Roles); errd != nil {
					log.Warnf("%s: could not delete volume %q: %v", scan, volumeARN, errd)
				} else {
					statsResourceTTL(resourceTypeVolume, scan, *scan.AttachedVolumeCreatedAt)
				}
			}
		}
	case nbdAttach:
		if diskDeviceName := scan.AttachedDeviceName; diskDeviceName != nil {
			stopEBSBlockDevice(ctx, *diskDeviceName)
		}
	case noAttach:
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

func ec2ARN(region, accountID string, resourceType resourceType, resourceID string) arn.ARN {
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
	_, resourceID, _ := getARNResource(snapshotARN)
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

// TODO: get rid of using context to pass these around.
type (
	keyAWSLimits struct{}
	keyAWSWaiter struct{}
)

func withAWSLimits(ctx context.Context, limits *awsLimits) context.Context {
	if limits != nil {
		return context.WithValue(ctx, keyAWSLimits{}, limits)
	}
	return ctx
}

func withAWSWaiter(ctx context.Context, waiter *awsWaiter) context.Context {
	if waiter != nil {
		return context.WithValue(ctx, keyAWSWaiter{}, waiter)
	}
	return ctx
}

func getAWSLimit(ctx context.Context) *awsLimits {
	limits := ctx.Value(keyAWSLimits{})
	if limits == nil {
		return newAWSLimits(awsLimitsOptions{})
	}
	return limits.(*awsLimits)
}

func getAWSWaiter(ctx context.Context) *awsWaiter {
	waiter := ctx.Value(keyAWSWaiter{})
	if waiter == nil {
		return &awsWaiter{}
	}
	return waiter.(*awsWaiter)
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
