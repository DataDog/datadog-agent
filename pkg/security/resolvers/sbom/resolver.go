// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && trivy
// +build linux,trivy

package sbom

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/aquasecurity/trivy/pkg/fanal/cache"
	"github.com/hashicorp/golang-lru/v2/simplelru"
	"go.uber.org/atomic"
	"k8s.io/utils/temp"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/trivy"
)

// SBOMSource defines is the default log source for the SBOM events
const SBOMSource = "runtime-security-agent"

type SBOM struct {
	sync.RWMutex

	report *trivy.TrivyReport
	files  map[string]*Package

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
		files:          make(map[string]*Package),
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
	config         *config.Config
	statsdClient   statsd.ClientInterface
	trivyScanner   trivy.Collector

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
func NewSBOMResolver(c *config.Config, tagsResolver *tags.Resolver, statsdClient statsd.ClientInterface) (*Resolver, error) {
	trivyConfiguration := trivy.DefaultCollectorConfig([]string{trivy.OSAnalyzers}, "")
	trivyConfiguration.CacheProvider = func() (cache.Cache, error) {
		tmpDir, err := temp.CreateTempDir("sbom-resolver")
		if err != nil {
			return nil, err
		}
		return cache.NewFSCache(tmpDir.Name)
	}
	trivyConfiguration.ClearCacheOnClose = true
	trivyConfiguration.ArtifactOption.Slow = false

	trivyScanner, err := trivy.NewCollector(trivyConfiguration)
	if err != nil {
		return nil, err
	}

	sbomsCache, err := simplelru.NewLRU[string, *SBOM](c.SBOMResolverWorkloadsCacheSize, nil)
	if err != nil {
		return nil, fmt.Errorf("couldn't create new SBOMResolver: %w", err)
	}

	resolver := &Resolver{
		config:                c,
		statsdClient:          statsdClient,
		sboms:                 make(map[string]*SBOM),
		sbomsCache:            sbomsCache,
		scannerChan:           make(chan *SBOM, 100),
		trivyScanner:          trivyScanner,
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
	for _, tag := range coreconfig.GetConfiguredTags(coreconfig.Datadog, true) {
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
	if !r.config.SBOMResolverEnabled {
		return
	}

	go func() {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		for {
			select {
			case <-ctx.Done():
				return
			case sbom := <-r.scannerChan:
				if err := r.analyzeWorkload(sbom); err != nil {
					seclog.Errorf("couldn't scan '%s': %v", sbom.ContainerID, err)
				}
			}
		}
	}()
}

// generateSBOM calls Trivy to generate the SBOM of a sbom
func (r *Resolver) generateSBOM(root string, sbom *SBOM) error {
	seclog.Infof("Generating SBOM for %s", root)
	r.sbomGenerations.Inc()

	report, err := r.trivyScanner.ScanFilesystem(context.Background(), root)
	if err != nil {
		r.failedSBOMGenerations.Inc()
		return fmt.Errorf("failed to generate SBOM for %s: %w", root, err)
	}

	seclog.Infof("SBOM successfully generated from %s", root)

	trivyReport, ok := report.(*trivy.TrivyReport)
	if !ok {
		return fmt.Errorf("failed to convert report for %s: %w", root, err)
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

	var lastErr error
	var scanned bool
	for _, rootCandidatePID := range sbom.cgroup.GetRootPIDs() {
		// check if this pid still exists and is in the expected container ID (if we loose an exit and need to wait for
		// the flush to remove a pid, there might be a significant delay before a PID is removed from this list. Checking
		// the container ID reduces drastically the likelihood of this race)
		computedID, err := utils.GetProcContainerID(rootCandidatePID, rootCandidatePID)
		if err != nil {
			sbom.cgroup.RemoveRootPID(rootCandidatePID)
			continue
		}
		if string(computedID) != sbom.ContainerID {
			sbom.cgroup.RemoveRootPID(rootCandidatePID)
			continue
		}

		lastErr = r.generateSBOM(utils.ProcRootPath(int32(rootCandidatePID)), sbom)
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
	sbom.files = make(map[string]*Package)

	// build file cache
	for _, result := range sbom.report.Results {
		for _, resultPkg := range result.Packages {
			pkg := &Package{
				Name:    resultPkg.Name,
				Version: resultPkg.Version,
			}
			for _, file := range resultPkg.SystemInstalledFiles {
				seclog.Tracef("indexing %s as %+v", file, pkg)
				sbom.files[file] = pkg
			}
		}
	}

	// we can get rid of the report now that we've generate the file mapping
	sbom.report = nil

	// mark the SBOM ass successful
	sbom.scanSuccessful.Store(true)

	seclog.Infof("new sbom generated for '%s': %d files added", sbom.ContainerID, len(sbom.files))
	return nil
}

// RefreshSBOM analyzes the file system of a sbom to refresh its SBOM.
func (r *Resolver) RefreshSBOM(id string, cgroup *cgroupModel.CacheEntry) error {
	if !r.config.SBOMResolverEnabled {
		return nil
	}

	r.sbomsLock.Lock()
	defer r.sbomsLock.Unlock()
	sbom, ok := r.sboms[id]
	if !ok {
		var err error
		sbom, err = r.newWorkloadEntry(id, cgroup)
		if err != nil {
			return err
		}
	}

	// push sbom to the scanner chan
	select {
	case r.scannerChan <- sbom:
	default:
	}
	return nil
}

// ResolvePackage returns the Package that owns the provided file. Make sure the internal fields of "file" are properly
// resolved.
func (r *Resolver) ResolvePackage(containerID string, file *model.FileEvent) *Package {
	if !r.config.SBOMResolverEnabled {
		return nil
	}

	r.sbomsLock.RLock()
	defer r.sbomsLock.RUnlock()
	sbom, ok := r.sboms[containerID]
	if !ok {
		return nil
	}

	sbom.Lock()
	defer sbom.Unlock()

	pkg := sbom.files[file.PathnameStr]
	if pkg == nil && strings.HasPrefix(file.PathnameStr, "/usr") {
		pkg = sbom.files[file.PathnameStr[4:]]
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
	if !r.config.SBOMResolverEnabled {
		return
	}

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
	if !r.config.SBOMResolverEnabled {
		return
	}

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

// AddContextTags Adds the tags resolved by the resolver to the provided SBOM
func (r *Resolver) AddContextTags(s *SBOM) {
	var tagName string
	var found bool

	if s.cgroup == nil {
		return
	}

	dumpTagNames := make([]string, 0, len(s.cgroup.Tags))
	for _, tag := range s.cgroup.Tags {
		dumpTagNames = append(dumpTagNames, utils.GetTagName(tag))
	}

	for _, tag := range r.contextTags {
		tagName = utils.GetTagName(tag)
		found = false

		for _, dumpTagName := range dumpTagNames {
			if tagName == dumpTagName {
				found = true
				break
			}
		}

		if !found {
			s.cgroup.Tags = append(s.cgroup.Tags, tag)
		}
	}
}

// processWorkload resolves the tags of the provided SBOM and delete it when applicable
func (r *Resolver) processWorkload(sbom *SBOM, now time.Time) error {
	sbom.Lock()
	defer sbom.Unlock()

	// Start by resolving the tags: even if we end up deleting the SBOM, we need the tags to put the SBOM in cache
	// if this is the first time we send the SBOM, resolve the context tags
	r.AddContextTags(sbom)

	// resolve the service if it is defined
	if sbom.cgroup != nil {
		sbom.Service = utils.GetTagValue("service", sbom.cgroup.Tags)
	}

	// check if we should delete the sbom
	if sbom.deleted.Load() || (!sbom.deleted.Load() && len(sbom.cgroup.GetRootPIDs()) == 0) {
		r.deleteSBOM(sbom)
	}

	return nil
}

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
