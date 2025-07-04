// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package process implements the process collector for Workloadmeta.
package process

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/benbjohnson/clock"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/core"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/languagedetection"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	collectorID               = "process-collector"
	componentName             = "workloadmeta-process"
	cacheValidityNoRT         = 2 * time.Second
	serviceCollectionInterval = 60 * time.Second // TODO: this should be made configurable in the future

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
	sysProbeClient *http.Client
	startTime      time.Time
	startupTimeout time.Duration
	serviceRetries map[int32]uint
	ignoredPids    core.PidSet
	pidHeartbeats  map[int32]time.Time
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
		sysProbeClient: sysprobeclient.Get(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")),
		startTime:      clock.Now(),
		startupTimeout: pkgconfigsetup.Datadog().GetDuration("check_system_probe_startup_time"),
		serviceRetries: make(map[int32]uint),
		ignoredPids:    make(core.PidSet),
		pidHeartbeats:  make(map[int32]time.Time),
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
	collector := newProcessCollector(collectorID, workloadmeta.NodeAgent, clock.New(), procutil.NewProcessProbe(), deps.Config, deps.Sysconfig)
	return workloadmeta.CollectorProvider{
		Collector: &collector,
	}, nil
}

// GetFxOptions returns the FX framework options for the collector
func GetFxOptions() fx.Option {
	return fx.Provide(NewProcessCollectorProvider)
}

// isProcessCollectionEnabled returns a boolean indicating if the process collector is enabled
func (c *collector) isProcessCollectionEnabled() bool {
	// TODO: implement the logic to check if the process collector is enabled based on dependent configs (process collection, language detection, service discovery)
	// hardcoded to false until the new collector has all functionality/consolidation completed (service discovery, language collection, etc)
	return false
}

// isServiceDiscoveryEnabled returns a boolean indicating if service discovery is enabled
func (c *collector) isServiceDiscoveryEnabled() bool {
	// TODO: implement the logic to check if service discovery is enabled based on configuration
	return false
}

// isLanguageCollectionEnabled returns a boolean indicating if language collection is enabled
func (c *collector) isLanguageCollectionEnabled() bool {
	return c.config.GetBool("language_detection.enabled")
}

// collectionIntervalConfig returns the configured collection interval
func (c *collector) collectionIntervalConfig() time.Duration {
	// TODO: read configured collection interval once implemented
	return time.Second * 10
}

// Start starts the collector. The collector should run until the context
// is done. It also gets a reference to the store that started it so it
// can use Notify, or get access to other entities in the store.
func (c *collector) Start(ctx context.Context, store workloadmeta.Component) error {
	if !c.isProcessCollectionEnabled() && !c.isServiceDiscoveryEnabled() {
		return errors.NewDisabled(componentName, "process collection and service discovery are disabled")
	}

	if c.containerProvider == nil {
		containerProvider, err := proccontainers.GetSharedContainerProvider()
		if err != nil {
			return err
		}
		c.containerProvider = containerProvider
	}
	c.store = store

	if c.isProcessCollectionEnabled() {
		go c.collectProcesses(ctx, c.clock.Ticker(c.collectionIntervalConfig()))
	}

	if c.isServiceDiscoveryEnabled() {
		if c.isProcessCollectionEnabled() {
			go c.collectServicesCached(ctx, c.clock.Ticker(serviceCollectionInterval))
		} else {
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

// processCacheDifference returns new processes that exist in procCacheA and not in procCacheB
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
		procB, exists := procCacheB[pid]

		// new process
		if !exists {
			newProcs = append(newProcs, procA)
		} else if procB.Stats.CreateTime != procA.Stats.CreateTime {
			// same process PID exists, but different process due to creation time
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

// filterPidsToRequest filters PIDs to only request services for new or stale processes.
// It returns a slice of pids to request (to be used as a request parameters), and
// a map of pids to *model.Service to be filled up with the response received from
// system-probe. This map is useful to know for which pids we have not received
// service info and that needs to be handled by the retry mechanism.
func (c *collector) filterPidsToRequest(alivePids core.PidSet, procs map[int32]*procutil.Process) ([]int32, map[int32]*model.Service) {
	now := c.clock.Now()
	pidsToRequest := make([]int32, 0, len(alivePids))
	pidsToService := make(map[int32]*model.Service, len(alivePids))

	for pid := range alivePids {
		if c.ignoredPids.Has(pid) {
			continue
		}

		// Filter out processes that started less than a minute ago
		if proc, exists := procs[pid]; exists {
			processStartTime := time.UnixMilli(proc.Stats.CreateTime)
			if now.Sub(processStartTime) < time.Minute {
				continue
			}
		}

		// Check if service data is stale or never collected
		lastHeartbeat, exists := c.pidHeartbeats[pid]
		if !exists || now.Sub(lastHeartbeat) > core.HeartbeatTime {
			// Service data is stale or never collected, need to refresh it
			pidsToRequest = append(pidsToRequest, pid)
			pidsToService[pid] = nil
		}
	}

	return pidsToRequest, pidsToService
}

// getDiscoveryServices calls the system-probe /discovery/services endpoint
func (c *collector) getDiscoveryServices(pids []int32) (*model.ServicesEndpointResponse, error) {
	var responseData model.ServicesEndpointResponse

	url := getDiscoveryURL("services", pids)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.sysProbeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("non-ok status code: url %s, status_code: %d, response: `%s`", req.URL, resp.StatusCode, string(body))
	}

	err = json.Unmarshal(body, &responseData)
	if err != nil {
		return nil, err
	}

	return &responseData, nil
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
func (c *collector) getProcessEntitiesFromServices(pids []int32, pidsToService map[int32]*model.Service) []*workloadmeta.Process {
	entities := make([]*workloadmeta.Process, 0, len(pids))
	now := c.clock.Now()

	for _, pid := range pids {
		service := pidsToService[pid]
		if service == nil {
			c.handleServiceRetries(pid)
			continue
		}

		// Update the heartbeat cache for this PID
		c.pidHeartbeats[int32(service.PID)] = now

		entity := &workloadmeta.Process{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindProcess,
				ID:   strconv.Itoa(service.PID),
			},
			Pid:     int32(service.PID),
			Service: convertModelServiceToService(service),
		}

		entities = append(entities, entity)
	}

	return entities
}

// convertModelServiceToService converts model.Service to workloadmeta.Service
func convertModelServiceToService(modelService *model.Service) *workloadmeta.Service {
	return &workloadmeta.Service{
		GeneratedName:            modelService.GeneratedName,
		GeneratedNameSource:      modelService.GeneratedNameSource,
		AdditionalGeneratedNames: modelService.AdditionalGeneratedNames,
		TracerMetadata:           modelService.TracerMetadata,
		DDService:                modelService.DDService,
		DDServiceInjected:        modelService.DDServiceInjected,
		Ports:                    modelService.Ports,
		APMInstrumentation:       modelService.APMInstrumentation,
		Type:                     modelService.Type,
	}
}

// updateServices retrieves service discovery data for alive processes and returns workloadmeta entities
func (c *collector) updateServices(alivePids core.PidSet, procs map[int32]*procutil.Process) ([]*workloadmeta.Process, map[int32]*model.Service) {
	pidsToRequest, pidsToService := c.filterPidsToRequest(alivePids, procs)
	if len(pidsToRequest) == 0 {
		return nil, nil
	}

	resp, err := c.getDiscoveryServices(pidsToRequest)
	if err != nil {
		if time.Since(c.startTime) < c.startupTimeout {
			log.Warnf("service collector: system-probe not started yet: %v", err)
		} else {
			log.Errorf("failed to get services: %s", err)
		}
		return nil, nil
	}

	for i, service := range resp.Services {
		pidsToService[int32(service.PID)] = &resp.Services[i]
	}

	return c.getProcessEntitiesFromServices(pidsToRequest, pidsToService), pidsToService
}

func (c *collector) updateServicesNoCache(alivePids core.PidSet, procs map[int32]*procutil.Process) []*workloadmeta.Process {
	entities, pidsToService := c.updateServices(alivePids, procs)

	// Only detect languages for services when process collection is disabled,
	// otherwise the collectProcesses goroutine already did it for us.
	var pidToLanguage map[int32]*languagemodels.Language
	serviceProcs := make([]*procutil.Process, 0, len(pidsToService))
	for pid := range pidsToService {
		if proc, exists := procs[pid]; exists {
			serviceProcs = append(serviceProcs, proc)
		}
	}
	languages := c.detectLanguages(serviceProcs)

	// Create pidToLanguage map directly
	pidToLanguage = make(map[int32]*languagemodels.Language)
	for i, proc := range serviceProcs {
		if i < len(languages) && languages[i] != nil {
			pidToLanguage[proc.Pid] = languages[i]
		}
	}

	for _, entity := range entities {
		if proc, exists := procs[entity.Pid]; exists {
			entity.Cmdline = proc.Cmdline
			entity.CreationTime = time.UnixMilli(proc.Stats.CreateTime).UTC()

			// Add language if available
			if language, hasLanguage := pidToLanguage[entity.Pid]; hasLanguage {
				entity.Language = language
			}
		}
	}

	return entities
}

// getProcessDataForServices returns alive pids and processes
func (c *collector) getProcessDataForServices() (core.PidSet, map[int32]*procutil.Process, error) {
	// If process collection is disabled, scan processes ourselves
	procs, err := c.processProbe.ProcessesByPID(c.clock.Now(), false)
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
}

// getDiscoveryURL builds the URL for the discovery endpoint
func getDiscoveryURL(endpoint string, pids []int32) string {
	URL := &url.URL{
		Scheme: "http",
		Host:   "sysprobe",
		Path:   "/discovery/" + endpoint,
	}

	if len(pids) > 0 {
		pidsStr := make([]string, len(pids))
		for i, pid := range pids {
			pidsStr[i] = strconv.Itoa(int(pid))
		}

		query := url.Values{}
		query.Add("pids", strings.Join(pidsStr, ","))
		URL.RawQuery = query.Encode()
	}

	return URL.String()
}

// collectProcesses captures all the required process data for the process check
func (c *collector) collectProcesses(ctx context.Context, collectionTicker *clock.Ticker) {
	// TODO: implement the full collection logic for the process collector. Once collection is done, submit events.
	ctx, cancel := context.WithCancel(ctx)
	defer collectionTicker.Stop()
	defer cancel()
	for {
		select {
		case <-collectionTicker.C:
			// fetch process data and submit events to streaming channel for asynchronous processing
			procs, err := c.processProbe.ProcessesByPID(c.clock.Now(), false)
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
