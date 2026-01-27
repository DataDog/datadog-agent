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
	"fmt"
	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
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
	"github.com/DataDog/datadog-go/v5/statsd"
)

type pendingProfile struct {
	firstSeen   time.Time
	events      *list.List
	containerID string // stored for metrics tagging when expired
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

	// storage
	localStorage              *storage.Directory
	remoteStorage             *storage.ActivityDumpRemoteStorageForwarder
	configuredStorageRequests map[config.StorageFormat][]config.StorageRequest

	profilePendingEvents map[containerutils.CGroupID]*pendingProfile

	// Metrics counters (gauges that need to be tracked)
	queueSize            *atomic.Uint64 // total events currently queued (gauge)
	pendingProfiles      *atomic.Uint64 // cgroups currently waiting for tags
	eventsDroppedMaxSize *atomic.Uint64 // events dropped because profile at max size
	lateInsertions       *atomic.Uint64 // events inserted after profile was sent

	// Track unique cgroups seen
	seenCgroups     map[containerutils.CGroupID]struct{}
	seenCgroupsLock sync.Mutex

	// Track cgroup to selector mapping for delayed cleanup
	cgroupToSelector     map[containerutils.CGroupID]cgroupModel.WorkloadSelector
	cgroupToSelectorLock sync.RWMutex

	// Pending profile removals (selector -> time when removal was queued)
	pendingProfileRemovals     map[cgroupModel.WorkloadSelector]time.Time
	pendingProfileRemovalsLock sync.Mutex
}

func NewManagerV2(cfg *config.Config, statsdClient statsd.ClientInterface, resolvers *resolvers.EBPFResolvers, kernelVersion *kernel.Version, dumpHandler backend.ActivityDumpHandler, sendAnomalyDetection func(*model.Event), hostname string) (*ManagerV2, error) {

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

	return &ManagerV2{
		config:                    cfg,
		statsdClient:              statsdClient,
		resolvers:                 resolvers,
		kernelVersion:             kernelVersion,
		profilePendingEvents:      make(map[containerutils.CGroupID]*pendingProfile),
		queueSize:                 atomic.NewUint64(0),
		pendingProfiles:           atomic.NewUint64(0),
		eventsDroppedMaxSize:      atomic.NewUint64(0),
		lateInsertions:            atomic.NewUint64(0),
		pathsReducer:              activity_tree.NewPathsReducer(),
		profiles:                  make(map[cgroupModel.WorkloadSelector]*profile.Profile),
		localStorage:              localStorage,
		remoteStorage:             remoteStorage,
		configuredStorageRequests: perFormatStorageRequests(configuredStorageRequests),
		hostname:                  hostname,
		sendAnomalyDetection:      sendAnomalyDetection,
		eventFiltering:            make(map[eventFilteringEntry]*atomic.Uint64),
		seenCgroups:               make(map[containerutils.CGroupID]struct{}),
		cgroupToSelector:          make(map[containerutils.CGroupID]cgroupModel.WorkloadSelector),
		pendingProfileRemovals:    make(map[cgroupModel.WorkloadSelector]time.Time),
	}, nil
}

func (m *ManagerV2) Start(ctx context.Context) {
	sendTickerChan := m.setupPersistenceTicker()
	nodeEvictionTickerChan := m.setupNodeEvictionTicker()
	profileCleanupTickerChan := m.setupProfileCleanupTicker()

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

// onCGroupDeleted is called when a cgroup is deleted from the system
func (m *ManagerV2) onCGroupDeleted(cgce *cgroupModel.CacheEntry) {
	cgroupID := cgce.GetCGroupID()

	// Remove from seenCgroups
	m.seenCgroupsLock.Lock()
	delete(m.seenCgroups, cgroupID)
	m.seenCgroupsLock.Unlock()

	// Get the selector for this cgroup and remove the mapping
	m.cgroupToSelectorLock.Lock()
	selector, exists := m.cgroupToSelector[cgroupID]
	if exists {
		delete(m.cgroupToSelector, cgroupID)
	}
	m.cgroupToSelectorLock.Unlock()

	if !exists {
		return
	}

	// Check if any other cgroups are using this selector
	m.cgroupToSelectorLock.RLock()
	hasOtherCgroups := false
	for _, s := range m.cgroupToSelector {
		if s == selector {
			hasOtherCgroups = true
			break
		}
	}
	m.cgroupToSelectorLock.RUnlock()

	// If no other cgroups use this selector, queue for delayed removal
	if !hasOtherCgroups {
		m.pendingProfileRemovalsLock.Lock()
		if _, alreadyPending := m.pendingProfileRemovals[selector]; !alreadyPending {
			m.pendingProfileRemovals[selector] = time.Now()
			seclog.Debugf("queued profile [%s] for delayed removal", selector.String())
		}
		m.pendingProfileRemovalsLock.Unlock()
	}
}

// cleanupPendingProfiles removes profiles that have been pending removal for longer than the cleanup delay
func (m *ManagerV2) cleanupPendingProfiles() {
	cleanupDelay := m.config.RuntimeSecurity.SecurityProfileCleanupDelay
	if cleanupDelay <= 0 {
		return
	}

	now := time.Now()
	var selectorsToRemove []cgroupModel.WorkloadSelector

	m.pendingProfileRemovalsLock.Lock()
	for selector, queuedAt := range m.pendingProfileRemovals {
		if now.Sub(queuedAt) >= cleanupDelay {
			selectorsToRemove = append(selectorsToRemove, selector)
			delete(m.pendingProfileRemovals, selector)
		}
	}
	m.pendingProfileRemovalsLock.Unlock()

	// Remove the profiles
	if len(selectorsToRemove) > 0 {
		m.profilesLock.Lock()
		for _, selector := range selectorsToRemove {
			if profile := m.profiles[selector]; profile != nil {
				seclog.Infof("removing profile [%s] after cleanup delay", selector.String())
				delete(m.profiles, selector)

				// Emit metric
				tags := []string{"image_name:" + selector.Image}
				if err := m.statsdClient.Count(metrics.MetricSecurityProfileV2CleanupProfilesRemoved, 1, tags, 1.0); err != nil {
					seclog.Warnf("couldn't send %s metric: %v", metrics.MetricSecurityProfileV2CleanupProfilesRemoved, err)
				}
			}
		}
		m.profilesLock.Unlock()
	}
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
	format := config.Protobuf
	requests := m.configuredStorageRequests[format]

	data, err := p.Encode(format)
	if err != nil {
		seclog.Errorf("couldn't encode profile [%s] to %s format: %v", p.GetSelectorStr(), format, err)
		return
	}

	for _, request := range requests {
		m.persistProfileToStorage(p, request, data)
	}

	p.SetHasAlreadyBeenSent()
}

// persistProfileToStorage persists profile data to a specific storage backend
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
		fmt.Sprintf("compression:%v", request.Compression),
	}

	if err := m.statsdClient.Count(metrics.MetricSecurityProfileV2SizeInBytes, int64(dataSize), tags, 1.0); err != nil {
		seclog.Warnf("couldn't send %s metric: %v", metrics.MetricSecurityProfileV2SizeInBytes, err)
	}
	if err := m.statsdClient.Count(metrics.MetricSecurityProfileV2PersistedProfiles, 1, tags, 1.0); err != nil {
		seclog.Warnf("couldn't send %s metric: %v", metrics.MetricSecurityProfileV2PersistedProfiles, err)
	}
}

func (m *ManagerV2) ProcessEvent(event *model.Event) {
	if !event.IsActivityDumpSample() {
		return
	}

	// Filter out systemd cgroups if not configured to trace them
	if event.ProcessContext.Process.ContainerContext.ContainerID == "" && !m.config.RuntimeSecurity.ActivityDumpTraceSystemdCgroups {
		return
	}

	// Resolve event source (runtime or replay)
	source := event.FieldHandlers.ResolveSource(event, &event.BaseEvent)
	sourceTags := []string{"source:" + source}

	// Emit metric for events that pass initial filters
	if err := m.statsdClient.Count(metrics.MetricSecurityProfileV2EventsTotalReceived, 1, sourceTags, 1.0); err != nil {
		seclog.Warnf("couldn't send %s metric: %v", metrics.MetricSecurityProfileV2EventsTotalReceived, err)
	}

	// Cleanup: purge pending entries that have been waiting too long (60s)
	m.purgeStalePendingEvents(event.Timestamp)

	// Try to resolve tags for this event
	event.FieldHandlers.ResolveContainerTags(event, &event.ProcessContext.Process.ContainerContext)
	tagsResolved := len(event.ProcessContext.Process.ContainerContext.Tags) != 0

	if tagsResolved {
		if err := m.statsdClient.Count(metrics.MetricSecurityProfileV2EventsTotalImmediate, 1, sourceTags, 1.0); err != nil {
			seclog.Warnf("couldn't send %s metric: %v", metrics.MetricSecurityProfileV2EventsTotalImmediate, err)
		}
		m.processEventWithResolvedTags(event)
	} else {
		m.queueEventForTagResolution(event, sourceTags)
	}
}

// purgeStalePendingEvents removes pending entries that have been waiting for tags for more than 60 seconds
func (m *ManagerV2) purgeStalePendingEvents(currentTimestamp time.Time) {
	for cgroupID, pendingEvents := range m.profilePendingEvents {
		if currentTimestamp.Sub(pendingEvents.firstSeen) > 60*time.Second {
			// Decrement queue size by the number of events being dropped
			if eventsLen := pendingEvents.events.Len(); eventsLen > 0 {
				m.queueSize.Sub(uint64(eventsLen))
			}

			delete(m.profilePendingEvents, cgroupID)
			m.pendingProfiles.Dec()

			// Emit metric with containerID tag
			tags := []string{"container_id:" + pendingEvents.containerID}
			if err := m.statsdClient.Count(metrics.MetricSecurityProfileV2TagResolutionCgroupsExpired, 1, tags, 1.0); err != nil {
				seclog.Warnf("couldn't send %s metric: %v", metrics.MetricSecurityProfileV2TagResolutionCgroupsExpired, err)
			}
		}
	}
}

// processEventWithResolvedTags handles events that have their tags resolved.
// It also dequeues and processes any pending events for the same cgroup.
func (m *ManagerV2) processEventWithResolvedTags(event *model.Event) {
	cgroupID := event.ProcessContext.Process.CGroup.CGroupID

	// Track unique cgroups with resolved tags (cgroups we're actually profiling)
	m.seenCgroupsLock.Lock()
	if _, seen := m.seenCgroups[cgroupID]; !seen {
		m.seenCgroups[cgroupID] = struct{}{}
	}
	m.seenCgroupsLock.Unlock()

	// Dequeue and process pending events if any exist for this cgroup
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

	// Process the current event
	m.onEventTagsResolved(event)
}

// queueEventForTagResolution queues an event while waiting for tag resolution
func (m *ManagerV2) queueEventForTagResolution(event *model.Event, sourceTags []string) {
	cgroupID := event.ProcessContext.Process.CGroup.CGroupID
	pendingEvents := m.profilePendingEvents[cgroupID]

	// Create pending entry if it doesn't exist
	if pendingEvents == nil {
		pendingEvents = &pendingProfile{
			firstSeen:   event.Timestamp,
			events:      list.New(),
			containerID: string(event.ProcessContext.Process.ContainerContext.ContainerID),
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
			// Emit dropped metric with source tag
			if err := m.statsdClient.Count(metrics.MetricSecurityProfileV2TagResolutionEventsDropped, int64(eventsLen), sourceTags, 1.0); err != nil {
				seclog.Warnf("couldn't send %s metric: %v", metrics.MetricSecurityProfileV2TagResolutionEventsDropped, err)
			}
		}
		return
	}

	// Queue the event (deep copy to preserve state)
	event.ResolveFieldsForAD()
	cpy := event.DeepCopy()
	pendingEvents.events.PushBack(cpy)
	m.queueSize.Inc()
	if err := m.statsdClient.Count(metrics.MetricSecurityProfileV2EventsTotalQueued, 1, sourceTags, 1.0); err != nil {
		seclog.Warnf("couldn't send %s metric: %v", metrics.MetricSecurityProfileV2EventsTotalQueued, err)
	}
}

// onEventTagsResolved is called when an event has its tags resolved and is ready to be inserted into a profile
func (m *ManagerV2) onEventTagsResolved(event *model.Event) {
	profile, inserted := m.insertEventIntoProfile(event)
	if !inserted || !profile.HasAlreadyBeenSent() {
		return
	}

	// Profile was updated after being sent - this is a late insertion (potential anomaly)
	m.lateInsertions.Inc()

	var workloadID containerutils.WorkloadID
	var imageTag string

	if containerID := event.ProcessContext.Process.ContainerContext.ContainerID; containerID != "" {
		workloadID = containerutils.ContainerID(containerID)
		imageTag = utils.GetTagValue("image_tag", event.ProcessContext.Process.ContainerContext.Tags)
	} else if cgroupID := event.ProcessContext.Process.CGroup.CGroupID; cgroupID != "" {
		workloadID = cgroupID
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
	// Note: events.total_received, events.total_immediate, events.total_queued, and
	// tag_resolution.events_dropped are emitted directly in ProcessEvent with source tags

	// Tag resolution gauges
	if err := m.statsdClient.Gauge(metrics.MetricSecurityProfileV2TagResolutionEventsQueued, float64(m.queueSize.Load()), []string{}, 1.0); err != nil {
		return err
	}
	if err := m.statsdClient.Gauge(metrics.MetricSecurityProfileV2TagResolutionCgroupsPending, float64(m.pendingProfiles.Load()), []string{}, 1.0); err != nil {
		return err
	}

	// Total unique cgroups seen (all time) - use gauge since it's a cumulative total
	m.seenCgroupsLock.Lock()
	totalCgroups := len(m.seenCgroups)
	m.seenCgroupsLock.Unlock()
	if err := m.statsdClient.Gauge(metrics.MetricSecurityProfileV2TagResolutionCgroupsReceived, float64(totalCgroups), []string{}, 1.0); err != nil {
		return err
	}

	// Event processing counts (swap to 0 after reading)
	if value := m.eventsDroppedMaxSize.Swap(0); value > 0 {
		if err := m.statsdClient.Count(metrics.MetricSecurityProfileV2EventsDroppedMaxSize, int64(value), []string{}, 1.0); err != nil {
			return err
		}
	}
	if value := m.lateInsertions.Swap(0); value > 0 {
		if err := m.statsdClient.Count(metrics.MetricSecurityProfileV2ProfileLateInsertions, int64(value), []string{}, 1.0); err != nil {
			return err
		}
	}

	// Debug: log difference between seenCgroups and CGroup Resolver cache
	m.logCgroupDifference()

	return nil
}

// logCgroupDifference logs the difference between V2's seenCgroups and the CGroup Resolver's cache
// This helps debug why cgroups_received might differ from active_containers
func (m *ManagerV2) logCgroupDifference() {
	// Collect all cgroup IDs from the CGroup Resolver's cache
	resolverCgroups := make(map[containerutils.CGroupID]containerutils.ContainerID)
	m.resolvers.CGroupResolver.IterateCacheEntries(func(entry *cgroupModel.CacheEntry) bool {
		cgroupID := entry.GetCGroupID()
		containerID := entry.GetContainerID()
		// Only track container cgroups (not host cgroups)
		if containerID != "" {
			resolverCgroups[cgroupID] = containerID
		}
		return false // continue iteration
	})

	// Get a snapshot of seenCgroups
	m.seenCgroupsLock.Lock()
	seenCgroupsCopy := make(map[containerutils.CGroupID]struct{}, len(m.seenCgroups))
	for k, v := range m.seenCgroups {
		seenCgroupsCopy[k] = v
	}
	m.seenCgroupsLock.Unlock()

	// Find cgroups in resolver but NOT in V2's seenCgroups
	var inResolverNotInV2 []string
	for cgroupID, containerID := range resolverCgroups {
		if _, exists := seenCgroupsCopy[cgroupID]; !exists {
			// Try to get tags for this container
			var tagsStr string
			if containerID != "" {
				var workloadID containerutils.WorkloadID = containerID
				tags, err := m.resolvers.TagsResolver.ResolveWithErr(workloadID)
				if err == nil && len(tags) > 0 {
					tagsStr = fmt.Sprintf("tags=%v", tags)
				} else {
					tagsStr = "no tags"
				}
			} else {
				tagsStr = "no container"
			}
			inResolverNotInV2 = append(inResolverNotInV2, fmt.Sprintf("cgroupID=%s containerID=%s %s", cgroupID, containerID, tagsStr))
		}
	}

	// Find cgroups in V2's seenCgroups but NOT in resolver
	var inV2NotInResolver []string
	for cgroupID := range seenCgroupsCopy {
		if _, exists := resolverCgroups[cgroupID]; !exists {
			inV2NotInResolver = append(inV2NotInResolver, string(cgroupID))
		}
	}

	// Log the differences
	if len(inResolverNotInV2) > 0 || len(inV2NotInResolver) > 0 {
		seclog.Debugf("V2 CGroup Difference: resolver=%d, v2_seen=%d", len(resolverCgroups), len(seenCgroupsCopy))
		if len(inResolverNotInV2) > 0 {
			seclog.Debugf("  In CGroup Resolver but NOT in V2 seenCgroups (%d):", len(inResolverNotInV2))
			for _, entry := range inResolverNotInV2 {
				seclog.Debugf("    - %s", entry)
			}
		}
		if len(inV2NotInResolver) > 0 {
			seclog.Debugf("  In V2 seenCgroups but NOT in CGroup Resolver (%d):", len(inV2NotInResolver))
			for _, entry := range inV2NotInResolver {
				seclog.Debugf("    - %s", entry)
			}
		}
	}
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

	cgroupID := event.ProcessContext.Process.CGroup.CGroupID

	// Track cgroup-to-selector mapping for delayed cleanup
	m.cgroupToSelectorLock.Lock()
	m.cgroupToSelector[cgroupID] = selector
	m.cgroupToSelectorLock.Unlock()

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

	// Check if profile has reached max size
	if secprof.ActivityTree.Stats.ApproximateSize() >= int64(m.config.RuntimeSecurity.ActivityDumpMaxDumpSize()) {
		m.incrementEventFilteringStat(event.GetEventType(), model.ProfileAtMaxSize, NA)
		m.eventsDroppedMaxSize.Inc()
		return nil, false
	}

	// Ensure version context exists for this selector
	m.ensureVersionContext(secprof, selector.Tag)

	// Insert the event into the profile's activity tree
	imageTag := secprof.GetTagValue("image_tag")
	inserted, err := secprof.Insert(event, true, imageTag, activity_tree.Runtime, m.resolvers)
	if err != nil {
		return nil, false
	}

	return secprof, inserted
}

// buildWorkloadSelector creates a workload selector from the event's container tags
func (m *ManagerV2) buildWorkloadSelector(event *model.Event) (cgroupModel.WorkloadSelector, error) {
	imageName := utils.GetTagValue("image_name", event.ProcessContext.Process.ContainerContext.Tags)
	return cgroupModel.NewWorkloadSelector(imageName, "*")
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
		profile.WithEventTypes(m.config.RuntimeSecurity.ActivityDumpTracedEventTypes),
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

	// Profile was loaded successfully
	secprof.SetTreeType(secprof, "security_profile")

	// Update metadata with current event context for proper matching
	secprof.Metadata.ContainerID = event.ProcessContext.Process.ContainerContext.ContainerID
	secprof.Metadata.CGroupContext = event.ProcessContext.Process.CGroup

	// Apply eviction right away if configured
	if m.config.RuntimeSecurity.SecurityProfileNodeEvictionTimeout > 0 {
		filepathsInProcessCache := m.GetNodesInProcessCache()
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
		profile.WithEventTypes(m.config.RuntimeSecurity.ActivityDumpTracedEventTypes),
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
	var workloadID any
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

	for _, profile := range m.profiles {
		profile.InstancesLock.Lock()
		for _, instance := range profile.Instances {
			instance.Lock()
			if instance.GetWorkloadID() == id {
				ctx.Name = profile.Metadata.Name
				profileContext, ok := profile.GetVersionContext(imageTag)
				if ok { // should always be the case
					ctx.Tags = profileContext.Tags
				}
			}
			instance.Unlock()
		}
		profile.InstancesLock.Unlock()
	}
}

func (m *ManagerV2) incrementEventFilteringStat(eventType model.EventType, state model.EventFilteringProfileState, result EventFilteringResult) {
	if entry, ok := m.eventFiltering[eventFilteringEntry{eventType, state, result}]; ok {
		entry.Inc()
	}
}

// evictUnusedNodes performs periodic eviction of non-touched nodes from all active profiles
func (m *ManagerV2) evictUnusedNodes() {
	if m.config.RuntimeSecurity.SecurityProfileNodeEvictionTimeout <= 0 {
		return
	}

	// Emit eviction run metric
	if err := m.statsdClient.Count(metrics.MetricSecurityProfileV2EvictionRuns, 1, []string{}, 1.0); err != nil {
		seclog.Warnf("couldn't send %s metric: %v", metrics.MetricSecurityProfileV2EvictionRuns, err)
	}

	evictionTime := time.Now().Add(-m.config.RuntimeSecurity.SecurityProfileNodeEvictionTimeout)
	totalEvicted := 0

	filepathsInProcessCache := m.GetNodesInProcessCache()

	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()

	for selector, profile := range m.profiles {
		if profile == nil {
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
			tags := []string{
				"image_name:" + selector.Image,
				"image_tag:" + selector.Tag,
			}
			if err := m.statsdClient.Count(metrics.MetricSecurityProfileV2EvictionNodesEvictedPerProfile, int64(evicted), tags, 1.0); err != nil {
				seclog.Warnf("couldn't send %s metric: %v", metrics.MetricSecurityProfileV2EvictionNodesEvictedPerProfile, err)
			}
		}
		profile.Unlock()
	}

	if totalEvicted > 0 {
		seclog.Infof("evicted %d total unused process nodes across all profiles", totalEvicted)
	}
}

// GetNodesInProcessCache returns a map with ImageProcessKey as key and bool as value for all filepaths in the process cache
func (m *ManagerV2) GetNodesInProcessCache() map[activity_tree.ImageProcessKey]bool {

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
		if id := cgce.GetContainerID(); id != "" {
			cgceTags, err = tagsResolver.ResolveWithErr(id)
			if err != nil {
				return false
			}
			imageName = utils.GetTagValue("image_name", cgceTags)
			imageTag = utils.GetTagValue("image_tag", cgceTags)
		} else if cgce.IsCGroupContextResolved() {
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
