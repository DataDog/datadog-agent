// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package securityprofile holds security profiles related files
package securityprofile

import (
	"bytes"
	"container/list"
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	lru "github.com/hashicorp/golang-lru/v2"
	"go.uber.org/atomic"

	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tags"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	mtdt "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree/metadata"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/storage"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/storage/backend"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type pendingProfile struct {
	firstSeen time.Time
	events    *list.List
}

// sampleCookieEntry maps a kernel dedup cookie to the profile and tree nodes it refreshes.
type sampleCookieEntry struct {
	profile       *profile.Profile
	processNode   *activity_tree.ProcessNode
	eventNodeBase *activity_tree.NodeBase
	imageTag      string
}

// TODO: tie sampleCookieMapSize to the kernel dedup map sizes (open_samples + bind_samples + connect_samples)
// so the cookie LRU can hold mappings for every possible dedup entry.
const sampleCookieMapSize = 4096

const (
	metricSourceRuntime = iota
	metricSourceReplay
	metricSourceRelated
	metricSourceCount
)

// insertionErrorKey identifies an unexpected activity-tree insertion failure by
// event type and a bounded error-type label.
type insertionErrorKey struct {
	eventType model.EventType
	errorType string
}

// perEventTypeMetrics holds the precomputed statsd tags and the counters for a
// (source, event_type) pair.
type perEventTypeMetrics struct {
	tags            []string
	eventsReceived  *atomic.Uint64
	eventsImmediate *atomic.Uint64
	eventsDropped   *atomic.Uint64
}

type ManagerV2 struct {
	config        *config.Config
	statsdClient  statsd.ClientInterface
	resolvers     *resolvers.EBPFResolvers
	kernelVersion *kernel.Version

	sendAnomalyDetection func(*model.Event)

	hostname string

	profiles     map[cgroupModel.WorkloadSelector]*profile.Profile
	profilesLock sync.Mutex
	pathsReducer *activity_tree.PathsReducer

	eventFiltering map[eventFilteringEntry]*atomic.Uint64

	insertionErrors map[insertionErrorKey]*atomic.Uint64

	eventMetrics [metricSourceCount]map[model.EventType]*perEventTypeMetrics

	// storage
	localStorage              *storage.Directory
	remoteStorage             *storage.ActivityDumpRemoteStorageForwarder
	configuredStorageRequests map[config.StorageFormat][]config.StorageRequest

	profilePendingEvents     map[containerutils.CGroupID]*pendingProfile
	profilePendingEventsLock sync.Mutex

	// Metrics counters (gauges that need to be tracked)
	queueSize       *atomic.Uint64 // total events currently queued (gauge)
	pendingProfiles *atomic.Uint64 // cgroups currently waiting for tags

	// Track cgroups with resolved tags (for cgroups_resolved gauge)
	resolvedCgroups     map[containerutils.CGroupID]struct{}
	resolvedCgroupsLock sync.Mutex

	// Pending profile removals (selector -> time when removal was queued)
	pendingProfileRemovals     map[cgroupModel.WorkloadSelector]time.Time
	pendingProfileRemovalsLock sync.Mutex

	// Sample refresh: maps kernel dedup cookie → (process node, event node, imageTag)
	sampleCookieMap       *lru.Cache[uint32, sampleCookieEntry]
	sampleRefreshReceived *atomic.Uint64
	sampleRefreshHits     *atomic.Uint64
	sampleRefreshMisses   *atomic.Uint64

	containerFilters workloadfilter.FilterBundle
	imageExcluder    *imageExcluder
}

func NewManagerV2(cfg *config.Config, statsdClient statsd.ClientInterface, resolvers *resolvers.EBPFResolvers, kernelVersion *kernel.Version, dumpHandler backend.ActivityDumpHandler, sendAnomalyDetection func(*model.Event), hostname string, filterStore workloadfilter.Component) (*ManagerV2, error) {

	localStorage, err := storage.NewDirectory(cfg.RuntimeSecurity.ActivityDumpLocalStorageDirectory, cfg.RuntimeSecurity.ActivityDumpLocalStorageMaxDumpsCount)
	if err != nil {
		return nil, fmt.Errorf("couldn't instantiate the local storage: %w", err)
	}

	remoteStorage, err := storage.NewActivityDumpRemoteStorageForwarder(dumpHandler)
	if err != nil {
		return nil, fmt.Errorf("couldn't instantiate the remote storage forwarder: %w", err)
	}

	var configuredStorageRequests []config.StorageRequest
	for _, format := range cfg.RuntimeSecurity.ActivityDumpLocalStorageFormats {
		configuredStorageRequests = append(configuredStorageRequests, config.NewStorageRequest(
			config.LocalStorage,
			format,
			cfg.RuntimeSecurity.ActivityDumpLocalStorageCompression,
			cfg.RuntimeSecurity.ActivityDumpLocalStorageDirectory,
		))
	}

	configuredStorageRequests = append(configuredStorageRequests, config.NewStorageRequest(
		config.RemoteStorage,
		config.Protobuf,
		true, // force remote compression
		"",
	))

	cookieMap, _ := lru.New[uint32, sampleCookieEntry](sampleCookieMapSize)

	var containerFilter workloadfilter.FilterBundle
	if filterStore != nil {
		containerFilter = filterStore.GetContainerRuntimeSecurityFilters()
		if errs := containerFilter.GetErrors(); len(errs) > 0 {
			return nil, errors.Join(errs...)
		}
	}

	imgExcluder, err := newImageExcluder(cfg.RuntimeSecurity.SecurityProfileV2ExcludedImages)
	if err != nil {
		return nil, fmt.Errorf("couldn't build security profile v2 image excluder: %w", err)
	}

	m := &ManagerV2{
		config:                    cfg,
		statsdClient:              statsdClient,
		resolvers:                 resolvers,
		kernelVersion:             kernelVersion,
		profilePendingEvents:      make(map[containerutils.CGroupID]*pendingProfile),
		queueSize:                 atomic.NewUint64(0),
		pendingProfiles:           atomic.NewUint64(0),
		pathsReducer:              activity_tree.NewPathsReducer(),
		profiles:                  make(map[cgroupModel.WorkloadSelector]*profile.Profile),
		localStorage:              localStorage,
		remoteStorage:             remoteStorage,
		configuredStorageRequests: perFormatStorageRequests(configuredStorageRequests),
		hostname:                  hostname,
		sendAnomalyDetection:      sendAnomalyDetection,
		eventFiltering:            make(map[eventFilteringEntry]*atomic.Uint64),
		insertionErrors:           make(map[insertionErrorKey]*atomic.Uint64),
		resolvedCgroups:           make(map[containerutils.CGroupID]struct{}),
		pendingProfileRemovals:    make(map[cgroupModel.WorkloadSelector]time.Time),
		sampleCookieMap:           cookieMap,
		sampleRefreshReceived:     atomic.NewUint64(0),
		sampleRefreshHits:         atomic.NewUint64(0),
		sampleRefreshMisses:       atomic.NewUint64(0),
		containerFilters:          containerFilter,
		imageExcluder:             imgExcluder,
	}

	m.initMetricsMap()
	m.initEventMetrics()
	return m, nil
}

// initMetricsMap initializes the event filtering metrics map with all combinations of event types, states, and results
func (m *ManagerV2) initMetricsMap() {
	for i := model.EventType(0); i < model.MaxKernelEventType; i++ {
		for _, errorType := range activity_tree.InsertionErrorTypes {
			m.insertionErrors[insertionErrorKey{eventType: i, errorType: errorType}] = atomic.NewUint64(0)
		}
		for _, state := range model.AllEventFilteringProfileState {
			for _, result := range allEventFilteringResults {
				m.eventFiltering[eventFilteringEntry{
					eventType: i,
					state:     state,
					result:    result,
				}] = atomic.NewUint64(0)
			}
		}
	}
}

// initEventMetrics precomputes the {source, event_type} tags and counters for every source and
// every configured profile event type. Event types that are never captured by V2 profiles are
// filtered out in ProcessEvent, so there's no point allocating counters for them.
func (m *ManagerV2) initEventMetrics() {
	sources := [metricSourceCount]model.EventSource{
		metricSourceRuntime: model.EventSourceRuntime,
		metricSourceReplay:  model.EventSourceReplay,
		metricSourceRelated: model.EventSourceRelated,
	}
	eventTypes := m.config.RuntimeSecurity.SecurityProfileV2EventTypes
	for src, sourceName := range sources {
		m.eventMetrics[src] = make(map[model.EventType]*perEventTypeMetrics, len(eventTypes))
		for _, et := range eventTypes {
			m.eventMetrics[src][et] = &perEventTypeMetrics{
				tags:            []string{"source:" + string(sourceName), "event_type:" + et.String()},
				eventsReceived:  atomic.NewUint64(0),
				eventsImmediate: atomic.NewUint64(0),
				eventsDropped:   atomic.NewUint64(0),
			}
		}
	}
}

// eventMetricsFor returns the metric counters for the given source/event type, or nil if the
// event type isn't one of the configured profile event types. The three known sources (runtime,
// replay, related) each map to their own shard so related traffic isn't misreported as runtime.
func (m *ManagerV2) eventMetricsFor(source model.EventSource, et model.EventType) *perEventTypeMetrics {
	src := metricSourceRuntime
	switch source {
	case model.EventSourceReplay:
		src = metricSourceReplay
	case model.EventSourceRelated:
		src = metricSourceRelated
	}
	return m.eventMetrics[src][et]
}

func (m *ManagerV2) Start(ctx context.Context) {
	sendTickerChan := m.setupPersistenceTicker()
	nodeEvictionTickerChan := m.setupNodeEvictionTicker()
	profileCleanupTickerChan := m.setupProfileCleanupTicker()
	stalePurgeTickerChan := m.setupStalePurgeTicker()

	// Register listener for cgroup deletions to track active cgroups
	if err := m.resolvers.CGroupResolver.RegisterListener(cgroup.CGroupDeleted, m.onCGroupDeleted); err != nil {
		seclog.Errorf("failed to register cgroup deletion listener: %v", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	seclog.Infof("security profile manager v2 started")

	for {
		select {
		case <-ctx.Done():
			return
		case <-sendTickerChan:
			m.persistAllProfiles()
		case <-nodeEvictionTickerChan:
			m.evictUnusedNodes()
		case <-profileCleanupTickerChan:
			m.cleanupPendingProfiles()
		case <-stalePurgeTickerChan:
			m.purgeStalePendingEvents(time.Now())
		}
	}
}

// setupPersistenceTicker creates the ticker channel for periodic profile persistence
func (m *ManagerV2) setupPersistenceTicker() <-chan time.Time {
	if !m.config.RuntimeSecurity.SecurityProfileEnabled {
		return make(chan time.Time)
	}

	return time.NewTicker(m.config.RuntimeSecurity.ActivityDumpCgroupDumpTimeout).C
}

// setupNodeEvictionTicker creates the ticker channel for periodic node eviction
func (m *ManagerV2) setupNodeEvictionTicker() <-chan time.Time {
	if !m.config.RuntimeSecurity.SecurityProfileEnabled || m.config.RuntimeSecurity.SecurityProfileNodeEvictionTimeout <= 0 {
		return make(chan time.Time)
	}

	return time.NewTicker(m.config.RuntimeSecurity.SecurityProfileNodeEvictionTimeout).C
}

// setupProfileCleanupTicker creates the ticker channel for periodic profile cleanup
func (m *ManagerV2) setupProfileCleanupTicker() <-chan time.Time {
	if !m.config.RuntimeSecurity.SecurityProfileEnabled || m.config.RuntimeSecurity.SecurityProfileCleanupDelay <= 0 {
		return make(chan time.Time)
	}

	// Check every minute for profiles that need to be cleaned up
	return time.NewTicker(1 * time.Minute).C
}

// setupStalePurgesTicker creates the ticker channel for periodic purge of stale pending events
func (m *ManagerV2) setupStalePurgeTicker() <-chan time.Time {
	if !m.config.RuntimeSecurity.SecurityProfileEnabled {
		return make(chan time.Time)
	}

	// Purge stale pending events every 10 seconds
	return time.NewTicker(10 * time.Second).C
}

// onCGroupDeleted is called when a cgroup is deleted from the system
func (m *ManagerV2) onCGroupDeleted(cgce *cgroupModel.CacheEntry) {
	cgroupID := cgce.GetCGroupID()

	// Remove from resolvedCgroups
	m.resolvedCgroupsLock.Lock()
	delete(m.resolvedCgroups, cgroupID)
	m.resolvedCgroupsLock.Unlock()

	// Find and unlink this workload from its profile
	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()
	for selector, prof := range m.profiles {
		if removed, remainingInstances := m.unlinkWorkloadFromProfile(prof, cgce); removed {
			if remainingInstances == 0 {
				// Queue for delayed removal
				m.pendingProfileRemovalsLock.Lock()
				if _, alreadyPending := m.pendingProfileRemovals[selector]; !alreadyPending {
					m.pendingProfileRemovals[selector] = time.Now()
					seclog.Debugf("queued profile [%s] for delayed removal", selector.String())
				}
				m.pendingProfileRemovalsLock.Unlock()
			}
			break
		}
	}
}

// cleanupPendingProfiles removes profiles that have been pending removal for longer than the cleanup delay
func (m *ManagerV2) cleanupPendingProfiles() {
	cleanupDelay := m.config.RuntimeSecurity.SecurityProfileCleanupDelay
	if cleanupDelay <= 0 {
		return
	}

	now := time.Now()

	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()

	m.pendingProfileRemovalsLock.Lock()
	defer m.pendingProfileRemovalsLock.Unlock()

	for selector, queuedAt := range m.pendingProfileRemovals {
		if now.Sub(queuedAt) < cleanupDelay {
			continue
		}

		prof := m.profiles[selector]
		if prof == nil {
			delete(m.pendingProfileRemovals, selector)
			continue
		}

		if m.profileHasActiveInstances(prof) {
			seclog.Debugf("profile [%s] has regained active instances, skipping removal", selector.String())
			continue
		}

		seclog.Infof("removing profile [%s] after cleanup delay", selector.String())
		m.purgeCookiesForProfile(prof)
		delete(m.profiles, selector)
		delete(m.pendingProfileRemovals, selector)

		if err := m.statsdClient.Count(metrics.MetricSecurityProfileV2CleanupProfilesRemoved, 1, []string{}, 1.0); err != nil {
			seclog.Warnf("couldn't send %s metric: %v", metrics.MetricSecurityProfileV2CleanupProfilesRemoved, err)
		}
	}
}

// profileHasActiveInstances checks if a profile has any active workload instances
func (m *ManagerV2) profileHasActiveInstances(prof *profile.Profile) bool {
	prof.InstancesLock.Lock()
	defer prof.InstancesLock.Unlock()
	return len(prof.Instances) > 0
}

// persistAllProfiles encodes and persists all profiles to configured storage backends
func (m *ManagerV2) persistAllProfiles() {
	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()

	for _, p := range m.profiles {
		m.persistProfile(p)
	}
}

// persistProfile encodes and persists a single profile to all configured storage backends
func (m *ManagerV2) persistProfile(p *profile.Profile) {
	enabled := p.IsEnabled()

	encoded := make(map[config.StorageFormat]*bytes.Buffer)
	for format, requests := range m.configuredStorageRequests {
		for _, request := range requests {
			if !enabled && request.Type != config.LocalStorage {
				continue
			}

			data, ok := encoded[format]
			if !ok {
				var err error
				data, err = p.Encode(format)
				if err != nil {
					seclog.Errorf("couldn't encode profile [%s] to %s format: %v", p.GetSelectorStr(), format, err)
					break
				}
				encoded[format] = data
			}

			m.persistProfileToStorage(p, request, data)
		}
	}

	if enabled {
		p.SetHasAlreadyBeenSent()
	}
}

// persistProfileToStorage persists profile data to a specific storage backend
// should we send profiles that have not changed ? Just setting a proper time interval should be enough ?
func (m *ManagerV2) persistProfileToStorage(p *profile.Profile, request config.StorageRequest, data *bytes.Buffer) {
	var storageBackend storage.ActivityDumpStorage
	switch request.Type {
	case config.LocalStorage:
		storageBackend = m.localStorage
	case config.RemoteStorage:
		storageBackend = m.remoteStorage
	default:
		seclog.Errorf("couldn't persist [%s]: unknown storage type: %s", p.GetSelectorStr(), request.Type)
		return
	}

	if err := storageBackend.Persist(request, p, data); err != nil {
		seclog.Errorf("couldn't persist [%s] to %s storage: %v", p.GetSelectorStr(), request.Type, err)
		return
	}

	m.sendPersistenceMetrics(request, data.Len())
}

// sendPersistenceMetrics sends metrics after successful profile persistence
func (m *ManagerV2) sendPersistenceMetrics(request config.StorageRequest, dataSize int) {
	tags := []string{
		"format:" + request.Format.String(),
		"storage_type:" + request.Type.String(),
		"compression:" + strconv.FormatBool(request.Compression),
	}

	if err := m.statsdClient.Count(metrics.MetricSecurityProfileV2SizeInBytes, int64(dataSize), tags, 1.0); err != nil {
		seclog.Warnf("couldn't send %s metric: %v", metrics.MetricSecurityProfileV2SizeInBytes, err)
	}
	if err := m.statsdClient.Count(metrics.MetricSecurityProfileV2PersistedProfiles, 1, tags, 1.0); err != nil {
		seclog.Warnf("couldn't send %s metric: %v", metrics.MetricSecurityProfileV2PersistedProfiles, err)
	}
}

func (m *ManagerV2) ProcessEvent(event *model.Event) {

	// Filter out events that are not in the configured V2 event types
	if !slices.Contains(m.config.RuntimeSecurity.SecurityProfileV2EventTypes, model.EventType(event.Type)) {
		return
	}

	// Filter out systemd cgroups for now, we will add support for them later
	if event.ProcessContext.Process.ContainerContext.IsNull() {
		return
	}

	workloadID := getWorkloadIDFromEvent(event)
	if workloadID == nil {
		return
	}

	// Resolve event source and look up its precomputed (source, event_type) counters.
	source := event.FieldHandlers.ResolveSource(event, &event.BaseEvent)
	em := m.eventMetricsFor(source, model.EventType(event.Type))

	if em != nil {
		em.eventsReceived.Inc()
	}

	// Try to resolve tags for this workload
	workloadTags, err := m.resolvers.TagsResolver.ResolveWithErr(workloadID)
	tagsResolved := err == nil && len(workloadTags) != 0 && utils.GetTagValue("image_tag", workloadTags) != ""

	if tagsResolved {
		// Set resolved tags on the event for downstream processing
		event.ProcessContext.Process.ContainerContext.Tags = workloadTags

		if em != nil {
			em.eventsImmediate.Inc()
		}
		m.processEventWithResolvedTags(event)
	} else {
		m.queueEventForTagResolution(event, em)
	}
}

// purgeStalePendingEvents removes pending entries that have been waiting for tags for more than 60 seconds
func (m *ManagerV2) purgeStalePendingEvents(currentTimestamp time.Time) {
	m.profilePendingEventsLock.Lock()
	defer m.profilePendingEventsLock.Unlock()

	for cgroupID, pendingEvents := range m.profilePendingEvents {
		if currentTimestamp.Sub(pendingEvents.firstSeen) > 60*time.Second {
			// Decrement queue size by the number of events being dropped
			eventsLen := pendingEvents.events.Len()
			if eventsLen > 0 {
				m.queueSize.Sub(uint64(eventsLen))
				// Emit dropped events metric (source unknown for queued events)
				if err := m.statsdClient.Count(metrics.MetricSecurityProfileV2TagResolutionEventsDropped, int64(eventsLen), []string{}, 1.0); err != nil {
					seclog.Warnf("couldn't send %s metric: %v", metrics.MetricSecurityProfileV2TagResolutionEventsDropped, err)
				}
			}

			delete(m.profilePendingEvents, cgroupID)
			m.pendingProfiles.Dec()

			// Emit metric for expired cgroup
			if err := m.statsdClient.Count(metrics.MetricSecurityProfileV2TagResolutionCgroupsExpired, 1, []string{}, 1.0); err != nil {
				seclog.Warnf("couldn't send %s metric: %v", metrics.MetricSecurityProfileV2TagResolutionCgroupsExpired, err)
			}
		}
	}
}

// processEventWithResolvedTags handles events that have their tags resolved.
// It also dequeues and processes any pending events for the same cgroup.
func (m *ManagerV2) processEventWithResolvedTags(event *model.Event) {
	cgroupID := event.ProcessContext.Process.CGroup.CGroupID

	// Track cgroups with resolved tags (for cgroups_resolved gauge)
	m.resolvedCgroupsLock.Lock()
	m.resolvedCgroups[cgroupID] = struct{}{}
	m.resolvedCgroupsLock.Unlock()

	// Dequeue and process pending events if any exist for this cgroup
	m.profilePendingEventsLock.Lock()
	if pendingEvents := m.profilePendingEvents[cgroupID]; pendingEvents != nil {
		// Track tag resolution latency (time from first event to successful resolution)
		latency := time.Since(pendingEvents.firstSeen)
		if err := m.statsdClient.Distribution(metrics.MetricSecurityProfileV2TagResolutionLatency, latency.Seconds(), []string{}, 1.0); err != nil {
			seclog.Warnf("couldn't send %s metric: %v", metrics.MetricSecurityProfileV2TagResolutionLatency, err)
		}

		for e := pendingEvents.events.Front(); e != nil; e = e.Next() {
			queuedEvent := e.Value.(*model.Event)
			// Copy resolved tags to queued event since it was queued before tags were available
			queuedEvent.ProcessContext.Process.ContainerContext.Tags = event.ProcessContext.Process.ContainerContext.Tags
			m.onEventTagsResolved(queuedEvent)
		}
		m.queueSize.Sub(uint64(pendingEvents.events.Len()))
		m.pendingProfiles.Dec()
		delete(m.profilePendingEvents, cgroupID)
	}
	m.profilePendingEventsLock.Unlock()

	// Process the current event
	m.onEventTagsResolved(event)
}

// queueEventForTagResolution queues an event while waiting for tag resolution
func (m *ManagerV2) queueEventForTagResolution(event *model.Event, em *perEventTypeMetrics) {
	cgroupID := event.ProcessContext.Process.CGroup.CGroupID

	m.profilePendingEventsLock.Lock()
	defer m.profilePendingEventsLock.Unlock()

	pendingEvents := m.profilePendingEvents[cgroupID]

	// Create pending entry if it doesn't exist
	if pendingEvents == nil {
		pendingEvents = &pendingProfile{
			firstSeen: event.Timestamp,
			events:    list.New(),
		}
		m.profilePendingEvents[cgroupID] = pendingEvents
		m.pendingProfiles.Inc()
	}

	// Check if event is too old (>10s since first event for this cgroup)
	// If so, drop this event and clear the queue - stale events won't be processed
	event.ResolveEventTime()
	if event.Timestamp.Sub(pendingEvents.firstSeen) > 10*time.Second {
		if eventsLen := pendingEvents.events.Len(); eventsLen > 0 {
			// Decrement queue size BEFORE clearing the list
			m.queueSize.Sub(uint64(eventsLen))
			pendingEvents.events.Init()
			if em != nil {
				em.eventsDropped.Add(uint64(eventsLen))
			}
		}
		return
	}

	// Queue the event (deep copy to preserve state)
	event.ResolveFieldsForAD()
	cpy := event.DeepCopy()
	pendingEvents.events.PushBack(cpy)
	m.queueSize.Inc()
}

// onEventTagsResolved is called when an event has its tags resolved and is ready to be inserted into a profile
func (m *ManagerV2) onEventTagsResolved(event *model.Event) {
	profile, inserted := m.insertEventIntoProfile(event)
	if !inserted || profile == nil || !profile.HasAlreadyBeenSent() {
		return
	}

	workloadID := getWorkloadIDFromEvent(event)
	var imageTag string

	if !event.ProcessContext.Process.ContainerContext.IsNull() {
		imageTag = utils.GetTagValue("image_tag", event.ProcessContext.Process.ContainerContext.Tags)
	} else if event.ProcessContext.Process.CGroup.IsResolved() {
		tags, err := m.resolvers.TagsResolver.ResolveWithErr(workloadID)
		if err != nil {
			seclog.Errorf("failed to resolve tags for cgroup %s: %v", workloadID, err)
			return
		}
		imageTag = utils.GetTagValue("version", tags)
	}

	if workloadID != nil {
		m.FillProfileContextFromWorkloadID(workloadID, &event.SecurityProfileContext, imageTag)
	}

	if m.config.RuntimeSecurity.AnomalyDetectionEnabled {
		m.sendAnomalyDetection(event)
	}
}

func (m *ManagerV2) SendStats() error {

	// Tag resolution gauges
	if err := m.statsdClient.Gauge(metrics.MetricSecurityProfileV2TagResolutionEventsQueued, float64(m.queueSize.Load()), []string{}, 1.0); err != nil {
		return err
	}
	if err := m.statsdClient.Gauge(metrics.MetricSecurityProfileV2TagResolutionCgroupsPending, float64(m.pendingProfiles.Load()), []string{}, 1.0); err != nil {
		return err
	}

	// Current cgroups with resolved tags (gauge of actively profiled cgroups)
	m.resolvedCgroupsLock.Lock()
	numResolved := len(m.resolvedCgroups)
	m.resolvedCgroupsLock.Unlock()
	if err := m.statsdClient.Gauge(metrics.MetricSecurityProfileV2TagResolutionCgroupsResolved, float64(numResolved), []string{}, 1.0); err != nil {
		return err
	}

	var tags [][]string
	m.profilesLock.Lock()
	for selector, prof := range m.profiles {
		if prof.IsEnabled() {
			continue
		}
		tags = append(tags, []string{"profile_image_name:" + selector.Image, "profile_image_tag:" + selector.Tag})
	}
	m.profilesLock.Unlock()

	for _, tag := range tags {
		if err := m.statsdClient.Gauge(metrics.MetricSecurityProfileV2DisabledProfiles, 1, tag, 1.0); err != nil {
			return err
		}
	}

	// Sample refresh metrics
	if value := m.sampleRefreshReceived.Swap(0); value > 0 {
		if err := m.statsdClient.Count(metrics.MetricSecurityProfileV2SampleRefreshReceived, int64(value), []string{}, 1.0); err != nil {
			return err
		}
	}
	if value := m.sampleRefreshHits.Swap(0); value > 0 {
		if err := m.statsdClient.Count(metrics.MetricSecurityProfileV2SampleRefreshHits, int64(value), []string{}, 1.0); err != nil {
			return err
		}
	}
	if value := m.sampleRefreshMisses.Swap(0); value > 0 {
		if err := m.statsdClient.Count(metrics.MetricSecurityProfileV2SampleRefreshMisses, int64(value), []string{}, 1.0); err != nil {
			return err
		}
	}

	// Event filtering metrics
	for entry, count := range m.eventFiltering {
		tags := []string{
			"event_type:" + entry.eventType.String(),
			entry.state.ToTag(),
			entry.result.toTag(),
		}
		if value := count.Swap(0); value > 0 {
			if err := m.statsdClient.Count(metrics.MetricSecurityProfileEventFiltering, int64(value), tags, 1.0); err != nil {
				return err
			}
		}
	}

	// Activity-tree insertion errors (unexpected failures only)
	for key, count := range m.insertionErrors {
		if value := count.Swap(0); value > 0 {
			tags := []string{"event_type:" + key.eventType.String(), "error_type:" + key.errorType}
			if err := m.statsdClient.Count(metrics.MetricSecurityProfileV2InsertionErrors, int64(value), tags, 1.0); err != nil {
				return err
			}
		}
	}

	// Per-(source, event_type) event counters.
	for src := range m.eventMetrics {
		for _, em := range m.eventMetrics[src] {
			if value := em.eventsReceived.Swap(0); value > 0 {
				if err := m.statsdClient.Count(metrics.MetricSecurityProfileV2EventsReceived, int64(value), em.tags, 1.0); err != nil {
					return err
				}
			}
			if value := em.eventsImmediate.Swap(0); value > 0 {
				if err := m.statsdClient.Count(metrics.MetricSecurityProfileV2EventsImmediate, int64(value), em.tags, 1.0); err != nil {
					return err
				}
			}
			if value := em.eventsDropped.Swap(0); value > 0 {
				if err := m.statsdClient.Count(metrics.MetricSecurityProfileV2TagResolutionEventsDropped, int64(value), em.tags, 1.0); err != nil {
					return err
				}
			}
		}
	}

	// Per-profile size (RAM and disk reported under the same metric, differentiated by storage tag).
	// Snapshot the active-profiles map under the lock and then call ComputeHeapSize / statsd
	// outside it — ComputeHeapSize takes the per-profile lock, and we don't want to hold
	// profilesLock across that or across the statsd send.
	type profileEntry struct {
		selector cgroupModel.WorkloadSelector
		profile  *profile.Profile
	}
	m.profilesLock.Lock()
	ramEntries := make([]profileEntry, 0, len(m.profiles))
	for selector, p := range m.profiles {
		ramEntries = append(ramEntries, profileEntry{selector: selector, profile: p})
	}
	m.profilesLock.Unlock()

	for _, entry := range ramEntries {
		tags := []string{
			"profile_image_name:" + entry.selector.Image,
			"profile_image_tag:" + entry.selector.Tag,
			"storage:ram",
		}
		if err := m.statsdClient.Gauge(metrics.MetricSecurityProfileV2ProfileSize, float64(entry.profile.ComputeHeapSize()), tags, 1.0); err != nil {
			return err
		}
	}

	for selector, size := range m.localStorage.SizesBySelector() {
		// Skip entries with an unresolved selector: the on-disk file is tracked for cleanup
		// but emitting metrics with empty image/tag tags pollutes cardinality without adding signal.
		if !selector.IsReady() {
			continue
		}
		tags := []string{
			"profile_image_name:" + selector.Image,
			"profile_image_tag:" + selector.Tag,
			"storage:disk",
		}
		if err := m.statsdClient.Gauge(metrics.MetricSecurityProfileV2ProfileSize, float64(size), tags, 1.0); err != nil {
			return err
		}
	}

	return nil
}

// insertEventIntoProfile gets or creates a profile for the workload and inserts the event into its ActivityTree.
// Returns the profile and whether the event was actually inserted (new data added).
func (m *ManagerV2) insertEventIntoProfile(event *model.Event) (*profile.Profile, bool) {
	if !m.config.RuntimeSecurity.SecurityProfileEnabled {
		return nil, false
	}

	// Build selector from event tags
	selector, err := m.buildWorkloadSelector(event)
	if err != nil {
		return nil, false
	}

	// Cancel any pending removal for this selector (it's back!)
	m.pendingProfileRemovalsLock.Lock()
	if _, pending := m.pendingProfileRemovals[selector]; pending {
		delete(m.pendingProfileRemovals, selector)
		seclog.Debugf("cancelled pending removal for profile [%s] - selector reappeared", selector.String())
	}
	m.pendingProfileRemovalsLock.Unlock()

	// Get or create the profile for this workload
	secprof, err := m.getOrCreateProfile(selector, event)
	if err != nil {
		return nil, false
	}
	if !secprof.IsEnabled() {
		m.incrementEventFilteringStat(event.GetEventType(), model.ProfileAtMaxSize, NA)
		return nil, false
	}

	// Build workloadID for cache entry lookup
	workloadID := getWorkloadIDFromEvent(event)

	// Link this workload to the profile (tracks in profile.Instances)
	workload := m.getOrCreateWorkload(event, selector, workloadID)
	m.linkWorkloadToProfile(secprof, workload)

	// Check if profile has reached max size. V2 uses its own knob evaluated against the
	// accurate heap footprint — V1's activity_dump.max_dump_size keeps its legacy shallow
	// semantics for ActivityDump/legacy Manager paths.
	// TODO: we should handle this in a better way

	if secprof.ComputeHeapSize() >= int64(m.config.RuntimeSecurity.SecurityProfileV2MaxDumpSize()) {
		secprof.Disable()
		seclog.Infof("Activity dump of %s was stopped because it reached the maximum allowed size of %d.", secprof.GetSelectorStr(), int64(m.config.RuntimeSecurity.SecurityProfileV2MaxDumpSize()))
		m.incrementEventFilteringStat(event.GetEventType(), model.ProfileAtMaxSize, NA)
		return nil, false
	}

	// Ensure version context exists for this selector
	m.ensureVersionContext(secprof, selector.Tag)

	// Insert the event into the profile's activity tree
	imageTag := secprof.GetTagValue("image_tag")
	inserted, processNode, eventNodeBase, err := secprof.Insert(event, true, imageTag, activity_tree.Runtime, m.resolvers)
	if err != nil {
		if !activity_tree.IsExpectedFilterError(err) {
			m.incrementInsertionError(event.GetEventType(), err)
			seclog.Debugf("couldn't insert event into profile: %v", err)
		}
		return nil, false
	}

	// Register the sample cookie → (process node, event node) mapping for sample refresh events
	if processNode != nil {
		var sampleCookie uint32
		switch event.GetEventType() {
		case model.FileOpenEventType:
			sampleCookie = event.Open.SampleCookie
		case model.BindEventType:
			sampleCookie = event.Bind.SampleCookie
		case model.ConnectEventType:
			sampleCookie = event.Connect.SampleCookie
		}
		if sampleCookie != 0 {
			m.sampleCookieMap.Add(sampleCookie, sampleCookieEntry{
				profile:       secprof,
				processNode:   processNode,
				eventNodeBase: eventNodeBase,
				imageTag:      imageTag,
			})
		}
	}

	return secprof, inserted
}

// buildWorkloadSelector creates a workload selector from the event's container tags
func (m *ManagerV2) buildWorkloadSelector(event *model.Event) (cgroupModel.WorkloadSelector, error) {
	imageName := utils.GetTagValue("image_name", event.ProcessContext.Process.ContainerContext.Tags)
	return cgroupModel.NewWorkloadSelector(imageName, "*")
}

// getWorkloadIDFromEvent extracts the workload ID from an event, preferring container ID over cgroup ID
func getWorkloadIDFromEvent(event *model.Event) containerutils.WorkloadID {
	if !event.ProcessContext.Process.ContainerContext.IsNull() {
		return event.ProcessContext.Process.ContainerContext.ContainerID
	}
	if event.ProcessContext.Process.CGroup.IsResolved() {
		return event.ProcessContext.Process.CGroup.CGroupID
	}
	return nil
}

// getOrCreateWorkload creates a workload object from an event for tracking in profile Instances
func (m *ManagerV2) getOrCreateWorkload(event *model.Event, selector cgroupModel.WorkloadSelector, workloadID containerutils.WorkloadID) *tags.Workload {
	var cacheEntry *cgroupModel.CacheEntry

	switch id := workloadID.(type) {
	case containerutils.ContainerID:
		cacheEntry = m.resolvers.CGroupResolver.GetCacheEntryContainerID(id)
	case containerutils.CGroupID:
		cacheEntry = m.resolvers.CGroupResolver.GetCacheEntryByCgroupID(id)
	}

	if cacheEntry == nil {
		return nil
	}

	return &tags.Workload{
		GCroupCacheEntry: cacheEntry,
		Selector:         selector,
		Tags:             event.ProcessContext.Process.ContainerContext.Tags,
	}
}

// linkWorkloadToProfile adds a workload to a profile's Instances if not already tracked
func (m *ManagerV2) linkWorkloadToProfile(prof *profile.Profile, workload *tags.Workload) {
	if workload == nil {
		return
	}

	prof.InstancesLock.Lock()
	defer prof.InstancesLock.Unlock()

	// Check if already tracked
	workloadID := workload.GetWorkloadID()
	for _, w := range prof.Instances {
		if w.GetWorkloadID() == workloadID {
			return
		}
	}

	prof.Instances = append(prof.Instances, workload)
}

// unlinkWorkloadFromProfile removes a workload from a profile's Instances
// Returns (removed, remainingInstances) - whether the workload was found and removed, and the remaining instance count
func (m *ManagerV2) unlinkWorkloadFromProfile(prof *profile.Profile, cgce *cgroupModel.CacheEntry) (bool, int) {
	prof.InstancesLock.Lock()
	defer prof.InstancesLock.Unlock()

	var workloadID containerutils.WorkloadID
	if cgce.IsContainerContextNull() {
		workloadID = cgce.GetContainerID()
	} else if cgce.IsCGroupContextResolved() {
		workloadID = cgce.GetCGroupID()
	} else {
		return false, len(prof.Instances)
	}

	for i, w := range prof.Instances {
		if w.GetWorkloadID() == workloadID {
			prof.Instances = slices.Delete(prof.Instances, i, i+1)
			return true, len(prof.Instances)
		}
	}
	return false, len(prof.Instances)
}

// getOrCreateProfile retrieves an existing profile or creates a new one for the given selector.
// It first tries to load the profile from local storage, and if not found, creates a new one.
func (m *ManagerV2) getOrCreateProfile(selector cgroupModel.WorkloadSelector, event *model.Event) (*profile.Profile, error) {
	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()

	secprof := m.profiles[selector]
	if secprof != nil {
		return secprof, nil
	}

	containerName, imageName, podNamespace := utils.GetContainerFilterTags(event.ProcessContext.Process.ContainerContext.Tags)
	if m.containerFilters != nil && m.containerFilters.IsExcluded(workloadfilter.CreateContainer("", containerName, imageName, workloadfilter.CreatePod("", "", podNamespace, nil, nil))) {
		seclog.Debugf("workload %s excluded by container filter (container=%s image=%s namespace=%s)", selector.String(), containerName, imageName, podNamespace)
		return nil, errors.New("workload excluded")
	}

	imageTag := utils.GetTagValue("image_tag", event.ProcessContext.Process.ContainerContext.Tags)
	if m.imageExcluder.IsExcluded(imageName, imageTag) {
		seclog.Debugf("workload %s excluded by image filter (image=%s tag=%s)", selector.String(), imageName, imageTag)
		return nil, errors.New("workload excluded")
	}

	// Try to load from local storage first
	secprof, loaded := m.loadProfileFromStorage(selector, event)
	if !loaded {
		// Create a new profile if not found in storage
		var err error
		secprof, err = m.createNewProfile(selector, event)
		if err != nil {
			return nil, err
		}
		seclog.Debugf("created new profile for selector %s", selector.String())
	} else {
		seclog.Debugf("loaded profile from storage for selector %s", selector.String())
	}

	m.profiles[selector] = secprof
	return secprof, nil
}

// loadProfileFromStorage attempts to load a profile from local storage.
// Returns the loaded profile and true if successful, otherwise nil and false.
func (m *ManagerV2) loadProfileFromStorage(selector cgroupModel.WorkloadSelector, event *model.Event) (*profile.Profile, bool) {
	// Create a base profile with the required options
	secprof := profile.New(
		profile.WithPathsReducer(m.pathsReducer),
		profile.WithDifferentiateArgs(m.config.RuntimeSecurity.ActivityDumpCgroupDifferentiateArgs),
		profile.WithDNSMatchMaxDepth(m.config.RuntimeSecurity.SecurityProfileDNSMatchMaxDepth),
		profile.WithEventTypes(m.config.RuntimeSecurity.SecurityProfileV2EventTypes),
		profile.WithWorkloadSelector(selector),
	)

	// Try to load from local storage
	ok, err := m.localStorage.Load(&selector, secprof)
	if err != nil {
		seclog.Warnf("couldn't load profile from local storage: %v", err)
		return nil, false
	}
	if !ok {
		return nil, false
	}

	// Profile was loaded successfully; recompute stats so SizeBytes reflects the loaded tree.
	secprof.ActivityTree.ComputeActivityTreeStats()
	secprof.SetTreeType(secprof, "security_profile")

	// Update metadata with current event context for proper matching
	secprof.Metadata.ContainerID = event.ProcessContext.Process.ContainerContext.ContainerID
	secprof.Metadata.CGroupContext = event.ProcessContext.Process.CGroup

	// Apply eviction right away if configured
	if m.config.RuntimeSecurity.SecurityProfileNodeEvictionTimeout > 0 {
		workloadID := getWorkloadIDFromEvent(event)
		containersOnly := !m.config.RuntimeSecurity.ActivityDumpTraceSystemdCgroups
		filepathsInProcessCache := m.GetNodesInProcessCache(workloadID, containersOnly)
		evicted := secprof.ActivityTree.EvictUnusedNodes(
			time.Now().Add(-m.config.RuntimeSecurity.SecurityProfileNodeEvictionTimeout),
			filepathsInProcessCache,
			selector.Image,
			selector.Tag,
		)
		if evicted > 0 {
			seclog.Debugf("evicted %d unused nodes from loaded profile [%s]", evicted, selector.String())
		}
	}

	return secprof, true
}

// createNewProfile initializes a new profile with all required metadata and tags
func (m *ManagerV2) createNewProfile(selector cgroupModel.WorkloadSelector, event *model.Event) (*profile.Profile, error) {
	secprof := profile.New(
		profile.WithPathsReducer(m.pathsReducer),
		profile.WithDifferentiateArgs(m.config.RuntimeSecurity.ActivityDumpCgroupDifferentiateArgs),
		profile.WithDNSMatchMaxDepth(m.config.RuntimeSecurity.SecurityProfileDNSMatchMaxDepth),
		profile.WithEventTypes(m.config.RuntimeSecurity.SecurityProfileV2EventTypes),
		profile.WithWorkloadSelector(selector),
	)
	secprof.SetTreeType(secprof, "security_profile")

	eventTime := event.Timestamp
	if eventTime.IsZero() {
		eventTime = time.Now()
	}

	// Initialize metadata
	secprof.Metadata = mtdt.Metadata{
		AgentVersion:      version.AgentVersion,
		AgentCommit:       version.Commit,
		KernelVersion:     m.kernelVersion.Code.String(),
		LinuxDistribution: m.kernelVersion.OsRelease["PRETTY_NAME"],
		Arch:              utils.RuntimeArch(),
		Name:              "activity-dump-" + utils.RandString(10),
		ProtobufVersion:   profile.ProtobufVersion,
		DifferentiateArgs: m.config.RuntimeSecurity.ActivityDumpCgroupDifferentiateArgs,
		ContainerID:       event.ProcessContext.Process.ContainerContext.ContainerID,
		CGroupContext:     event.ProcessContext.Process.CGroup,
		Start:             eventTime,
		End:               eventTime,
	}
	secprof.Header.Host = m.hostname
	secprof.Header.Source = ActivityDumpSource

	// Resolve and add tags
	if err := m.resolveAndAddProfileTags(secprof); err != nil {
		return nil, err
	}

	return secprof, nil
}

// resolveAndAddProfileTags resolves tags for the profile's workload and adds them to the profile
func (m *ManagerV2) resolveAndAddProfileTags(secprof *profile.Profile) error {
	var workloadID containerutils.WorkloadID
	if len(secprof.Metadata.ContainerID) > 0 {
		workloadID = containerutils.ContainerID(secprof.Metadata.ContainerID)
	} else if len(secprof.Metadata.CGroupContext.CGroupID) > 0 {
		workloadID = secprof.Metadata.CGroupContext.CGroupID
	}

	if workloadID == nil {
		return nil
	}

	tags, err := m.resolvers.TagsResolver.ResolveWithErr(workloadID)
	if err != nil {
		return err
	}
	secprof.AddTags(tags)
	return nil
}

// ensureVersionContext creates a version context for the given tag if it doesn't exist
func (m *ManagerV2) ensureVersionContext(secprof *profile.Profile, tag string) {
	if _, ok := secprof.GetVersionContext(tag); ok {
		return
	}

	now := time.Now()
	nowNano := uint64(m.resolvers.TimeResolver.ComputeMonotonicTimestamp(now))
	profileTags := secprof.GetTags()

	vCtx := &profile.VersionContext{
		FirstSeenNano:  nowNano,
		LastSeenNano:   nowNano,
		EventTypeState: make(map[model.EventType]*profile.EventTypeState),
		Syscalls:       secprof.ComputeSyscallsList(),
		Tags:           make([]string, len(profileTags)),
	}
	copy(vCtx.Tags, profileTags)

	secprof.AddVersionContext(tag, vCtx)
}

// FillProfileContextFromWorkloadID fills the given ctx with workload id infos
func (m *ManagerV2) FillProfileContextFromWorkloadID(id containerutils.WorkloadID, ctx *model.SecurityProfileContext, imageTag string) {
	if !m.config.RuntimeSecurity.SecurityProfileEnabled {
		return
	}

	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()

	// Iterate through profiles and their instances to find matching workload
	for _, prof := range m.profiles {
		prof.InstancesLock.Lock()
		for _, instance := range prof.Instances {
			instance.Lock()
			if instance.GetWorkloadID() == id {
				ctx.Name = prof.Metadata.Name
				if profileContext, ok := prof.GetVersionContext(imageTag); ok {
					ctx.Tags = profileContext.Tags
				}
				instance.Unlock()
				prof.InstancesLock.Unlock()
				return
			}
			instance.Unlock()
		}
		prof.InstancesLock.Unlock()
	}
}

func (m *ManagerV2) incrementEventFilteringStat(eventType model.EventType, state model.EventFilteringProfileState, result EventFilteringResult) {
	if entry, ok := m.eventFiltering[eventFilteringEntry{eventType, state, result}]; ok {
		entry.Inc()
	}
}

// incrementInsertionError records an unexpected activity-tree insertion failure for the given
// event type, categorizing err into a bounded error-type label.
func (m *ManagerV2) incrementInsertionError(eventType model.EventType, err error) {
	key := insertionErrorKey{eventType: eventType, errorType: activity_tree.InsertionErrorType(err)}
	if entry, ok := m.insertionErrors[key]; ok {
		entry.Inc()
	}
}

// evictUnusedNodes performs periodic eviction of non-touched nodes from all active profiles
func (m *ManagerV2) evictUnusedNodes() {
	// Emit eviction run metric
	if err := m.statsdClient.Count(metrics.MetricSecurityProfileV2EvictionRuns, 1, []string{}, 1.0); err != nil {
		seclog.Warnf("couldn't send %s metric: %v", metrics.MetricSecurityProfileV2EvictionRuns, err)
	}

	evictionTime := time.Now().Add(-m.config.RuntimeSecurity.SecurityProfileNodeEvictionTimeout)
	totalEvicted := 0

	containersOnly := !m.config.RuntimeSecurity.ActivityDumpTraceSystemdCgroups
	filepathsInProcessCache := m.GetNodesInProcessCache(nil, containersOnly)

	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()

	for selector, profile := range m.profiles {
		if profile == nil || !profile.IsEnabled() {
			continue
		}

		profile.Lock()
		if profile.ActivityTree == nil {
			profile.Unlock()
			continue
		}
		evicted := profile.ActivityTree.EvictUnusedNodes(evictionTime, filepathsInProcessCache, selector.Image, selector.Tag)
		if evicted > 0 {
			totalEvicted += evicted
			seclog.Debugf("evicted %d unused process nodes from profile [%s] ", evicted, selector.String())

			// Emit per-profile eviction metric
			if err := m.statsdClient.Count(metrics.MetricSecurityProfileV2EvictionNodesEvictedPerProfile, int64(evicted), []string{}, 1.0); err != nil {
				seclog.Warnf("couldn't send %s metric: %v", metrics.MetricSecurityProfileV2EvictionNodesEvictedPerProfile, err)
			}
		}
		profile.Unlock()
	}

	if totalEvicted > 0 {
		seclog.Infof("evicted %d total unused process nodes across all profiles", totalEvicted)
	}
}

// GetNodesInProcessCache returns a map with ImageProcessKey as key and bool as value for filepaths in the process cache
func (m *ManagerV2) GetNodesInProcessCache(workloadID containerutils.WorkloadID, containersOnly bool) map[activity_tree.ImageProcessKey]bool {
	// If workloadID provided, do direct lookup for the given workload
	if workloadID != nil {
		return m.getNodesForSingleWorkload(workloadID, containersOnly)
	}

	// Otherwise iterate through all cache entries for all workloads
	return m.getNodesForAllWorkloads(containersOnly)
}

// getNodesForSingleWorkload returns nodes for a specific workload by direct cache lookup
func (m *ManagerV2) getNodesForSingleWorkload(workloadID containerutils.WorkloadID, containersOnly bool) map[activity_tree.ImageProcessKey]bool {
	result := make(map[activity_tree.ImageProcessKey]bool)

	cgr := m.resolvers.CGroupResolver
	pr := m.resolvers.ProcessResolver
	tagsResolver := m.resolvers.TagsResolver

	var cacheEntry *cgroupModel.CacheEntry
	var imageName, imageTag string

	switch id := workloadID.(type) {
	case containerutils.ContainerID:
		cacheEntry = cgr.GetCacheEntryContainerID(id)
		if cacheEntry == nil {
			return result
		}
		tags, err := tagsResolver.ResolveWithErr(id)
		if err != nil {
			return result
		}
		imageName = utils.GetTagValue("image_name", tags)
		imageTag = utils.GetTagValue("image_tag", tags)

	case containerutils.CGroupID:
		// Skip systemd cgroups if containersOnly is true
		if containersOnly {
			return result
		}
		cacheEntry = cgr.GetCacheEntryByCgroupID(id)
		if cacheEntry == nil {
			return result
		}
		tags, err := tagsResolver.ResolveWithErr(id)
		if err != nil {
			return result
		}
		imageName = utils.GetTagValue("service", tags)
		imageTag = utils.GetTagValue("version", tags)

	default:
		return result
	}

	if imageTag == "" {
		imageTag = "latest"
	}

	// Get PIDs and resolve filepaths
	pids := cacheEntry.GetPIDs()
	key := activity_tree.ImageProcessKey{
		ImageName: imageName,
		ImageTag:  imageTag,
	}

	for _, pid := range pids {
		pce := pr.Resolve(pid, pid, 0, true, nil)
		if pce == nil {
			continue
		}
		key.Filepath = pce.FileEvent.PathnameStr
		result[key] = true
	}

	return result
}

// getNodesForAllWorkloads returns nodes for all cgroups, optionally filtering by container type
func (m *ManagerV2) getNodesForAllWorkloads(containersOnly bool) map[activity_tree.ImageProcessKey]bool {
	cgr := m.resolvers.CGroupResolver
	pr := m.resolvers.ProcessResolver
	tagsResolver := m.resolvers.TagsResolver

	type imageTagKey struct {
		imageName string
		imageTag  string
	}

	pids := make(map[imageTagKey][]uint32)

	result := make(map[activity_tree.ImageProcessKey]bool)

	cgr.IterateCacheEntries(func(cgce *cgroupModel.CacheEntry) bool {
		var cgceTags []string
		var err error
		var imageName, imageTag string
		if !cgce.IsContainerContextNull() {
			cgceTags, err = tagsResolver.ResolveWithErr(cgce.GetContainerID())
			if err != nil {
				return false
			}
			imageName = utils.GetTagValue("image_name", cgceTags)
			imageTag = utils.GetTagValue("image_tag", cgceTags)
		} else if cgce.IsCGroupContextResolved() {
			// Skip non-container cgroups if containersOnly is true
			if containersOnly {
				return false
			}
			cgceTags, err = tagsResolver.ResolveWithErr(cgce.GetCGroupID())
			if err != nil {
				return false
			}
			imageName = utils.GetTagValue("service", cgceTags)
			imageTag = utils.GetTagValue("version", cgceTags)
		} else {
			return false
		}

		if imageTag == "" {
			imageTag = "latest"
		}

		imageTagKey := imageTagKey{
			imageName: imageName,
			imageTag:  imageTag,
		}
		pids[imageTagKey] = append(pids[imageTagKey], cgce.GetPIDs()...)

		return false
	})

	// we do the resolution of filepaths here so that we can release the cgroup resolver lock before acquiring the process resolver lock
	for k, pids := range pids {

		key := activity_tree.ImageProcessKey{
			ImageName: k.imageName,
			ImageTag:  k.imageTag,
			Filepath:  "",
		}

		for _, pid := range pids {
			pce := pr.Resolve(pid, pid, 0, true, nil)
			if pce == nil {
				seclog.Warnf("couldn't resolve process cache entry for pid %d, this process may have exited", pid)
				continue
			}

			key.Filepath = pce.FileEvent.PathnameStr
			result[key] = true
		}
	}

	return result
}

// ============================================================================
// NO-OP METHODS - ProfileManager Interface Compatibility
// ============================================================================
//
// The following methods are no-ops in ManagerV2. They exist solely to satisfy
// the ProfileManager interface, allowing V1 and V2 managers to be used
// interchangeably through the same interface.
//
// These methods will be removed once V2 is fully validated and V1 is deprecated.
// ============================================================================

// HandleSampleRefresh handles a sample refresh event from the kernel.
// It updates the LastSeen timestamp of the process node associated with the given cookie.
func (m *ManagerV2) HandleSampleRefresh(cookie uint32) {
	m.sampleRefreshReceived.Inc()

	entry, ok := m.sampleCookieMap.Get(cookie)
	if !ok {
		m.sampleRefreshMisses.Inc()
		return
	}

	m.sampleRefreshHits.Inc()

	entry.profile.Lock()
	defer entry.profile.Unlock()

	if entry.processNode == nil || entry.processNode.SeenIsEmpty() {
		m.sampleCookieMap.Remove(cookie)
		return
	}

	imageTagID := entry.profile.ActivityTree.GetImageTagID(entry.imageTag)
	if imageTagID == 0 {
		return
	}

	now := time.Now()
	entry.processNode.AppendImageTagID(imageTagID, now)
	if entry.eventNodeBase != nil {
		entry.eventNodeBase.AppendImageTagID(imageTagID, now)
	}
}

func (m *ManagerV2) purgeCookiesForProfile(prof *profile.Profile) {
	for _, key := range m.sampleCookieMap.Keys() {
		if entry, ok := m.sampleCookieMap.Peek(key); ok && entry.profile == prof {
			m.sampleCookieMap.Remove(key)
		}
	}
}

// LookupEventInProfiles lookups event in profiles.
// NO-OP in V2: Event filtering is handled differently through ProcessEvent which builds
// profiles from activity dump samples. The profile lookup/filtering logic from V1 is not
// applicable to the V2 lifecycle.
func (m *ManagerV2) LookupEventInProfiles(_ *model.Event) {}

// HasActiveActivityDump returns true if the given event has an active dump.
// NO-OP in V2: Always returns false. V2 doesn't use the traditional activity dump mechanism
// with kernel-space traced cgroups. Instead, it builds profiles directly from activity dump samples.
func (m *ManagerV2) HasActiveActivityDump(_ *model.Event) bool { return false }

// HandleCGroupTracingEvent handles a cgroup tracing event.
// NO-OP in V2: V2 doesn't use cgroup tracing events from kernel space. Profiles are built
// from activity dump samples instead.
func (m *ManagerV2) HandleCGroupTracingEvent(_ *model.CgroupTracingEvent) {}

// SyncTracedCgroups recovers lost CGroup tracing events by going through the kernel space map of cgroups.
// NO-OP in V2: V2 doesn't manage kernel-space traced cgroups maps.
func (m *ManagerV2) SyncTracedCgroups() {}

// ListActivityDumps lists the activity dumps.
// NO-OP in V2: V2 doesn't expose individual activity dumps through this API.
func (m *ManagerV2) ListActivityDumps(_ *api.ActivityDumpListParams) (*api.ActivityDumpListMessage, error) {
	return nil, nil
}

// StopActivityDump stops an active activity dump.
// NO-OP in V2: V2 doesn't manage activity dumps the traditional way.
func (m *ManagerV2) StopActivityDump(_ *api.ActivityDumpStopParams) (*api.ActivityDumpStopMessage, error) {
	return nil, nil
}

// DumpActivity dumps the activity.
// NO-OP in V2: V2 doesn't support on-demand activity dumping through this API.
func (m *ManagerV2) DumpActivity(_ *api.ActivityDumpParams) (*api.ActivityDumpMessage, error) {
	return nil, nil
}

// GenerateTranscoding generates a transcoding request for the given activity dump.
// NO-OP in V2: V2 doesn't support transcoding through this API.
func (m *ManagerV2) GenerateTranscoding(_ *api.TranscodingRequestParams) (*api.TranscodingRequestMessage, error) {
	return nil, nil
}
