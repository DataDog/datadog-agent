package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	// DataDog agent: config stuffs
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	commonpath "github.com/DataDog/datadog-agent/cmd/agent/common/path"
	"github.com/DataDog/datadog-agent/cmd/internal/runcmd"
	compconfig "github.com/DataDog/datadog-agent/comp/core/config"
	complog "github.com/DataDog/datadog-agent/comp/core/log"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/version"
	"go.uber.org/fx"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/lambda"

	// DataDog agent: SBOM + proto stuffs
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

	// Trivy stuffs
	"github.com/aquasecurity/trivy-db/pkg/db"
	"github.com/aquasecurity/trivy/pkg/detector/ospkg"
	"github.com/aquasecurity/trivy/pkg/fanal/analyzer"
	"github.com/aquasecurity/trivy/pkg/fanal/applier"
	"github.com/aquasecurity/trivy/pkg/fanal/artifact"
	local2 "github.com/aquasecurity/trivy/pkg/fanal/artifact/local"
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
	maxSnapshotRetries  = 3
	scansWorkerPoolSize = 10
)

var statsd *ddgostatsd.Client

var (
	globalParams struct {
		ConfigFilePath string
	}

	configPath string
)

func main() {
	flavor.SetFlavor(flavor.SideScannerAgent)
	os.Exit(runcmd.Run(rootCommand()))
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

	sideScannerCmd.PersistentFlags().StringVarP(&configPath, "config-path", "c", path.Join(commonpath.DefaultConfPath, "side-scanner.yaml"), "specify the path to side-scanner configuration yaml file")
	sideScannerCmd.AddCommand(runCommand())
	sideScannerCmd.AddCommand(scanCommand())

	return sideScannerCmd
}

func runCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Runs the side-scanner",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(
				func(_ complog.Component, _ compconfig.Component) error {
					return runCmd()
				},
				fx.Supply(compconfig.NewAgentParamsWithSecrets(configPath)),
				fx.Supply(complog.ForDaemon("SIDESCANNER", "log_file", pkgconfig.DefaultSideScannerLogFile)),
				complog.Module,
				compconfig.Module,
			)
		},
	}
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
				fx.Supply(compconfig.NewAgentParamsWithSecrets(configPath)),
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

func runCmd() error {
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

	scanner := newSideScanner(hostname, rcClient, eventForwarder)
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
			fmt.Printf("scanning result %s: %s\n", scan, entity)
		}
	}
	return nil
}

type scansTask struct {
	Type     string            `json:"type"`
	RawScans []json.RawMessage `json:"scans"`
}

type scanTask struct {
	Type string
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
		case "ebs-scan":
			var scan ebsScan
			if err := json.Unmarshal(rawScan, &scan); err != nil {
				return nil, err
			}
			tasks = append(tasks, scanTask{
				Type: task.Type,
				Scan: scan,
			})
		case "lambda-scan":
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
	Region     string `json:"region"`
	SnapshotID string `json:"snapshotId"`
	VolumeID   string `json:"volumeId"`
	Hostname   string `json:"hostname"`
}

func (s ebsScan) String() string {
	return fmt.Sprintf("region=%q snapshot_id=%q volume_id=%q hostname=%q",
		s.Region,
		s.SnapshotID,
		s.VolumeID,
		s.Hostname)
}

type lambdaScan struct {
	Region       string `json:"region"`
	FunctionName string `json:"function_name"`
}

func (s lambdaScan) String() string {
	return fmt.Sprintf("region=%q function_name=%q",
		s.Region,
		s.FunctionName)
}

type sideScanner struct {
	hostname       string
	log            complog.Component
	rcClient       *remote.Client
	eventForwarder epforwarder.EventPlatformForwarder

	scansCh           chan scanTask
	scansInProgress   map[interface{}]struct{}
	scansInProgressMu sync.RWMutex
}

func newSideScanner(hostname string, rcClient *remote.Client, eventForwarder epforwarder.EventPlatformForwarder) *sideScanner {
	return &sideScanner{
		hostname:        hostname,
		rcClient:        rcClient,
		eventForwarder:  eventForwarder,
		scansCh:         make(chan scanTask),
		scansInProgress: make(map[interface{}]struct{}),
	}
}

func (s *sideScanner) start(ctx context.Context) {
	log.Infof("starting side-scanner main loop with %d scan workers", scansWorkerPoolSize)
	defer log.Infof("stopped side-scanner main loop")

	s.eventForwarder.Start()
	defer s.eventForwarder.Stop()

	s.rcClient.Start()
	defer s.rcClient.Close()

	s.rcClient.Subscribe(state.ProductDebug, func(update map[string]state.RawConfig, _ func(string, state.ApplyStatus)) {
		for _, rawConfig := range update {
			s.pushOrSkipScan(ctx, rawConfig)
		}
	})

	done := make(chan struct{})
	for i := 0; i < scansWorkerPoolSize; i++ {
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
	for i := 0; i < scansWorkerPoolSize; i++ {
		<-done
	}
	<-ctx.Done()

}

func (s *sideScanner) pushOrSkipScan(ctx context.Context, rawConfig state.RawConfig) {
	log.Debugf("received new task from remote-config: %s", rawConfig.Metadata.ID)
	scans, err := unmarshalScanTasks(rawConfig.Config)
	if err != nil {
		log.Errorf("could not parse side-scanner task: %w", err)
		return
	}
	for _, scan := range scans {
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
	case "ebs-scan":
		defer log.Debugf("finished ebs-scan of %s", scan)
		return scanEBS(ctx, scan.Scan.(ebsScan))
	case "lambda-scan":
		defer log.Debugf("finished lambda-scan of %s", scan)
		return scanLambda(ctx, scan.Scan.(lambdaScan))
	default:
		return nil, fmt.Errorf("unknown scan type: %s", scan.Type)
	}
}

func createEBSSnapshot(ctx context.Context, svc *ec2.EC2, scan ebsScan) (string, error) {
	tagList := &ec2.TagSpecification{
		Tags:         []*ec2.Tag{{Key: aws.String("source"), Value: aws.String("datadog-side-scanner")}},
		ResourceType: aws.String(ec2.ResourceTypeSnapshot),
	}
	retries := 0
retry:
	result, err := svc.CreateSnapshotWithContext(ctx, &ec2.CreateSnapshotInput{
		VolumeId:          aws.String(scan.VolumeID),
		TagSpecifications: []*ec2.TagSpecification{tagList},
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			isRateExceededError := aerr.Code() == "SnapshotCreationPerVolumeRateExceeded"
			if retries <= maxSnapshotRetries {
				retries++
				if isRateExceededError {
					// https://docs.aws.amazon.com/AWSEC2/latest/APIReference/errors-overview.html
					// Wait at least 15 seconds between concurrent volume snapshots.
					d := 15 * time.Second
					log.Debugf("snapshot creation rate exceeded for volume %s; retrying after %v (%d/%d)", scan.VolumeID, d, retries, maxSnapshotRetries)
					time.Sleep(d)
					goto retry
				}
			}
			if isRateExceededError {
				log.Debugf("snapshot creation rate exceeded for volume %s; skipping)", scan.VolumeID)
			}
		}
		return "", err
	}
	err = svc.WaitUntilSnapshotCompletedWithContext(ctx, &ec2.DescribeSnapshotsInput{
		SnapshotIds: []*string{result.SnapshotId},
	})
	if err != nil {
		return "", err
	}
	return *result.SnapshotId, nil
}

func deleteEBSSnapshot(ctx context.Context, svc *ec2.EC2, snapshotID string) error {
	// do not use context here: we want to force snapshot deletion
	_, err := svc.DeleteSnapshot(&ec2.DeleteSnapshotInput{
		SnapshotId: &snapshotID,
	})
	return err
}

func tagFailure(s []string) []string {
	return append(s, fmt.Sprint("status:failure"))
}

func tagSuccess(s []string) []string {
	return append(s, fmt.Sprint("status:success"))
}

func scanEBS(ctx context.Context, scan ebsScan) (*sbommodel.SBOMEntity, error) {
	if scan.Region == "" {
		return nil, fmt.Errorf("ebs-scan: missing region")
	}
	if scan.Hostname == "" {
		return nil, fmt.Errorf("ebs-scan: missing hostname")
	}

	defer statsd.Flush()

	tags := []string{
		fmt.Sprintf("region:%s", scan.Region),
		fmt.Sprintf("type:%s", "ebs-scan"),
	}

	snapshotID := scan.SnapshotID
	if snapshotID == "" {
		if scan.VolumeID == "" {
			return nil, fmt.Errorf("ebs-scan: missing volume ID")
		}
		snapshotStartedAt := time.Now()
		sess, err := session.NewSession(&aws.Config{
			Region: aws.String(scan.Region),
		})
		if err != nil {
			return nil, err
		}
		svc := ec2.New(sess)
		statsd.Count("datadog.sidescanner.snapshots.started", 1.0, tags, 1.0)
		log.Debugf("starting volume snapshotting %q", scan.VolumeID)
		snapshotID, err = createEBSSnapshot(ctx, svc, scan)
		if err != nil {
			statsd.Count("datadog.sidescanner.snapshots.finished", 1.0, tagFailure(tags), 1.0)
			return nil, err
		}
		log.Debugf("volume snapshotting finished sucessfully %q", snapshotID)
		statsd.Count("datadog.sidescanner.snapshots.finished", 1.0, tagSuccess(tags), 1.0)
		statsd.Histogram("datadog.sidescanner.snapshots.duration", float64(time.Since(snapshotStartedAt).Milliseconds()), tags, 1.0)
		defer func() {
			log.Debugf("deleting snapshot %q", snapshotID)
			deleteEBSSnapshot(ctx, svc, snapshotID)
		}()
	}

	log.Infof("start EBS scanning %s", scan)
	statsd.Count("datadog.sidescanner.scans.started", 1.0, tags, 1.0)
	scanStartedAt := time.Now()
	target := "ebs:" + snapshotID
	trivyCache := newMemoryCache()
	trivyDisabledAnalyzers := []analyzer.Type{analyzer.TypeSecret, analyzer.TypeLicenseFile}
	trivyDisabledAnalyzers = append(trivyDisabledAnalyzers, analyzer.TypeConfigFiles...)
	trivyDisabledAnalyzers = append(trivyDisabledAnalyzers, analyzer.TypeLanguages...)
	trivyVMArtifact, err := vm.NewArtifact(target, trivyCache, artifact.Option{
		Offline:           true,
		NoProgress:        true,
		DisabledAnalyzers: trivyDisabledAnalyzers,
		Slow:              true,
		SBOMSources:       []string{},
		DisabledHandlers:  []ftypes.HandlerType{ftypes.UnpackagedPostHandler},
		OnlyDirs:          []string{"etc", "var/lib/dpkg", "var/lib/rpm", "lib/apk"},
		AWSRegion:         scan.Region,
	})
	if err != nil {
		statsd.Count("datadog.sidescanner.scans.finished", 1.0, tagFailure(tags), 1.0)
		return nil, fmt.Errorf("unable to create artifact from image: %w", err)
	}
	trivyDetector := ospkg.Detector{}
	trivyVulnClient := vulnerability.NewClient(db.Config{})
	trivyApplier := applier.NewApplier(trivyCache)
	trivyLocalScanner := local.NewScanner(trivyApplier, trivyDetector, trivyVulnClient)
	trivyScanner := scanner.NewScanner(trivyLocalScanner, trivyVMArtifact)
	trivyReport, err := trivyScanner.ScanArtifact(ctx, types.ScanOptions{
		VulnType:            []string{},
		SecurityChecks:      []string{},
		ScanRemovedPackages: false,
		ListAllPackages:     true,
	})
	statsd.Histogram("datadog.sidescanner.scans.duration", float64(time.Since(scanStartedAt).Milliseconds()), tags, 1.0)
	if err != nil {
		statsd.Count("datadog.sidescanner.scans.finished", 1.0, tagFailure(tags), 1.0)
		return nil, fmt.Errorf("trivy scan failed: %w", err)
	}
	statsd.Count("datadog.sidescanner.scans.finished", 1.0, tagSuccess(tags), 1.0)

	createdAt := time.Now()
	duration := time.Since(scanStartedAt)
	marshaler := cyclonedx.NewMarshaler("")
	bom, err := marshaler.Marshal(trivyReport)
	if err != nil {
		return nil, fmt.Errorf("unable to marshal report to sbom format: %w", err)
	}

	entity := &sbommodel.SBOMEntity{
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

	return entity, nil
}

func scanLambda(ctx context.Context, scan lambdaScan) (*sbommodel.SBOMEntity, error) {
	if scan.Region == "" {
		return nil, fmt.Errorf("ebs-scan: missing region")
	}
	if scan.FunctionName == "" {
		return nil, fmt.Errorf("ebs-scan: missing function name")
	}

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(scan.Region),
	})
	if err != nil {
		return nil, err
	}
	svc := lambda.New(sess)
	lambdaFunc, err := svc.GetFunctionWithContext(ctx, &lambda.GetFunctionInput{
		FunctionName: aws.String(scan.FunctionName),
	})
	if err != nil {
		return nil, err
	}

	if lambdaFunc.Code.Location == nil {
		return nil, fmt.Errorf("lambdaFunc.Code.Location is nil")
	}

	tempDir, err := os.MkdirTemp("", "aws-lambda")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)

	archivePath := filepath.Join(tempDir, "code.zip")
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		return nil, err
	}
	defer archiveFile.Close()

	lambdaURL := *lambdaFunc.Code.Location
	resp, err := http.Get(lambdaURL) // TODO: create an http.Client with sane defaults
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status: %s", resp.Status)
	}

	_, err = io.Copy(archiveFile, resp.Body)
	if err != nil {
		return nil, err
	}

	extractedPath := filepath.Join(tempDir, "extract")
	err = os.Mkdir(extractedPath, 0700)
	if err != nil {
		return nil, err
	}

	err = extractZip(archivePath, extractedPath)
	if err != nil {
		return nil, err
	}

	scanStartedAt := time.Now()
	trivyCache := newMemoryCache()
	trivyFSArtifact, err := local2.NewArtifact(extractedPath, trivyCache, artifact.Option{
		Offline:           true,
		NoProgress:        true,
		DisabledAnalyzers: []analyzer.Type{},
		Slow:              true,
		SBOMSources:       []string{},
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

	createdAt := time.Now()
	duration := time.Since(scanStartedAt)
	marshaler := cyclonedx.NewMarshaler("")
	bom, err := marshaler.Marshal(trivyReport)
	if err != nil {
		return nil, err
	}

	entity := &sbommodel.SBOMEntity{
		Status: sbommodel.SBOMStatus_SUCCESS,
		Type:   sbommodel.SBOMSourceType_HOST_FILE_SYSTEM, // TODO: SBOMSourceType_LAMBDA
		Id:     "",
		InUse:  true,
		DdTags: []string{
			"function:" + scan.FunctionName,
		},
		GeneratedAt:        timestamppb.New(createdAt),
		GenerationDuration: convertDuration(duration),
		Hash:               "",
		Sbom: &sbommodel.SBOMEntity_Cyclonedx{
			Cyclonedx: convertBOM(bom),
		},
	}
	return entity, nil
}

func extractZip(zipPath, destinationPath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("extractZip: openreader: %w", err)
	}
	defer r.Close()

	// TODO: be more rebust against zip bombs
	for _, f := range r.File {
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
