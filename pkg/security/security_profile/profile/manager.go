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
	"github.com/hashicorp/golang-lru/v2/simplelru"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	proto "github.com/DataDog/datadog-agent/pkg/security/proto/security_profile/v1"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

// SecurityProfileManager is used to manage Security Profiles
type SecurityProfileManager struct {
	config         *config.Config
	statsdClient   statsd.ClientInterface
	cgroupResolver *cgroup.Resolver
	providers      []Provider

	profilesLock sync.Mutex
	profiles     map[cgroupModel.WorkloadSelector]*SecurityProfile

	cacheLock sync.Mutex
	cache     *simplelru.LRU[cgroupModel.WorkloadSelector, *SecurityProfile]
	cacheHit  *atomic.Uint64
	cacheMiss *atomic.Uint64
}

// NewSecurityProfileManager returns a new instance of SecurityProfileManager
func NewSecurityProfileManager(config *config.Config, statsdClient statsd.ClientInterface, cgroupResolver *cgroup.Resolver) (*SecurityProfileManager, error) {
	var providers []Provider

	// instantiate directory provider
	if len(config.SecurityProfileDir) != 0 {
		dirProvider, err := NewDirectoryProvider(config.SecurityProfileDir, config.SecurityProfileWatchDir)
		if err != nil {
			return nil, fmt.Errorf("couldn't instantiate a new security profile directory provider: %w", err)
		}
		providers = append(providers, dirProvider)
	}

	profileCache, err := simplelru.NewLRU[cgroupModel.WorkloadSelector, *SecurityProfile](config.SBOMResolverWorkloadsCacheSize, nil)
	if err != nil {
		return nil, fmt.Errorf("couldn't create security profile cache: %w", err)
	}

	m := &SecurityProfileManager{
		config:         config,
		statsdClient:   statsdClient,
		providers:      providers,
		cgroupResolver: cgroupResolver,
		profiles:       make(map[cgroupModel.WorkloadSelector]*SecurityProfile),
		cache:          profileCache,
		cacheHit:       atomic.NewUint64(0),
		cacheMiss:      atomic.NewUint64(0),
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

	for {
		select {
		case <-ctx.Done():
			m.stop()
			return
		}
	}
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
		m.cacheLock.Lock()
		defer m.cacheLock.Unlock()
		profile, ok = m.cache.Get(workload.WorkloadSelector)
		if ok {
			// remove profile from cache
			_ = m.cache.Remove(workload.WorkloadSelector)

			// since the profile was in cache, it was removed from kernel space, load it now
			// (locking isn't necessary here, but added as a safeguard)
			profile.Lock()
			m.loadProfile(profile)
			profile.Unlock()

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
	found := false
	for _, w := range profile.Instances {
		if w.ID == workload.ID {
			found = true
		}
	}
	if found {
		// nothing to do, leave
		return
	}

	// update the list of tracked instances
	profile.Instances = append(profile.Instances, workload)

	// can we apply the profile or is it not ready yet ?
	if profile.loaded {
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

// OnCGroupDeletedEvent is used to handle a CGroupDeleted event
func (m *SecurityProfileManager) OnCGroupDeletedEvent(workload *cgroupModel.CacheEntry) {
	// lookup the profile
	profile := m.GetProfile(workload.WorkloadSelector)

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

	// cleanup profile before insertion in cache
	profile.reset()

	// add profile in cache
	m.cacheLock.Lock()
	defer m.cacheLock.Unlock()
	m.cache.Add(profile.selector, profile)

	// remove profile from kernel space
	m.unloadProfile(profile)
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
	profile.loaded = false

	// decode the content of the profile
	protoToSecurityProfile(profile, newProfile)

	// prepare the profile for insertion
	m.prepareProfile(profile)

	if !ok {
		// insert in cache and leave
		m.cacheLock.Lock()
		defer m.cacheLock.Unlock()
		m.cache.Add(selector, profile)
		return
	}

	// load the profile in kernel space
	m.loadProfile(profile)

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

	m.cacheLock.Lock()
	defer m.cacheLock.Unlock()
	if val := float64(m.cache.Len()); val > 0 {
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

	return nil
}

// prepareProfile (thread unsafe) generates eBPF programs and cookies to prepare for kernel space insertion
func (m *SecurityProfileManager) prepareProfile(profile *SecurityProfile) {
	// TODO: generate eBPF programs and prepare the workload to be inserted in kernel space
}

// loadProfile (thread unsafe) loads a Security Profile in kernel space
func (m *SecurityProfileManager) loadProfile(profile *SecurityProfile) {
	// TODO: load generated programs and push kernel space filters
	profile.loaded = true
}

// unloadProfile (thread unsafe) unloads a Security Profile from kernel space
func (m *SecurityProfileManager) unloadProfile(profile *SecurityProfile) {
	// TODO: delete all kernel space programs and map entries for this profile
}

// linkProfile (thread unsafe) updates the kernel space mapping between a workload and its profile
func (m *SecurityProfileManager) linkProfile(profile *SecurityProfile, workload *cgroupModel.CacheEntry) {
	// TODO: link profile <-> container ID in kernel space
}

// unlinkProfile (thread unsafe) updates the kernel space mapping between a workload and its profile
func (m *SecurityProfileManager) unlinkProfile(profile *SecurityProfile, workload *cgroupModel.CacheEntry) {
	// TODO: unlink profile <-> container ID in kernel space
}
