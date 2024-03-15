// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package runner provides the main runner of our scanner that can be used to
// schedule scans and report findings.
package runner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	cdx "github.com/CycloneDX/cyclonedx-go"
	sbommodel "github.com/DataDog/agent-payload/v5/sbom"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/awsutils"
	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/devices"
	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/nbd"
	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/scanners"
	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/types"

	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"

	ddogstatsd "github.com/DataDog/datadog-go/v5/statsd"
)

const (
	// LoggerName is the name of the logger used by this package.
	LoggerName = "AGENTLESSSCANER"

	cleanupMaxDuration     = 2 * time.Minute
	defaultSnapshotsMaxTTL = 24 * time.Hour
)

// Options are the runner options.
type Options struct {
	ScannerID      types.ScannerID
	DdEnv          string
	Workers        int
	ScannersMax    int
	PrintResults   bool
	NoFork         bool
	DefaultRoles   types.RolesMapping
	DefaultActions []types.ScanAction
	Statsd         *ddogstatsd.Client
}

type scanRecord struct {
	Role   types.CloudID `json:"Role"`
	Region string        `json:"Region"`
}

// Runner is the main agentless-scanner runner that schedules scanning tasks.
type Runner struct {
	Options

	eventForwarder   epforwarder.EventPlatformForwarder
	findingsReporter *LogReporter
	rcClient         *client.Client

	waiter awsutils.ResourceWaiter

	touchedMu sync.Mutex
	touched   map[scanRecord]struct{}

	scansInProgress   map[types.CloudID]struct{}
	scansInProgressMu sync.RWMutex

	configsCh chan *types.ScanConfig
	scansCh   chan *types.ScanTask

	resultsCh chan types.ScanResult
}

// New creates a new runner.
func New(opts Options) (*Runner, error) {
	if opts.ScannerID == (types.ScannerID{}) {
		panic("programmer error: empty ScannerID option")
	}
	if opts.Statsd == nil {
		panic("programmer error: missing Statsd option")
	}
	if opts.Workers == 0 {
		panic("programmer error: Workers is 0")
	}
	if opts.ScannersMax == 0 {
		panic("programmer error: ScannersMax is 0")
	}
	eventForwarder := epforwarder.NewEventPlatformForwarder()
	findingsReporter, err := newFindingsReporter()
	if err != nil {
		return nil, err
	}
	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		return nil, err
	}

	rcClient, err := client.NewUnverifiedGRPCClient(ipcAddress, config.GetIPCPort(), security.FetchAuthToken,
		client.WithAgent("sidescanner", version.AgentVersion),
		client.WithPollInterval(5*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("could not init Remote Config client: %w", err)
	}
	return &Runner{
		Options: opts,

		eventForwarder:   eventForwarder,
		findingsReporter: findingsReporter,
		rcClient:         rcClient,

		scansInProgress: make(map[types.CloudID]struct{}),

		configsCh: make(chan *types.ScanConfig),
		scansCh:   make(chan *types.ScanTask),
		resultsCh: make(chan types.ScanResult),
	}, nil
}

// Cleanup cleans up all the resources created by the runner.
func (s *Runner) Cleanup(ctx context.Context, maxTTL time.Duration, region string, assumedRole types.CloudID) error {
	toBeDeleted, err := awsutils.ListResourcesForCleanup(ctx, maxTTL, region, assumedRole)
	if err != nil {
		return err
	}
	awsutils.ResourcesCleanup(ctx, toBeDeleted, region, assumedRole)
	return nil
}

func (s *Runner) cleanupProcess(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}

		log.Infof("starting cleanup process")
		s.touchedMu.Lock()
		touched := make(map[scanRecord]struct{}, len(s.touched))
		for record := range s.touched {
			touched[record] = struct{}{}
		}
		s.touched = nil
		s.touchedMu.Unlock()

		if len(touched) > 0 {
			for record := range touched {
				if err := s.Cleanup(ctx, defaultSnapshotsMaxTTL, record.Region, record.Role); err != nil {
					log.Warnf("cleanupProcess failed on region %q with role %q: %v", record.Region, record.Role, err)
				}
			}
		}
	}
}

func (s *Runner) statsResourceTTL(resourceType types.ResourceType, scan *types.ScanTask, createTime time.Time) {
	ttl := time.Since(createTime)
	tags := scan.Tags(fmt.Sprintf("aws_resource_type:%s", string(resourceType)))
	if err := s.Statsd.Histogram("datadog.agentless_scanner.aws.resources_ttl", float64(ttl.Milliseconds()), tags, 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
}

// SubscribeRemoteConfig subscribes to remote-config polling for scan tasks.
func (s *Runner) SubscribeRemoteConfig(ctx context.Context) error {
	log.Infof("subscribing to remote-config")
	s.rcClient.Subscribe(state.ProductCSMSideScanning, func(update map[string]state.RawConfig, _ func(string, state.ApplyStatus)) {
		log.Debugf("received %d remote config config updates", len(update))
		for _, rawConfig := range update {
			log.Debugf("received new config %q from remote-config of size %d", rawConfig.Metadata.ID, len(rawConfig.Config))
			config, err := types.UnmarshalConfig(rawConfig.Config, s.ScannerID, s.DefaultActions, s.DefaultRoles)
			if err != nil {
				log.Errorf("could not parse agentless-scanner task: %v", err)
				return
			}
			if !s.PushConfig(ctx, config) {
				return
			}
		}
	})
	return nil
}

func (s *Runner) healthServer(ctx context.Context) error {
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

// CleanSlate removes all the files, directories and cloud resources created
// by the scanner that could still be present at startup.
func (s *Runner) CleanSlate() error {
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
			case strings.HasPrefix(scanDir.Name(), string(types.TaskTypeLambda)+"-"):
				if err := os.RemoveAll(name); err != nil {
					log.Warnf("clean slate: could not remove directory %q", name)
				}
			case strings.HasPrefix(scanDir.Name(), string(types.TaskTypeEBS)),
				strings.HasPrefix(scanDir.Name(), string(types.TaskTypeAMI)):
				scanDirname := filepath.Join(types.ScansRootDir, scanDir.Name())
				scanEntries, err := os.ReadDir(scanDirname)
				if err != nil {
					log.Errorf("clean slate: %v", err)
				} else {
					for _, scanEntry := range scanEntries {
						switch {
						case strings.HasPrefix(scanEntry.Name(), types.EBSMountPrefix):
							ebsMountPoints = append(ebsMountPoints, filepath.Join(scanDirname, scanEntry.Name()))
						case strings.HasPrefix(scanEntry.Name(), types.ContainerMountPrefix):
							ctrMountPoints = append(ctrMountPoints, filepath.Join(scanDirname, scanEntry.Name()))
						}
					}
				}
			}
		}
	}

	for _, mountPoint := range ctrMountPoints {
		log.Warnf("clean slate: unmounting %q", mountPoint)
		devices.Umount(ctx, nil, mountPoint)
	}
	// unmount "ebs-*" entrypoint last as the other mountpoint may depend on it
	for _, mountPoint := range ebsMountPoints {
		log.Warnf("clean slate: unmounting %q", mountPoint)
		devices.Umount(ctx, nil, mountPoint)
	}

	for _, scanDir := range scanDirs {
		scanDirname := filepath.Join(types.ScansRootDir, scanDir.Name())
		log.Warnf("clean slate: removing directory %q", scanDirname)
		if err := os.RemoveAll(scanDirname); err != nil {
			log.Errorf("clean slate: could not remove directory %q", scanDirname)
		}
	}

	blockDevices, err := devices.List(ctx)
	if err == nil {
		for _, bd := range blockDevices {
			if strings.HasPrefix(bd.Name, "nbd") || strings.HasPrefix(bd.Serial, "vol") {
				devices.DetachLVMs(nil, bd)
			}
			if strings.HasPrefix(bd.Name, "nbd") {
				if err := exec.CommandContext(ctx, "nbd-client", "-d", path.Join("/dev", bd.Name)).Run(); err != nil {
					log.Errorf("clean slate: could not detach nbd device %q: %v", bd.Name, err)
				}
			}
		}
		awsutils.CleanSlate(ctx, blockDevices, s.DefaultRoles)
	}

	return nil
}

// Start starts the runner main loop.
func (s *Runner) Start(ctx context.Context) {
	log.Infof("starting agentless-scanner main loop with %d scan workers", s.Workers)
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
					log.Errorf("%s: %s: %s scanner reported a failure: %v", result.Scan, result.Scan.TargetID, result.Action, result.Err)
				}
				if err := s.Statsd.Count("datadog.agentless_scanner.scans.finished", 1.0, result.Scan.TagsFailure(result.Err), 1.0); err != nil {
					log.Warnf("failed to send metric: %v", err)
				}
			} else {
				log.Infof("%s: %s: scanner %s finished (waited %s | took %s): %s", result.Scan, result.Scan.TargetID, result.Action, result.StartedAt.Sub(result.CreatedAt), time.Since(result.StartedAt), nResults(result))
				if vulns := result.Vulns; vulns != nil {
					if hasResults(vulns.BOM) {
						if err := s.Statsd.Count("datadog.agentless_scanner.scans.finished", 1.0, result.Scan.TagsSuccess(), 1.0); err != nil {
							log.Warnf("failed to send metric: %v", err)
						}
					} else {
						if err := s.Statsd.Count("datadog.agentless_scanner.scans.finished", 1.0, result.Scan.TagsNoResult(), 1.0); err != nil {
							log.Warnf("failed to send metric: %v", err)
						}
					}
					if err := s.sendSBOM(result); err != nil {
						log.Errorf("%s: failed to send SBOM: %v", result.Scan, err)
					}
					if s.PrintResults {
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
					if err := s.Statsd.Count("datadog.agentless_scanner.scans.finished", 1.0, result.Scan.TagsSuccess(), 1.0); err != nil {
						log.Warnf("failed to send metric: %v", err)
					}
					log.Debugf("%s: sending findings", result.Scan)
					s.sendFindings(malware.Findings)
					if s.PrintResults {
						b, _ := json.MarshalIndent(malware.Findings, "", "  ")
						fmt.Printf("scanning types.Malware result %s (took %s): %s\n", result.Scan, time.Since(result.StartedAt), string(b))
					}
				}
			}
		}
	}()

	for i := 0; i < s.Workers; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for scan := range s.scansCh {
				// Gather the  scanned roles / accounts as we go. We only ever
				// need to store one role associated with one region. They
				// will be used for cleanup process.
				s.touchedMu.Lock()
				{
					// TODO: we could persist this "touched" map on the
					// filesystem to have a more robust knowledge of the
					// accounts / regions with scanned.
					if s.touched == nil {
						s.touched = make(map[scanRecord]struct{})
					}
					record := scanRecord{
						Role:   scan.Roles.GetCloudIDRole(scan.TargetID),
						Region: scan.TargetID.Region(),
					}
					s.touched[record] = struct{}{}
				}
				s.touchedMu.Unlock()

				// Avoid pushing a scan that we are already performing.
				// TODO: this guardrail could be avoided with a smarter scheduling.
				s.scansInProgressMu.Lock()
				if _, ok := s.scansInProgress[scan.TargetID]; ok {
					s.scansInProgressMu.Unlock()
					continue
				}
				s.scansInProgress[scan.TargetID] = struct{}{}
				s.scansInProgressMu.Unlock()

				if err := s.launchScan(ctx, scan); err != nil {
					if !errors.Is(err, context.Canceled) {
						log.Errorf("%s: could not be setup properly: %v", scan, err)
					}
				}

				s.scansInProgressMu.Lock()
				delete(s.scansInProgress, scan.TargetID)
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

	for i := 0; i < s.Workers; i++ {
		<-done
	}
	close(s.resultsCh)
	<-done // waiting for done in range resultsCh goroutine
}

// Stop stops the runner main loop.
func (s *Runner) Stop() {
	log.Infof("stopping agentless-scanner main loop")
	close(s.configsCh)
}

// PushConfig pushes a new scan configuration to the runner.
func (s *Runner) PushConfig(ctx context.Context, config *types.ScanConfig) bool {
	select {
	case s.configsCh <- config:
		return true
	case <-ctx.Done():
		return false
	}
}

func (s *Runner) launchScan(ctx context.Context, scan *types.ScanTask) (err error) {
	if err := s.Statsd.Count("datadog.agentless_scanner.scans.started", 1.0, scan.Tags(), 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
	defer func() {
		if err != nil {
			if err := s.Statsd.Count("datadog.agentless_scanner.scans.finished", 1.0, scan.TagsFailure(err), 1.0); err != nil {
				log.Warnf("failed to send metric: %v", err)
			}
		}
	}()

	if err := os.MkdirAll(scan.Path(), 0700); err != nil {
		return err
	}

	pool := newScannersPool(s.NoFork, s.ScannersMax)
	scan.StartedAt = time.Now()
	defer s.cleanupScan(scan)
	switch scan.Type {
	case types.TaskTypeHost:
		assert(s.ScannerID.Provider == types.CloudProviderNone)
		s.scanRootFilesystems(ctx, scan, []string{scan.TargetID.ResourceName()}, pool)

	case types.TaskTypeAMI:
		if err := awsutils.SetupEBS(ctx, scan, &s.waiter); err != nil {
			return err
		}
		partitions, err := devices.ListPartitions(ctx, scan, *scan.AttachedDeviceName)
		if err != nil {
			return err
		}
		mountpoints, err := devices.Mount(ctx, scan, partitions)
		if err != nil {
			return err
		}
		s.scanImage(ctx, scan, mountpoints, pool)

	case types.TaskTypeEBS:
		if err := awsutils.SetupEBS(ctx, scan, &s.waiter); err != nil {
			return err
		}
		switch scan.DiskMode {
		case types.DiskModeNoAttach:
			s.scanSnaphotNoAttach(ctx, scan, pool)
		case types.DiskModeNBDAttach, types.DiskModeVolumeAttach:
			partitions, err := devices.ListPartitions(ctx, scan, *scan.AttachedDeviceName)
			if err != nil {
				return err
			}
			mountpoints, err := devices.Mount(ctx, scan, partitions)
			if err != nil {
				return err
			}
			s.scanRootFilesystems(ctx, scan, mountpoints, pool)
		}

	case types.TaskTypeLambda:
		mountpoint, err := awsutils.SetupLambda(ctx, scan)
		if err != nil {
			return err
		}
		s.scanLambda(ctx, scan, mountpoint, pool)

	default:
		return fmt.Errorf("unknown scan type: %s", scan.Type)
	}
	return nil
}

// CleanupScanDir cleans up the scan directory on the filesystem: mountpoints,
// pidfiles, sockets...
func CleanupScanDir(ctx context.Context, scan *types.ScanTask) {
	scanRoot := scan.Path()

	log.Debugf("%s: cleaning up scan data on filesystem", scan)

	entries, err := os.ReadDir(scanRoot)
	if err == nil {
		var wg sync.WaitGroup

		umount := func(mountPoint string) {
			defer wg.Done()
			devices.Umount(ctx, scan, mountPoint)
		}

		var ebsMountPoints []fs.DirEntry
		var ctrMountPoints []fs.DirEntry
		var pidFiles []fs.DirEntry

		for _, entry := range entries {
			if entry.IsDir() {
				if strings.HasPrefix(entry.Name(), types.EBSMountPrefix) {
					ebsMountPoints = append(ebsMountPoints, entry)
				}
				if strings.HasPrefix(entry.Name(), types.ContainerMountPrefix) {
					ctrMountPoints = append(ctrMountPoints, entry)
				}
				if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".pid") {
					pidFiles = append(pidFiles, entry)
				}
			}
		}

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

	}

	log.Debugf("%s: removing folder %q", scan, scanRoot)
	if err := os.RemoveAll(scanRoot); err != nil {
		log.Errorf("%s: could not cleanup mount root %q: %v", scan, scanRoot, err)
	}

	if scan.AttachedDeviceName != nil {
		blockDevices, err := devices.List(ctx, *scan.AttachedDeviceName)
		if err == nil && len(blockDevices) == 1 {
			devices.DetachLVMs(scan, blockDevices[0])
		}
		if scan.DiskMode == types.DiskModeNBDAttach {
			nbd.StopNBDBlockDevice(ctx, *scan.AttachedDeviceName)
		}
	}
}

func (s *Runner) cleanupScan(scan *types.ScanTask) {
	ctx, cancel := context.WithTimeout(context.Background(), cleanupMaxDuration)
	defer cancel()

	CleanupScanDir(ctx, scan)

	switch scan.Type {
	case types.TaskTypeEBS, types.TaskTypeAMI:
		for resourceID, createdAt := range scan.CreatedResources {
			if err := awsutils.CleanupScanEBS(ctx, scan, resourceID); err != nil {
				log.Warnf("%s: failed to cleanup EBS resource %q: %v", scan, resourceID, err)
			} else {
				s.statsResourceTTL(resourceID.ResourceType(), scan, createdAt)
			}
		}
	case types.TaskTypeLambda, types.TaskTypeHost:
		if len(scan.CreatedResources) > 0 {
			panic(fmt.Errorf("unexpected resources created in %s scan", scan.Type))
		}
		// nothing to do
	default:
		panic("unreachable")
	}
}

func (s *Runner) sendSBOM(result types.ScanResult) error {
	vulns := result.Vulns
	sourceAgent := "agentless-scanner"

	reservedTags := [3]string{"agentless_scanner_host", "region", "account_id"}
	ddTags := []string{
		"agentless_scanner_host:" + s.ScannerID.Hostname,
		"region:" + result.Scan.TargetID.Region(),
		"account_id:" + result.Scan.TargetID.AccountID(),
	}

	for _, tag := range vulns.Tags {
		tagS := strings.SplitN(tag, ":", 2)
		tagP := tagS[0]
		skip := false
		for _, reserved := range reservedTags {
			if tagP == reserved {
				skip = true
			}
		}
		if !skip {
			ddTags = append(ddTags, tag)
		}
	}

	entity := &sbommodel.SBOMEntity{
		Status:             sbommodel.SBOMStatus_SUCCESS,
		Type:               vulns.SourceType,
		Id:                 vulns.ID,
		InUse:              true,
		DdTags:             ddTags,
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
		DdEnv:    &s.DdEnv,
	})
	if err != nil {
		return fmt.Errorf("unable to proto marhsal sbom: %w", err)
	}

	m := message.NewMessage(rawEvent, nil, "", 0)
	return s.eventForwarder.SendEventPlatformEvent(m, epforwarder.EventTypeContainerSBOM)
}

func (s *Runner) sendFindings(findings []*types.ScanFinding) {
	var tags []string // TODO: tags
	expireAt := time.Now().Add(24 * time.Hour)
	for _, finding := range findings {
		finding.ExpireAt = &expireAt
		finding.AgentVersion = version.AgentVersion
		s.findingsReporter.ReportEvent(finding, tags...)
	}
}

func (s *Runner) scanImage(ctx context.Context, scan *types.ScanTask, roots []string, pool *scannersPool) {
	assert(scan.Type == types.TaskTypeAMI)

	var wg sync.WaitGroup
	for _, root := range roots {
		wg.Add(1)
		go func(root string) {
			defer wg.Done()
			s.resultsCh <- pool.launchScannerVulns(ctx,
				sbommodel.SBOMSourceType_HOST_IMAGE,
				scan.TargetName,
				scan.TargetTags,
				types.ScannerOptions{
					Action:    types.ScanActionVulnsHostOS,
					Scan:      scan,
					Root:      root,
					CreatedAt: time.Now(),
				})
		}(root)
	}
	wg.Wait()
}

func (s *Runner) scanSnaphotNoAttach(ctx context.Context, scan *types.ScanTask, pool *scannersPool) {
	assert(scan.Type == types.TaskTypeEBS)
	s.resultsCh <- pool.launchScannerVulns(ctx,
		sbommodel.SBOMSourceType_HOST_FILE_SYSTEM,
		scan.TargetName,
		scan.TargetTags,
		types.ScannerOptions{
			Action:    types.ScanActionVulnsHostOSVm,
			Scan:      scan,
			CreatedAt: time.Now(),
		})
}

func (s *Runner) scanRootFilesystems(ctx context.Context, scan *types.ScanTask, roots []string, pool *scannersPool) {
	assert(scan.Type == types.TaskTypeHost || scan.Type == types.TaskTypeEBS)

	var wg sync.WaitGroup

	scanHost := func(root string, actions []types.ScanAction) {
		defer wg.Done()

		for _, action := range actions {
			assert(action == types.ScanActionVulnsHostOS || action == types.ScanActionMalware)
			switch action {
			case types.ScanActionVulnsHostOS:
				s.resultsCh <- pool.launchScannerVulns(ctx,
					sbommodel.SBOMSourceType_HOST_FILE_SYSTEM,
					scan.TargetName,
					scan.TargetTags,
					types.ScannerOptions{
						Action:    types.ScanActionVulnsHostOS,
						Scan:      scan,
						Root:      root,
						CreatedAt: time.Now(),
					})
			case types.ScanActionMalware:
				s.resultsCh <- pool.launchScanner(ctx, types.ScannerOptions{
					Action:    types.ScanActionMalware,
					Scan:      scan,
					Root:      root,
					CreatedAt: time.Now(),
				})
			}
		}
	}

	scanContainers := func(root string, actions []types.ScanAction) {
		defer wg.Done()

		ctrPrepareResult := pool.launchScanner(ctx, types.ScannerOptions{
			Action:    types.ScanActionContainersInspect,
			Scan:      scan,
			Root:      root,
			CreatedAt: time.Now(),
		})
		if ctrPrepareResult.Err != nil {
			s.resultsCh <- ctrPrepareResult
			return
		}
		containers := ctrPrepareResult.Containers.Containers
		if len(containers) == 0 {
			return
		}
		log.Infof("%s: found %d containers on %q", scan, len(containers), root)
		runtimes := make(map[string]int64)
		for _, ctr := range containers {
			runtimes[ctr.Runtime]++
		}
		for runtime, count := range runtimes {
			tags := scan.Tags(fmt.Sprintf("container_runtime:%s", runtime))
			if err := s.Statsd.Count("datadog.agentless_scanner.containers.count", count, tags, 1.0); err != nil {
				log.Warnf("failed to send metric: %v", err)
			}
		}
		ctrDoneCh := make(chan *types.Container)
		for _, ctr := range containers {
			go func(ctr *types.Container, actions []types.ScanAction) {
				s.scanContainer(ctx, scan, ctr, actions, pool)
				ctrDoneCh <- ctr
			}(ctr, actions)
		}
		for i := 0; i < len(containers); i++ {
			ctr := <-ctrDoneCh
			// We cleanup overlays as we go instead of acumulating them. However
			// the cleanupScan routine also cleans up any leftover. We do not rely
			// on the parent ctx as we still want to clean these mounts even for a
			// canceled/timeouted context.
			cleanupctx, abort := context.WithTimeout(context.Background(), 5*time.Second)
			devices.Umount(cleanupctx, scan, ctr.MountPoint)
			abort()
		}
	}

	for _, root := range roots {
		var hostActions []types.ScanAction
		var ctrsActions []types.ScanAction

		for _, action := range scan.Actions {
			switch action {
			case types.ScanActionVulnsHostOS, types.ScanActionMalware:
				hostActions = append(hostActions, action)
			case types.ScanActionVulnsContainersApp, types.ScanActionVulnsContainersOS:
				ctrsActions = append(ctrsActions, action)
			default:
				log.Infof("%s: unexpected scan action %q", scan, action)
			}
		}

		if len(hostActions) > 0 {
			wg.Add(1)
			go scanHost(root, hostActions)
		}

		if len(ctrsActions) > 0 {
			wg.Add(1)
			go scanContainers(root, ctrsActions)
		}
	}
	wg.Wait()

	if err := s.Statsd.Histogram("datadog.agentless_scanner.scans.duration", float64(time.Since(scan.StartedAt).Milliseconds()), scan.Tags(), 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
}

func (s *Runner) scanContainer(ctx context.Context, scan *types.ScanTask, ctr *types.Container, actions []types.ScanAction, pool *scannersPool) {
	imageRefTagged, imageRefCanonical, imageTags := scanners.ContainerRefs(*ctr)
	for _, action := range actions {
		assert(action == types.ScanActionVulnsContainersApp || action == types.ScanActionVulnsContainersOS)

		var sourceType sbommodel.SBOMSourceType
		var tags []string
		switch action {
		case types.ScanActionVulnsContainersOS:
			sourceType = sbommodel.SBOMSourceType_CONTAINER_IMAGE_LAYERS // TODO: sbommodel.SBOMSourceType_CONTAINER_FILE_SYSTEM
			tags = imageTags
		case types.ScanActionVulnsContainersApp:
			sourceType = sbommodel.SBOMSourceType_CI_PIPELINE // TODO: sbommodel.SBOMSourceType_CONTAINER_APP
			tags = append([]string{
				fmt.Sprintf("runtime_id:%s", imageRefTagged.Name()),
				fmt.Sprintf("service_version:%s", imageRefTagged.Tag()),
			}, imageTags...)
		default:
			panic("unreachable")
		}

		sbomID := imageRefCanonical.String()
		result := pool.launchScannerVulns(ctx,
			sourceType,
			sbomID,
			tags,
			types.ScannerOptions{
				Action:    action,
				Scan:      scan,
				Root:      ctr.MountPoint,
				Container: ctr,
				CreatedAt: time.Now(),
			})
		// TODO: remove this when we backport
		// https://github.com/DataDog/datadog-agent/pull/22161
		if result.Vulns != nil && result.Vulns.BOM != nil {
			appendSBOMRepoMetadata(result.Vulns.BOM, imageRefTagged, imageRefCanonical)
		}
		s.resultsCh <- result
	}
}

func (s *Runner) scanLambda(ctx context.Context, scan *types.ScanTask, root string, pool *scannersPool) {
	assert(scan.Type == types.TaskTypeLambda)

	tags := append([]string{
		fmt.Sprintf("runtime_id:%s", scan.TargetID.AsText()),
		fmt.Sprintf("service_version:%s", scan.TargetName),
	}, scan.TargetTags...)

	s.resultsCh <- pool.launchScannerVulns(ctx,
		sbommodel.SBOMSourceType_CI_PIPELINE, // TODO: sbommodel.SBOMSourceType_LAMBDA
		scan.TargetID.AsText(),
		tags,
		types.ScannerOptions{
			Action:    types.ScanActionAppVulns,
			Scan:      scan,
			Root:      root,
			CreatedAt: time.Now(),
		})
	if err := s.Statsd.Histogram("datadog.agentless_scanner.scans.duration", float64(time.Since(scan.StartedAt).Milliseconds()), scan.Tags(), 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
}

type scannersPool struct {
	sem    chan struct{}
	noFork bool
}

func newScannersPool(noFork bool, size int) *scannersPool {
	return &scannersPool{
		sem:    make(chan struct{}, size),
		noFork: noFork,
	}
}

func (p *scannersPool) launchScannerVulns(ctx context.Context, sourceType sbommodel.SBOMSourceType, sbomID string, tags []string, opts types.ScannerOptions) types.ScanResult {
	result := p.launchScanner(ctx, opts)
	if result.Vulns != nil {
		result.Vulns.SourceType = sourceType
		result.Vulns.ID = sbomID
		result.Vulns.Tags = tags
	}
	return result
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
		if p.noFork {
			result = LaunchScannerInSameProcess(ctx, opts)
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

// LaunchScannerInSameProcess launches the scanner in the same process (no fork).
func LaunchScannerInSameProcess(ctx context.Context, opts types.ScannerOptions) types.ScanResult {
	switch opts.Action {
	case types.ScanActionVulnsHostOS, types.ScanActionVulnsContainersOS:
		bom, err := scanners.LaunchTrivyHost(ctx, opts)
		if err != nil {
			return opts.ErrResult(err)
		}
		return types.ScanResult{ScannerOptions: opts, Vulns: &types.ScanVulnsResult{BOM: bom}}

	case types.ScanActionAppVulns, types.ScanActionVulnsContainersApp:
		bom, err := scanners.LaunchTrivyApp(ctx, opts)
		if err != nil {
			return opts.ErrResult(err)
		}
		return types.ScanResult{ScannerOptions: opts, Vulns: &types.ScanVulnsResult{BOM: bom}}

	case types.ScanActionVulnsHostOSVm:
		bom, err := scanners.LaunchTrivyHostVM(ctx, opts)
		if err != nil {
			return opts.ErrResult(err)
		}
		return types.ScanResult{ScannerOptions: opts, Vulns: &types.ScanVulnsResult{BOM: bom}}

	case types.ScanActionContainersInspect:
		containers, err := scanners.LaunchContainersInspect(ctx, opts)
		if err != nil {
			return opts.ErrResult(err)
		}
		return types.ScanResult{ScannerOptions: opts, Containers: &containers}

	case types.ScanActionMalware:
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
			if len(line) > 24 && strings.HasPrefix(line[24:], "| "+LoggerName+" |") {
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
			log.Errorf("%s: execed scanner %q with pid=%d: %v: with output:%s", opts.Scan, opts.Action, cmd.Process.Pid, errx, stderrx)
		} else {
			log.Errorf("%s: execed scanner %q: %v", opts.Scan, opts.Action, err)
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

func hasResults(bom *cdx.BOM) bool {
	// We can't use Dependencies > 0, since len(Dependencies) == 1 when there are no components.
	// See https://github.com/aquasecurity/trivy/blob/main/pkg/sbom/cyclonedx/core/cyclonedx.go
	return bom.Components != nil && len(*bom.Components) > 0
}

func nResults(result types.ScanResult) string {
	if vulns := result.Vulns; vulns != nil {
		if vulns.BOM.Components != nil {
			return fmt.Sprintf("%d components", len(*vulns.BOM.Components))
		}
		return "no components"
	}

	if malware := result.Malware; malware != nil {
		return fmt.Sprintf("%d findings", len(malware.Findings))
	}

	return "no results"
}

func assert(b bool) {
	if !b {
		panic("assertion failed")
	}
}
