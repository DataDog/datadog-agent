// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package runner provides the main runner of our scanner that can be used to
// schedule scans and report findings.
package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	cdx "github.com/CycloneDX/cyclonedx-go"
	sbommodel "github.com/DataDog/agent-payload/v5/sbom"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/DataDog/datadog-agent/pkg/agentless/awsbackend"
	"github.com/DataDog/datadog-agent/pkg/agentless/devices"
	"github.com/DataDog/datadog-agent/pkg/agentless/types"

	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"

	ddogstatsd "github.com/DataDog/datadog-go/v5/statsd"
)

const (
	// LoggerName is the name of the logger used by this package.
	LoggerName = "AGENTLESSSCANNER"

	cleanupMaxDuration     = 2 * time.Minute
	defaultSnapshotsMaxTTL = 24 * time.Hour
)

// Options are the runner options.
type Options struct {
	ScannerID      types.ScannerID
	Workers        int
	ScannersMax    int
	PrintResults   bool
	Statsd         ddogstatsd.ClientInterface
	EventForwarder eventplatform.Component
}

type scanRecord struct {
	Role   types.CloudID `json:"Role"`
	Region string        `json:"Region"`
}

// Runner is the main agentless-scanner runner that schedules scanning tasks.
type Runner struct {
	types.ScannerConfig
	Options

	findingsReporter *LogReporter
	rcClient         *client.Client

	touchedMu sync.Mutex
	touched   map[scanRecord]struct{}

	runningScans   map[types.CloudID]*types.ScanTask
	runningScansMu sync.RWMutex

	configsCh chan *types.ScanConfig
	scansCh   chan *types.ScanTask
	resultsCh chan types.ScanResult
}

// New creates a new runner.
func New(config types.ScannerConfig, opts Options) (*Runner, error) {
	if opts.ScannerID == (types.ScannerID{}) {
		panic("programmer error: empty ScannerID option")
	}
	if opts.Statsd == nil {
		panic("programmer error: missing Statsd option")
	}
	if opts.EventForwarder == nil {
		panic("programmer error: missing EventForwarder option")
	}
	if opts.Workers == 0 {
		panic("programmer error: Workers is 0")
	}
	if opts.ScannersMax == 0 {
		panic("programmer error: ScannersMax is 0")
	}
	findingsReporter, err := newFindingsReporter()
	if err != nil {
		return nil, err
	}

	return &Runner{
		ScannerConfig: config,
		Options:       opts,

		findingsReporter: findingsReporter,

		runningScans: make(map[types.CloudID]*types.ScanTask),

		configsCh: make(chan *types.ScanConfig),
		scansCh:   make(chan *types.ScanTask),
		resultsCh: make(chan types.ScanResult),
	}, nil
}

// Cleanup cleans up all the resources created by the runner.
func (s *Runner) Cleanup(ctx context.Context, maxTTL time.Duration, region string, assumedRole types.CloudID) error {
	switch s.ScannerID.Provider {
	case types.CloudProviderAWS:
		toBeDeleted, err := awsbackend.ListResourcesForCleanup(ctx, s.Statsd, &s.ScannerConfig, maxTTL, region, assumedRole)
		if err != nil {
			return err
		}
		awsbackend.ResourcesCleanup(ctx, s.Statsd, &s.ScannerConfig, toBeDeleted, region, assumedRole)
		return nil
	case types.CloudProviderAzure:
		// TODO: implement Azure cleanup
		return nil
	case types.CloudProviderNone:
		return nil
	default:
		panic("programmer error: unknown cloud provider")
	}
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

// SubscribeRemoteConfig subscribes to remote-config polling for scan tasks.
func (s *Runner) SubscribeRemoteConfig(ctx context.Context) error {
	ipcAddress, err := config.GetIPCAddress()
	if err != nil {
		return fmt.Errorf("could not init Remote Config: could not get IPC address: %w", err)
	}

	s.rcClient, err = client.NewUnverifiedGRPCClient(ipcAddress, config.GetIPCPort(),
		func() (string, error) { return security.FetchAuthToken(config.Datadog) },
		client.WithAgent("sidescanner", version.AgentVersion),
		client.WithPollInterval(5*time.Second),
	)
	if err != nil {
		return fmt.Errorf("could not init Remote Config client: %w", err)
	}

	log.Infof("subscribing to remote-config")
	s.rcClient.Subscribe(state.ProductCSMSideScanning, func(update map[string]state.RawConfig, _ func(string, state.ApplyStatus)) {
		log.Debugf("received %d remote config config updates", len(update))
		for _, rawConfig := range update {
			log.Debugf("received new config %q from remote-config of size %d", rawConfig.Metadata.ID, len(rawConfig.Config))
			config, err := types.UnmarshalConfig(rawConfig.Config, &s.ScannerConfig, s.ScannerID)
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

	scansRootDir := types.ScansRootDir()
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
			case strings.HasPrefix(scanDir.Name(), string(types.TaskTypeLambda)+"-"):
				if err := os.RemoveAll(name); err != nil {
					log.Warnf("clean slate: could not remove directory %q", name)
				}
			case strings.HasPrefix(scanDir.Name(), string(types.TaskTypeEBS)),
				strings.HasPrefix(scanDir.Name(), string(types.TaskTypeAMI)):
				scanDirname := filepath.Join(scansRootDir, scanDir.Name())
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
		scanDirname := filepath.Join(scansRootDir, scanDir.Name())
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
		switch s.ScannerID.Provider {
		case types.CloudProviderAWS:
			awsbackend.CleanSlate(ctx, s.Statsd, &s.ScannerConfig, blockDevices, s.DefaultRolesMapping)
		case types.CloudProviderAzure:
			// TODO: implement Azure clean slate
		case types.CloudProviderNone:
			// Nothing to do
		default:
			panic("programmer error: unknown cloud provider")
		}
	}

	return nil
}

func (s *Runner) init(ctx context.Context) (<-chan *types.ScanTask, chan<- *types.ScanTask, <-chan struct{}) {
	eventPlatform, found := s.EventForwarder.Get()
	if found {
		eventPlatform.Start()
	} else {
		log.Info("not starting the event platform forwarder")
	}

	if s.rcClient != nil {
		s.rcClient.Start()
	}

	go func() {
		err := s.healthServer(ctx)
		if err != nil {
			log.Warnf("healthServer: %v", err)
		}
	}()

	go s.cleanupProcess(ctx)

	doneCh := make(chan struct{})
	go func() {
		s.reportResults(s.resultsCh)
		eventPlatform.Stop()
		s.findingsReporter.Stop()
		close(doneCh)
	}()

	triggeredScansCh := make(chan *types.ScanTask)
	finishedScansCh := make(chan *types.ScanTask)
	go func() {
		defer close(triggeredScansCh)
		for scan := range s.scansCh {
			if !s.isScanRunning(scan) {
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
				triggeredScansCh <- scan
				s.recordTriggeredScan(scan)
			}
		}
	}()

	go func() {
		for scan := range finishedScansCh {
			s.recordFinishedScan(scan)
		}
	}()

	return triggeredScansCh, finishedScansCh, doneCh
}

// Start starts the runner main loop.
func (s *Runner) Start(ctx context.Context) {
	triggeredScansCh, finishedScansCh, resultsDoneCh := s.init(ctx)

	var wg sync.WaitGroup
	wg.Add(s.Workers)
	for i := 0; i < s.Workers; i++ {
		go func(id int) {
			w := NewWorker(id, s.ScannerConfig, WorkerOptions{
				ScannerID:   s.ScannerID,
				ScannersMax: s.ScannersMax,
				Statsd:      s.Statsd,
			})
			w.Run(ctx, triggeredScansCh, finishedScansCh, s.resultsCh)
			wg.Done()
		}(i)
	}

	defer func() {
		log.Infof("stopping agentless-scanner main loop")
		close(s.scansCh)
		wg.Wait()
		close(finishedScansCh)
		close(s.resultsCh)
		<-resultsDoneCh
		log.Infof("stopped agentless-scanner main loop")
	}()

	log.Infof("starting agentless-scanner main loop with %d scan workers", s.Workers)
	for {
		select {
		case <-ctx.Done():
			return
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
		}
	}
}

// StartWithRemoteWorkers starts the runner main loop with remote workers. It
// spawns an HTTP server to dispatch scan tasks to remote workers.
func (s *Runner) StartWithRemoteWorkers(ctx context.Context) {
	triggeredScansCh, finishedScansCh, resultsDoneCh := s.init(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/scans", func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodGet:
			s.runningScansMu.RLock()
			defer s.runningScansMu.RUnlock()
			w.WriteHeader(http.StatusOK)
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(s.runningScans); err != nil {
				http.Error(w, "could not encode scans", http.StatusInternalServerError)
				return
			}
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/scan", func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodGet:
			if scan, ok := <-triggeredScansCh; ok {
				w.WriteHeader(http.StatusOK)
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(scan); err != nil {
					http.Error(w, "could not encode scan", http.StatusInternalServerError)
				}
			} else {
				http.Error(w, "no scan available", http.StatusNoContent)
			}
		case http.MethodPut:
			var scan types.ScanTask
			if err := json.NewDecoder(req.Body).Decode(&scan); err != nil {
				http.Error(w, "could not decode scan", http.StatusBadRequest)
				return
			}
			s.runningScansMu.Lock()
			defer s.runningScansMu.Unlock()
			if _, ok := s.runningScans[scan.TargetID]; ok {
				w.WriteHeader(http.StatusAccepted)
				s.runningScans[scan.TargetID] = &scan
			} else {
				http.Error(w, "scan not in progress", http.StatusNotFound)
			}
		case http.MethodPost:
			var scan types.ScanTask
			if err := json.NewDecoder(req.Body).Decode(&scan); err != nil {
				http.Error(w, "could not decode scan", http.StatusBadRequest)
				return
			}
			finishedScansCh <- &scan
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/result", func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodPost:
			var result types.ScanResult
			if err := json.NewDecoder(req.Body).Decode(&result); err != nil {
				http.Error(w, "could not decode result", http.StatusBadRequest)
				return
			}
			s.resultsCh <- result
			w.WriteHeader(http.StatusAccepted)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	addr := "127.0.0.1:6254"
	srv := &http.Server{Addr: "127.0.0.1:6254"}
	srv.Handler = mux

	go func() {
		log.Infof("Starting server for agentless-scanner on address %q", addr)
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("runner: could not start HTTP server: %v", err)
			os.Exit(1)
		}
	}()

	go func() {
		<-ctx.Done()
		ctxcleanup, cancel := context.WithTimeout(context.Background(), cleanupMaxDuration)
		defer cancel()
		err := srv.Shutdown(ctxcleanup)
		if err != nil {
			log.Warnf("error shutting down: %v", err)
		}
		close(s.resultsCh)
		close(finishedScansCh)
	}()

	defer func() {
		close(s.scansCh)
		<-resultsDoneCh
	}()

	for {
		select {
		case <-ctx.Done():
			return
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
		}
	}
}

func (s *Runner) isScanRunning(scan *types.ScanTask) bool {
	s.runningScansMu.RLock()
	defer s.runningScansMu.RUnlock()
	_, ok := s.runningScans[scan.TargetID]
	return ok
}

func (s *Runner) recordTriggeredScan(scan *types.ScanTask) {
	// Avoid pushing a scan that we are already performing.
	// TODO: this guardrail could be avoided with a smarter scheduling.
	s.runningScansMu.Lock()
	defer s.runningScansMu.Unlock()
	if _, ok := s.runningScans[scan.TargetID]; !ok {
		s.runningScans[scan.TargetID] = scan
	}
}

func (s *Runner) recordFinishedScan(scan *types.ScanTask) {
	s.runningScansMu.Lock()
	defer s.runningScansMu.Unlock()
	delete(s.runningScans, scan.TargetID)
}

func (s *Runner) reportResults(resultsCh <-chan types.ScanResult) {
	for result := range resultsCh {
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
	payload := &sbommodel.SBOMPayload{
		Version:  1,
		Source:   &sourceAgent,
		Entities: []*sbommodel.SBOMEntity{entity},
		DdEnv:    &s.ScannerConfig.Env,
	}
	if result.Scan.Type == types.TaskTypeEBS {
		payload.Host = result.Scan.TargetName
	}
	rawEvent, err := proto.Marshal(payload)
	if err != nil {
		return fmt.Errorf("unable to proto marhsal sbom: %w", err)
	}

	eventPlatform, found := s.EventForwarder.Get()
	if !found {
		return errors.New("event platform forwarder not initialized")
	}

	m := message.NewMessage(rawEvent, nil, "", 0)
	return eventPlatform.SendEventPlatformEvent(m, eventplatform.EventTypeContainerSBOM)
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

// Stop stops the runner main loop.
func (s *Runner) Stop() {
	if s.rcClient != nil {
		s.rcClient.Close()
	}
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
