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
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/avast/retry-go/v4"
	"github.com/hashicorp/golang-lru/v2/simplelru"
	"github.com/skydive-project/go-debouncer"
	"go.uber.org/atomic"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors/host"
	sbomscanner "github.com/DataDog/datadog-agent/pkg/sbom/scanner"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/DataDog/datadog-agent/pkg/util/trivy"
)

// SBOMSource defines is the default log source for the SBOM events
const SBOMSource = "runtime-security-agent"

const maxSBOMGenerationRetries = 3

// SBOM defines an SBOM
type SBOM struct {
	sync.RWMutex

	report *trivy.Report
	files  fileQuerier

	Host        string
	Source      string
	Service     string
	ContainerID string
	workloadKey string

	deleted        *atomic.Bool
	scanSuccessful *atomic.Bool
	cgroup         *cgroupModel.CacheEntry

	refresh *debouncer.Debouncer
}

func getWorkloadKey(selector *cgroupModel.WorkloadSelector) string {
	return selector.Image + ":" + selector.Tag
}

// IsComputed returns true if SBOM was successfully generated
func (s *SBOM) IsComputed() bool {
	return s.scanSuccessful.Load()
}

// SetReport sets the SBOM report
func (s *SBOM) SetReport(report *trivy.Report) {
	// build file cache
	s.files = newFileQuerier(report)
}

// reset (thread unsafe) cleans up internal fields before a SBOM is inserted in cache, the goal is to save space and delete references
// to structs that will be GCed
func (s *SBOM) reset() {
	s.Host = ""
	s.Source = ""
	s.Service = ""
	s.ContainerID = ""
	s.cgroup = nil
	s.deleted.Store(true)
	if s.refresh != nil {
		s.refresh.Stop()
		s.refresh = nil
	}
}

// NewSBOM returns a new empty instance of SBOM
func NewSBOM(host string, source string, id string, cgroup *cgroupModel.CacheEntry, workloadKey string) (*SBOM, error) {
	sbom := &SBOM{
		files:          fileQuerier{},
		Host:           host,
		Source:         source,
		ContainerID:    id,
		workloadKey:    workloadKey,
		deleted:        atomic.NewBool(false),
		scanSuccessful: atomic.NewBool(false),
		cgroup:         cgroup,
	}

	return sbom, nil
}

// Resolver is the Software Bill-Of-material resolver
type Resolver struct {
	cfg            *config.RuntimeSecurityConfig
	sbomsLock      sync.RWMutex
	sboms          map[string]*SBOM
	sbomsCacheLock sync.RWMutex
	sbomsCache     *simplelru.LRU[string, *SBOM]
	scannerChan    chan *SBOM
	statsdClient   statsd.ClientInterface
	sbomScanner    *sbomscanner.Scanner
	hostRootDevice uint64
	hostSBOM       *SBOM

	sbomGenerations       *atomic.Uint64
	failedSBOMGenerations *atomic.Uint64
	sbomsCacheHit         *atomic.Uint64
	sbomsCacheMiss        *atomic.Uint64

	// context tags and attributes
	hostname    string
	source      string
	contextTags []string
}

// NewSBOMResolver returns a new instance of Resolver
func NewSBOMResolver(c *config.RuntimeSecurityConfig, statsdClient statsd.ClientInterface) (*Resolver, error) {
	sbomScanner, err := sbomscanner.CreateGlobalScanner(pkgconfigsetup.SystemProbe(), optional.NewNoneOption[workloadmeta.Component]())
	if err != nil {
		return nil, err
	}
	if sbomScanner == nil {
		return nil, errors.New("sbom is disabled")
	}

	sbomsCache, err := simplelru.NewLRU[string, *SBOM](c.SBOMResolverWorkloadsCacheSize, nil)
	if err != nil {
		return nil, fmt.Errorf("couldn't create new SBOMResolver: %w", err)
	}

	hostProcRootPath := utils.ProcRootPath(1)
	fi, err := os.Stat(hostProcRootPath)
	if err != nil {
		return nil, fmt.Errorf("stat failed for `%s`: couldn't stat host proc root path: %w", hostProcRootPath, err)
	}
	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, fmt.Errorf("stat failed for `%s`: couldn't stat host proc root path", hostProcRootPath)
	}

	resolver := &Resolver{
		cfg:                   c,
		statsdClient:          statsdClient,
		sboms:                 make(map[string]*SBOM),
		sbomsCache:            sbomsCache,
		scannerChan:           make(chan *SBOM, 100),
		sbomScanner:           sbomScanner,
		hostRootDevice:        stat.Dev,
		sbomGenerations:       atomic.NewUint64(0),
		sbomsCacheHit:         atomic.NewUint64(0),
		sbomsCacheMiss:        atomic.NewUint64(0),
		failedSBOMGenerations: atomic.NewUint64(0),
	}

	if !c.SBOMResolverEnabled {
		return resolver, nil
	}

	resolver.prepareContextTags()
	return resolver, nil
}

func (r *Resolver) prepareContextTags() {
	// add hostname tag
	hostname, err := utils.GetHostname()
	if err != nil || hostname == "" {
		hostname = "unknown"
	}
	r.hostname = hostname
	r.contextTags = append(r.contextTags, fmt.Sprintf("host:%s", r.hostname))

	// merge tags from config
	for _, tag := range configUtils.GetConfiguredTags(pkgconfigsetup.Datadog(), true) {
		if strings.HasPrefix(tag, "host") {
			continue
		}
		r.contextTags = append(r.contextTags, tag)
	}

	// add source tag
	r.source = utils.GetTagValue("source", r.contextTags)
	if len(r.source) == 0 {
		r.source = SBOMSource
		r.contextTags = append(r.contextTags, fmt.Sprintf("source:%s", SBOMSource))
	}
}

// Start starts the goroutine of the SBOM resolver
func (r *Resolver) Start(ctx context.Context) error {
	r.sbomScanner.Start(ctx)

	if r.cfg.SBOMResolverHostEnabled {
		hostRoot := os.Getenv("HOST_ROOT")
		if hostRoot == "" {
			hostRoot = "/"
		}

		hostSBOM, err := NewSBOM(r.hostname, r.source, "", nil, "")
		if err != nil {
			return err
		}
		r.hostSBOM = hostSBOM

		report, err := r.generateSBOM(hostRoot)
		if err != nil {
			return err
		}
		r.hostSBOM.SetReport(report)
	}

	go func() {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		for {
			select {
			case <-ctx.Done():
				return
			case sbom := <-r.scannerChan:
				if err := retry.Do(func() error {
					return r.analyzeWorkload(sbom)
				}, retry.Attempts(maxSBOMGenerationRetries), retry.Delay(200*time.Millisecond)); err != nil {
					seclog.Errorf("%s", err.Error())
				}
			}
		}
	}()

	return nil
}

// RefreshSBOM regenerates a SBOM for a container
func (r *Resolver) RefreshSBOM(containerID string) error {
	if sbom := r.getSBOM(containerID); sbom != nil {
		seclog.Debugf("Refreshing SBOM for container %s", containerID)
		sbom.refresh.Call()
		return nil
	}
	return fmt.Errorf("container %s not found", containerID)
}

// generateSBOM calls Trivy to generate the SBOM of a sbom
func (r *Resolver) generateSBOM(root string) (report *trivy.Report, err error) {
	seclog.Infof("Generating SBOM for %s", root)
	r.sbomGenerations.Inc()

	scanRequest := host.NewScanRequest(root, os.DirFS("/"))
	ch := collectors.GetHostScanner().Channel()
	if ch == nil {
		return nil, fmt.Errorf("couldn't retrieve global host scanner result channel")
	}
	if err := r.sbomScanner.Scan(scanRequest); err != nil {
		r.failedSBOMGenerations.Inc()
		return nil, fmt.Errorf("failed to trigger SBOM generation for %s: %w", root, err)
	}

	result, more := <-ch
	if !more {
		return nil, fmt.Errorf("failed to generate SBOM for %s: result channel is closed", root)
	}

	if result.Error != nil {
		// TODO: add a retry mechanism for retryable errors
		return nil, fmt.Errorf("failed to generate SBOM for %s: %w", root, result.Error)
	}

	seclog.Infof("SBOM successfully generated from %s", root)

	trivyReport, ok := result.Report.(*trivy.Report)
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

	for _, rootCandidatePID := range sbom.cgroup.GetPIDs() {
		// check if this pid still exists and is in the expected container ID (if we loose an exit and need to wait for
		// the flush to remove a pid, there might be a significant delay before a PID is removed from this list. Checking
		// the container ID reduces drastically the likelihood of this race)
		computedID, err := utils.GetProcContainerID(rootCandidatePID, rootCandidatePID)
		if err != nil {
			sbom.cgroup.RemovePID(rootCandidatePID)
			continue
		}
		if string(computedID) != sbom.ContainerID {
			sbom.cgroup.RemovePID(rootCandidatePID)
			continue
		}

		containerProcRootPath := utils.ProcRootPath(rootCandidatePID)
		if sbom.ContainerID != "" {
			fi, err := os.Stat(containerProcRootPath)
			if err != nil {
				return nil, fmt.Errorf("stat failed for `%s`: couldn't stat container proc root path: %w", containerProcRootPath, err)
			}
			stat, ok := fi.Sys().(*syscall.Stat_t)
			if !ok {
				return nil, fmt.Errorf("stat failed for `%s`: couldn't stat container proc root path", containerProcRootPath)
			}
			if stat.Dev == r.hostRootDevice {
				return nil, fmt.Errorf("couldn't generate sbom: filesystem of container '%s' matches the host root filesystem", sbom.ContainerID)
			}
		}

		if report, lastErr = r.generateSBOM(containerProcRootPath); lastErr == nil {
			sbom.SetReport(report)
			scanned = true
			break
		}

		seclog.Errorf("couldn't generate SBOM: %v", lastErr)
	}
	if lastErr != nil {
		return nil, lastErr
	}
	if !scanned {
		return nil, fmt.Errorf("couldn't generate sbom: all root candidates failed")
	}
	return report, nil
}

func (r *Resolver) invalidateWorkflow(sbom *SBOM) {
	r.sbomsCacheLock.Lock()
	r.sbomsCache.Remove(sbom.workloadKey)
	r.sbomsCacheLock.Unlock()
}

// analyzeWorkload generates the SBOM of the provided sbom and send it to the security agent
func (r *Resolver) analyzeWorkload(sbom *SBOM) error {
	seclog.Infof("analyzing sbom '%s'", sbom.ContainerID)
	sbom.Lock()
	defer sbom.Unlock()

	if sbom.deleted.Load() {
		// this sbom has been deleted, ignore
		return nil
	}

	// bail out if the workload has been analyzed while queued up
	r.sbomsCacheLock.RLock()
	if r.sbomsCache.Contains(sbom.workloadKey) {
		r.sbomsCacheLock.RUnlock()
		return nil
	}
	r.sbomsCacheLock.RUnlock()

	report, err := r.doScan(sbom)
	if err != nil {
		return err
	}

	// build file cache
	sbom.files = newFileQuerier(report)

	// we can get rid of the report now that we've generate the file mapping
	sbom.report = nil

	// mark the SBOM ass successful
	sbom.scanSuccessful.Store(true)

	// add to cache
	r.sbomsCacheLock.Lock()
	r.sbomsCache.Add(sbom.workloadKey, sbom)
	r.sbomsCacheLock.Unlock()

	seclog.Infof("new sbom generated for '%s': %d files added", sbom.ContainerID, sbom.files.len())
	return nil
}

func (r *Resolver) getSBOM(containerID string) *SBOM {
	r.sbomsLock.RLock()
	defer r.sbomsLock.RUnlock()

	sbom := r.hostSBOM
	if containerID != "" {
		sbom = r.sboms[containerID]
	}
	return sbom
}

// ResolvePackage returns the Package that owns the provided file. Make sure the internal fields of "file" are properly
// resolved.
func (r *Resolver) ResolvePackage(containerID string, file *model.FileEvent) *Package {
	sbom := r.getSBOM(containerID)
	if sbom == nil {
		return nil
	}

	sbom.Lock()
	defer sbom.Unlock()

	return sbom.files.queryFile(file.PathnameStr)
}

// newWorkloadEntry (thread unsafe) creates a new SBOM entry for the sbom designated by the provided process cache
// entry
func (r *Resolver) newWorkloadEntry(id string, cgroup *cgroupModel.CacheEntry, workloadKey string) (*SBOM, error) {
	sbom, err := NewSBOM(r.hostname, r.source, id, cgroup, workloadKey)
	if err != nil {
		return nil, err
	}

	sbom.refresh = debouncer.New(
		3*time.Second, func() {
			r.invalidateWorkflow(sbom)
			r.triggerScan(sbom)
		},
	)
	r.sboms[id] = sbom
	sbom.refresh.Start()

	return sbom, nil
}

// queueWorkload inserts the provided sbom in a SBOM resolver chan, it will be inserted in the scannerChan or the
// delayerChan depending on the tags that have been resolved
func (r *Resolver) queueWorkload(sbom *SBOM) {
	sbom.Lock()
	defer sbom.Unlock()

	if sbom.deleted.Load() {
		// this sbom was deleted before we could scan it, ignore it
		return
	}

	// check if this sbom has been scanned before
	r.sbomsCacheLock.Lock()
	defer r.sbomsCacheLock.Unlock()

	cachedSBOM, ok := r.sbomsCache.Get(sbom.workloadKey)
	if ok {
		// copy report and file cache (keeping a reference is fine, we won't be modifying the content)
		sbom.files = cachedSBOM.files
		sbom.report = cachedSBOM.report
		r.sbomsCacheHit.Inc()
		return
	}
	r.sbomsCacheMiss.Inc()

	r.triggerScan(sbom)
}

func (r *Resolver) triggerScan(sbom *SBOM) {
	// push sbom to the scanner chan
	select {
	case r.scannerChan <- sbom:
	default:
	}
}

// OnWorkloadSelectorResolvedEvent is used to handle the creation of a new cgroup with its resolved tags
func (r *Resolver) OnWorkloadSelectorResolvedEvent(cgroup *cgroupModel.CacheEntry) {
	r.sbomsLock.Lock()
	defer r.sbomsLock.Unlock()

	if cgroup == nil {
		return
	}

	id := string(cgroup.ContainerID)
	// We don't scan hosts for now
	if len(id) == 0 {
		return
	}

	_, ok := r.sboms[id]
	if !ok {
		workloadKey := getWorkloadKey(cgroup.GetWorkloadSelectorCopy())
		sbom, err := r.newWorkloadEntry(id, cgroup, workloadKey)
		if err != nil {
			seclog.Errorf("couldn't create new SBOM entry for sbom '%s': %v", id, err)
		}
		r.queueWorkload(sbom)
	}
}

// GetWorkload returns the sbom of a provided ID
func (r *Resolver) GetWorkload(id string) *SBOM {
	r.sbomsLock.RLock()
	defer r.sbomsLock.RUnlock()

	if id == "" {
		return r.hostSBOM
	}

	return r.sboms[id]
}

// OnCGroupDeletedEvent is used to handle a CGroupDeleted event
func (r *Resolver) OnCGroupDeletedEvent(cgroup *cgroupModel.CacheEntry) {
	r.Delete(string(cgroup.CGroupID))
}

// Delete removes the SBOM of the provided cgroup id
func (r *Resolver) Delete(id string) {
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
	// remove SBOM entry
	delete(r.sboms, sbom.ContainerID)

	// check if the scan was successful
	if !sbom.scanSuccessful.Load() {
		// exit now, we don't want to cache a failed scan
		return
	}

	// save the sbom key before reset
	sbomKey := sbom.workloadKey

	// cleanup and insert SBOM in cache
	sbom.reset()

	// push the sbom to the cache
	r.sbomsCacheLock.Lock()
	defer r.sbomsCacheLock.Unlock()
	r.sbomsCache.Add(sbomKey, sbom)
}

// SendStats sends stats
func (r *Resolver) SendStats() error {
	r.sbomsLock.RLock()
	defer r.sbomsLock.RUnlock()
	if val := float64(len(r.sboms)); val > 0 {
		if err := r.statsdClient.Gauge(metrics.MetricSBOMResolverActiveSBOMs, val, []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricSBOMResolverActiveSBOMs: %w", err)
		}
	}

	if val := r.sbomGenerations.Swap(0); val > 0 {
		if err := r.statsdClient.Count(metrics.MetricSBOMResolverSBOMGenerations, int64(val), []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricSBOMResolverSBOMGenerations: %w", err)
		}
	}

	r.sbomsCacheLock.Lock()
	defer r.sbomsCacheLock.Unlock()
	if val := float64(r.sbomsCache.Len()); val > 0 {
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
