// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package sbom

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	trivyReport "github.com/aquasecurity/trivy/pkg/types"
	"github.com/hashicorp/golang-lru/v2/simplelru"
	"go.uber.org/atomic"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// SBOMSource defines is the default log source for the SBOM events
const SBOMSource = "runtime-security-agent"

type Package struct {
	Name    string
	Version string
}

type SBOM struct {
	sync.RWMutex

	report *trivyReport.Report
	files  map[string]*Package

	Host        string
	Source      string
	Service     string
	Tags        []string
	ContainerID string

	deleted *atomic.Bool
	cgroup  *cgroupModel.CacheEntry
}

// getWorkloadKey (thread unsafe) returns a key to indentify the workload
func (s *SBOM) getWorkloadKey() string {
	return utils.GetTagValue("image_name", s.Tags) + ":" + utils.GetTagValue("image_tag", s.Tags)
}

// reset (thread unsafe) cleans up internal fields before a SBOM is inserted in cache, the goal is to save space and delete references
// to structs that will be GCed
func (s *SBOM) reset() {
	s.Host = ""
	s.Source = ""
	s.Service = ""
	s.Tags = nil
	s.ContainerID = ""
	s.cgroup = nil
	s.deleted.Store(true)
}

// NewSBOM returns a new empty instance of SBOM
func NewSBOM(host string, source string, id string, cgroup *cgroupModel.CacheEntry) (*SBOM, error) {
	return &SBOM{
		files:       make(map[string]*Package),
		Host:        host,
		Source:      source,
		ContainerID: id,
		deleted:     atomic.NewBool(false),
		cgroup:      cgroup,
	}, nil
}

// Resolver is the Software Bill-Of-material resolver
type Resolver struct {
	workloadsLock      sync.RWMutex
	workloads          map[string]*SBOM
	workloadsCacheLock sync.RWMutex
	workloadsCache     *simplelru.LRU[string, *SBOM]
	scannerChan        chan *SBOM
	delayedWorkloads   chan *SBOM
	config             *config.Config
	statsdClient       statsd.ClientInterface
	tagsResolver       *tags.Resolver
	trivyScanner       *TrivyCollector

	sbomGenerations       *atomic.Uint64
	failedSBOMGenerations *atomic.Uint64
	workloadsCacheHit     *atomic.Uint64
	workloadsCacheMiss    *atomic.Uint64

	// context tags and attributes
	hostname    string
	source      string
	contextTags []string
}

// NewSBOMResolver returns a new instance of Resolver
func NewSBOMResolver(c *config.Config, tagsResolver *tags.Resolver, statsdClient statsd.ClientInterface) (*Resolver, error) {
	trivyScanner, err := NewTrivyCollector()
	if err != nil {
		return nil, err
	}

	workloadsCache, err := simplelru.NewLRU[string, *SBOM](c.SBOMResolverWorkloadsCacheSize, nil)
	if err != nil {
		return nil, fmt.Errorf("couldn't create new SBOMResolver: %w", err)
	}

	resolver := &Resolver{
		config:                c,
		statsdClient:          statsdClient,
		tagsResolver:          tagsResolver,
		workloads:             make(map[string]*SBOM),
		workloadsCache:        workloadsCache,
		scannerChan:           make(chan *SBOM, 100),
		delayedWorkloads:      make(chan *SBOM, 100),
		trivyScanner:          trivyScanner,
		sbomGenerations:       atomic.NewUint64(0),
		workloadsCacheHit:     atomic.NewUint64(0),
		workloadsCacheMiss:    atomic.NewUint64(0),
		failedSBOMGenerations: atomic.NewUint64(0),
	}

	if !c.SBOMResolverEnabled {
		return resolver, nil
	}

	resolver.prepareContextTags()
	return resolver, nil
}

// resolveTags (thread unsafe) resolve the tags of a SBOM
func (r *Resolver) resolveTags(s *SBOM) error {
	if len(s.Tags) >= 10 || len(s.ContainerID) == 0 {
		return nil
	}

	newTags, err := r.tagsResolver.ResolveWithErr(s.ContainerID)
	if err != nil {
		return fmt.Errorf("failed to resolve %s: %w", s.ContainerID, err)
	}
	if len(newTags) > len(s.Tags) {
		s.Tags = newTags
	}
	return nil
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

		delayerTick := time.NewTicker(10 * time.Second)
		defer delayerTick.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case workload := <-r.scannerChan:
				if err := r.analyzeWorkload(workload); err != nil {
					seclog.Errorf("couldn't scan '%s': %v", workload.ContainerID, err)
				}
			case <-delayerTick.C:
				select {
				case workload := <-r.delayedWorkloads:
					r.queueWorkload(workload)
				default:
				}

			}
		}
	}()
}

// generateSBOM calls Trivy to generate the SBOM of a workload
func (r *Resolver) generateSBOM(root string, workload *SBOM) error {
	seclog.Infof("Generating SBOM for %s", root)
	r.sbomGenerations.Inc()

	report, err := r.trivyScanner.ScanRootfs(context.Background(), root)
	if err != nil {
		r.failedSBOMGenerations.Inc()
		return fmt.Errorf("failed to generate SBOM for %s: %w", root, err)
	}

	workload.report = report

	seclog.Infof("SBOM successfully generated from %s", root)

	return nil
}

// analyzeWorkload generates the SBOM of the provided workload and send it to the security agent
func (r *Resolver) analyzeWorkload(workload *SBOM) error {
	seclog.Infof("analyzing workload '%s'", workload.ContainerID)
	workload.Lock()
	defer workload.Unlock()

	if workload.deleted.Load() {
		// this workload has been deleted, ignore
		return nil
	}

	var lastErr error
	var scanned bool
	for _, rootCandidatePID := range workload.cgroup.GetRootPIDs() {
		// check if this pid still exists and is in the expected container ID (if we loose an exit and need to wait for
		// the flush to remove a pid, there might be a significant delay before a PID is removed from this list. Checking
		// the container ID reduces drastically the likelihood of this race)
		computedID, err := utils.GetProcContainerID(rootCandidatePID, rootCandidatePID)
		if err != nil {
			workload.cgroup.RemoveRootPID(rootCandidatePID)
			continue
		}
		if string(computedID) != workload.ContainerID {
			workload.cgroup.RemoveRootPID(rootCandidatePID)
			continue
		}

		lastErr = r.generateSBOM(utils.ProcRootPath(int32(rootCandidatePID)), workload)
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
		return fmt.Errorf("couldn't generate workload: all root candidates failed")
	}

	// cleanup file cache
	workload.files = make(map[string]*Package)

	// build file cache
	for _, result := range workload.report.Results {
		for _, resultPkg := range result.Packages {
			pkg := &Package{
				Name:    resultPkg.Name,
				Version: resultPkg.Version,
			}
			for _, file := range resultPkg.SystemInstalledFiles {
				seclog.Tracef("indexing %s as %+v", file, pkg)
				workload.files[file] = pkg
			}
		}
	}

	// we can get rid of the report now that we've generate the file mapping
	workload.report = nil

	seclog.Infof("new sbom generated for '%s': %d files added", workload.ContainerID, len(workload.files))
	return nil
}

// RefreshSBOM analyzes the file system of a workload to refresh its SBOM.
func (r *Resolver) RefreshSBOM(id string, cgroup *cgroupModel.CacheEntry) error {
	if !r.config.SBOMResolverEnabled {
		return nil
	}

	r.workloadsLock.Lock()
	defer r.workloadsLock.Unlock()
	workload, ok := r.workloads[id]
	if !ok {
		var err error
		workload, err = r.newWorkloadEntry(id, cgroup)
		if err != nil {
			return err
		}
	}

	// push workload to the scanner chan
	select {
	case r.scannerChan <- workload:
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

	r.workloadsLock.RLock()
	defer r.workloadsLock.RUnlock()
	workload, ok := r.workloads[containerID]
	if !ok {
		return nil
	}

	seclog.Tracef("resolving %s for container %s", file.PathnameStr, containerID)

	workload.Lock()
	defer workload.Unlock()

	seclog.Tracef("returning %v", workload.files[file.PathnameStr])
	return workload.files[file.PathnameStr]
}

// newWorkloadEntry (thread unsafe) creates a new SBOM entry for the workload designated by the provided process cache
// entry
func (r *Resolver) newWorkloadEntry(id string, cgroup *cgroupModel.CacheEntry) (*SBOM, error) {
	workload, err := NewSBOM(r.hostname, r.source, id, cgroup)
	if err != nil {
		return nil, err
	}
	r.workloads[id] = workload
	return workload, nil
}

// queueWorkload inserts the provided workload in a SBOM resolver chan, it will be inserted in the scannerChan or the
// delayerChan depending on the tags that have been resolved
func (r *Resolver) queueWorkload(workload *SBOM) {
	workload.Lock()
	defer workload.Unlock()

	if workload.deleted.Load() {
		// this workload was deleted before we could scan it, ignore it
		return
	}

	_ = r.resolveTags(workload)

	workloadKey := workload.getWorkloadKey()
	if workloadKey == ":" {
		// this workload doesn't have its tags yet, delay its processing
		select {
		case r.delayedWorkloads <- workload:
		default:
		}
		return
	}

	// check if this workload has been scanned before
	r.workloadsCacheLock.Lock()
	defer r.workloadsCacheLock.Unlock()
	cachedSBOM, ok := r.workloadsCache.Get(workloadKey)
	if ok {
		// copy report and file cache (keeping a reference is fine, we won't be modifying the content)
		workload.files = cachedSBOM.files
		workload.report = cachedSBOM.report
		r.workloadsCacheHit.Inc()
		return
	}
	r.workloadsCacheMiss.Inc()

	// push workload to the scanner chan
	select {
	case r.scannerChan <- workload:
	default:
	}
}

// Retain increments the reference counter of the SBOM of a workload
func (r *Resolver) Retain(id string, cgroup *cgroupModel.CacheEntry) {
	if !r.config.SBOMResolverEnabled {
		return
	}

	r.workloadsLock.Lock()
	defer r.workloadsLock.Unlock()

	// We don't scan hosts for now
	if len(id) == 0 {
		return
	}

	_, ok := r.workloads[id]
	if !ok {
		workload, err := r.newWorkloadEntry(id, cgroup)
		if err != nil {
			seclog.Errorf("couldn't create new SBOM entry for workload '%s': %v", id, err)
		}
		r.queueWorkload(workload)
	}
	return
}

// GetWorkload returns the workload of a provided ID
func (r *Resolver) GetWorkload(id string) *SBOM {
	r.workloadsLock.RLock()
	defer r.workloadsLock.RUnlock()

	return r.workloads[id]
}

// Delete removes the SBOM of the provided cgroup
func (r *Resolver) Delete(id string) {
	if !r.config.SBOMResolverEnabled {
		return
	}

	workload := r.GetWorkload(id)
	if workload == nil {
		return
	}
	workload.Lock()
	defer workload.Unlock()

	// Remove this SBOM
	r.deleteSBOM(workload)
}

// deleteSBOM delete all data indexed by the provided container ID
func (r *Resolver) deleteSBOM(workload *SBOM) {
	r.workloadsLock.Lock()
	defer r.workloadsLock.Unlock()

	seclog.Infof("deleting SBOM entry for '%s'", workload.ContainerID)
	// remove SBOM entry
	delete(r.workloads, workload.ContainerID)

	// compute workload key before reset
	workloadKey := workload.getWorkloadKey()

	// cleanup and insert SBOM in cache
	workload.reset()

	// push the workload to the cache
	r.workloadsCacheLock.Lock()
	defer r.workloadsCacheLock.Unlock()
	r.workloadsCache.Add(workloadKey, workload)
}

// AddContextTags Adds the tags resolved by the resolver to the provided SBOM
func (r *Resolver) AddContextTags(s *SBOM) {
	var tagName string
	var found bool

	dumpTagNames := make([]string, 0, len(s.Tags))
	for _, tag := range s.Tags {
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
			s.Tags = append(s.Tags, tag)
		}
	}
}

// processWorkload resolves the tags of the provided SBOM and delete it when applicable
func (r *Resolver) processWorkload(workload *SBOM, now time.Time) error {
	workload.Lock()
	defer workload.Unlock()

	// Start by resolving the tags: even if we end up deleting the SBOM, we need the tags to put the SBOM in cache
	// if this is the first time we send the SBOM, resolve the context tags
	r.AddContextTags(workload)

	// resolve the service if it is defined
	workload.Service = utils.GetTagValue("service", workload.Tags)

	// check if we should delete the sbom
	if workload.deleted.Load() || (!workload.deleted.Load() && len(workload.cgroup.GetRootPIDs()) == 0) {
		r.deleteSBOM(workload)
	}

	return nil
}

func (r *Resolver) SendStats() error {
	r.workloadsLock.RLock()
	defer r.workloadsLock.RUnlock()
	if val := float64(len(r.workloads)); val > 0 {
		if err := r.statsdClient.Gauge(metrics.MetricSBOMResolverActiveSBOMs, val, []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricSBOMResolverActiveSBOMs: %w", err)
		}
	}

	if val := r.sbomGenerations.Swap(0); val > 0 {
		if err := r.statsdClient.Count(metrics.MetricSBOMResolverSBOMGenerations, int64(val), []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricSBOMResolverSBOMGenerations: %w", err)
		}
	}

	if val := float64(r.workloadsCache.Len()); val > 0 {
		if err := r.statsdClient.Gauge(metrics.MetricSBOMResolverSBOMCacheLen, val, []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricSBOMResolverSBOMCacheLen: %w", err)
		}
	}

	if val := int64(r.workloadsCacheHit.Swap(0)); val > 0 {
		if err := r.statsdClient.Count(metrics.MetricSBOMResolverSBOMCacheHit, val, []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricSBOMResolverSBOMCacheHit: %w", err)
		}
	}

	if val := int64(r.workloadsCacheMiss.Swap(0)); val > 0 {
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
