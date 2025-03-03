// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package securityprofile holds security profiles related files
package securityprofile

import (
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
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
