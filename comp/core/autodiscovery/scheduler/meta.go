// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scheduler

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"
)

const (
	// MaxRetries is the maximum number of retries for a failed task
	MaxRetries = 5
)

// MetaScheduler is a scheduler dispatching to all its registered schedulers
type MetaScheduler struct {
	// m protects all fields in this struct.
	m sync.Mutex

	// activeSchedulers is the set of schedulers currently subscribed to configs.
	activeSchedulers map[string]Scheduler

	// scheduledConfigs contains the set of configs that have been scheduled
	// via the metascheduler, but not subsequently unscheduled.
	scheduledConfigs map[string]*integration.Config

	// configStateStore contains the desired state of configs
	configStateStore *ConfigStateStore

	// a workqueue to process the config events
	queue workqueue.RateLimitingInterface

	started     bool
	stopChannel chan struct{}
}

type workItem struct {
	config *integration.Config
}

// NewMetaScheduler inits a meta scheduler
func NewMetaScheduler() *MetaScheduler {
	metaScheduler := MetaScheduler{
		scheduledConfigs: make(map[string]*integration.Config),
		activeSchedulers: make(map[string]Scheduler),
		queue:            workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "MetaScheduler"),
		stopChannel:      make(chan struct{}),
		configStateStore: NewConfigStateStore(),
	}
	metaScheduler.start()
	return &metaScheduler
}

func (ms *MetaScheduler) start() {
	ms.m.Lock()
	if ms.started {
		return
	}
	ms.started = true
	ms.m.Unlock()
	go wait.Until(ms.worker, time.Second, ms.stopChannel)
}

// Register a new scheduler to receive configurations.
// Previously scheduled configurations that have not subsequently been
// unscheduled can be replayed with the replayConfigs flag.  This replay occurs
// immediately, before the AddScheduler call returns.
func (ms *MetaScheduler) Register(name string, s Scheduler, replayConfigs bool) {
	ms.m.Lock()
	if _, ok := ms.activeSchedulers[name]; ok {
		log.Warnf("Scheduler %s already registered, overriding it", name)
	}
	ms.activeSchedulers[name] = s
	ms.m.Unlock()

	// if replaying configs, replay the currently-scheduled configs; note that
	// this occurs under the protection of `ms.m`, so no config may be double-
	// scheduled or missed in this process.
	if replayConfigs {
		configStates := ms.configStateStore.List()

		configs := make([]integration.Config, 0, len(configStates))
		for _, config := range configStates {
			if config.desiredState == Scheduled {
				configs = append(configs, *config.config)
			}
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

// Schedule updates desired state of configs and add to queue
func (ms *MetaScheduler) Schedule(configs []integration.Config) {
	ms.configStateStore.UpdateDesiredState(configs, Scheduled)
	for _, config := range configs {
		log.Tracef("Scheduling %s\n", config.Dump(false))
		ms.queue.AddRateLimited(workItem{config: &config})
	}
}

// Unschedule updates desired state of configs and add to queue
func (ms *MetaScheduler) Unschedule(configs []integration.Config) {
	ms.configStateStore.UpdateDesiredState(configs, Unscheduled)
	for _, config := range configs {
		log.Tracef("Unscheduling %s\n", config.Dump(false))
		ms.queue.AddRateLimited(workItem{config: &config})
	}
}

func (ms *MetaScheduler) worker() {
	for ms.processNextWorkItem() {
	}
}

func (ms *MetaScheduler) processNextWorkItem() bool {
	item, quit := ms.queue.Get()

	if quit {
		return false
	}
	config := item.(workItem).config
	configDigest := (*config).Digest()
	configName := (*config).Name
	status := TaskSuccess
	currentState := Unscheduled
	newState := Unscheduled
	if _, found := ms.scheduledConfigs[configDigest]; found {
		currentState = Scheduled
	}
	ms.m.Lock() //lock on activeSchedulers
	for _, scheduler := range ms.activeSchedulers {
		status, newState = ms.configStateStore.HandleEvents(Digest(configDigest),
			configName,
			currentState,
			scheduler.Schedule,
			scheduler.Unschedule)
		if status == TaskFailed {
			log.Debugf("Failed to handle event for config %s", configName)
			break
		}
	}
	ms.m.Unlock()

	if status == TaskFailed {
		attempt := ms.queue.NumRequeues(item)
		if attempt < MaxRetries {
			ms.queue.AddRateLimited(item)
		} else {
			log.Warnf("Failed to handle event for config %s after %d attempts, giving up", configName, attempt)
		}
	} else if status == TaskSuccess {
		ms.queue.Forget(item)
		if newState == Scheduled {
			ms.scheduledConfigs[configDigest] = config
		} else {
			delete(ms.scheduledConfigs, configDigest)
		}
	}
	ms.queue.Done(item)
	return true
}

// Stop handles clean stop of registered schedulers
func (ms *MetaScheduler) Stop() {
	ms.m.Lock()
	defer ms.m.Unlock()
	for _, scheduler := range ms.activeSchedulers {
		scheduler.Stop()
	}
	close(ms.stopChannel)
	ms.queue.ShutDown()
	ms.started = false
}
