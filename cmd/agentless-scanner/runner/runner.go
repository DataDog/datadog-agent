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
	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/scanners"
	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/types"

	"github.com/DataDog/datadog-agent/pkg/config/remote"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"

	ddogstatsd "github.com/DataDog/datadog-go/v5/statsd"

	"github.com/docker/distribution/reference"
)

const (
	// LoggerName is the name of the logger used by this package.
	LoggerName = "AGENTLESSSCANER"

	cleanupMaxDuration     = 2 * time.Minute
	defaultSnapshotsMaxTTL = 24 * time.Hour
)

// Options are the runner options.
type Options struct {
	Hostname       string
	DdEnv          string
	Workers        int
	ScannersMax    int
	PrintResults   bool
	NoFork         bool
	DefaultRoles   types.RolesMapping
	DefaultActions []types.ScanAction
	Statsd         *ddogstatsd.Client
}

// Runner is the main agentless-scanner runner that schedules scanning tasks.
type Runner struct {
	Options

	eventForwarder   epforwarder.EventPlatformForwarder
	findingsReporter *LogReporter
	rcClient         *remote.Client

	waiter awsutils.SnapshotWaiter

	regionsCleanupMu sync.Mutex
	regionsCleanup   map[string]*types.CloudID

	scansInProgress   map[types.CloudID]struct{}
	scansInProgressMu sync.RWMutex

	configsCh chan *types.ScanConfig
	scansCh   chan *types.ScanTask

	resultsCh chan types.ScanResult
}

// New creates a new runner.
func New(opts Options) (*Runner, error) {
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
	rcClient, err := remote.NewUnverifiedGRPCClient("sidescanner", version.AgentVersion, nil, 5*time.Second)
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
func (s *Runner) Cleanup(ctx context.Context, maxTTL time.Duration, region string, assumedRole *types.CloudID) error {
	toBeDeleted := awsutils.ListResourcesForCleanup(ctx, maxTTL, region, assumedRole)
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
		s.regionsCleanupMu.Lock()
		regionsCleanup := make(map[string]*types.CloudID, len(s.regionsCleanup))
		for region, role := range s.regionsCleanup {
			regionsCleanup[region] = role
		}
		s.regionsCleanup = nil
		s.regionsCleanupMu.Unlock()

		if len(regionsCleanup) > 0 {
			for region, role := range regionsCleanup {
				if err := s.Cleanup(ctx, defaultSnapshotsMaxTTL, region, role); err != nil {
					log.Warnf("cleanupProcess failed on region %q with role %q: %v", region, role, err)
				}
			}
		}
	}
}

// SubscribeRemoteConfig subscribes to remote-config polling for scan tasks.
func (s *Runner) SubscribeRemoteConfig(ctx context.Context) error {
	log.Infof("subscribing to remote-config")
	s.rcClient.Subscribe(state.ProductCSMSideScanning, func(update map[string]state.RawConfig, _ func(string, state.ApplyStatus)) {
		log.Debugf("received %d remote config config updates", len(update))
		for _, rawConfig := range update {
			log.Debugf("received new config %q from remote-config of size %d", rawConfig.Metadata.ID, len(rawConfig.Config))
			config, err := types.UnmarshalConfig(rawConfig.Config, s.Hostname, s.DefaultActions, s.DefaultRoles)
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
			case strings.HasPrefix(scanDir.Name(), string(types.ScanTypeLambda)+"-"):
				if err := os.RemoveAll(name); err != nil {
					log.Warnf("clean slate: could not remove directory %q", name)
				}
			case strings.HasPrefix(scanDir.Name(), string(types.ScanTypeEBS)):
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
				for _, child := range bd.GetChildrenType("lvm") {
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
					log.Errorf("%s: %s scanner reported a failure: %v", result.Scan, result.Scanner, result.Err)
				}
				if err := s.Statsd.Count("datadog.agentless_scanner.scans.finished", 1.0, result.Scan.TagsFailure(result.Err), 1.0); err != nil {
					log.Warnf("failed to send metric: %v", err)
				}
			} else {
				log.Infof("%s: scanner %s finished successfully (waited %s | took %s)", result.Scan, result.Scanner, result.StartedAt.Sub(result.CreatedAt), time.Since(result.StartedAt))
				if vulns := result.Vulns; vulns != nil {
					if hasResults(vulns.BOM) {
						if err := s.Statsd.Count("datadog.agentless_scanner.scans.finished", 1.0, result.Scan.TagsSuccess(), 1.0); err != nil {
							log.Warnf("failed to send metric: %v", err)
						}
					} else {
						log.Debugf("%s: scanner %s finished successfully without results", result.Scan, result.Scanner)
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
				s.regionsCleanupMu.Lock()
				if s.regionsCleanup == nil {
					s.regionsCleanup = make(map[string]*types.CloudID)
				}
				s.regionsCleanup[scan.CloudID.Region] = scan.Roles[scan.CloudID.Region]
				s.regionsCleanupMu.Unlock()

				// Avoid pushing a scan that we are already performing.
				// TODO: this guardrail could be avoided with a smarter scheduling.
				s.scansInProgressMu.Lock()
				if _, ok := s.scansInProgress[scan.CloudID]; ok {
					s.scansInProgressMu.Unlock()
					continue
				}
				s.scansInProgress[scan.CloudID] = struct{}{}
				s.scansInProgressMu.Unlock()

				if err := s.launchScan(ctx, scan); err != nil {
					if !errors.Is(err, context.Canceled) {
						log.Errorf("%s: could not be setup properly: %v", scan, err)
					}
				}

				s.scansInProgressMu.Lock()
				delete(s.scansInProgress, scan.CloudID)
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
	case types.ScanTypeHost:
		s.scanRootFilesystems(ctx, scan, []string{scan.CloudID.ResourceName()}, pool, s.resultsCh)
	case types.ScanTypeEBS:
		mountpoints, err := awsutils.SetupEBS(ctx, scan, &s.waiter)
		if err != nil {
			return err
		}
		s.scanRootFilesystems(ctx, scan, mountpoints, pool, s.resultsCh)
	case types.ScanTypeLambda:
		mountpoint, err := awsutils.SetupLambda(ctx, scan)
		if err != nil {
			return err
		}
		s.scanApplication(ctx, scan, mountpoint, pool, s.resultsCh)
	default:
		return fmt.Errorf("unknown scan type: %s", scan.Type)
	}
	return nil
}

func (s *Runner) cleanupScan(scan *types.ScanTask) {
	ctx, cancel := context.WithTimeout(context.Background(), cleanupMaxDuration)
	defer cancel()

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
		blockDevices, err := devices.List(ctx, *scan.AttachedDeviceName)
		if err == nil && len(blockDevices) == 1 {
			for _, child := range blockDevices[0].GetChildrenType("lvm") {
				if err := exec.Command("dmsetup", "remove", child.Path).Run(); err != nil {
					log.Errorf("%s: could not remove logical device %q from block device %q: %v", scan, child.Path, child.Name, err)
				}
			}
		}
	}

	switch scan.Type {
	case types.ScanTypeEBS:
		awsutils.CleanupScanEBS(ctx, scan)
	case types.ScanTypeLambda:
		// nothing to do
	case types.ScanTypeHost:
		// nothing to do
	default:
		panic("unreachable")
	}
}

func (s *Runner) sendSBOM(result types.ScanResult) error {
	vulns := result.Vulns
	sourceAgent := "agentless-scanner"

	entity := &sbommodel.SBOMEntity{
		Status: sbommodel.SBOMStatus_SUCCESS,
		Type:   vulns.SourceType,
		Id:     vulns.ID,
		InUse:  true,
		DdTags: append([]string{
			"agentless_scanner_host:" + s.Hostname,
			"region:" + result.Scan.CloudID.Region,
			"account_id:" + result.Scan.CloudID.AccountID,
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

func (s *Runner) scanRootFilesystems(ctx context.Context, scan *types.ScanTask, roots []string, pool *scannersPool, resultsCh chan types.ScanResult) {
	var wg sync.WaitGroup

	scanRoot := func(root string, action types.ScanAction) {
		defer wg.Done()

		switch action {
		case types.ScanActionVulnsHost:
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
		case types.ScanActionVulnsContainers:
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
					tags := scan.Tags(fmt.Sprintf("container_runtime:%s", runtime))
					if err := s.Statsd.Count("datadog.agentless_scanner.containers.count", count, tags, 1.0); err != nil {
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
		case types.ScanActionMalware:
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

	if err := s.Statsd.Histogram("datadog.agentless_scanner.scans.duration", float64(time.Since(scan.StartedAt).Milliseconds()), scan.Tags(), 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
}

func (s *Runner) scanApplication(ctx context.Context, scan *types.ScanTask, root string, pool *scannersPool, resultsCh chan types.ScanResult) {
	result := pool.launchScanner(ctx, types.ScannerOptions{
		Scanner:   types.ScannerNameAppVulns,
		Scan:      scan,
		Root:      root,
		CreatedAt: time.Now(),
	})
	if result.Vulns != nil {
		result.Vulns.SourceType = sbommodel.SBOMSourceType_CI_PIPELINE // TODO: SBOMSourceType_LAMBDA
		result.Vulns.ID = scan.CloudID.String()
		result.Vulns.Tags = []string{
			"runtime_id:" + scan.CloudID.String(),
			"service_version:TODO", // XXX
		}
	}
	resultsCh <- result
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
	switch opts.Scanner {
	case types.ScannerNameHostVulns:
		bom, err := scanners.LaunchTrivyHost(ctx, opts)
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
		devices.Umount(cleanupctx, opts.Scan, mountPoint)
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

func hasResults(bom *cdx.BOM) bool {
	// We can't use Dependencies > 0, since len(Dependencies) == 1 when there are no components.
	// See https://github.com/aquasecurity/trivy/blob/main/pkg/sbom/cyclonedx/core/cyclonedx.go
	return bom.Components != nil && len(*bom.Components) > 0
}
