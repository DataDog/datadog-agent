// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package profile holds profile related files
package profile

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/hashicorp/golang-lru/v2/simplelru"
	"go.uber.org/atomic"

	proto "github.com/DataDog/agent-payload/v5/cws/dumpsv1"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/rconfig"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// DefaultProfileName used as default profile name
const DefaultProfileName = "default"

// EventFilteringProfileState is used to compute metrics for the event filtering feature
type EventFilteringProfileState uint8

const (
	// NoProfile is used to count the events for which we didn't have a profile
	NoProfile EventFilteringProfileState = iota
	// ProfileAtMaxSize is used to count the events that didn't make it into a profile because their matching profile
	// reached the max size threshold
	ProfileAtMaxSize
	// UnstableEventType is used to count the events that didn't make it into a profile because their matching profile was
	// unstable for their event type
	UnstableEventType
	// StableEventType is used to count the events linked to a stable profile for their event type
	StableEventType
	// AutoLearning is used to count the event during the auto learning phase
	AutoLearning
	// WorkloadWarmup is used to count the learned events due to workload warm up time
	WorkloadWarmup
)

func (efr EventFilteringProfileState) toTag() string {
	switch efr {
	case NoProfile:
		return "profile_state:no_profile"
	case ProfileAtMaxSize:
		return "profile_state:profile_at_max_size"
	case UnstableEventType:
		return "profile_state:unstable_event_type"
	case StableEventType:
		return "profile_state:stable_event_type"
	case AutoLearning:
		return "profile_state:auto_learning"
	case WorkloadWarmup:
		return "profile_state:workload_warmup"
	}
	return ""
}

// EventFilteringResult is used to compute metrics for the event filtering feature
type EventFilteringResult uint8

const (
	// NA not applicable for profil NoProfile and ProfileAtMaxSize state
	NA EventFilteringResult = iota
	// InProfile is used to count the events that matched a profile
	InProfile
	// NotInProfile is used to count the events that didn't match their profile
	NotInProfile
)

func (efr EventFilteringResult) toTag() string {
	switch efr {
	case NA:
		return ""
	case InProfile:
		return "in_profile:true"
	case NotInProfile:
		return "in_profile:false"
	}
	return ""
}

var (
	allEventFilteringProfileState = []EventFilteringProfileState{NoProfile, ProfileAtMaxSize, UnstableEventType, StableEventType, AutoLearning, WorkloadWarmup}
	allEventFilteringResults      = []EventFilteringResult{InProfile, NotInProfile, NA}
)

type eventFilteringEntry struct {
	eventType model.EventType
	state     EventFilteringProfileState
	result    EventFilteringResult
}

// ActivityDumpManager is a generic interface to reach the Activity Dump manager
type ActivityDumpManager interface {
	StopDumpsWithSelector(selector cgroupModel.WorkloadSelector)
}

// SecurityProfileManager is used to manage Security Profiles
type SecurityProfileManager struct {
	config              *config.Config
	statsdClient        statsd.ClientInterface
	resolvers           *resolvers.Resolvers
	providers           []Provider
	activityDumpManager ActivityDumpManager

	manager                    *manager.Manager
	securityProfileMap         *ebpf.Map
	securityProfileSyscallsMap *ebpf.Map

	profilesLock sync.Mutex
	profiles     map[cgroupModel.WorkloadSelector]*SecurityProfile

	pendingCacheLock sync.Mutex
	pendingCache     *simplelru.LRU[cgroupModel.WorkloadSelector, *SecurityProfile]
	cacheHit         *atomic.Uint64
	cacheMiss        *atomic.Uint64

	eventFiltering        map[eventFilteringEntry]*atomic.Uint64
	pathsReducer          *activity_tree.PathsReducer
	onLocalStorageCleanup func(files []string)
}

// NewSecurityProfileManager returns a new instance of SecurityProfileManager
func NewSecurityProfileManager(config *config.Config, statsdClient statsd.ClientInterface, resolvers *resolvers.Resolvers, manager *manager.Manager) (*SecurityProfileManager, error) {
	profileCache, err := simplelru.NewLRU[cgroupModel.WorkloadSelector, *SecurityProfile](config.RuntimeSecurity.SecurityProfileCacheSize, nil)
	if err != nil {
		return nil, fmt.Errorf("couldn't create security profile cache: %w", err)
	}

	securityProfileMap, ok, _ := manager.GetMap("security_profiles")
	if !ok {
		return nil, fmt.Errorf("security_profiles map not found")
	}

	securityProfileSyscallsMap, ok, _ := manager.GetMap("secprofs_syscalls")
	if !ok {
		return nil, fmt.Errorf("secprofs_syscalls map not found")
	}

	m := &SecurityProfileManager{
		config:                     config,
		statsdClient:               statsdClient,
		manager:                    manager,
		securityProfileMap:         securityProfileMap,
		securityProfileSyscallsMap: securityProfileSyscallsMap,
		resolvers:                  resolvers,
		profiles:                   make(map[cgroupModel.WorkloadSelector]*SecurityProfile),
		pendingCache:               profileCache,
		cacheHit:                   atomic.NewUint64(0),
		cacheMiss:                  atomic.NewUint64(0),
		eventFiltering:             make(map[eventFilteringEntry]*atomic.Uint64),
		pathsReducer:               activity_tree.NewPathsReducer(),
	}

	// instantiate directory provider
	if len(config.RuntimeSecurity.SecurityProfileDir) != 0 {
		dirProvider, err := NewDirectoryProvider(config.RuntimeSecurity.SecurityProfileDir, config.RuntimeSecurity.SecurityProfileWatchDir)
		if err != nil {
			return nil, fmt.Errorf("couldn't instantiate a new security profile directory provider: %w", err)
		}
		m.providers = append(m.providers, dirProvider)
		m.onLocalStorageCleanup = dirProvider.OnLocalStorageCleanup
	}

	// instantiate remote-config provider
	if config.RuntimeSecurity.RemoteConfigurationEnabled && config.RuntimeSecurity.SecurityProfileRCEnabled {
		rcProvider, err := rconfig.NewRCProfileProvider()
		if err != nil {
			return nil, fmt.Errorf("couldn't instantiate a new security profile remote-config provider: %w", err)
		}
		m.providers = append(m.providers, rcProvider)
	}

	m.initMetricsMap()

	// register the manager to the provider(s)
	for _, p := range m.providers {
		p.SetOnNewProfileCallback(m.OnNewProfileEvent)
	}
	return m, nil
}

// OnLocalStorageCleanup performs the necessary cleanup when the Activity Dump Manager local storage cleans up an entry
func (m *SecurityProfileManager) OnLocalStorageCleanup(files []string) {
	if m.onLocalStorageCleanup != nil {
		m.onLocalStorageCleanup(files)
	}
}

func (m *SecurityProfileManager) initMetricsMap() {
	for i := model.EventType(0); i < model.MaxKernelEventType; i++ {
		for _, state := range allEventFilteringProfileState {
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

// SetActivityDumpManager sets the stopDumpsWithSelectorCallback function
func (m *SecurityProfileManager) SetActivityDumpManager(manager ActivityDumpManager) {
	m.activityDumpManager = manager
}

// Start runs the manager of Security Profiles
func (m *SecurityProfileManager) Start(ctx context.Context) {
	// start all providers
	for _, p := range m.providers {
		if err := p.Start(ctx); err != nil {
			seclog.Errorf("couldn't start profile provider: %v", err)
		}
	}

	// register the manager to the CGroup resolver
	_ = m.resolvers.CGroupResolver.RegisterListener(cgroup.WorkloadSelectorResolved, m.OnWorkloadSelectorResolvedEvent)
	_ = m.resolvers.CGroupResolver.RegisterListener(cgroup.CGroupDeleted, m.OnCGroupDeletedEvent)

	seclog.Infof("security profile manager started")

	<-ctx.Done()
	m.stop()
}

// propagateWorkloadSelectorsToProviders (thread unsafe) propagates the list of workload selectors to the Security
// Profiles providers.
func (m *SecurityProfileManager) propagateWorkloadSelectorsToProviders() {
	var selectors []cgroupModel.WorkloadSelector
	for selector := range m.profiles {
		selectors = append(selectors, selector)
	}

	for _, p := range m.providers {
		p.UpdateWorkloadSelectors(selectors)
	}
}

// OnWorkloadSelectorResolvedEvent is used to handle the creation of a new cgroup with its resolved tags
func (m *SecurityProfileManager) OnWorkloadSelectorResolvedEvent(workload *cgroupModel.CacheEntry) {
	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()
	workload.Lock()
	defer workload.Unlock()

	if workload.Deleted.Load() {
		// this workload was deleted before we had time to apply its profile, ignore
		return
	}

	// check if the workload of this selector already exists
	profile, ok := m.profiles[workload.WorkloadSelector]
	if !ok {
		// check the cache
		m.pendingCacheLock.Lock()
		defer m.pendingCacheLock.Unlock()
		profile, ok = m.pendingCache.Get(workload.WorkloadSelector)
		if ok {
			m.cacheHit.Inc()

			// remove profile from cache
			_ = m.pendingCache.Remove(workload.WorkloadSelector)

			// since the profile was in cache, it was removed from kernel space, load it now
			// (locking isn't necessary here, but added as a safeguard)
			profile.Lock()
			err := m.loadProfile(profile)
			profile.Unlock()

			if err != nil {
				seclog.Errorf("couldn't load security profile %s in kernel space: %v", profile.selector, err)
				return
			}

			// insert the profile in the list of active profiles
			m.profiles[workload.WorkloadSelector] = profile
		} else {
			m.cacheMiss.Inc()

			// create a new entry
			profile = NewSecurityProfile(workload.WorkloadSelector, m.config.RuntimeSecurity.AnomalyDetectionEventTypes)
			m.profiles[workload.WorkloadSelector] = profile

			// notify the providers that we're interested in a new workload selector
			m.propagateWorkloadSelectorsToProviders()
		}
	}

	// make sure the profile keeps a reference to the workload
	m.LinkProfile(profile, workload)
}

// LinkProfile applies a profile to the provided workload
func (m *SecurityProfileManager) LinkProfile(profile *SecurityProfile, workload *cgroupModel.CacheEntry) {
	profile.Lock()
	defer profile.Unlock()

	// check if this instance of this workload is already tracked
	for _, w := range profile.Instances {
		if w.ID == workload.ID {
			// nothing to do, leave
			return
		}
	}

	// update the list of tracked instances
	profile.Instances = append(profile.Instances, workload)

	// can we apply the profile or is it not ready yet ?
	if profile.loadedInKernel {
		m.linkProfile(profile, workload)
	}
}

// UnlinkProfile removes the link between a workload and a profile
func (m *SecurityProfileManager) UnlinkProfile(profile *SecurityProfile, workload *cgroupModel.CacheEntry) {
	profile.Lock()
	defer profile.Unlock()

	// remove the workload from the list of instances of the Security Profile
	for key, val := range profile.Instances {
		if workload.ID == val.ID {
			profile.Instances = append(profile.Instances[0:key], profile.Instances[key+1:]...)
			break
		}
	}

	// remove link between the profile and the workload
	m.unlinkProfile(profile, workload)
}

// GetProfile returns a profile by its selector
func (m *SecurityProfileManager) GetProfile(selector cgroupModel.WorkloadSelector) *SecurityProfile {
	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()

	// check if this workload had a Security Profile
	return m.profiles[selector]
}

// FillProfileContextFromContainerID populates a SecurityProfileContext for the given container ID
func (m *SecurityProfileManager) FillProfileContextFromContainerID(id string, ctx *model.SecurityProfileContext) {
	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()

	for _, profile := range m.profiles {
		profile.Lock()
		for _, instance := range profile.Instances {
			instance.Lock()
			if instance.ID == id {
				ctx.Name = profile.Metadata.Name
				ctx.Version = profile.Version
				ctx.Tags = profile.Tags
				ctx.Status = profile.Status
			}
			instance.Unlock()
		}
		profile.Unlock()
	}
}

// FillProfileContextFromProfile fills the given ctx with profile infos
func FillProfileContextFromProfile(ctx *model.SecurityProfileContext, profile *SecurityProfile) {
	profile.Lock()
	defer profile.Unlock()

	ctx.Name = profile.Metadata.Name
	if ctx.Name == "" {
		ctx.Name = DefaultProfileName
	}

	ctx.Version = profile.Version
	ctx.Tags = profile.Tags
	ctx.Status = profile.Status
	ctx.AnomalyDetectionEventTypes = profile.anomalyDetectionEvents
}

// OnCGroupDeletedEvent is used to handle a CGroupDeleted event
func (m *SecurityProfileManager) OnCGroupDeletedEvent(workload *cgroupModel.CacheEntry) {
	// lookup the profile
	profile := m.GetProfile(workload.WorkloadSelector)
	if profile == nil {
		// nothing to do, leave
		return
	}

	// removes the link between the profile and this workload
	m.UnlinkProfile(profile, workload)

	// check if the profile should be deleted
	m.ShouldDeleteProfile(profile)
}

// ShouldDeleteProfile checks if a profile should be deleted (happens if no instance is linked to it)
func (m *SecurityProfileManager) ShouldDeleteProfile(profile *SecurityProfile) {
	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()
	m.pendingCacheLock.Lock()
	defer m.pendingCacheLock.Unlock()
	profile.Lock()
	defer profile.Unlock()

	// check if the profile should be deleted
	if len(profile.Instances) != 0 {
		// this profile is still in use, leave now
		return
	}

	// remove the profile from the list of profiles
	delete(m.profiles, profile.selector)

	// propagate the workload selectors
	m.propagateWorkloadSelectorsToProviders()

	if profile.loadedInKernel {
		// remove profile from kernel space
		m.unloadProfile(profile)
	}

	// cleanup profile before insertion in cache
	profile.reset()

	if profile.selector.IsReady() {
		// do not insert in cache
		return
	}

	// add profile in cache
	m.pendingCache.Add(profile.selector, profile)
}

// OnNewProfileEvent handles the arrival of a new profile (or the new version of a profile) from a provider
func (m *SecurityProfileManager) OnNewProfileEvent(selector cgroupModel.WorkloadSelector, newProfile *proto.SecurityProfile) {
	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()

	// Update the Security Profile content
	profile, ok := m.profiles[selector]
	if !ok {
		// this was likely a short-lived workload, cache the profile in case this workload comes back
		profile = NewSecurityProfile(selector, m.config.RuntimeSecurity.AnomalyDetectionEventTypes)
	}

	if profile.Version == newProfile.Version {
		// this is the same file, ignore
		return
	}

	m.pendingCacheLock.Lock()
	defer m.pendingCacheLock.Unlock()

	profile.Lock()
	defer profile.Unlock()
	profile.loadedInKernel = false

	// decode the content of the profile
	ProtoToSecurityProfile(profile, m.pathsReducer, newProfile)
	profile.ActivityTree.DNSMatchMaxDepth = m.config.RuntimeSecurity.SecurityProfileDNSMatchMaxDepth

	// compute activity tree initial stats
	profile.ActivityTree.ComputeActivityTreeStats()

	// prepare the profile for insertion
	m.prepareProfile(profile)

	if !ok {
		// insert in cache and leave
		m.pendingCache.Add(selector, profile)
		return
	}

	// load the profile in kernel space
	if err := m.loadProfile(profile); err != nil {
		seclog.Errorf("couldn't load security profile %s in kernel space: %v", profile.selector, err)
		return
	}

	// link all workloads
	for _, workload := range profile.Instances {
		m.linkProfile(profile, workload)
	}
}

func (m *SecurityProfileManager) stop() {
	// stop all providers
	for _, p := range m.providers {
		if err := p.Stop(); err != nil {
			seclog.Errorf("couldn't stop profile provider: %v", err)
		}
	}
}

func (m *SecurityProfileManager) incrementEventFilteringStat(eventType model.EventType, state EventFilteringProfileState, result EventFilteringResult) {
	m.eventFiltering[eventFilteringEntry{eventType, state, result}].Inc()
}

// SendStats sends metrics about the Security Profile manager
func (m *SecurityProfileManager) SendStats() error {
	// Send metrics for profile provider first to prevent a deadlock with the call to "dp.onNewProfileCallback" on
	// "m.profilesLock"
	for _, provider := range m.providers {
		if err := provider.SendStats(m.statsdClient); err != nil {
			return err
		}
	}

	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()
	m.pendingCacheLock.Lock()
	defer m.pendingCacheLock.Unlock()

	profileStats := make(map[model.Status]map[bool]float64)
	for _, profile := range m.profiles {
		if profile.loadedInKernel { // make sure the profile is loaded
			if err := profile.SendStats(m.statsdClient); err != nil {
				return fmt.Errorf("couldn't send metrics for [%s]: %w", profile.selector.String(), err)
			}
		}
		if profileStats[profile.Status] == nil {
			profileStats[profile.Status] = make(map[bool]float64)
		}
		profileStats[profile.Status][profile.loadedInKernel]++
	}

	for status, counts := range profileStats {
		for inKernel, count := range counts {
			tags := []string{
				fmt.Sprintf("in_kernel:%v", inKernel),
				fmt.Sprintf("anomaly_detection:%v", status.IsEnabled(model.AnomalyDetection)),
				fmt.Sprintf("auto_suppression:%v", status.IsEnabled(model.AutoSuppression)),
				fmt.Sprintf("workload_hardening:%v", status.IsEnabled(model.WorkloadHardening)),
			}
			if err := m.statsdClient.Gauge(metrics.MetricSecurityProfileProfiles, count, tags, 1.0); err != nil {
				return fmt.Errorf("couldn't send MetricSecurityProfileProfiles: %w", err)
			}
		}
	}

	if val := float64(m.pendingCache.Len()); val > 0 {
		if err := m.statsdClient.Gauge(metrics.MetricSecurityProfileCacheLen, val, []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricSecurityProfileCacheLen: %w", err)
		}
	}

	if val := int64(m.cacheHit.Swap(0)); val > 0 {
		if err := m.statsdClient.Count(metrics.MetricSecurityProfileCacheHit, val, []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricSecurityProfileCacheHit: %w", err)
		}
	}

	if val := int64(m.cacheMiss.Swap(0)); val > 0 {
		if err := m.statsdClient.Count(metrics.MetricSecurityProfileCacheMiss, val, []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricSecurityProfileCacheMiss: %w", err)
		}
	}

	for entry, count := range m.eventFiltering {
		tags := []string{fmt.Sprintf("event_type:%s", entry.eventType), entry.state.toTag(), entry.result.toTag()}
		if value := count.Swap(0); value > 0 {
			if err := m.statsdClient.Count(metrics.MetricSecurityProfileEventFiltering, int64(value), tags, 1.0); err != nil {
				return fmt.Errorf("couldn't send MetricSecurityProfileEventFiltering metric: %w", err)
			}
		}
	}

	return nil
}

// prepareProfile (thread unsafe) generates eBPF programs and cookies to prepare for kernel space insertion
func (m *SecurityProfileManager) prepareProfile(profile *SecurityProfile) {
	// generate cookies for the profile
	profile.generateCookies()

	// TODO: generate eBPF programs and make sure the profile is ready to be inserted in kernel space
}

// loadProfile (thread unsafe) loads a Security Profile in kernel space
func (m *SecurityProfileManager) loadProfile(profile *SecurityProfile) error {
	profile.loadedInKernel = true
	profile.loadedNano = uint64(m.resolvers.TimeResolver.ComputeMonotonicTimestamp(time.Now()))

	// push kernel space filters
	if err := m.securityProfileSyscallsMap.Put(profile.profileCookie, profile.generateSyscallsFilters()); err != nil {
		return fmt.Errorf("couldn't push syscalls filter (check map size limit ?): %w", err)
	}

	// TODO: load generated programs
	seclog.Debugf("security profile %s (version:%s status:%s) loaded in kernel space", profile.Metadata.Name, profile.Version, profile.Status.String())
	return nil
}

// unloadProfile (thread unsafe) unloads a Security Profile from kernel space
func (m *SecurityProfileManager) unloadProfile(profile *SecurityProfile) {
	profile.loadedInKernel = false

	// remove kernel space filters
	if err := m.securityProfileSyscallsMap.Delete(profile.profileCookie); err != nil {
		seclog.Errorf("couldn't remove syscalls filter: %v", err)
	}

	// TODO: delete all kernel space programs
	seclog.Debugf("security profile %s (version:%s status:%s) unloaded from kernel space", profile.Metadata.Name, profile.Version, profile.Status.String())
}

// linkProfile (thread unsafe) updates the kernel space mapping between a workload and its profile
func (m *SecurityProfileManager) linkProfile(profile *SecurityProfile, workload *cgroupModel.CacheEntry) {
	if err := m.securityProfileMap.Put([]byte(workload.ID), profile.generateKernelSecurityProfileDefinition()); err != nil {
		seclog.Errorf("couldn't link workload %s (selector: %s) with profile %s (check map size limit ?): %v", workload.ID, workload.WorkloadSelector.String(), profile.Metadata.Name, err)
		return
	}
	seclog.Infof("workload %s (selector: %s) successfully linked to profile %s", workload.ID, workload.WorkloadSelector.String(), profile.Metadata.Name)
}

// unlinkProfile (thread unsafe) updates the kernel space mapping between a workload and its profile
func (m *SecurityProfileManager) unlinkProfile(profile *SecurityProfile, workload *cgroupModel.CacheEntry) {
	if !profile.loadedInKernel {
		return
	}

	if err := m.securityProfileMap.Delete([]byte(workload.ID)); err != nil {
		seclog.Errorf("couldn't unlink workload %s with profile %s: %v", workload.WorkloadSelector.String(), profile.Metadata.Name, err)
	}
	seclog.Infof("workload %s (selector: %s) successfully unlinked from profile %s", workload.ID, workload.WorkloadSelector.String(), profile.Metadata.Name)
}

// LookupEventInProfiles lookups event in profiles
func (m *SecurityProfileManager) LookupEventInProfiles(event *model.Event) {
	// ignore events with an error
	if event.Error != nil {
		return
	}

	// shortcut for dedicated anomaly detection events
	if event.IsKernelSpaceAnomalyDetectionEvent() {
		event.AddToFlags(model.EventFlagsSecurityProfileInProfile)
		return
	}

	// create profile selector
	event.FieldHandlers.ResolveContainerTags(event, event.ContainerContext)
	if len(event.ContainerContext.Tags) == 0 {
		return
	}

	selector, err := cgroupModel.NewWorkloadSelector(utils.GetTagValue("image_name", event.ContainerContext.Tags), utils.GetTagValue("image_tag", event.ContainerContext.Tags))
	if err != nil {
		return
	}

	// lookup profile
	profile := m.GetProfile(selector)
	if profile == nil || profile.Status == 0 {
		m.incrementEventFilteringStat(event.GetEventType(), NoProfile, NA)
		return
	}

	_ = event.FieldHandlers.ResolveContainerCreatedAt(event, event.ContainerContext)

	// check if the event should be injected in the profile automatically
	profileState := m.tryAutolearn(profile, event)
	switch profileState {
	case NoProfile, ProfileAtMaxSize, UnstableEventType:
		// an error occurred or we are in unstable state
		// do not link the profile to avoid sending anomalies
		return
	case AutoLearning, WorkloadWarmup:
		// the event was either already in the profile, or has just been inserted
		FillProfileContextFromProfile(&event.SecurityProfileContext, profile)
		event.AddToFlags(model.EventFlagsSecurityProfileInProfile)
		return
	case StableEventType:
		// check if the event is in its profile
		found, err := profile.ActivityTree.Contains(event, activity_tree.ProfileDrift, m.resolvers)
		if err != nil {
			// ignore, evaluation failed
			m.incrementEventFilteringStat(event.GetEventType(), NoProfile, NA)
			return
		}
		FillProfileContextFromProfile(&event.SecurityProfileContext, profile)
		if found {
			event.AddToFlags(model.EventFlagsSecurityProfileInProfile)
			m.incrementEventFilteringStat(event.GetEventType(), profileState, InProfile)
		} else {
			m.incrementEventFilteringStat(event.GetEventType(), profileState, NotInProfile)
		}
	}
}

// tryAutolearn tries to autolearn the input event. It returns the profile state: stable, unstable, autolearning or workloadwarmup
func (m *SecurityProfileManager) tryAutolearn(profile *SecurityProfile, event *model.Event) EventFilteringProfileState {
	var nodeType activity_tree.NodeGenerationType
	var profileState EventFilteringProfileState

	profile.eventTypeStateLock.Lock()
	defer profile.eventTypeStateLock.Unlock()
	eventState, ok := profile.eventTypeState[event.GetEventType()]
	if !ok {
		eventState = &EventTypeState{
			lastAnomalyNano: profile.loadedNano,
			state:           NoProfile,
		}
		profile.eventTypeState[event.GetEventType()] = eventState
	} else if eventState.state == UnstableEventType {
		// If for the given event type we already are on UnstableEventType, just return
		// (once reached, this state is immutable)
		m.incrementEventFilteringStat(event.GetEventType(), UnstableEventType, NA)
		return UnstableEventType
	}

	// check if we are at the beginning of a workload lifetime
	if event.ResolveEventTime().Sub(time.Unix(0, int64(event.ContainerContext.CreatedAt))) < m.config.RuntimeSecurity.AnomalyDetectionWorkloadWarmupPeriod {
		nodeType = activity_tree.WorkloadWarmup
		profileState = WorkloadWarmup
	} else {
		// If for the given event type we already are on StableEventType (and outside of the warmup period), just return
		if eventState.state == StableEventType {
			return StableEventType
		}

		// did we reached the stable state time limit ?
		if time.Duration(event.TimestampRaw-eventState.lastAnomalyNano) >= m.config.RuntimeSecurity.GetAnomalyDetectionMinimumStablePeriod(event.GetEventType()) {
			eventState.state = StableEventType
			// call the activity dump manager to stop dumping workloads from the current profile selector
			if m.activityDumpManager != nil {
				m.activityDumpManager.StopDumpsWithSelector(profile.selector)
			}
			return StableEventType
		}

		// did we reached the unstable time limit ?
		if time.Duration(event.TimestampRaw-profile.loadedNano) >= m.config.RuntimeSecurity.AnomalyDetectionUnstableProfileTimeThreshold {
			eventState.state = UnstableEventType
			return UnstableEventType
		}

		nodeType = activity_tree.ProfileDrift
		profileState = AutoLearning
	}

	// check if the unstable size limit was reached
	if profile.ActivityTree.Stats.ApproximateSize() >= m.config.RuntimeSecurity.AnomalyDetectionUnstableProfileSizeThreshold {
		// for each event type we want to reach either the StableEventType or UnstableEventType states, even
		// if we already reach the AnomalyDetectionUnstableProfileSizeThreshold. That's why we have to keep
		// rearming the lastAnomalyNano timer based on if it's something new or not.
		found, err := profile.ActivityTree.Contains(event, nodeType, m.resolvers)
		if err != nil {
			m.incrementEventFilteringStat(event.GetEventType(), NoProfile, NA)
			return NoProfile
		} else if !found {
			eventState.lastAnomalyNano = event.TimestampRaw
		} else if profileState == WorkloadWarmup {
			// if it's NOT something's new AND we are on container warmup period, just pretend
			// we are in learning/warmup phase (as we know, this event is already present on the profile)
			return WorkloadWarmup
		}
		return ProfileAtMaxSize
	}

	// here we are either in AutoLearning or WorkloadWarmup
	// try to insert the event in the profile
	newEntry, err := profile.ActivityTree.Insert(event, nodeType, m.resolvers)
	if err != nil {
		m.incrementEventFilteringStat(event.GetEventType(), NoProfile, NA)
		return NoProfile
	} else if newEntry {
		eventState.lastAnomalyNano = event.TimestampRaw
		m.incrementEventFilteringStat(event.GetEventType(), profileState, NotInProfile)
	} else { // no newEntry
		m.incrementEventFilteringStat(event.GetEventType(), profileState, InProfile)
	}
	return profileState
}

// ListSecurityProfiles returns the list of security profiles
func (m *SecurityProfileManager) ListSecurityProfiles(params *api.SecurityProfileListParams) (*api.SecurityProfileListMessage, error) {
	var out api.SecurityProfileListMessage

	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()

	for _, p := range m.profiles {
		msg := p.ToSecurityProfileMessage(m.resolvers.TimeResolver, m.config.RuntimeSecurity)
		out.Profiles = append(out.Profiles, msg)
	}

	if params.GetIncludeCache() {
		m.pendingCacheLock.Lock()
		defer m.pendingCacheLock.Unlock()
		for _, k := range m.pendingCache.Keys() {
			p, ok := m.pendingCache.Peek(k)
			if !ok {
				continue
			}
			msg := p.ToSecurityProfileMessage(m.resolvers.TimeResolver, m.config.RuntimeSecurity)
			out.Profiles = append(out.Profiles, msg)
		}
	}
	return &out, nil
}

// SaveSecurityProfile saves the requested security profile to disk
func (m *SecurityProfileManager) SaveSecurityProfile(params *api.SecurityProfileSaveParams) (*api.SecurityProfileSaveMessage, error) {
	selector, err := cgroupModel.NewWorkloadSelector(params.GetSelector().GetName(), params.GetSelector().GetTag())
	if err != nil {
		return &api.SecurityProfileSaveMessage{
			Error: err.Error(),
		}, nil
	}

	p := m.GetProfile(selector)
	if p == nil || p.Status == 0 || p.ActivityTree == nil {
		return &api.SecurityProfileSaveMessage{
			Error: "security profile not found",
		}, nil
	}

	// encode profile
	psp := SecurityProfileToProto(p)
	if psp == nil {
		return &api.SecurityProfileSaveMessage{
			Error: "security profile not found",
		}, nil
	}

	raw, err := psp.MarshalVT()
	if err != nil {
		return nil, fmt.Errorf("couldn't encode security profile in %s: %v", config.Protobuf, err)
	}

	// write profile to encoded profile to disk
	f, err := os.CreateTemp("/tmp", fmt.Sprintf("%s-*.profile", p.Metadata.Name))
	if err != nil {
		return nil, fmt.Errorf("couldn't create temporary file: %w", err)
	}
	defer f.Close()

	if _, err = f.Write(raw); err != nil {
		return nil, fmt.Errorf("couldn't write to temporary file: %w", err)
	}

	return &api.SecurityProfileSaveMessage{
		File: f.Name(),
	}, nil
}

// FetchSilentWorkloads returns the list of workloads for which we haven't received any profile
func (m *SecurityProfileManager) FetchSilentWorkloads() map[cgroupModel.WorkloadSelector][]*cgroupModel.CacheEntry {
	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()

	out := make(map[cgroupModel.WorkloadSelector][]*cgroupModel.CacheEntry)

	for selector, profile := range m.profiles {
		profile.Lock()
		if profile.loadedInKernel == false {
			out[selector] = profile.Instances
		}
		profile.Unlock()
	}

	return out
}
