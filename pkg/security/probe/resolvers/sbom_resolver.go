// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package resolvers

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	trivyReport "github.com/aquasecurity/trivy/pkg/types"
	"go.uber.org/atomic"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/security/api"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/sbom"
)

// SBOMSource defines is the default log source for the SBOM events
const SBOMSource = "runtime-security-agent"

type Package struct {
	Name    string
	Version string
}

type SBOM struct {
	sbomMutex sync.RWMutex

	report *trivyReport.Report
	files  map[string]*Package

	Host        string
	Source      string
	Service     string
	Tags        []string
	ContainerID string

	shouldScan      bool
	doNotSendBefore time.Time
	sent            bool
	cgroup          *CGroupCacheEntry
}

func (s *SBOM) Lock() {
	s.sbomMutex.Lock()
	if s.cgroup != nil {
		s.cgroup.Lock()
	}
}

func (s *SBOM) Unlock() {
	s.sbomMutex.Unlock()
	if s.cgroup != nil {
		s.cgroup.Unlock()
	}
}

// NewSBOM returns a new empty instance of SBOM
func NewSBOM(host string, source string, id string, doNotSendBefore time.Time, cgroup *CGroupCacheEntry) (*SBOM, error) {
	return &SBOM{
		files:           make(map[string]*Package),
		Host:            host,
		Source:          source,
		ContainerID:     id,
		shouldScan:      true,
		doNotSendBefore: doNotSendBefore,
		cgroup:          cgroup,
	}, nil
}

// SBOMResolver is the Software Bill-Of-material resolver
type SBOMResolver struct {
	workloadsLock   sync.RWMutex
	workloads       map[string]*SBOM
	scannerChan     chan *SBOM
	config          *config.Config
	statsdClient    statsd.ClientInterface
	tagsResolver    *TagsResolver
	trivyScanner    *sbom.TrivyCollector
	dispatcher      SBOMDispatcher
	sbomGenerations *atomic.Uint64

	// context tags and attributes
	hostname    string
	source      string
	contextTags []string
}

// NewSBOMResolver returns a new instance of SBOMResolver
func NewSBOMResolver(c *config.Config, tagsResolver *TagsResolver, dispatcher SBOMDispatcher, statsdClient statsd.ClientInterface) (*SBOMResolver, error) {
	trivyScanner, err := sbom.NewTrivyCollector()
	if err != nil {
		return nil, err
	}

	resolver := &SBOMResolver{
		config:          c,
		statsdClient:    statsdClient,
		tagsResolver:    tagsResolver,
		dispatcher:      dispatcher,
		workloads:       make(map[string]*SBOM),
		scannerChan:     make(chan *SBOM, 100),
		trivyScanner:    trivyScanner,
		sbomGenerations: atomic.NewUint64(0),
	}

	if !c.SBOMResolverEnabled {
		return resolver, nil
	}

	resolver.prepareContextTags()
	return resolver, nil
}

// resolveTags resolve the tags of a SBOM
func (r *SBOMResolver) resolveTags(s *SBOM) error {
	if len(s.Tags) >= 10 || len(s.ContainerID) == 0 {
		return nil
	}

	var err error
	s.Tags, err = r.tagsResolver.ResolveWithErr(s.ContainerID)
	if err != nil {
		return fmt.Errorf("failed to resolve %s: %w", s.ContainerID, err)
	}
	return nil
}

func (r *SBOMResolver) prepareContextTags() {
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
func (r *SBOMResolver) Start(ctx context.Context) {
	if !r.config.SBOMResolverEnabled {
		return
	}

	go func() {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		senderTick := time.NewTicker(r.config.SBOMResolverSBOMSenderTick)
		defer senderTick.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case workload := <-r.scannerChan:
				if err := r.analyzeWorkload(workload); err != nil {
					seclog.Errorf("couldn't scan '%s': %v", workload.ContainerID, err)
				}
			case <-senderTick.C:
				if r.config.SBOMResolverSBOMSenderEnabled {
					if err := r.SendAvailableSBOMs(); err != nil {
						seclog.Errorf("couldn't send SBOMs: %v", err)
					}
				}

			}
		}
	}()
}

// generateSBOM calls Trivy to generate the SBOM of a workload
func (r *SBOMResolver) generateSBOM(root string, workload *SBOM) error {
	seclog.Infof("Generating SBOM for %s", root)
	r.sbomGenerations.Add(1)

	report, err := r.trivyScanner.ScanRootfs(context.Background(), root)
	if err != nil {
		return fmt.Errorf("failed to generate SBOM for %s: %w", root, err)
	}

	workload.report = report
	workload.shouldScan = false

	seclog.Infof("SBOM successfully generated from %s", root)

	return nil
}

// analyzeWorkload generates the SBOM of the provided workload and send it to the security agent
func (r *SBOMResolver) analyzeWorkload(workload *SBOM) error {
	seclog.Infof("analyzing workload '%s'", workload.ContainerID)
	workload.Lock()
	defer workload.Unlock()

	var lastErr error
	for _, rootCandidatePID := range workload.cgroup.PIDs.Keys() {
		// check if this pid still exists and is in the expected container ID (if we loose an exit and need to wait for
		// the flush to remove a pid, there might be a significant delay before a PID is removed from this list. Checking
		// the container ID reduces drastically the likelihood of this race)
		computedID, err := utils.GetProcContainerID(rootCandidatePID, rootCandidatePID)
		if err != nil {
			workload.cgroup.PIDs.Remove(rootCandidatePID)
			continue
		}
		if string(computedID) != workload.ContainerID {
			workload.cgroup.PIDs.Remove(rootCandidatePID)
			continue
		}

		lastErr = r.generateSBOM(utils.ProcRootPath(int32(rootCandidatePID)), workload)
		if lastErr == nil {
			break
		} else {
			seclog.Errorf("couldn't generate SBOM: %v", lastErr)
		}
	}
	if lastErr != nil {
		return lastErr
	}
	if workload.shouldScan {
		return fmt.Errorf("couldn't generate workload: all root candidates failed")
	}

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

	// we can get rid of the report if don't plan to send it to the backend
	if !r.config.SBOMResolverSBOMSenderEnabled {
		workload.report = nil
	}

	seclog.Infof("new sbom generated for '%s'", workload.ContainerID)
	return nil
}

// RefreshSBOM analyzes the file system of a workload to refresh its SBOM.
func (r *SBOMResolver) RefreshSBOM(id string, cgroup *CGroupCacheEntry) error {
	if !r.config.SBOMResolverEnabled {
		return nil
	}

	r.workloadsLock.Lock()
	defer r.workloadsLock.Unlock()
	workload, ok := r.workloads[id]
	if ok {
		workload.Lock()
		defer workload.Unlock()
		workload.shouldScan = true
	} else {
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
func (r *SBOMResolver) ResolvePackage(containerID string, file *model.FileEvent) *Package {
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
func (r *SBOMResolver) newWorkloadEntry(id string, cgroup *CGroupCacheEntry) (*SBOM, error) {
	workload, err := NewSBOM(r.hostname, r.source, id, time.Now().Add(r.config.SBOMResolverSBOMSenderDelay), cgroup)
	if err != nil {
		return nil, err
	}
	r.workloads[id] = workload
	return workload, nil
}

// Retain increments the reference counter of the SBOM of a workload
func (r *SBOMResolver) Retain(id string, cgroup *CGroupCacheEntry) {
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

		// push workload to the scanner chan
		select {
		case r.scannerChan <- workload:
		default:
		}
		return
	}
	return
}

// Release decrements the reference counter of the SBOM of a workload
func (r *SBOMResolver) Release(id string) {
	if !r.config.SBOMResolverEnabled {
		return
	}

	r.workloadsLock.RLock()
	defer r.workloadsLock.RUnlock()

	workload, ok := r.workloads[id]
	if !ok {
		return
	}

	// only delete sbom if it has already been sent, delay the deletion to the sender otherwise
	if !r.config.SBOMResolverSBOMSenderEnabled || workload.cgroup.PIDs.Len() <= 0 && workload.sent {
		r.deleteSBOM(id)
	}
}

// deleteSBOM thread unsafe delete all data indexed by the provided container ID
func (r *SBOMResolver) deleteSBOM(containerID string) {
	seclog.Infof("deleting SBOM entry for '%s'", containerID)
	// remove SBOM entry
	delete(r.workloads, containerID)
}

// AddContextTags Adds the tags resolved by the resolver to the provided SBOM
func (r *SBOMResolver) AddContextTags(s *SBOM) {
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

// SendAvailableSBOMs sends all SBOMs that are ready to be sent
func (r *SBOMResolver) SendAvailableSBOMs() error {
	// make sure we don't lock the main map of workloads for too long
	r.workloadsLock.Lock()
	allWorkloads := make([]*SBOM, 0, len(r.workloads))
	for _, workload := range r.workloads {
		allWorkloads = append(allWorkloads, workload)
	}
	r.workloadsLock.Unlock()
	now := time.Now()

	for _, workload := range allWorkloads {
		if err := r.processWorkload(workload, now); err != nil {
			return err
		}
	}

	return nil
}

// SBOMDispatcher dispatches an SBOM
type SBOMDispatcher interface {
	DispatchSBOM(sbom *api.SBOMMessage)
}

// processWorkload resolves the tags of the provided SBOM, send it and delete it when applicable
func (r *SBOMResolver) processWorkload(workload *SBOM, now time.Time) error {
	workload.Lock()
	defer workload.Unlock()

	if !workload.sent {
		// resolve tags
		_ = r.resolveTags(workload)
	}

	if now.After(workload.doNotSendBefore) {

		// if this is the first time we send the SBOM, resolve the context tags
		if !workload.sent {
			r.AddContextTags(workload)

			// resolve the service if it is defined
			workload.Service = utils.GetTagValue("service", workload.Tags)
			workload.sent = true
		}

		// send SBOM to the security agent
		sbomMsg, err := workload.ToSBOMMessage()
		if err != nil {
			return fmt.Errorf("couldn't serialize SBOM to protobuf: %w", err)
		}
		seclog.Infof("dispatching SBOM for workload '%s'", workload.ContainerID)
		r.dispatcher.DispatchSBOM(sbomMsg)

		// check if we should delete the sbom
		if workload.cgroup.PIDs.Len() == 0 {
			r.workloadsLock.Lock()
			r.deleteSBOM(workload.ContainerID)
			r.workloadsLock.Unlock()
		}
	}
	return nil
}

func (r *SBOMResolver) SendStats() error {
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

	return nil
}
