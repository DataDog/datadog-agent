// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests

// Package securityprofile holds security profiles related files
package securityprofile

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/security_profile/profile"
)

// GetProfile returns the live profile of the provided selector, or nil if the manager has none
func (m *ManagerV2) GetProfile(selector cgroupModel.WorkloadSelector) *profile.Profile {
	m.profilesLock.Lock()
	defer m.profilesLock.Unlock()
	return m.profiles[selector]
}

// GetProfileSnapshot returns a decoded copy of the profile of the provided selector, so that its activity tree
// can be inspected while the manager keeps inserting events in the live profile
func (m *ManagerV2) GetProfileSnapshot(selector cgroupModel.WorkloadSelector) (*profile.Profile, error) {
	live := m.GetProfile(selector)
	if live == nil {
		return nil, nil
	}

	raw, err := live.Encode(config.Profile)
	if err != nil {
		return nil, err
	}

	snapshot := profile.New(
		profile.WithWorkloadSelector(selector),
		profile.WithEventTypes(m.config.RuntimeSecurity.SecurityProfileV2EventTypes),
	)
	if err := snapshot.DecodeFromReader(raw, config.Profile); err != nil {
		return nil, err
	}
	return snapshot, nil
}

// PersistAllProfiles persists all the profiles, as the persistence ticker does
func (m *ManagerV2) PersistAllProfiles() {
	m.persistAllProfiles()
}

// EvictUnusedNodes runs one node eviction cycle, as the eviction ticker does
func (m *ManagerV2) EvictUnusedNodes() {
	m.evictUnusedNodes()
}

// ProfilingStartupDelayRemaining returns how long the manager keeps ignoring events because of the profiling
// startup delay, or a negative duration once the delay is over
func (m *ManagerV2) ProfilingStartupDelayRemaining() time.Duration {
	now := m.resolvers.TimeResolver.ComputeMonotonicTimestamp(time.Now())
	return m.config.RuntimeSecurity.SecurityProfileV2ProfilingStartupDelay - time.Duration(now-m.startTimeMono)
}
