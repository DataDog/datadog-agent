// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package securityprofile holds security profiles related files
package securityprofile

import (
	"errors"

	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
	"github.com/cilium/ebpf"
)

// AddProfile adds a profile to the manager
func (m *Manager) AddProfile(profile *profile.Profile) {
	m.newProfiles <- profile
}

// FakeDumpOverweight fakes a dump stats to force triggering the load controller. For unitary tests purpose only.
func (m *Manager) FakeDumpOverweight(name string) {
	m.m.Lock()
	defer m.m.Unlock()
	for _, p := range m.activeDumps {
		if p.Profile.Metadata.Name == name {
			p.Profile.FakeOverweight()
		}
	}
}

// ListAllProfileStates list all profiles and their versions (debug purpose only)
func (m *Manager) ListAllProfileStates() {
	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()
	for _, profile := range m.profiles {
		profile.ListAllVersionStates()
	}
}

// GetProfile returns a profile by its selector
func (m *Manager) GetProfile(selector cgroupModel.WorkloadSelector) *profile.Profile {
	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()

	// check if this workload had a Security Profile
	return m.profiles[selector]
}

// EvictAllTracedCgroups blacklists all currently traced cgroups by adding them to the discarded map
func (m *Manager) EvictAllTracedCgroups() {
	if !m.config.RuntimeSecurity.ActivityDumpEnabled {
		return
	}

	// Iterate through the kernel traced_cgroups map and evict everything
	var cgroupInode uint64
	var cookie uint64
	iterator := m.tracedCgroupsMap.Iterate()

	var cgroupsToEvict []uint64
	for iterator.Next(&cgroupInode, &cookie) {
		cgroupsToEvict = append(cgroupsToEvict, cgroupInode)
	}

	if err := iterator.Err(); err != nil {
		seclog.Warnf("couldn't iterate over the map traced_cgroups: %v", err)
	}

	for _, cgroupInode := range cgroupsToEvict {
		// Add to discarded map to blacklist
		if err := m.tracedCgroupsDiscardedMap.Put(cgroupInode, uint8(1)); err != nil {
			if !errors.Is(err, ebpf.ErrKeyNotExist) {
				seclog.Warnf("couldn't add cgroup to discarded map: %v", err)
			}
		}
	}
}

// ClearTracedCgroups clears all entries from the traced cgroups map
func (m *Manager) ClearTracedCgroups() {
	if !m.config.RuntimeSecurity.ActivityDumpEnabled {
		return
	}

	m.m.Lock()
	defer m.m.Unlock()

	// First, disable and remove all active dumps AND add them to discarded map
	for _, ad := range m.activeDumps {
		// Add to discarded map BEFORE disabling
		if !ad.Profile.Metadata.CGroupContext.CGroupPathKey.IsNull() {
			if err := m.tracedCgroupsDiscardedMap.Put(ad.Profile.Metadata.CGroupContext.CGroupPathKey.Inode, uint8(1)); err != nil {
				if !errors.Is(err, ebpf.ErrKeyNotExist) {
					seclog.Warnf("couldn't add cgroup to discarded map: %v", err)
				}
			}
		}

		_ = m.disableKernelEventCollection(ad)
	}
	m.activeDumps = nil

	// Then clear the kernel maps (both traced and discarded)
	var err error
	var cgroupInode uint64
	var cookie uint64
	iterator := m.tracedCgroupsMap.Iterate()

	var cgroupsToDelete []uint64
	for iterator.Next(&cgroupInode, &cookie) {
		cgroupsToDelete = append(cgroupsToDelete, cgroupInode)
	}

	if err = iterator.Err(); err != nil {
		seclog.Warnf("couldn't iterate over the map traced_cgroups: %v", err)
	}

	for _, cgroupInode := range cgroupsToDelete {
		// Add to discarded map FIRST to prevent kernel from re-adding it
		if err := m.tracedCgroupsDiscardedMap.Put(cgroupInode, uint8(1)); err != nil {
			if !errors.Is(err, ebpf.ErrKeyNotExist) {
				seclog.Warnf("couldn't add cgroup to discarded map: %v", err)
			}
		}
		// Then delete from traced map
		_ = m.tracedCgroupsMap.Delete(cgroupInode)
	}
}
