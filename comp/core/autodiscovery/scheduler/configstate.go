// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scheduler

import (
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/mohae/deepcopy"
)

// ConfigState represents the state of the config: scheduled or unscheduled
type ConfigState int8

// TaskStatus represents the status of the schedule task: success, failed or config not found
type TaskStatus int8

// Digest is the unique identifier of the config
type Digest uint64

const (
	// Scheduled config should be scheduled or is scheduled
	Scheduled ConfigState = 1 << iota
	// Unscheduled config is unscheduled or should be unscheduled
	Unscheduled
)

// ConfigStateData contains
// desiredState which is the eventually desired state of the config (to be scheduled or unscheduled)
type ConfigStateData struct {
	desiredState ConfigState
	config       *integration.Config
}

// copy returns a copy of ConfigStateData
func (c ConfigStateData) copy() ConfigStateData {
	configCopy := deepcopy.Copy(c.config).(*integration.Config)
	return ConfigStateData{
		desiredState: c.desiredState,
		config:       configCopy,
	}
}

// ConfigStateStore is in charge of storing
// 1. the scheduled integration.Config
// 2. desired state of the config to be scheduled/unscheduled compared to existing desiredState + currentState:
// for example, a new config has been called
// with [schedule, unschedule, schedule], the config will be scheduled, but only one schedule should be executed
type ConfigStateStore struct {
	configsLock sync.Mutex

	// Events to Controller would update immediately configStatusMap
	configStateMap map[Digest]ConfigStateData
}

// NewConfigStateStore creates a new NewConfigStateStore
func NewConfigStateStore() *ConfigStateStore {
	return &ConfigStateStore{
		configStateMap: make(map[Digest]ConfigStateData),
	}
}

// UpdateDesiredState update the desiredState of the config immediately
func (store *ConfigStateStore) UpdateDesiredState(changes integration.ConfigChanges) []Digest {
	store.configsLock.Lock()
	defer store.configsLock.Unlock()
	digests := make([]Digest, 0, len(changes.Unschedule)+len(changes.Schedule))
	if len(changes.Unschedule) > 0 {
		for idx, config := range changes.Unschedule {
			configDigest := Digest(config.FastDigest())
			store.configStateMap[configDigest] = ConfigStateData{
				desiredState: Unscheduled,
				config:       &changes.Unschedule[idx],
			}
			digests = append(digests, configDigest)
		}
	}
	if len(changes.Schedule) > 0 {
		for idx, config := range changes.Schedule {
			configDigest := Digest(config.FastDigest())
			store.configStateMap[configDigest] = ConfigStateData{
				desiredState: Scheduled,
				config:       &changes.Schedule[idx],
			}
			digests = append(digests, configDigest)
		}
	}
	return digests
}

// List returns the list of all configs states
func (store *ConfigStateStore) List() []ConfigStateData {
	store.configsLock.Lock()
	defer store.configsLock.Unlock()
	v := make([]ConfigStateData, 0, len(store.configStateMap))
	for _, value := range store.configStateMap {
		v = append(v, value.copy())
	}
	return v
}

// GetConfigState returns the config state
func (store *ConfigStateStore) GetConfigState(configDigest Digest) (ConfigStateData, bool) {
	store.configsLock.Lock()
	defer store.configsLock.Unlock()
	configStateData, found := store.configStateMap[configDigest]
	configStateDataCopy := configStateData.copy()
	return configStateDataCopy, found
}

// PurgeConfigStates purges all config desired states
func (store *ConfigStateStore) PurgeConfigStates() {
	store.configsLock.Lock()
	defer store.configsLock.Unlock()
	store.configStateMap = make(map[Digest]ConfigStateData)
}
