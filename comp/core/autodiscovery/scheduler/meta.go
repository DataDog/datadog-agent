// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/configstore"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// MetaScheduler is a scheduler dispatching to all its registered schedulers
type MetaScheduler struct {
	// m protects all fields in this struct.
	m sync.Mutex

	// activeSchedulers is the set of schedulers currently subscribed to configs.
	activeSchedulers map[string]Scheduler

	// configStore contains the set of configs that have been scheduled and to be scheduled
	configStore *configstore.ConfigStore

	scheduledEventCh chan scheduledEvent

	stopCh chan bool

	started bool
}

type scheduledEvent struct {
	configs   []integration.Config
	eventType configstore.EventType
}

const (
	scheduledEventChBufferSize = 50
	reTryInterval              = 30 * time.Second
)

// NewMetaScheduler inits a meta scheduler
func NewMetaScheduler() *MetaScheduler {
	metaScheduler := MetaScheduler{
		activeSchedulers: make(map[string]Scheduler),
		configStore:      configstore.NewConfigStore(),
		scheduledEventCh: make(chan scheduledEvent, scheduledEventChBufferSize),
		stopCh:           make(chan bool),
		started:          false,
	}
	metaScheduler.start()
	return &metaScheduler
}

func (ms *MetaScheduler) start() {
	ms.m.Lock()
	if ms.started {
		return
	}
	ms.m.Unlock()
	ms.started = true
	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		for {
			select {

			case ev := <-ms.scheduledEventCh:
				ms.configStore.Push(ev.configs, ev.eventType)
				ms.processQueue()

			case <-ms.stopCh:
				ms.started = false
				return

			case <-ctx.Done():
				ms.started = false
				return
			}
		}
	}()
	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ticker := time.NewTicker(reTryInterval)
		for {
			select {
			case <-ticker.C:
				ms.processQueue()

			case <-ms.stopCh:
				ms.started = false
				return

			case <-ctx.Done():
				ms.started = false
				return
			}
		}
	}()
}

// Register a new scheduler to receive configurations.
//
// Previously scheduled configurations that have not subsequently been
// unscheduled can be replayed with the replayConfigs flag.  This replay occurs
// immediately, before the AddScheduler call returns.
func (ms *MetaScheduler) Register(name string, s Scheduler, replayConfigs bool) {
	ms.m.Lock()
	defer ms.m.Unlock()
	if _, ok := ms.activeSchedulers[name]; ok {
		log.Warnf("Scheduler %s already registered, overriding it", name)
	}
	ms.activeSchedulers[name] = s

	// if replaying configs, replay the currently-scheduled configs; note that
	// this occurs under the protection of `ms.m`, so no config may be double-
	// scheduled or missed in this process.
	if replayConfigs {
		configs := ms.configStore.List()
		s.Schedule(configs)
	}
}

// Deregister a scheduler in the meta scheduler to dispatch to
func (ms *MetaScheduler) Deregister(name string) {
	ms.m.Lock()
	defer ms.m.Unlock()
	if _, ok := ms.activeSchedulers[name]; !ok {
		log.Warnf("Scheduler %s no registered, skipping", name)
		return
	}
	delete(ms.activeSchedulers, name)
}

// Schedule schedules configs to all registered schedulers
func (ms *MetaScheduler) Schedule(configs []integration.Config) {
	for _, config := range configs {
		log.Tracef("Scheduling %s\n", config.Dump(false))
	}
	ms.scheduledEventCh <- scheduledEvent{
		configs:   configs,
		eventType: configstore.SCHEDULE_TASK,
	}
}

// Unschedule unschedules configs to all registered schedulers
func (ms *MetaScheduler) Unschedule(configs []integration.Config) {

	for _, config := range configs {
		log.Tracef("Unscheduling %s\n", config.Dump(false))
	}
	ms.scheduledEventCh <- scheduledEvent{
		configs:   configs,
		eventType: configstore.UNSCHEDULE_TASK,
	}
}

func (ms *MetaScheduler) processQueue() {
	ms.m.Lock()
	defer ms.m.Unlock()
	for _, scheduler := range ms.activeSchedulers {
		ms.configStore.HandleEvents(scheduler.Schedule, scheduler.Unschedule)
	}
}

// Stop handles clean stop of registered schedulers
func (ms *MetaScheduler) Stop() {
	ms.m.Lock()
	defer ms.m.Unlock()
	ms.stopCh <- true
	ms.configStore.Stop()
	for _, scheduler := range ms.activeSchedulers {
		scheduler.Stop()
	}
}

// PurgeStoreEvents purges task events in the config store
// Used for testing purposes
func (ms *MetaScheduler) PurgeStoreEvents() {
	ms.m.Lock()
	defer ms.m.Unlock()
	ms.configStore.PurgeEvents()
}
