// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && trivy

// Package sbom holds sbom related files
package sbom

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/avast/retry-go/v4"
	"github.com/hashicorp/golang-lru/v2/simplelru"
	"github.com/skydive-project/go-debouncer"
	"go.uber.org/atomic"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors/host"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/trivy"
)

const (
	// state of the sboms
	pendingState int64 = iota + 1
	computedState
	stoppedState

	maxSBOMGenerationRetries = 3
	maxSBOMEntries           = 1024
	scanQueueSize            = 100
)

var errNoProcessForContainerID = errors.New("found no running process matching the given container ID")

// Data use the keep the result of a scan of a same workload across multiple
// container
type Data struct {
	files fileQuerier
}

// SBOM defines an SBOM
type SBOM struct {
	sync.RWMutex

	ContainerID containerutils.ContainerID

	data *Data

	workloadKey workloadKey

	cgroup *cgroupModel.CacheEntry
	state  *atomic.Int64

	refresher *debouncer.Debouncer
}

type workloadKey string

func getWorkloadKey(selector *cgroupModel.WorkloadSelector) workloadKey {
	return workloadKey(selector.Image + ":" + selector.Tag)
}

// IsComputed returns true if SBOM was successfully generated
func (s *SBOM) IsComputed() bool {
	return s.state.Load() == computedState
}

// SetReport sets the SBOM report
func (s *SBOM) setReport(report *trivy.Report) {
	// build file cache
	s.data.files = newFileQuerier(report)
}

func (s *SBOM) stop() {
	if s.refresher != nil {
		s.refresher.Stop()

		// don't forget to set the refresher to nil otherwise it generates a memleak
		s.refresher = nil
	}

	// change the state so that already queued sbom won't be handled
	s.state.Store(stoppedState)
}

// NewSBOM returns a new empty instance of SBOM
func NewSBOM(id containerutils.ContainerID, cgroup *cgroupModel.CacheEntry, workloadKey workloadKey) *SBOM {
	return &SBOM{
		ContainerID: id,
		workloadKey: workloadKey,
		state:       atomic.NewInt64(pendingState),
		cgroup:      cgroup,
		data:        &Data{},
	}
}

// Resolver is the Software Bill-Of-material resolver
type Resolver struct {
	cfg *config.RuntimeSecurityConfig

	sbomsLock sync.RWMutex
	sboms     *simplelru.LRU[containerutils.ContainerID, *SBOM]

	// cache
	dataCacheLock sync.RWMutex
	dataCache     *simplelru.LRU[workloadKey, *Data] // cache per workload key

	// queue
	scanChan        chan *SBOM
	pendingScanLock sync.Mutex
	pendingScan     []containerutils.ContainerID

	statsdClient   statsd.ClientInterface
	sbomCollector  *host.Collector
	hostRootDevice uint64
	hostSBOM       *SBOM

	sbomGenerations       *atomic.Uint64
	failedSBOMGenerations *atomic.Uint64
	sbomsCacheHit         *atomic.Uint64
	sbomsCacheMiss        *atomic.Uint64
}

// NewSBOMResolver returns a new instance of Resolver
func NewSBOMResolver(c *config.RuntimeSecurityConfig, statsdClient statsd.ClientInterface) (*Resolver, error) {
	opts := sbom.ScanOptions{
		Analyzers: c.SBOMResolverAnalyzers,
	}
	sbomCollector, err := host.NewCollectorForCWS(pkgconfigsetup.SystemProbe(), opts)
	if err != nil {
		return nil, err
	}

	dataCache, err := simplelru.NewLRU[workloadKey, *Data](c.SBOMResolverWorkloadsCacheSize, nil)
	if err != nil {
		return nil, fmt.Errorf("couldn't create new SBOMResolver: %w", err)
	}

	hostProcRootPath := utils.ProcRootPath(1)
	stat, err := utils.UnixStat(hostProcRootPath)
	if err != nil {
		return nil, fmt.Errorf("stat failed for `%s`: couldn't stat host proc root path: %w", hostProcRootPath, err)
	}

	resolver := &Resolver{
		cfg:                   c,
		statsdClient:          statsdClient,
		dataCache:             dataCache,
		scanChan:              make(chan *SBOM, 100),
		sbomCollector:         sbomCollector,
		hostRootDevice:        stat.Dev,
		sbomGenerations:       atomic.NewUint64(0),
		sbomsCacheHit:         atomic.NewUint64(0),
		sbomsCacheMiss:        atomic.NewUint64(0),
		failedSBOMGenerations: atomic.NewUint64(0),
	}

	sboms, err := simplelru.NewLRU[containerutils.ContainerID, *SBOM](maxSBOMEntries, func(_ containerutils.ContainerID, sbom *SBOM) {
		// should be trigger from a function already locking the sbom, see Add, Delete
		sbom.stop()
		resolver.removePendingScan(sbom.ContainerID)
	})
	if err != nil {
		return nil, fmt.Errorf("couldn't create new SBOM resolver: %w", err)
	}
	resolver.sboms = sboms

	if !c.SBOMResolverEnabled {
		return resolver, nil
	}

	return resolver, nil
}

// Start starts the goroutine of the SBOM resolver
func (r *Resolver) Start(ctx context.Context) error {
	if r.cfg.SBOMResolverHostEnabled {
		hostRoot := os.Getenv("HOST_ROOT")
		if hostRoot == "" {
			hostRoot = "/"
		}

		r.hostSBOM = NewSBOM("", nil, "")

		report, err := r.generateSBOM(hostRoot)
		if err != nil {
			return err
		}
		r.hostSBOM.setReport(report)
		r.hostSBOM.state.Store(computedState)
	}

	go func() {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		for {
			select {
			case <-ctx.Done():
				return
			case sbom := <-r.scanChan:
				if err := retry.Do(func() error {
					return r.analyzeWorkload(sbom)
				}, retry.Attempts(maxSBOMGenerationRetries), retry.Delay(200*time.Millisecond)); err != nil {
					if errors.Is(err, errNoProcessForContainerID) {
						seclog.Debugf("Couldn't generate SBOM for '%s': %v", sbom.ContainerID, err)
					} else {
						seclog.Warnf("Failed to generate SBOM for '%s': %v", sbom.ContainerID, err)
					}
				}
			}
		}
	}()

	return nil
}

// RefreshSBOM regenerates a SBOM for a container
func (r *Resolver) RefreshSBOM(containerID containerutils.ContainerID) error {
	if sbom := r.getSBOM(containerID); sbom != nil {
		seclog.Debugf("Refreshing SBOM for container %s", containerID)

		var refresher *debouncer.Debouncer

		// create a refresher debouncer on demand
		sbom.Lock()
		refresher = sbom.refresher
		if refresher == nil {
			refresher = debouncer.New(
				3*time.Second, func() {
					// invalid cache data
					r.removeSBOMData(sbom.workloadKey)

					sbom.Lock()
					r.triggerScan(sbom)
					sbom.Unlock()
				},
			)
			refresher.Start()
			sbom.refresher = refresher
		}
		sbom.Unlock()

		refresher.Call()

		return nil
	}
	return fmt.Errorf("container %s not found", containerID)
}

// generateSBOM calls Trivy to generate the SBOM of a sbom
func (r *Resolver) generateSBOM(root string) (*trivy.Report, error) {
	seclog.Infof("Generating SBOM for %s", root)
	r.sbomGenerations.Inc()

	report, err := r.sbomCollector.DirectScan(context.Background(), root)
	if err != nil {
		r.failedSBOMGenerations.Inc()
		return nil, fmt.Errorf("failed to generate SBOM for %s: %w", root, err)
	}

	seclog.Infof("SBOM successfully generated from %s", root)

	trivyReport, ok := report.(*trivy.Report)
	if !ok {
		return nil, fmt.Errorf("failed to convert report for %s", root)
	}

	return trivyReport, nil
}

func (r *Resolver) doScan(sbom *SBOM) (*trivy.Report, error) {
	var (
		lastErr error
		scanned bool
		report  *trivy.Report
	)

	cfs := utils.DefaultCGroupFS()

	for _, rootCandidatePID := range sbom.cgroup.GetPIDs() {
		// check if this pid still exists and is in the expected container ID (if we loose an exit and need to wait for
		// the flush to remove a pid, there might be a significant delay before a PID is removed from this list. Checking
		// the container ID reduces drastically the likelihood of this race)
		computedID, _, _, err := cfs.FindCGroupContext(rootCandidatePID, rootCandidatePID)
		if err != nil {
			continue
		}
		if computedID != sbom.ContainerID {
			continue
		}

		containerProcRootPath := utils.ProcRootPath(rootCandidatePID)
		if sbom.ContainerID != "" {
			stat, err := utils.UnixStat(containerProcRootPath)
			if err != nil {
				return nil, fmt.Errorf("stat failed for `%s`: couldn't stat container proc root path: %w", containerProcRootPath, err)
			}
			if stat.Dev == r.hostRootDevice {
				return nil, fmt.Errorf("couldn't generate sbom: filesystem of container '%s' matches the host root filesystem", sbom.ContainerID)
			}
		}

		if report, lastErr = r.generateSBOM(containerProcRootPath); lastErr == nil {
			sbom.setReport(report)
			scanned = true
			break
		}

		seclog.Errorf("couldn't generate SBOM: %v", lastErr)
	}
	if lastErr != nil {
		return nil, lastErr
	}
	if !scanned {
		return nil, errNoProcessForContainerID
	}
	return report, nil
}

func (r *Resolver) removeSBOMData(key workloadKey) {
	r.dataCacheLock.Lock()
	r.dataCache.Remove(key)
	r.dataCacheLock.Unlock()
}

func (r *Resolver) addPendingScan(containerID containerutils.ContainerID) bool {
	r.pendingScanLock.Lock()
	defer r.pendingScanLock.Unlock()

	if len(r.pendingScan) >= scanQueueSize {
		return false
	}

	if slices.Contains(r.pendingScan, containerID) {
		return false
	}
	r.pendingScan = append(r.pendingScan, containerID)

	return true
}

func (r *Resolver) removePendingScan(containerID containerutils.ContainerID) {
	r.pendingScanLock.Lock()
	defer r.pendingScanLock.Unlock()

	r.pendingScan = slices.DeleteFunc(r.pendingScan, func(v containerutils.ContainerID) bool {
		return v == containerID
	})
}

// analyzeWorkload generates the SBOM of the provided sbom and send it to the security agent
func (r *Resolver) analyzeWorkload(sbom *SBOM) error {
	sbom.Lock()
	defer sbom.Unlock()

	seclog.Infof("analyzing sbom '%s'", sbom.ContainerID)

	if sbom.state.Load() != pendingState {
		r.removePendingScan(sbom.ContainerID)

		// should not append, ignore
		seclog.Warnf("trying to analyze a sbom not in pending state for '%s': %d", sbom.ContainerID, sbom.state.Load())
		return nil
	}

	// bail out if the workload has been analyzed while queued up
	r.dataCacheLock.RLock()
	if data, exists := r.dataCache.Get(sbom.workloadKey); exists {
		r.dataCacheLock.RUnlock()
		sbom.data = data

		r.removePendingScan(sbom.ContainerID)

		return nil
	}
	r.dataCacheLock.RUnlock()

	report, err := r.doScan(sbom)
	if err != nil {
		return err
	}

	data := &Data{
		files: newFileQuerier(report),
	}
	sbom.data = data

	// mark the SBOM as successful
	sbom.state.Store(computedState)

	// add to cache
	r.dataCacheLock.Lock()
	r.dataCache.Add(sbom.workloadKey, data)
	r.dataCacheLock.Unlock()

	r.removePendingScan(sbom.ContainerID)

	seclog.Infof("new sbom generated for '%s': %d files added", sbom.ContainerID, data.files.len())
	return nil
}

func (r *Resolver) getSBOM(containerID containerutils.ContainerID) *SBOM {
	r.sbomsLock.RLock()
	defer r.sbomsLock.RUnlock()

	sbom := r.hostSBOM
	if containerID != "" {
		sbom, _ = r.sboms.Get(containerID)
	}
	return sbom
}

// ResolvePackage returns the Package that owns the provided file. Make sure the internal fields of "file" are properly
// resolved.
func (r *Resolver) ResolvePackage(containerID containerutils.ContainerID, file *model.FileEvent) *Package {
	sbom := r.getSBOM(containerID)
	if sbom == nil {
		return nil
	}

	sbom.Lock()
	defer sbom.Unlock()

	return sbom.data.files.queryFile(file.PathnameStr)
}

// newSBOM (thread unsafe) creates a new SBOM entry for the sbom designated by the provided process cache
// entry
func (r *Resolver) newSBOM(id containerutils.ContainerID, cgroup *cgroupModel.CacheEntry, workloadKey workloadKey) *SBOM {
	sbom := NewSBOM(id, cgroup, workloadKey)
	r.sboms.Add(id, sbom)
	return sbom
}

// queueWorkload inserts the provided sbom in a SBOM resolver chan, it will be inserted in the scanChan or the
// delayerChan depending on the tags that have been resolved
func (r *Resolver) queueWorkload(sbom *SBOM) {
	sbom.Lock()
	defer sbom.Unlock()

	if sbom.state.Load() != pendingState {
		// this sbom was deleted before we could scan it, ignore it
		return
	}

	// check if this sbom has been scanned before
	r.dataCacheLock.Lock()
	defer r.dataCacheLock.Unlock()

	if data, ok := r.dataCache.Get(sbom.workloadKey); ok {
		sbom.data = data

		sbom.state.Store(computedState)

		r.sbomsCacheHit.Inc()
		return
	}
	r.sbomsCacheMiss.Inc()

	r.triggerScan(sbom)
}

func (r *Resolver) triggerScan(sbom *SBOM) {
	if !r.addPendingScan(sbom.ContainerID) {
		r.deleteSBOM(sbom)
		return
	}

	// push sbom to the scanner chan
	select {
	case r.scanChan <- sbom:
	default:
		r.removePendingScan(sbom.ContainerID)
		r.deleteSBOM(sbom)
	}
}

// OnWorkloadSelectorResolvedEvent is used to handle the creation of a new cgroup with its resolved tags
func (r *Resolver) OnWorkloadSelectorResolvedEvent(workload *tags.Workload) {
	r.sbomsLock.Lock()
	defer r.sbomsLock.Unlock()

	if workload == nil {
		return
	}

	id := workload.ContainerID
	// We don't scan hosts for now
	if len(id) == 0 {
		return
	}

	_, ok := r.sboms.Get(id)
	if !ok {
		workloadKey := getWorkloadKey(workload.Selector.Copy())
		sbom := r.newSBOM(id, workload.CacheEntry, workloadKey)
		r.queueWorkload(sbom)
	}
}

// GetWorkload returns the sbom of a provided ID
func (r *Resolver) GetWorkload(id containerutils.ContainerID) *SBOM {
	r.sbomsLock.RLock()
	defer r.sbomsLock.RUnlock()

	if id == "" {
		return r.hostSBOM
	}

	sbom, _ := r.sboms.Get(id)
	return sbom
}

// OnCGroupDeletedEvent is used to handle a CGroupDeleted event
func (r *Resolver) OnCGroupDeletedEvent(cgroup *cgroupModel.CacheEntry) {
	if cgroup.ContainerID != "" {
		r.Delete(cgroup.ContainerID)
	}
}

// Delete removes the SBOM of the provided cgroup id
func (r *Resolver) Delete(id containerutils.ContainerID) {
	sbom := r.GetWorkload(id)
	if sbom == nil {
		return
	}
	sbom.Lock()
	defer sbom.Unlock()

	// Remove this SBOM
	r.deleteSBOM(sbom)
}

// deleteSBOM delete all data indexed by the provided container ID
func (r *Resolver) deleteSBOM(sbom *SBOM) {
	r.sbomsLock.Lock()
	defer r.sbomsLock.Unlock()

	seclog.Infof("deleting SBOM entry for '%s'", sbom.ContainerID)

	// should be called under sbom.Lock
	r.sboms.Remove(sbom.ContainerID)
}

// SendStats sends stats
func (r *Resolver) SendStats() error {
	r.sbomsLock.RLock()
	defer r.sbomsLock.RUnlock()
	if val := float64(r.sboms.Len()); val > 0 {
		if err := r.statsdClient.Gauge(metrics.MetricSBOMResolverActiveSBOMs, val, []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricSBOMResolverActiveSBOMs: %w", err)
		}
	}

	if val := r.sbomGenerations.Swap(0); val > 0 {
		if err := r.statsdClient.Count(metrics.MetricSBOMResolverSBOMGenerations, int64(val), []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricSBOMResolverSBOMGenerations: %w", err)
		}
	}

	r.dataCacheLock.Lock()
	defer r.dataCacheLock.Unlock()
	if val := float64(r.dataCache.Len()); val > 0 {
		if err := r.statsdClient.Gauge(metrics.MetricSBOMResolverSBOMCacheLen, val, []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricSBOMResolverSBOMCacheLen: %w", err)
		}
	}

	if val := int64(r.sbomsCacheHit.Swap(0)); val > 0 {
		if err := r.statsdClient.Count(metrics.MetricSBOMResolverSBOMCacheHit, val, []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricSBOMResolverSBOMCacheHit: %w", err)
		}
	}

	if val := int64(r.sbomsCacheMiss.Swap(0)); val > 0 {
		if err := r.statsdClient.Count(metrics.MetricSBOMResolverSBOMCacheMiss, val, []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricSBOMResolverSBOMCacheMiss: %w", err)
		}
	}

	if val := int64(r.failedSBOMGenerations.Swap(0)); val > 0 {
		if err := r.statsdClient.Count(metrics.MetricSBOMResolverFailedSBOMGenerations, val, []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricSBOMResolverFailedSBOMGenerations: %w", err)
		}
	}

	return nil
}
