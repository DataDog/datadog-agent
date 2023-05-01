// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package profile

import (
	"context"
	"fmt"
	"sync"

	"github.com/DataDog/datadog-go/v5/statsd"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/hashicorp/golang-lru/v2/simplelru"
	"go.uber.org/atomic"

	proto "github.com/DataDog/agent-payload/v5/cws/dumpsv1"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// SecurityProfileManager is used to manage Security Profiles
type SecurityProfileManager struct {
	config         *config.Config
	statsdClient   statsd.ClientInterface
	cgroupResolver *cgroup.Resolver
	providers      []Provider

	manager                    *manager.Manager
	securityProfileMap         *ebpf.Map
	securityProfileSyscallsMap *ebpf.Map

	profilesLock sync.Mutex
	profiles     map[cgroupModel.WorkloadSelector]*SecurityProfile

	pendingCacheLock sync.Mutex
	pendingCache     *simplelru.LRU[cgroupModel.WorkloadSelector, *SecurityProfile]
	cacheHit         *atomic.Uint64
	cacheMiss        *atomic.Uint64

	eventFilteringNoProfile map[model.EventType]*atomic.Uint64
	eventFilteringAbsent    map[model.EventType]*atomic.Uint64
	eventFilteringPresent   map[model.EventType]*atomic.Uint64
}

// NewSecurityProfileManager returns a new instance of SecurityProfileManager
func NewSecurityProfileManager(config *config.Config, statsdClient statsd.ClientInterface, cgroupResolver *cgroup.Resolver, manager *manager.Manager) (*SecurityProfileManager, error) {
	var providers []Provider

	// instantiate directory provider
	if len(config.RuntimeSecurity.SecurityProfileDir) != 0 {
		dirProvider, err := NewDirectoryProvider(config.RuntimeSecurity.SecurityProfileDir, config.RuntimeSecurity.SecurityProfileWatchDir)
		if err != nil {
			return nil, fmt.Errorf("couldn't instantiate a new security profile directory provider: %w", err)
		}
		providers = append(providers, dirProvider)
	}

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
		providers:                  providers,
		manager:                    manager,
		securityProfileMap:         securityProfileMap,
		securityProfileSyscallsMap: securityProfileSyscallsMap,
		cgroupResolver:             cgroupResolver,
		profiles:                   make(map[cgroupModel.WorkloadSelector]*SecurityProfile),
		pendingCache:               profileCache,
		cacheHit:                   atomic.NewUint64(0),
		cacheMiss:                  atomic.NewUint64(0),
		eventFilteringNoProfile:    make(map[model.EventType]*atomic.Uint64),
		eventFilteringAbsent:       make(map[model.EventType]*atomic.Uint64),
		eventFilteringPresent:      make(map[model.EventType]*atomic.Uint64),
	}
	for i := model.EventType(0); i < model.MaxKernelEventType; i++ {
		m.eventFilteringNoProfile[i] = atomic.NewUint64(0)
		m.eventFilteringAbsent[i] = atomic.NewUint64(0)
		m.eventFilteringPresent[i] = atomic.NewUint64(0)
	}

	// register the manager to the provider(s)
	for _, p := range m.providers {
		p.SetOnNewProfileCallback(m.OnNewProfileEvent)
	}
	return m, nil
}

// Start runs the manager of Security Profiles
func (m *SecurityProfileManager) Start(ctx context.Context) {
	// start all providers
	for _, p := range m.providers {
		if err := p.Start(ctx); err != nil {
			seclog.Errorf("couldn't start profile provider: %v", err)
			return
		}
	}

	// register the manager to the CGroup resolver
	_ = m.cgroupResolver.RegisterListener(cgroup.WorkloadSelectorResolved, m.OnWorkloadSelectorResolvedEvent)
	_ = m.cgroupResolver.RegisterListener(cgroup.CGroupDeleted, m.OnCGroupDeletedEvent)

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
			// create a new entry
			profile = NewSecurityProfile(workload.WorkloadSelector)
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

// FillProfileContextFromContainerID returns the profile of a container ID
func (m *SecurityProfileManager) FillProfileContextFromContainerID(id string, ctx *model.SecurityProfileContext) *SecurityProfile {
	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()

	var output *SecurityProfile
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

	return output
}

// FillProfileContextFromProfile fills the given ctx with profile infos
func FillProfileContextFromProfile(ctx *model.SecurityProfileContext, profile *SecurityProfile) {
	profile.Lock()
	defer profile.Unlock()

	ctx.Name = profile.Metadata.Name
	ctx.Version = profile.Version
	ctx.Tags = profile.Tags
	ctx.Status = profile.Status
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

	if profile.selector.IsEmpty() {
		// do not insert in cache
		return
	}

	// add profile in cache
	m.pendingCacheLock.Lock()
	defer m.pendingCacheLock.Unlock()
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
		profile = NewSecurityProfile(selector)
	}

	if profile.Version == newProfile.Version {
		// this is the same file, ignore
		return
	}

	profile.Lock()
	defer profile.Unlock()
	profile.loadedInKernel = false

	// decode the content of the profile
	protoToSecurityProfile(profile, newProfile)

	// prepare the profile for insertion
	m.prepareProfile(profile)

	if !ok {
		// insert in cache and leave
		m.pendingCacheLock.Lock()
		defer m.pendingCacheLock.Unlock()
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

// SendStats sends metrics about the Security Profile manager
func (m *SecurityProfileManager) SendStats() error {
	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()
	if val := float64(len(m.profiles)); val > 0 {
		if err := m.statsdClient.Gauge(metrics.MetricSecurityProfileActiveProfiles, val, []string{}, 1.0); err != nil {
			return fmt.Errorf("couldn't send MetricSecurityProfileActiveProfiles: %w", err)
		}
	}

	m.pendingCacheLock.Lock()
	defer m.pendingCacheLock.Unlock()
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

	for evtType, count := range m.eventFilteringNoProfile {
		tags := []string{fmt.Sprintf("event_type:%s", evtType)}
		if value := count.Swap(0); value > 0 {
			if err := m.statsdClient.Count(metrics.MetricSecurityProfileEventFiltering, int64(value), tags, 1.0); err != nil {
				return fmt.Errorf("couldn't send MetricSecurityProfileEventFiltering metric: %w", err)
			}
		}
	}

	for evtType, count := range m.eventFilteringAbsent {
		tags := []string{fmt.Sprintf("event_type:%s", evtType), "in_profile:false"}
		if value := count.Swap(0); value > 0 {
			if err := m.statsdClient.Count(metrics.MetricSecurityProfileEventFiltering, int64(value), tags, 1.0); err != nil {
				return fmt.Errorf("couldn't send MetricSecurityProfileEventFiltering metric: %w", err)
			}
		}
	}

	for evtType, count := range m.eventFilteringPresent {
		tags := []string{fmt.Sprintf("event_type:%s", evtType), "in_profile:true"}
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

	// push kernel space filters
	if err := m.securityProfileSyscallsMap.Put(profile.profileCookie, profile.generateSyscallsFilters()); err != nil {
		return fmt.Errorf("couldn't push syscalls filter: %w", err)
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
		seclog.Errorf("coudln't remove syscalls filter: %v", err)
	}

	// TODO: delete all kernel space programs
	seclog.Debugf("security profile %s (version:%s status:%s) unloaded from kernel space", profile.Metadata.Name, profile.Version, profile.Status.String())
}

// linkProfile (thread unsafe) updates the kernel space mapping between a workload and its profile
func (m *SecurityProfileManager) linkProfile(profile *SecurityProfile, workload *cgroupModel.CacheEntry) {
	if err := m.securityProfileMap.Put([]byte(workload.ID), profile.generateKernelSecurityProfileDefinition()); err != nil {
		seclog.Errorf("couldn't link workload %s (selector: %s) with profile %s: %v", workload.ID, workload.WorkloadSelector.String(), profile.Metadata.Name, err)
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

func (m *SecurityProfileManager) LookupEventOnProfiles(event *model.Event) {
	evtType := event.GetEventType()
	if evtType == model.SyscallsEventType || // syscall matching for anomaly detection is already done kernel side
		evtType == model.FileOpenEventType || evtType == model.BindEventType || // disabled for now
		evtType == model.ForkEventType || evtType == model.ExitEventType { // no interest in fork/exit events
		return
	}

	if event.Error != nil {
		m.eventFilteringAbsent[evtType].Inc()
		return
	}

	event.FieldHandlers.ResolveContainerID(event, &event.ContainerContext)
	event.FieldHandlers.ResolveContainerTags(event, &event.ContainerContext)
	if event.ContainerContext.ID == "" || len(event.ContainerContext.Tags) == 0 {
		return
	}

	// if time.Now()-event.ContainerContext.CreatedAt < time.Second*30 {
	// 	// TODO: put the event in a cache to be pop back after x sec to have a chance to
	// 	// retrieve a profile for that workload
	// }

	selector, err := cgroupModel.NewWorkloadSelector(utils.GetTagValue("image_name", event.ContainerContext.Tags), utils.GetTagValue("image_tag", event.ContainerContext.Tags))
	if err != nil {
		return
	}
	profile := m.GetProfile(selector)
	if profile == nil || profile.Status == 0 {
		m.eventFilteringNoProfile[evtType].Inc()
		return
	}

	FillProfileContextFromProfile(&event.SecurityProfileContext, profile)

	processNodes := profile.findProfileProcessNodes(event.ProcessContext)
	if len(processNodes) == 0 {
		m.eventFilteringAbsent[evtType].Inc()
		return
	}

	switch evtType {
	// for fork/exec/exit events, as we already found some nodes, no need to investigate further
	case model.ExecEventType:
		event.AddToFlags(model.EventFlagsSecurityProfileInProfile)

	case model.DNSEventType:
		if findDNSInNodes(processNodes, event) {
			event.AddToFlags(model.EventFlagsSecurityProfileInProfile)
		}
	}

	if event.IsInProfile() {
		m.eventFilteringPresent[evtType].Inc()
	} else {
		m.eventFilteringAbsent[evtType].Inc()
	}
}
