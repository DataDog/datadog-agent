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
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/avast/retry-go/v4"
	"github.com/hashicorp/golang-lru/v2/simplelru"
	"github.com/twmb/murmur3"
	"go.uber.org/atomic"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	sbompkg "github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/collectors/host"
	sbomscanner "github.com/DataDog/datadog-agent/pkg/sbom/scanner"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/trivy"
)

// SBOMSource defines is the default log source for the SBOM events
const SBOMSource = "runtime-security-agent"

const maxSBOMGenerationRetries = 3

// SBOM defines an SBOM
type SBOM struct {
	sync.RWMutex

	report *trivy.Report
	files  map[uint64]*Package

	Host        string
	Source      string
	Service     string
	ContainerID string

	deleted        *atomic.Bool
	scanSuccessful *atomic.Bool
	cgroup         *cgroupModel.CacheEntry
}

// getWorkloadKey (thread unsafe) returns a key to indentify the workload
func (s *SBOM) getWorkloadKey() string {
	if s.cgroup == nil {
		return ""
	}
	return s.cgroup.WorkloadSelector.Image + ":" + s.cgroup.WorkloadSelector.Tag
}

// IsComputed returns true if SBOM was successfully generated
func (s *SBOM) IsComputed() bool {
	return s.scanSuccessful.Load()
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
}

// NewSBOM returns a new empty instance of SBOM
func NewSBOM(host string, source string, id string, cgroup *cgroupModel.CacheEntry) (*SBOM, error) {
	return &SBOM{
		files:          make(map[uint64]*Package),
		Host:           host,
		Source:         source,
		ContainerID:    id,
		deleted:        atomic.NewBool(false),
		scanSuccessful: atomic.NewBool(false),
		cgroup:         cgroup,
	}, nil
}

// Resolver is the Software Bill-Of-material resolver
type Resolver struct {
	sbomsLock      sync.RWMutex
	sboms          map[string]*SBOM
	sbomsCacheLock sync.RWMutex
	sbomsCache     *simplelru.LRU[string, *SBOM]
	scannerChan    chan *SBOM
	statsdClient   statsd.ClientInterface
	sbomScanner    *sbomscanner.Scanner

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
	sbomScanner, err := sbomscanner.CreateGlobalScanner(coreconfig.SystemProbe)
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

	resolver := &Resolver{
		statsdClient:          statsdClient,
		sboms:                 make(map[string]*SBOM),
		sbomsCache:            sbomsCache,
		scannerChan:           make(chan *SBOM, 100),
		sbomScanner:           sbomScanner,
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
	for _, tag := range configUtils.GetConfiguredTags(coreconfig.Datadog, true) {
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
func (r *Resolver) Start(ctx context.Context) {
	r.sbomScanner.Start(ctx)

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
				}, retry.Attempts(maxSBOMGenerationRetries), retry.Delay(20*time.Millisecond)); err != nil {
					seclog.Errorf(err.Error())
				}
			}
		}
	}()
}

// generateSBOM calls Trivy to generate the SBOM of a sbom
func (r *Resolver) generateSBOM(root string, sbom *SBOM) error {
	seclog.Infof("Generating SBOM for %s", root)
	r.sbomGenerations.Inc()

	scanRequest := &host.ScanRequest{Path: root}
	ch := make(chan sbompkg.ScanResult, 1)
	if err := r.sbomScanner.Scan(scanRequest, sbompkg.ScanOptions{Analyzers: []string{trivy.OSAnalyzers}, Fast: true}, ch); err != nil {
		r.failedSBOMGenerations.Inc()
		return fmt.Errorf("failed to trigger SBOM generation for %s: %w", root, err)
	}

	result := <-ch

	if result.Error != nil {
		// TODO: add a retry mechanism for retryable errors
		return fmt.Errorf("failed to generate SBOM for %s: %w", root, result.Error)
	}

	seclog.Infof("SBOM successfully generated from %s", root)

	trivyReport, ok := result.Report.(*trivy.Report)
	if !ok {
		return fmt.Errorf("failed to convert report for %s", root)
	}
	sbom.report = trivyReport

	return nil
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
	sbomKey := sbom.getWorkloadKey()
	r.sbomsCacheLock.RLock()
	if r.sbomsCache.Contains(sbomKey) {
		r.sbomsCacheLock.RUnlock()
		return nil
	}
	r.sbomsCacheLock.RUnlock()

	var lastErr error
	var scanned bool
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

		lastErr = r.generateSBOM(utils.ProcRootPath(rootCandidatePID), sbom)
		if lastErr == nil {
			scanned = true
			break
		} else {
			seclog.Errorf("couldn't generate SBOM: %v", lastErr)
		}
	}
	if lastErr != nil {
		return lastErr
	}
	if !scanned {
		return fmt.Errorf("couldn't generate sbom: all root candidates failed")
	}

	// cleanup file cache
	sbom.files = make(map[uint64]*Package)

	// build file cache
	for _, result := range sbom.report.Results {
		for _, resultPkg := range result.Packages {
			pkg := &Package{
				Name:       resultPkg.Name,
				Version:    resultPkg.Version,
				SrcVersion: resultPkg.SrcVersion,
			}
			for _, file := range resultPkg.SystemInstalledFiles {
				seclog.Tracef("indexing %s as %+v", file, pkg)
				sbom.files[murmur3.StringSum64(file)] = pkg
			}
		}
	}

	// we can get rid of the report now that we've generate the file mapping
	sbom.report = nil

	// mark the SBOM ass successful
	sbom.scanSuccessful.Store(true)

	// add to cache
	r.sbomsCacheLock.Lock()
	r.sbomsCache.Add(sbomKey, sbom)
	r.sbomsCacheLock.Unlock()

	seclog.Infof("new sbom generated for '%s': %d files added", sbom.ContainerID, len(sbom.files))
	return nil
}

// ResolvePackage returns the Package that owns the provided file. Make sure the internal fields of "file" are properly
// resolved.
func (r *Resolver) ResolvePackage(containerID string, file *model.FileEvent) *Package {
	r.sbomsLock.RLock()
	defer r.sbomsLock.RUnlock()
	sbom, ok := r.sboms[containerID]
	if !ok {
		return nil
	}

	sbom.Lock()
	defer sbom.Unlock()

	pkg := sbom.files[murmur3.StringSum64(file.PathnameStr)]
	if pkg == nil && strings.HasPrefix(file.PathnameStr, "/usr") {
		pkg = sbom.files[murmur3.StringSum64(file.PathnameStr[4:])]
	}

	return pkg
}

// newWorkloadEntry (thread unsafe) creates a new SBOM entry for the sbom designated by the provided process cache
// entry
func (r *Resolver) newWorkloadEntry(id string, cgroup *cgroupModel.CacheEntry) (*SBOM, error) {
	sbom, err := NewSBOM(r.hostname, r.source, id, cgroup)
	if err != nil {
		return nil, err
	}
	r.sboms[id] = sbom
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

	if workloadKey := sbom.getWorkloadKey(); workloadKey != "" {
		cachedSBOM, ok := r.sbomsCache.Get(workloadKey)
		if ok {
			// copy report and file cache (keeping a reference is fine, we won't be modifying the content)
			sbom.files = cachedSBOM.files
			sbom.report = cachedSBOM.report
			r.sbomsCacheHit.Inc()
			return
		}
	}
	r.sbomsCacheMiss.Inc()

	// push sbom to the scanner chan
	select {
	case r.scannerChan <- sbom:
	default:
	}
}

// OnWorkloadSelectorResolvedEvent is used to handle the creation of a new cgroup with its resolved tags
func (r *Resolver) OnWorkloadSelectorResolvedEvent(sbom *cgroupModel.CacheEntry) {
	r.Retain(sbom.ID, sbom)
}

// Retain increments the reference counter of the SBOM of a sbom
func (r *Resolver) Retain(id string, cgroup *cgroupModel.CacheEntry) {
	r.sbomsLock.Lock()
	defer r.sbomsLock.Unlock()

	// We don't scan hosts for now
	if len(id) == 0 {
		return
	}

	_, ok := r.sboms[id]
	if !ok {
		sbom, err := r.newWorkloadEntry(id, cgroup)
		if err != nil {
			seclog.Errorf("couldn't create new SBOM entry for sbom '%s': %v", id, err)
		}
		r.queueWorkload(sbom)
	}
	return
}

// GetWorkload returns the sbom of a provided ID
func (r *Resolver) GetWorkload(id string) *SBOM {
	r.sbomsLock.RLock()
	defer r.sbomsLock.RUnlock()

	return r.sboms[id]
}

// OnCGroupDeletedEvent is used to handle a CGroupDeleted event
func (r *Resolver) OnCGroupDeletedEvent(sbom *cgroupModel.CacheEntry) {
	r.Delete(sbom.ID)
}

// Delete removes the SBOM of the provided cgroup
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

	// compute sbom key before reset
	sbomKey := sbom.getWorkloadKey()

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
