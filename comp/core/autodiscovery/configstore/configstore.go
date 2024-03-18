// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package configstore implements the store for scheduled integration.Config

package configstore

import (
	"container/list"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

type EventType int8

const (
	SCHEDULE_TASK EventType = 1 << iota
	UNSCHEDULE_TASK
)

// SchedulerEvent is the task to do to store/remove configs to the config store
type SchedulerEvent struct {
	Type   EventType
	Config *integration.Config
	//TODO: add a counter to count retry attempts
}

// ConfigStore is in charge of storing
// 1. schedule/unschedule events for integration.Config
// 2. the scheduled integration.Config
type ConfigStore struct {
	configs_lock sync.Mutex
	// scheduledConfigs contains the set of configs that have been scheduled
	// via the metascheduler, but not subsequently unscheduled.
	scheduledConfigs map[string]integration.Config
	eventQueue       *list.List
}

// NewConfigStore creates a new ConfigStore
func NewConfigStore() *ConfigStore {
	return &ConfigStore{
		scheduledConfigs: make(map[string]integration.Config),
		eventQueue:       list.New(),
	}
}

// Push pushes a new config to the task queue to be processed,
// eventType: SCHEDULE_TASK or UNSCHEDULE_TASK
func (store *ConfigStore) Push(configs []integration.Config, eventType EventType) {
	store.configs_lock.Lock()
	defer store.configs_lock.Unlock()
	for _, config := range configs {
		if eventType == SCHEDULE_TASK {
			store.scheduledConfigs[config.Digest()] = config
		} else {
			delete(store.scheduledConfigs, config.Digest())
		}
		store.eventQueue.PushBack(&SchedulerEvent{
			Type:   eventType,
			Config: &config,
		})
	}
}

// List returns the list of all scheduled configs
func (store *ConfigStore) List() []integration.Config {
	store.configs_lock.Lock()
	defer store.configs_lock.Unlock()
	v := make([]integration.Config, 0, len(store.scheduledConfigs))
	for _, value := range store.scheduledConfigs {
		v = append(v, value)
	}
	return v
}

// HandleEvents processes the task queue
func (store *ConfigStore) HandleEvents(scheduleCB func(configs []integration.Config),
	unscheduleCB func(configs []integration.Config)) {
	var events *list.List
	store.configs_lock.Lock()
	events = store.eventQueue
	store.eventQueue = list.New()
	store.configs_lock.Unlock()

	if events.Len() == 0 {
		return
	}

	// Process the events
	var scheduleConfigs, unscheduleConfigs []integration.Config
	for e := events.Front(); e != nil; e = e.Next() {
		event := e.Value.(*SchedulerEvent)
		if event.Type == SCHEDULE_TASK {
			scheduleConfigs = append(scheduleConfigs, *event.Config)
		} else {
			unscheduleConfigs = append(unscheduleConfigs, *event.Config)
		}
		events.Remove(e)
	}
	//TODO: update the retry counter in case of failure
	scheduleCB(scheduleConfigs)
	unscheduleCB(unscheduleConfigs)
}

// PurgeEvents purges the task queue
func (store *ConfigStore) PurgeEvents() {
	store.configs_lock.Lock()
	defer store.configs_lock.Unlock()
	store.eventQueue.Init() //purge the event queue
}

func (store *ConfigStore) Stop() {
	store.configs_lock.Lock()
	defer store.configs_lock.Unlock()
	store.eventQueue.Init()
	clear(store.scheduledConfigs)
}
