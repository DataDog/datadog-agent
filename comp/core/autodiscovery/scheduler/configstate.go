// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scheduler

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

// ConfigState represents the state of the config: scheduled or unscheduled
type ConfigState int8

// TaskStatus represents the status of the schedule task: success, failed or config not found
type TaskStatus int8

// Digest is the unique identifier of the config
type Digest string

const (
	// Scheduled config should be scheduled or is scheduled
	Scheduled ConfigState = 1 << iota
	// UnScheduled config is unscheduled or should be unscheduled
	Unscheduled
)

const (
	// TaskSuccess handleEvents is successful
	TaskSuccess TaskStatus = 1 << iota
	// TaskFailed handleEvents is failed
	TaskFailed
	// ConfigNotFound config is not found in configStateMap
	ConfigNotFound
)

// ConfigStateData contains
// desiredState which is the eventually desired state of the config (to be scheduled or unscheduled)
type ConfigStateData struct {
	desiredState ConfigState
	config       *integration.Config
}

// ConfigStateStore is in charge of storing
// 1. the scheduled integration.Config
// 2. desired state of the config to be scheduled/unscheduled compared to existing desiredState + currentState:
// for example, a new config has been called
// with [schedule, unschedule, schedule], the config will be scheduled, but only one schedule should be executed
type ConfigStateStore struct {
	configsLock sync.Mutex

	// Events to MetaScheduler would update immediately configStatusMap
	configStateMap map[Digest]ConfigStateData
}

// NewConfigStateStore creates a new NewConfigStateStore
func NewConfigStateStore() *ConfigStateStore {
	return &ConfigStateStore{
		configStateMap: make(map[Digest]ConfigStateData),
	}
}

// UpdateDesiredState update the desiredState of the config immediately
func (store *ConfigStateStore) UpdateDesiredState(configs []integration.Config, desireState ConfigState) {
	store.configsLock.Lock()
	defer store.configsLock.Unlock()
	for idx, config := range configs {
		configDigest := Digest(config.Digest())
		store.configStateMap[configDigest] = ConfigStateData{
			desiredState: desireState,
			config:       &configs[idx],
		}
	}
}

// List returns the list of all configs states
func (store *ConfigStateStore) List() []ConfigStateData {
	store.configsLock.Lock()
	defer store.configsLock.Unlock()
	v := make([]ConfigStateData, 0, len(store.configStateMap))
	for _, value := range store.configStateMap {
		v = append(v, value)
	}
	return v
}

// HandleEvents processes the task queue
// Return execute status and attempt count
// Action type will be calculated as following:
// Current State,   Desired State     Action
// Unscheduled,     Schedule,         Schedule
// Unscheduled,     Unschedule,       None
// Scheduled,       Schedule,         None
// Scheduled,       Unschedule,       Unschedule
func (store *ConfigStateStore) HandleEvents(configDigest Digest,
	configName string,
	currentState ConfigState,
	scheduleCB func(configs []integration.Config),
	unscheduleCB func(configs []integration.Config)) (TaskStatus, ConfigState) {

	store.configsLock.Lock()
	configStateData, found := store.configStateMap[configDigest]
	if !found {
		log.Warnf("config %s not found in configStatusMap", configName)
		return ConfigNotFound, currentState
	}
	store.configStateMap[configDigest] = configStateData
	store.configsLock.Unlock()

	if configStateData.desiredState == currentState {
		//desired state has been achieved
		return TaskSuccess, configStateData.desiredState
	}

	if configStateData.desiredState == Scheduled {
		//to be scheduled
		scheduleCB([]integration.Config{*configStateData.config}) // TODO: check status of action
	} else {
		//to be unscheduled
		unscheduleCB([]integration.Config{*configStateData.config})
	}
	return TaskSuccess, configStateData.desiredState
}

// PurgeConfigStates purges all config desired states
func (store *ConfigStateStore) PurgeConfigStates() {
	store.configsLock.Lock()
	defer store.configsLock.Unlock()
	store.configStateMap = make(map[Digest]ConfigStateData)
}
