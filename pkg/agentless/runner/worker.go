// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

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
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	sbommodel "github.com/DataDog/agent-payload/v5/sbom"

	"github.com/DataDog/datadog-agent/pkg/agentless/awsbackend"
	"github.com/DataDog/datadog-agent/pkg/agentless/devices"
	"github.com/DataDog/datadog-agent/pkg/agentless/nbd"
	"github.com/DataDog/datadog-agent/pkg/agentless/scanners"
	"github.com/DataDog/datadog-agent/pkg/agentless/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	ddogstatsd "github.com/DataDog/datadog-go/v5/statsd"
)

// WorkerOptions holds the configuration for a worker.
type WorkerOptions struct {
	ScannerID   types.ScannerID
	ScannersMax int
	NoFork      bool
	Statsd      ddogstatsd.ClientInterface
}

// Worker is a worker that runs scans.
type Worker struct {
	WorkerOptions

	id     int
	waiter awsbackend.ResourceWaiter
}

// NewWorker creates a new worker.
func NewWorker(id int, opts WorkerOptions) *Worker {
	return &Worker{
		WorkerOptions: opts,
		id:            id,
	}
}

func (w *Worker) String() string {
	return fmt.Sprintf("worker-%s-%s-%d", w.ScannerID.Provider, w.ScannerID.Hostname, w.id)
}

// Run runs the worker. It goes through the triggered scans from
// triggeredScansCh chan and runs them. It sends the results in the resultsCh
// chan and signals the finished scans in the finishedScansCh chan.
func (w *Worker) Run(ctx context.Context, statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, triggeredScansCh <-chan *types.ScanTask, finishedScansCh chan<- *types.ScanTask, resultsCh chan<- types.ScanResult) {
	for scan := range triggeredScansCh {
		if err := w.triggerScan(ctx, statsd, sc, scan, resultsCh); err != nil {
			if !errors.Is(err, context.Canceled) {
				log.Errorf("%s: could not be setup properly: %v", scan, err)
			}
			select {
			case finishedScansCh <- scan:
			case <-ctx.Done():
				return
			}
		}
	}
}

// RunWithHTTP runs the worker with an HTTP server. It polls for new tasks on
// the server and sends the results "/task" endpoint back to the server.
func (w *Worker) RunWithHTTP(ctx context.Context, statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, srv url.URL) {
	resultsCh := make(chan types.ScanResult)
	cli := &http.Client{Timeout: 2 * time.Minute}
	go func() {
		for {
			scan, err := w.httpPollScan(ctx, cli, srv)
			if err != nil {
				log.Errorf("%s: could not poll a new task: %v", w, err)
				continue // XXX: retry
			}

			if err := w.triggerScan(ctx, statsd, sc, scan, resultsCh); err != nil {
				if !errors.Is(err, context.Canceled) {
					log.Errorf("%s: could not be setup properly: %v", w, err)
				}
			}

			if err := w.httpSignalFinishedScan(ctx, cli, srv, scan); err != nil {
				log.Errorf("%s: could not signal finished scan: %v", w, err)
				continue // XXX: retry
			}
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case result := <-resultsCh:
				if err := w.httpSendResult(ctx, cli, srv, result); err != nil {
					log.Errorf("%s: could not send result: %v", w, err)
				}
			}
		}
	}()
}

func (w *Worker) httpPollScan(ctx context.Context, cli *http.Client, srv url.URL) (*types.ScanTask, error) {
	u := srv
	u.Path = "/scan"
	var scan types.ScanTask
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("%s: not create request: %v", w, err)
	}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: not get task: %v", w, err) // XXX: retry
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&scan); err != nil {
		return nil, fmt.Errorf("%s: could not decode scan task: %v", w, err) // XXX: retry
	}
	return &scan, nil
}

func (w *Worker) httpSignalFinishedScan(ctx context.Context, cli *http.Client, srv url.URL, scan *types.ScanTask) error {
	u := srv
	u.Path = "/scan"
	body, err := json.Marshal(scan)
	if err != nil {
		return fmt.Errorf("could not encode result: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("could not create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("not send result: %w", err)
	}
	resp.Body.Close()
	return nil
}

func (w *Worker) httpSendResult(ctx context.Context, cli *http.Client, srv url.URL, result types.ScanResult) error {
	u := srv
	u.Path = "/result"
	body, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("%s: could not encode result: %v", w, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%s: could not create request: %v", w, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := cli.Do(req)
	if err != nil {
		return fmt.Errorf("%s: not send result: %v", w, err)
	}
	resp.Body.Close()
	return nil
}

func (w *Worker) triggerScan(ctx context.Context, statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, scan *types.ScanTask, resultsCh chan<- types.ScanResult) (err error) {
	if err := w.Statsd.Count("datadog.agentless_scanner.scans.started", 1.0, scan.Tags(), 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
	defer func() {
		if err != nil {
			if err := w.Statsd.Count("datadog.agentless_scanner.scans.finished", 1.0, scan.TagsFailure(err), 1.0); err != nil {
				log.Warnf("failed to send metric: %v", err)
			}
		}
	}()

	if err := os.MkdirAll(scan.Path(), 0700); err != nil {
		return err
	}

	pool := newScannersPool(w.NoFork, w.ScannersMax)
	scan.StartedAt = time.Now()
	defer w.cleanupScan(statsd, sc, scan)
	switch scan.Type {
	case types.TaskTypeHost:
		w.scanRootFilesystems(ctx, statsd, sc, scan, []string{scan.TargetID.ResourceName()}, pool, resultsCh)

	case types.TaskTypeAMI:
		snapshotID, err := awsbackend.SetupEBSSnapshot(ctx, statsd, sc, scan, &w.waiter)
		if err != nil {
			return err
		}
		switch scan.DiskMode {
		case types.DiskModeNoAttach:
			w.scanSnaphotNoAttach(ctx, statsd, sc, scan, snapshotID, pool, resultsCh)
		case types.DiskModeNBDAttach, types.DiskModeVolumeAttach:
			err = awsbackend.SetupEBSVolume(ctx, statsd, sc, scan, &w.waiter, snapshotID)
			if err != nil {
				return err
			}
			assert(scan.AttachedDeviceName != nil)
			partitions, err := devices.ListPartitions(ctx, scan, *scan.AttachedDeviceName)
			if err != nil {
				return err
			}
			mountpoints, err := devices.Mount(ctx, scan, partitions)
			if err != nil {
				return err
			}
			w.scanImage(ctx, statsd, sc, scan, mountpoints, pool, resultsCh)
		}

	case types.TaskTypeEBS:
		snapshotID, err := awsbackend.SetupEBSSnapshot(ctx, statsd, sc, scan, &w.waiter)
		if err != nil {
			return err
		}
		switch scan.DiskMode {
		case types.DiskModeNoAttach:
			w.scanSnaphotNoAttach(ctx, statsd, sc, scan, snapshotID, pool, resultsCh)
		case types.DiskModeNBDAttach, types.DiskModeVolumeAttach:
			err := awsbackend.SetupEBSVolume(ctx, statsd, sc, scan, &w.waiter, snapshotID)
			if err != nil {
				return err
			}
			assert(scan.AttachedDeviceName != nil)
			partitions, err := devices.ListPartitions(ctx, scan, *scan.AttachedDeviceName)
			if err != nil {
				return err
			}
			mountpoints, err := devices.Mount(ctx, scan, partitions)
			if err != nil {
				return err
			}
			w.scanRootFilesystems(ctx, statsd, sc, scan, mountpoints, pool, resultsCh)
		}

	case types.TaskTypeLambda:
		mountpoint, err := awsbackend.SetupLambda(ctx, statsd, sc, scan)
		if err != nil {
			return err
		}
		w.scanLambda(ctx, statsd, sc, scan, mountpoint, pool, resultsCh)

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

func (w *Worker) cleanupScan(statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, scan *types.ScanTask) {
	ctx, cancel := context.WithTimeout(context.Background(), cleanupMaxDuration)
	defer cancel()

	CleanupScanDir(ctx, scan)

	switch scan.Type {
	case types.TaskTypeEBS, types.TaskTypeAMI:
		for resourceID, createdAt := range scan.CreatedResources {
			if err := awsbackend.CleanupScanEBS(ctx, statsd, sc, scan, resourceID); err != nil {
				log.Warnf("%s: failed to cleanup EBS resource %q: %v", scan, resourceID, err)
			} else {
				w.statsResourceTTL(resourceID.ResourceType(), scan, createdAt)
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

func (w *Worker) scanImage(ctx context.Context, statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, scan *types.ScanTask, roots []string, pool *scannersPool, resultsCh chan<- types.ScanResult) {
	assert(scan.Type == types.TaskTypeAMI)

	var wg sync.WaitGroup
	for _, root := range roots {
		wg.Add(1)
		go func(root string) {
			defer wg.Done()
			resultsCh <- pool.launchScannerVulns(ctx,
				statsd,
				sc,
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

func (w *Worker) scanSnaphotNoAttach(ctx context.Context, statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, scan *types.ScanTask, snapshotID types.CloudID, pool *scannersPool, resultsCh chan<- types.ScanResult) {
	assert(scan.Type == types.TaskTypeEBS)
	resultsCh <- pool.launchScannerVulns(ctx,
		statsd,
		sc,
		sbommodel.SBOMSourceType_HOST_FILE_SYSTEM,
		scan.TargetName,
		scan.TargetTags,
		types.ScannerOptions{
			Action:     types.ScanActionVulnsHostOSVm,
			Scan:       scan,
			CreatedAt:  time.Now(),
			SnapshotID: &snapshotID,
		})
}

func (w *Worker) scanRootFilesystems(ctx context.Context, statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, scan *types.ScanTask, roots []string, pool *scannersPool, resultsCh chan<- types.ScanResult) {
	assert(scan.Type == types.TaskTypeHost || scan.Type == types.TaskTypeEBS)

	var wg sync.WaitGroup

	scanHost := func(root string, actions []types.ScanAction) {
		defer wg.Done()

		for _, action := range actions {
			assert(action == types.ScanActionVulnsHostOS || action == types.ScanActionMalware)
			switch action {
			case types.ScanActionVulnsHostOS:
				resultsCh <- pool.launchScannerVulns(ctx,
					statsd,
					sc,
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
				resultsCh <- pool.launchScanner(ctx, statsd, sc, types.ScannerOptions{
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

		ctrPrepareResult := pool.launchScanner(ctx, statsd, sc, types.ScannerOptions{
			Action:    types.ScanActionContainersInspect,
			Scan:      scan,
			Root:      root,
			CreatedAt: time.Now(),
		})
		if ctrPrepareResult.Err != nil {
			resultsCh <- ctrPrepareResult
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
			if err := w.Statsd.Count("datadog.agentless_scanner.containers.count", count, tags, 1.0); err != nil {
				log.Warnf("failed to send metric: %v", err)
			}
		}
		ctrDoneCh := make(chan *types.Container)
		for _, ctr := range containers {
			go func(ctr *types.Container, actions []types.ScanAction) {
				w.scanContainer(ctx, statsd, sc, scan, ctr, actions, pool, resultsCh)
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

	if err := w.Statsd.Histogram("datadog.agentless_scanner.scans.duration", float64(time.Since(scan.StartedAt).Milliseconds()), scan.Tags(), 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
}

func (w *Worker) scanContainer(ctx context.Context, statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, scan *types.ScanTask, ctr *types.Container, actions []types.ScanAction, pool *scannersPool, resultsCh chan<- types.ScanResult) {
	imageRefTagged, imageRefCanonical, imageTags := scanners.ContainerRefs(*ctr)
	for _, action := range actions {
		assert(action == types.ScanActionVulnsContainersApp || action == types.ScanActionVulnsContainersOS)
		sbomID := imageRefCanonical.String()
		result := pool.launchScannerVulns(ctx,
			statsd,
			sc,
			sbommodel.SBOMSourceType_CONTAINER_IMAGE_LAYERS,
			sbomID,
			imageTags,
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
		resultsCh <- result
	}
}

func (w *Worker) scanLambda(ctx context.Context, statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, scan *types.ScanTask, root string, pool *scannersPool, resultsCh chan<- types.ScanResult) {
	assert(scan.Type == types.TaskTypeLambda)

	serviceName := scan.TargetID.ResourceName()
	for _, tag := range scan.TargetTags {
		if strings.HasPrefix(tag, "service:") {
			serviceName = strings.TrimPrefix(tag, "service:")
			break
		}
	}

	tags := append([]string{
		fmt.Sprintf("runtime_id:%s", scan.TargetID.AsText()),
		fmt.Sprintf("service_version:%s", scan.TargetName),
	}, scan.TargetTags...)

	resultsCh <- pool.launchScannerVulns(ctx,
		statsd,
		sc,
		sbommodel.SBOMSourceType_CI_PIPELINE, // TODO: sbommodel.SBOMSourceType_LAMBDA
		serviceName,
		tags,
		types.ScannerOptions{
			Action:    types.ScanActionAppVulns,
			Scan:      scan,
			Root:      root,
			CreatedAt: time.Now(),
		})
	if err := w.Statsd.Histogram("datadog.agentless_scanner.scans.duration", float64(time.Since(scan.StartedAt).Milliseconds()), scan.Tags(), 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
}

func (w *Worker) statsResourceTTL(resourceType types.ResourceType, scan *types.ScanTask, createTime time.Time) {
	ttl := time.Since(createTime)
	tags := scan.Tags(fmt.Sprintf("aws_resource_type:%s", string(resourceType)))
	if err := w.Statsd.Histogram("datadog.agentless_scanner.aws.resources_ttl", float64(ttl.Milliseconds()), tags, 1.0); err != nil {
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

func (p *scannersPool) launchScannerVulns(ctx context.Context, statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, sourceType sbommodel.SBOMSourceType, sbomID string, tags []string, opts types.ScannerOptions) types.ScanResult {
	result := p.launchScanner(ctx, statsd, sc, opts)
	if result.Vulns != nil {
		result.Vulns.SourceType = sourceType
		result.Vulns.ID = sbomID
		result.Vulns.Tags = tags
	}
	return result
}

func (p *scannersPool) launchScanner(ctx context.Context, statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, opts types.ScannerOptions) types.ScanResult {
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
			result = LaunchScannerInSameProcess(ctx, statsd, sc, opts)
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
func LaunchScannerInSameProcess(ctx context.Context, statsd ddogstatsd.ClientInterface, sc *types.ScannerConfig, opts types.ScannerOptions) types.ScanResult {
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
		bom, err := scanners.LaunchTrivyHostVM(ctx, statsd, sc, opts)
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
