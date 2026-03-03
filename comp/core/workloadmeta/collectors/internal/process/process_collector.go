// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package process implements the process collector for Workloadmeta.
package process

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/benbjohnson/clock"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/discovery/language"
	"github.com/DataDog/datadog-agent/pkg/process/checks"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/discovery/core"
	"github.com/DataDog/datadog-agent/pkg/discovery/model"
	"github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/languagedetection"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil/normalize"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	collectorID       = "process-collector"
	componentName     = "workloadmeta-process"
	cacheValidityNoRT = 2 * time.Second

	// Service discovery constants
	maxPortCheckTries = 10
)

type collector struct {
	id                     string
	store                  workloadmeta.Component
	catalog                workloadmeta.AgentType
	clock                  clock.Clock
	processProbe           procutil.Probe
	config                 pkgconfigmodel.Reader
	systemProbeConfig      pkgconfigmodel.Reader
	processEventsCh        chan *Event
	lastCollectedProcesses map[int32]*procutil.Process
	mux                    sync.RWMutex
	containerProvider      proccontainers.ContainerProvider

	// Service discovery fields
	sysProbeClient           *sysprobeclient.CheckClient
	serviceRetries           map[int32]uint
	ignoredPids              core.PidSet
	pidHeartbeats            map[int32]time.Time
	knownInjectionStatusPids core.PidSet // Track PIDs whose injection status we've already reported (but have no service data yet)
	metricDiscoveredServices telemetry.Gauge
}

// EventType represents the type of collector event
type EventType int

const (
	// EventTypeProcess means the event comes from Process Discovery.
	EventTypeProcess EventType = iota
	// EventTypeServiceDiscovery means the event comes from Service Discovery.
	EventTypeServiceDiscovery
)

// Event is a message type used to communicate with the stream function asynchronously
type Event struct {
	Type    EventType
	Created []*workloadmeta.Process
	Deleted []*workloadmeta.Process
}

func newProcessCollector(id string, catalog workloadmeta.AgentType, clock clock.Clock, processProbe procutil.Probe, config pkgconfigmodel.Reader, systemProbeConfig pkgconfigmodel.Reader) collector {
	return collector{
		id:                     id,
		catalog:                catalog,
		clock:                  clock,
		processProbe:           processProbe,
		config:                 config,
		systemProbeConfig:      systemProbeConfig,
		processEventsCh:        make(chan *Event),
		lastCollectedProcesses: make(map[int32]*procutil.Process),

		// Initialize service discovery fields
		sysProbeClient:           sysprobeclient.GetCheckClient(),
		serviceRetries:           make(map[int32]uint),
		ignoredPids:              make(core.PidSet),
		pidHeartbeats:            make(map[int32]time.Time),
		knownInjectionStatusPids: make(core.PidSet),
	}
}

type dependencies struct {
	fx.In
	Config    config.Component
	Sysconfig sysprobeconfig.Component
}

// NewProcessCollectorProvider returns a new process collector provider and an error.
// Currently, this is only used on Linux when language detection and run in core agent are enabled.
func NewProcessCollectorProvider(deps dependencies) (workloadmeta.CollectorProvider, error) {
	// process probe is not yet componentized, so we can't use fx injection for that
	probe := procutil.NewProcessProbe(
		procutil.WithIgnoreZombieProcesses(deps.Config.GetBool("process_config.ignore_zombie_processes")),
	)
	collector := newProcessCollector(collectorID, workloadmeta.NodeAgent, clock.New(), probe, deps.Config, deps.Sysconfig)
	return workloadmeta.CollectorProvider{
		Collector: &collector,
	}, nil
}

// Pull triggers an entity collection. To be used by collectors that
// don't have streaming functionality, and called periodically by the
// store. This is not needed for the process collector.
func (c *collector) Pull(_ context.Context) error {
	return nil
}

// GetID returns the identifier for the respective component.
func (c *collector) GetID() string {
	return c.id
}

// GetTargetCatalog gets the expected catalog.
func (c *collector) GetTargetCatalog() workloadmeta.AgentType {
	return c.catalog
}

// GetFxOptions returns the FX framework options for the collector
func GetFxOptions() fx.Option {
	return fx.Provide(NewProcessCollectorProvider)
}

// isProcessCollectionEnabled returns a boolean indicating if the process collector is enabled
func (c *collector) isProcessCollectionEnabled() bool {
	return c.config.GetBool("process_config.process_collection.enabled")
}

// isServiceDiscoveryEnabled returns a boolean indicating if service discovery is enabled
func (c *collector) isServiceDiscoveryEnabled() bool {
	return c.systemProbeConfig.GetBool("discovery.enabled")
}

// isGPUMonitoringEnabled returns a boolean indicating if GPU monitoring is enabled
func (c *collector) isGPUMonitoringEnabled() bool {
	return c.config.GetBool("gpu.enabled")
}

func (c *collector) getServiceCollectionInterval() time.Duration {
	return c.systemProbeConfig.GetDuration("discovery.service_collection_interval")
}

// isLanguageCollectionEnabled returns a boolean indicating if language collection is enabled
func (c *collector) isLanguageCollectionEnabled() bool {
	return c.config.GetBool("language_detection.enabled")
}

// processCollectionIntervalConfig returns the configured collection interval
func (c *collector) processCollectionIntervalConfig() time.Duration {
	processCollectionInterval := checks.GetInterval(c.config, checks.ProcessCheckName)
	serviceCollectionInterval := c.getServiceCollectionInterval()
	// service discovery data will be incorrect/empty if the process collection interval > service collection interval
	// therefore, the service collection interval must be the max interval for process collection
	if processCollectionInterval > serviceCollectionInterval {
		log.Warnf("process collection interval %v cannot be larger than the service collection interval %v. falling back to service collection interval",
			processCollectionInterval, serviceCollectionInterval)
		return serviceCollectionInterval
	}
	return processCollectionInterval
}

// Start starts the collector. The collector should run until the context
// is done. It also gets a reference to the store that started it so it
// can use Notify, or get access to other entities in the store.
func (c *collector) Start(ctx context.Context, store workloadmeta.Component) error {
	if !c.isProcessCollectionEnabled() && !c.isServiceDiscoveryEnabled() && !c.isLanguageCollectionEnabled() && !c.isGPUMonitoringEnabled() {
		return errors.NewDisabled(componentName, "process collection, service discovery, language collection, and GPU monitoring are disabled")
	}

	if c.containerProvider == nil {
		containerProvider, err := proccontainers.GetSharedContainerProvider()
		if err != nil {
			return err
		}
		c.containerProvider = containerProvider
	}
	c.store = store

	if c.isProcessCollectionEnabled() || c.isLanguageCollectionEnabled() || c.isGPUMonitoringEnabled() {
		go c.collectProcesses(ctx, c.clock.Ticker(c.processCollectionIntervalConfig()))
	}

	if c.isServiceDiscoveryEnabled() {
		serviceCollectionInterval := c.getServiceCollectionInterval()
		// Initialize service discovery metric
		c.metricDiscoveredServices = telemetry.NewGaugeWithOpts(
			collectorID,
			"discovered_services",
			[]string{},
			"Number of discovered alive services.",
			telemetry.DefaultOptions,
		)

		if c.isProcessCollectionEnabled() || c.isLanguageCollectionEnabled() {
			log.Debug("Starting cached service collection (process collection enabled)")
			go c.collectServicesCached(ctx, c.clock.Ticker(serviceCollectionInterval))
		} else {
			log.Debug("Starting non-cached service collection (process collection disabled)")
			go c.collectServicesNoCache(ctx, c.clock.Ticker(serviceCollectionInterval))
		}
	}

	go c.stream(ctx)

	return nil
}

// createdProcessesToWorkloadMetaProcesses helper function to convert createdProcs with container data into wlm entities
func createdProcessesToWorkloadmetaProcesses(createdProcs []*procutil.Process, pidToCid map[int]string, languages []*languagemodels.Language) []*workloadmeta.Process {
	wlmProcs := make([]*workloadmeta.Process, len(createdProcs))
	isLanguageDataAvailable := len(languages) == len(createdProcs) // language data is not always available, so we have to check
	for i, proc := range createdProcs {
		wlmProcs[i] = processToWorkloadMetaProcess(proc)
		if isLanguageDataAvailable {
			wlmProcs[i].Language = languages[i] // assuming order of language slice (not ideal but works for now)
		}

		// enrich with container data if possible
		cid, exists := pidToCid[int(proc.Pid)]
		if exists {
			wlmProcs[i].ContainerID = cid // existing behaviour which we will maintain until the collector is enabled
			// storing container id as an entity field
			wlmProcs[i].Owner = &workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   cid,
			}
		}
	}
	return wlmProcs
}

// deletedProcessesToWorkloadMetaProcesses helper function to convert deletedProcs into wlm entities. wlm only uses the EventType, Source, ID, and Kind for deletion events
func deletedProcessesToWorkloadmetaProcesses(deletedProcs []*procutil.Process) []*workloadmeta.Process {
	wlmProcs := make([]*workloadmeta.Process, len(deletedProcs))
	for i, proc := range deletedProcs {
		wlmProcs[i] = &workloadmeta.Process{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindProcess,
				ID:   strconv.Itoa(int(proc.Pid)),
			},
		}
	}
	return wlmProcs
}

// processCacheDifference returns new processes that exist in procCacheA and not in procCacheB.
// It uses PID, creation time, and command line hash to detect new processes
func processCacheDifference(procCacheA map[int32]*procutil.Process, procCacheB map[int32]*procutil.Process) []*procutil.Process {
	// attempt to pre-allocate right slice size to reduce number of slice growths
	diffSize := 0
	if len(procCacheA) > len(procCacheB) {
		diffSize = len(procCacheA) - len(procCacheB)
	} else {
		diffSize = len(procCacheB) - len(procCacheA)
	}
	newProcs := make([]*procutil.Process, 0, diffSize)
	for pid, procA := range procCacheA {
		procB, pidExists := procCacheB[pid]

		// New process - PID not in cache
		if !pidExists {
			newProcs = append(newProcs, procA)
			continue
		}

		if !procutil.IsSameProcess(procA, procB) {
			newProcs = append(newProcs, procA)
		}
	}
	return newProcs
}

// detectLanguages collects languages from given processes if language collection is enabled
func (c *collector) detectLanguages(processes []*procutil.Process) []*languagemodels.Language {
	if c.isLanguageCollectionEnabled() {
		languageInterfaceProcs := make([]languagemodels.Process, len(processes))
		for i, proc := range processes {
			languageInterfaceProcs[i] = languagemodels.Process(proc)
		}
		return languagedetection.DetectLanguage(languageInterfaceProcs, c.systemProbeConfig)
	}
	return nil
}

// filterPidsToRequest filters PIDs to categorize them as new or needing heartbeat refresh.
// It returns separate slices for new PIDs and heartbeat PIDs, along with a map of pids to *model.Service
// to be filled up with the response received from system-probe.
func (c *collector) filterPidsToRequest(alivePids core.PidSet, procs map[int32]*procutil.Process) ([]int32, []int32, map[int32]*model.Service) {
	now := c.clock.Now().UTC()
	newPids := make([]int32, 0, len(alivePids))
	heartbeatPids := make([]int32, 0, len(alivePids))
	pidsToService := make(map[int32]*model.Service, len(alivePids))

	for pid := range alivePids {
		if c.ignoredPids.Has(pid) {
			continue
		}

		// Filter out processes that started less than a minute ago
		if proc, exists := procs[pid]; exists {
			processStartTime := time.UnixMilli(proc.Stats.CreateTime).UTC()
			if now.Sub(processStartTime) < time.Minute {
				continue
			}
		}

		// Check if service data is stale or never collected
		lastHeartbeat, exists := c.pidHeartbeats[pid]
		if !exists {
			// Never seen this process before, need full service info
			newPids = append(newPids, pid)
			pidsToService[pid] = nil
		} else if now.Sub(lastHeartbeat) > core.HeartbeatTime {
			// Service data is stale, need heartbeat refresh
			// Since we have a pidHeartbeats entry, we know service data exists
			heartbeatPids = append(heartbeatPids, pid)
			pidsToService[pid] = nil
		}
	}

	return newPids, heartbeatPids, pidsToService
}

// getDiscoveryServices calls the system-probe /discovery/services endpoint
func (c *collector) getDiscoveryServices(newPids []int32, heartbeatPids []int32) (*model.ServicesResponse, error) {
	// Create params with categorized PIDs
	params := core.Params{
		NewPids:       newPids,
		HeartbeatPids: heartbeatPids,
	}

	response, err := sysprobeclient.Post[model.ServicesResponse](c.sysProbeClient, "/services", params, sysconfig.DiscoveryModule)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

func (c *collector) handleServiceRetries(pid int32) {
	tries := c.serviceRetries[pid]
	tries++
	if tries < maxPortCheckTries {
		c.serviceRetries[pid] = tries
	} else {
		log.Tracef("[pid: %d] ignoring due to max number of retries", pid)
		c.ignoredPids.Add(pid)
		delete(c.serviceRetries, pid)
	}
}

// getProcessEntitiesFromServices creates Process entities with service discovery data
func (c *collector) getProcessEntitiesFromServices(newPids []int32, heartbeatPids []int32, pidsToService map[int32]*model.Service, injectedPids core.PidSet, gpuPids core.PidSet) []*workloadmeta.Process {
	entities := make([]*workloadmeta.Process, 0, len(pidsToService))
	now := c.clock.Now().UTC()

	// Process new PIDs - create complete entities
	for _, pid := range newPids {
		service := pidsToService[pid]

		if service == nil {
			c.handleServiceRetries(pid)

			// Skip creating entity if we already reported this PID's injection status
			if c.knownInjectionStatusPids.Has(pid) {
				continue
			}

			c.knownInjectionStatusPids.Add(pid)
		} else {
			c.knownInjectionStatusPids.Remove(pid)
			c.pidHeartbeats[int32(service.PID)] = now
		}

		injectionState := workloadmeta.InjectionNotInjected
		if injectedPids.Has(pid) {
			injectionState = workloadmeta.InjectionInjected
		}

		entity := &workloadmeta.Process{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindProcess,
				ID:   strconv.Itoa(int(pid)),
			},
			Pid:            pid,
			InjectionState: injectionState,
			UsesGPU:        gpuPids.Has(pid),
		}

		if service != nil {
			entity.Service = convertModelServiceToService(service)
			// language is captured here since language+process collection can be disabled
			entity.Language = convertServiceLanguageToWLMLanguage(service.Language)
		}

		entities = append(entities, entity)
	}

	// Process heartbeat PIDs - only update entities that have existing Service data
	for _, pid := range heartbeatPids {
		service := pidsToService[pid]
		if service == nil {
			c.handleServiceRetries(pid)
			continue
		}

		// Verify existing entity has Service data (should always be true for heartbeat PIDs)
		existingProcess, err := c.store.GetProcess(pid)
		if err != nil || existingProcess == nil || existingProcess.Service == nil {
			log.Debugf("Heartbeat for pid %d but no existing service found, skipping", pid)
			continue
		}

		c.pidHeartbeats[int32(service.PID)] = now

		// For heartbeat updates, preserve static fields from existing service
		// Since workloadmeta replaces entities from the same source instead of merging,
		// we need to preserve static fields here
		newService := convertModelServiceToService(service)

		// Copy existing service and update only dynamic fields
		preservedService := *existingProcess.Service
		preservedService.TCPPorts = newService.TCPPorts
		preservedService.UDPPorts = newService.UDPPorts
		preservedService.LogFiles = newService.LogFiles

		// The following fields are preserved across the lifetime of the process.
		entity := &workloadmeta.Process{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindProcess,
				ID:   strconv.Itoa(service.PID),
			},
			Pid:            int32(service.PID),
			Service:        &preservedService,
			InjectionState: existingProcess.InjectionState,
			Language:       existingProcess.Language,
			UsesGPU:        existingProcess.UsesGPU,
		}

		entities = append(entities, entity)
	}

	return entities
}

// updateServices retrieves service discovery data for alive processes and returns workloadmeta entities
func (c *collector) updateServices(alivePids core.PidSet, procs map[int32]*procutil.Process) ([]*workloadmeta.Process, core.PidSet) {
	newPids, heartbeatPids, pidsToService := c.filterPidsToRequest(alivePids, procs)
	if len(newPids) == 0 && len(heartbeatPids) == 0 {
		return nil, nil
	}

	resp, err := c.getDiscoveryServices(newPids, heartbeatPids)
	if err != nil {
		// CheckClient handles startup warnings internally, but we still need to suppress
		// the error if system-probe hasn't started yet
		if sysprobeclient.IgnoreStartupError(err) == nil {
			return nil, nil
		}
		log.Errorf("failed to get services: %s", err)
		return nil, nil
	}

	for i, service := range resp.Services {
		pidsToService[int32(service.PID)] = &resp.Services[i]
	}

	injectedPids := make(core.PidSet)
	for _, pid := range resp.InjectedPIDs {
		injectedPids.Add(int32(pid))
	}

	gpuPids := make(core.PidSet)
	for _, pid := range resp.GPUPIDs {
		gpuPids.Add(int32(pid))
	}

	return c.getProcessEntitiesFromServices(newPids, heartbeatPids, pidsToService, injectedPids, gpuPids), injectedPids
}

func (c *collector) updateServicesNoCache(alivePids core.PidSet, procs map[int32]*procutil.Process) []*workloadmeta.Process {
	entities, _ := c.updateServices(alivePids, procs)
	if len(entities) == 0 {
		return nil
	}

	pidToCid := c.containerProvider.GetPidToCid(cacheValidityNoRT)

	for _, entity := range entities {
		if proc, exists := procs[entity.Pid]; exists {
			// process fields should be set when the process collector is disabled
			entity.NsPid = proc.NsPid
			entity.Ppid = proc.Ppid
			entity.Name = proc.Name
			entity.Cwd = proc.Cwd
			entity.Exe = proc.Exe
			entity.Comm = proc.Comm
			entity.Cmdline = proc.Cmdline
			entity.CreationTime = time.UnixMilli(proc.Stats.CreateTime).UTC()
			entity.Uids = proc.Uids
			entity.Gids = proc.Gids
		}

		if cid, exists := pidToCid[int(entity.Pid)]; exists {
			entity.ContainerID = cid
			entity.Owner = &workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   cid,
			}
		}
	}

	return entities
}

// getProcessDataForServices returns alive pids and processes
func (c *collector) getProcessDataForServices() (core.PidSet, map[int32]*procutil.Process, error) {
	// If process collection is disabled, scan processes ourselves
	procs, err := c.processProbe.ProcessesByPID(c.clock.Now().UTC(), false)
	if err != nil {
		return nil, nil, err
	}

	alivePids := make(core.PidSet, len(procs))
	for pid := range procs {
		alivePids.Add(pid)
	}

	return alivePids, procs, nil
}

func (c *collector) getCachedProcessData() (core.PidSet, error) {
	// Get alive PIDs from last collected processes (populated by collectProcesses)
	c.mux.RLock()
	defer c.mux.RUnlock()

	if len(c.lastCollectedProcesses) == 0 {
		return nil, nil // no processes to check
	}

	alivePids := make(core.PidSet, len(c.lastCollectedProcesses))
	for pid := range c.lastCollectedProcesses {
		alivePids.Add(pid)
	}

	return alivePids, nil
}

// cleanPidMaps deletes dead PIDs from the provided maps.
func cleanPidMaps[T any](alivePids core.PidSet, maps ...map[int32]T) {
	for _, m := range maps {
		for pid := range m {
			if alivePids.Has(pid) {
				continue
			}

			delete(m, pid)
		}
	}
}

// findDeletedProcesses finds deleted processes by comparing current processes with the last collected cache.
// Returns workloadmeta entities for deleted processes.
// Used by both collectProcesses and collectServicesNoCache for consistency.
func (c *collector) findDeletedProcesses(currentProcs map[int32]*procutil.Process) []*workloadmeta.Process {
	c.mux.RLock()
	lastProcs := c.lastCollectedProcesses
	c.mux.RUnlock()

	deletedProcs := processCacheDifference(lastProcs, currentProcs)
	return deletedProcessesToWorkloadmetaProcesses(deletedProcs)
}

// cleanDiscoveryMaps cleans up stale PID mappings for service discovery.
// Used by both service collection methods to maintain clean state.
func (c *collector) cleanDiscoveryMaps(alivePids core.PidSet) {
	cleanPidMaps(alivePids, c.ignoredPids)
	cleanPidMaps(alivePids, c.serviceRetries)
	cleanPidMaps(alivePids, c.pidHeartbeats)
	cleanPidMaps(alivePids, c.knownInjectionStatusPids)
}

// updateDiscoveredServicesMetric updates the metric with the count of discovered services
func (c *collector) updateDiscoveredServicesMetric() {
	if c.metricDiscoveredServices == nil {
		return
	}

	count := len(c.pidHeartbeats)
	log.Debugf("discovered services count: %d", count)
	c.metricDiscoveredServices.Set(float64(count))
}

// collectProcesses captures all the required process data for the process check
func (c *collector) collectProcesses(ctx context.Context, collectionTicker *clock.Ticker) {
	// TODO: implement the full collection logic for the process collector. Once collection is done, submit events.
	ctx, cancel := context.WithCancel(ctx)
	defer collectionTicker.Stop()
	defer cancel()
	// Run collection immediately on startup, then wait for ticker to repeat
	for {
		// fetch process data and submit events to streaming channel for asynchronous processing
		procs, err := c.processProbe.ProcessesByPID(c.clock.Now().UTC(), false)
		if err != nil {
			log.Errorf("Error getting processes by pid: %v", err)
			return
		}

		// some processes are in a container so we want to store the container_id for them
		pidToCid := c.containerProvider.GetPidToCid(cacheValidityNoRT)
		// TODO: potentially scrub process data here instead of in the check?

		// categorize the processes into events for workloadmeta
		createdProcs := processCacheDifference(procs, c.lastCollectedProcesses)
		languages := c.detectLanguages(createdProcs)
		wlmCreatedProcs := createdProcessesToWorkloadmetaProcesses(createdProcs, pidToCid, languages)

		wlmDeletedProcs := c.findDeletedProcesses(procs)

		// send these events to the channel
		c.processEventsCh <- &Event{
			Type:    EventTypeProcess,
			Created: wlmCreatedProcs,
			Deleted: wlmDeletedProcs,
		}

		// store latest collected processes
		c.mux.Lock()
		c.lastCollectedProcesses = procs
		c.mux.Unlock()

		select {
		case <-collectionTicker.C:
			// Continue to next iteration
		case <-ctx.Done():
			log.Infof("The %s collector has stopped", collectorID)
			return
		}
	}
}

func (c *collector) collectServicesNoCache(ctx context.Context, collectionTicker *clock.Ticker) {
	ctx, cancel := context.WithCancel(ctx)
	defer collectionTicker.Stop()
	defer cancel()
	for {
		select {
		case <-collectionTicker.C:
			alivePids, procs, err := c.getProcessDataForServices()
			if err != nil {
				log.Errorf("Error getting processes for service discovery: %v", err)
				continue
			}
			if len(alivePids) == 0 {
				continue // no processes to check
			}

			wlmServiceEntities := c.updateServicesNoCache(alivePids, procs)
			deletedProcesses := c.findDeletedProcesses(procs)

			if len(wlmServiceEntities) > 0 || len(deletedProcesses) > 0 {
				c.processEventsCh <- &Event{
					Type:    EventTypeServiceDiscovery,
					Created: wlmServiceEntities,
					Deleted: deletedProcesses,
				}
			}

			c.mux.Lock()
			c.lastCollectedProcesses = procs
			c.mux.Unlock()

			c.cleanDiscoveryMaps(alivePids)
			c.updateDiscoveredServicesMetric()
		case <-ctx.Done():
			log.Infof("The %s service collector has stopped", collectorID)
			return
		}
	}
}

// collectServices captures service discovery data for alive processes
func (c *collector) collectServicesCached(ctx context.Context, collectionTicker *clock.Ticker) {
	ctx, cancel := context.WithCancel(ctx)
	defer collectionTicker.Stop()
	defer cancel()
	for {
		select {
		case <-collectionTicker.C:
			alivePids, err := c.getCachedProcessData()
			if err != nil {
				log.Errorf("Error getting processes for service discovery: %v", err)
				continue
			}

			var wlmDeletedProcs []*workloadmeta.Process
			for pid := range c.pidHeartbeats {
				if alivePids.Has(pid) {
					continue
				}

				wlmDeletedProcs = append(wlmDeletedProcs, &workloadmeta.Process{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindProcess,
						ID:   strconv.Itoa(int(pid)),
					},
				})
			}

			// Check for deleted processes whose injection status we reported (but had no service)
			for pid := range c.knownInjectionStatusPids {
				if alivePids.Has(pid) {
					continue
				}

				wlmDeletedProcs = append(wlmDeletedProcs, &workloadmeta.Process{
					EntityID: workloadmeta.EntityID{
						Kind: workloadmeta.KindProcess,
						ID:   strconv.Itoa(int(pid)),
					},
				})
			}

			c.mux.RLock()
			wlmServiceEntities, _ := c.updateServices(alivePids, c.lastCollectedProcesses)
			c.mux.RUnlock()

			if len(wlmServiceEntities) > 0 || len(wlmDeletedProcs) > 0 {
				c.processEventsCh <- &Event{
					Type:    EventTypeServiceDiscovery,
					Created: wlmServiceEntities,
					Deleted: wlmDeletedProcs,
				}
			}

			c.cleanDiscoveryMaps(alivePids)
			c.updateDiscoveredServicesMetric()
		case <-ctx.Done():
			log.Infof("The %s service collector has stopped", collectorID)
			return
		}
	}
}

// stream processes events sent from data collection and notifies WorkloadMeta that updates have occurred
func (c *collector) stream(ctx context.Context) {
	healthCheck := health.RegisterLiveness(componentName)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for {
		select {
		case <-healthCheck.C:

		case processEvent := <-c.processEventsCh:
			var source workloadmeta.Source

			// Choose source based on event type
			switch processEvent.Type {
			case EventTypeProcess:
				source = workloadmeta.SourceProcessCollector
			case EventTypeServiceDiscovery:
				source = workloadmeta.SourceServiceDiscovery
			}

			events := make([]workloadmeta.CollectorEvent, 0, len(processEvent.Deleted)+len(processEvent.Created))
			for _, proc := range processEvent.Deleted {
				events = append(events, workloadmeta.CollectorEvent{
					Type:   workloadmeta.EventTypeUnset,
					Entity: proc,
					Source: source,
				})
			}

			for _, proc := range processEvent.Created {
				events = append(events, workloadmeta.CollectorEvent{
					Type:   workloadmeta.EventTypeSet,
					Entity: proc,
					Source: source,
				})
			}

			c.store.Notify(events)

		case <-ctx.Done():
			err := healthCheck.Deregister()
			if err != nil {
				log.Warnf("error de-registering health check: %s", err)
			}
			return
		}
	}
}

// processToWorkloadMetaProcess maps a procutil process to a workloadmeta process
func processToWorkloadMetaProcess(process *procutil.Process) *workloadmeta.Process {
	return &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   strconv.Itoa(int(process.Pid)),
		},
		Pid:          process.Pid,
		NsPid:        process.NsPid,
		Ppid:         process.Ppid,
		Name:         process.Name,
		Cwd:          process.Cwd, // requires permission check
		Exe:          process.Exe, // requires permission check
		Comm:         process.Comm,
		Cmdline:      process.Cmdline,
		Uids:         process.Uids,
		Gids:         process.Gids,
		CreationTime: time.UnixMilli(process.Stats.CreateTime).UTC(),
	}
}

func normalizeNames(serviceName string, additionalNames []string, lang string) (string, []string) {
	// NormalizeService returns the fallback service name ("unknown-service")
	// for empty languages, without this check it would return
	// "unnamed-unknown-service" for language.Unknown.
	if lang == string(language.Unknown) {
		lang = ""
	}

	serviceName, _ = normalize.NormalizeService(serviceName, lang)
	additionalNames = normalizeAdditionalServiceNames(additionalNames)
	return serviceName, additionalNames
}

func normalizeAdditionalServiceNames(names []string) []string {
	if len(names) == 0 {
		return names
	}

	out := make([]string, 0, len(names))
	for _, v := range names {
		if len(strings.TrimSpace(v)) == 0 {
			continue
		}

		// lang is only used for fallback names, which we don't use since we
		// check for errors.
		norm, err := normalize.NormalizeService(v, "")
		if err == nil {
			out = append(out, norm)
		}
	}
	slices.Sort(out)
	return out
}

// tracerCollectsLogs checks if any tracer metadata indicates the tracer is already collecting logs.
func tracerCollectsLogs(tracerMetadata []tracermetadata.TracerMetadata) bool {
	for _, tm := range tracerMetadata {
		if tm.LogsCollected {
			return true
		}
	}
	return false
}

// convertModelServiceToService converts model.Service to workloadmeta.Service
func convertModelServiceToService(modelService *model.Service) *workloadmeta.Service {
	generatedName, additionalNames := normalizeNames(modelService.GeneratedName, modelService.AdditionalGeneratedNames, modelService.Language)

	var logFiles []string
	if !tracerCollectsLogs(modelService.TracerMetadata) {
		logFiles = modelService.LogFiles
	} else {
		log.Debugf("Skipping log file for pid %d: tracer is already collecting logs, files: %v", modelService.PID, modelService.LogFiles)
	}

	return &workloadmeta.Service{
		GeneratedName:            generatedName,
		GeneratedNameSource:      modelService.GeneratedNameSource,
		AdditionalGeneratedNames: additionalNames,
		TracerMetadata:           modelService.TracerMetadata,
		TCPPorts:                 modelService.TCPPorts,
		UDPPorts:                 modelService.UDPPorts,
		APMInstrumentation:       modelService.APMInstrumentation,
		Type:                     modelService.Type,
		LogFiles:                 logFiles,
		UST: workloadmeta.UST{
			Service: modelService.UST.Service,
			Env:     modelService.UST.Env,
			Version: modelService.UST.Version,
		},
	}
}

// convertServiceLanguageToWLMLanguage converts service language to the support language in workloadmeta since there are
// enum value differences between service discovery and our language model
// TODO: this is something we could consolidate in the future
func convertServiceLanguageToWLMLanguage(serviceLanguage string) *languagemodels.Language {
	switch serviceLanguage {
	case string(language.Java):
		return &languagemodels.Language{
			Name: languagemodels.Java,
		}
	case string(language.Node):
		return &languagemodels.Language{
			Name: languagemodels.Node,
		}
	case string(language.Python):
		return &languagemodels.Language{
			Name: languagemodels.Python,
		}
	case string(language.Ruby):
		return &languagemodels.Language{
			Name: languagemodels.Ruby,
		}
	case string(language.DotNet):
		return &languagemodels.Language{
			Name: languagemodels.Dotnet,
		}
	case string(language.Go):
		return &languagemodels.Language{
			Name: languagemodels.Go,
		}
	case string(language.CPlusPlus):
		return &languagemodels.Language{
			Name: languagemodels.CPP,
		}
	case string(language.PHP):
		return &languagemodels.Language{
			Name: languagemodels.PHP,
		}
	default:
		return &languagemodels.Language{
			Name: languagemodels.Unknown,
		}
	}
}
