// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package rcsnmpprofiles

import (
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"sync"
)

// Callback is when profiles updates are available (rc product NDM_DEVICE_PROFILES_CUSTOM)
func (rc *RemoteConfigSNMPProfilesManager) Callback(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	log.Infof("RC Callback, updates: %+v", updates)
}

// RemoteConfigSNMPProfilesManager receives configuration from remote-config
type RemoteConfigSNMPProfilesManager struct {
	mu       sync.RWMutex
	upToDate bool
}

// NewRemoteConfigSNMPProfilesManager creates a new RemoteConfigSNMPProfilesManager.
func NewRemoteConfigSNMPProfilesManager() *RemoteConfigSNMPProfilesManager {
	return &RemoteConfigSNMPProfilesManager{
		upToDate: false,
	}
}
