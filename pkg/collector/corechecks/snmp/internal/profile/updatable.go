// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package profile

import (
	"sync"
	"time"
)

// UpdatableProvider is a thread-safe Provider that supports updating the default/user profiles
type UpdatableProvider struct {
	lock             sync.RWMutex
	defaultProfiles  ProfileConfigMap
	userProfiles     ProfileConfigMap
	resolvedProfiles ProfileConfigMap
	lastUpdated      time.Time
}

// type assertion
var _ Provider = (*UpdatableProvider)(nil)

// Update installs new user and default profiles.
func (up *UpdatableProvider) Update(userProfiles, defaultProfiles ProfileConfigMap, now time.Time) {
	up.lock.Lock()
	defer up.lock.Unlock()
	up.userProfiles = userProfiles
	up.defaultProfiles = defaultProfiles
	up.resolvedProfiles = resolveProfiles(up.userProfiles, up.defaultProfiles)
	up.lastUpdated = now
}

// HasProfile implements Provider.HasProfile
func (up *UpdatableProvider) HasProfile(profileName string) bool {
	up.lock.RLock()
	defer up.lock.RUnlock()
	_, ok := up.resolvedProfiles[profileName]
	return ok
}

// GetProfile implements Provider.GetProfile
func (up *UpdatableProvider) GetProfile(profileName string) *ProfileConfig {
	up.lock.RLock()
	defer up.lock.RUnlock()
	profile, ok := up.resolvedProfiles[profileName]
	if !ok {
		return nil
	}
	return &profile
}

// LastUpdated implements Provider.LastUpdated
func (up *UpdatableProvider) LastUpdated() time.Time {
	up.lock.RLock()
	defer up.lock.RUnlock()
	return up.lastUpdated
}

// GetProfileForSysObjectID implements Provider.GetProfileForSysObjectID
func (up *UpdatableProvider) GetProfileForSysObjectID(sysObjectID string) (*ProfileConfig, error) {
	up.lock.RLock()
	defer up.lock.RUnlock()
	return getProfileForSysObjectID(up.resolvedProfiles, sysObjectID)
}
