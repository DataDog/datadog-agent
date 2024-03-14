// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scheduler

import (
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// MetaScheduler is a scheduler dispatching to all its registered schedulers
type MetaScheduler struct {
	// m protects all fields in this struct.
	m sync.Mutex

	// scheduledConfigs contains the set of configs that have been scheduled
	// via the metascheduler, but not subsequently unscheduled.
	scheduledConfigs map[string]integration.Config

	// activeSchedulers is the set of schedulers currently subscribed to configs.
	activeSchedulers map[string]Scheduler
}

// NewMetaScheduler inits a meta scheduler
func NewMetaScheduler() *MetaScheduler {
	return &MetaScheduler{
		scheduledConfigs: make(map[string]integration.Config),
		activeSchedulers: make(map[string]Scheduler),
	}
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
		configs := make([]integration.Config, 0, len(ms.scheduledConfigs))
		for _, config := range ms.scheduledConfigs {
			configs = append(configs, config)
		}
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
	ms.m.Lock()
	defer ms.m.Unlock()
	for _, config := range configs {
		log.Tracef("Scheduling %s\n", config.Dump(false))
		ms.scheduledConfigs[config.Digest()] = config
	}
	for _, scheduler := range ms.activeSchedulers {
		scheduler.Schedule(configs)
	}
}

// Unschedule unschedules configs to all registered schedulers
func (ms *MetaScheduler) Unschedule(configs []integration.Config) {
	ms.m.Lock()
	defer ms.m.Unlock()
	for _, config := range configs {
		log.Tracef("Unscheduling %s\n", config.Dump(false))
		delete(ms.scheduledConfigs, config.Digest())
	}
	for _, scheduler := range ms.activeSchedulers {
		scheduler.Unschedule(configs)
	}
}

// Stop handles clean stop of registered schedulers
func (ms *MetaScheduler) Stop() {
	ms.m.Lock()
	defer ms.m.Unlock()
	for _, scheduler := range ms.activeSchedulers {
		scheduler.Stop()
	}
}
