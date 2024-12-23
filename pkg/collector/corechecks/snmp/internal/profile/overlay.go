// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package profile

import (
	"sync"
	"time"
)

// OverlayProvider sits atop an UpdatableProvider, updating when it does and
// overlaying a set of initial profiles over the user and default profiles
// provided by RC.
type OverlayProvider struct {
	base             *UpdatableProvider
	lastUpdated      time.Time
	initProfiles     ProfileConfigMap
	lock             sync.RWMutex
	resolvedProfiles ProfileConfigMap
}

// ensureUpdate is called before every normal method to check if our base has
// updated since our last update - if so, it rebuilds our computed
// resolvedProfiles.
func (op *OverlayProvider) ensureUpdate() {
	if op.lastUpdated.After(op.base.LastUpdated()) {
		// Nothing to d, up-to-date
		return
	}
	op.lock.Lock()
	defer op.lock.Unlock()
	// double check in case we updated during the Lock()
	if op.lastUpdated.After(op.base.LastUpdated()) {
		// Nothing to d, up-to-date
		return
	}
	mergedProfiles := mergeProfiles(op.base.getUserProfiles(), op.initProfiles)
	op.resolvedProfiles = resolveProfiles(mergedProfiles, op.base.getDefaultProfiles())
	op.lastUpdated = time.Now()
	return
}

func (op *OverlayProvider) HasProfile(profileName string) bool {
	op.ensureUpdate()
	op.lock.RLock()
	defer op.lock.RUnlock()
	_, ok := op.resolvedProfiles[profileName]
	return ok
}

func (op *OverlayProvider) GetProfile(profileName string) *ProfileConfig {
	op.ensureUpdate()
	op.lock.RLock()
	defer op.lock.RUnlock()
	profile, ok := op.resolvedProfiles[profileName]
	if !ok {
		return nil
	}
	return &profile
}

func (op *OverlayProvider) LastUpdated() time.Time {
	op.ensureUpdate()
	op.lock.RLock()
	defer op.lock.RUnlock()
	return op.base.LastUpdated()
}

func (op *OverlayProvider) GetProfileForSysObjectID(sysObjectID string) (*ProfileConfig, error) {
	op.ensureUpdate()
	op.lock.RLock()
	defer op.lock.RUnlock()
	return getProfileForSysObjectID(op.resolvedProfiles, sysObjectID)
}

// type assertion
var _ Provider = (*OverlayProvider)(nil)
