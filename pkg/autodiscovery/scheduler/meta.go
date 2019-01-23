// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package scheduler

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// MetaScheduler is a scheduler dispatching to all its registered schedulers
type MetaScheduler struct {
	m                sync.Mutex
	activeSchedulers map[string]Scheduler
}

// NewMetaScheduler inits a meta scheduler
func NewMetaScheduler() *MetaScheduler {
	return &MetaScheduler{
		activeSchedulers: make(map[string]Scheduler),
	}
}

// Register a scheduler in the meta scheduler to dispatch to
func (ms *MetaScheduler) Register(name string, s Scheduler) {
	ms.m.Lock()
	defer ms.m.Unlock()
	if _, ok := ms.activeSchedulers[name]; ok {
		log.Warnf("Scheduler %s already registered, overriding it", name)
	}
	ms.activeSchedulers[name] = s
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
	for _, scheduler := range ms.activeSchedulers {
		scheduler.Schedule(configs)
	}
}

// Unschedule unschedules configs to all registered schedulers
func (ms *MetaScheduler) Unschedule(configs []integration.Config) {
	ms.m.Lock()
	defer ms.m.Unlock()
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
