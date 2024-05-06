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
// MaxRetries is the maximum number of retries for a failed task (TODO: implement retries on failure)
// MaxRetries = 5
)

// Controller is a scheduler dispatching to all its registered schedulers
type Controller struct {
	// m protects all fields in this struct.
	m sync.Mutex

	// activeSchedulers is the set of schedulers currently subscribed to configs.
	activeSchedulers map[string]Scheduler

	// scheduledConfigs contains the set of configs that have been scheduled
	// via the schedulerController, but not subsequently unscheduled.
	scheduledConfigs map[string]*integration.Config

	// ConfigStateStore contains the desired state of configs
	ConfigStateStore *ConfigStateStore

	// a workqueue to process the config events
	queue workqueue.RateLimitingInterface

	started     bool
	stopChannel chan struct{}
}

type workItem struct {
	digest Digest
}

// NewController inits a scheduler controller
func NewController() *Controller {
	schedulerController := Controller{
		scheduledConfigs: make(map[string]*integration.Config),
		activeSchedulers: make(map[string]Scheduler),
		queue:            workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Controller"),
		stopChannel:      make(chan struct{}),
		ConfigStateStore: NewConfigStateStore(),
	}
	schedulerController.start()
	return &schedulerController
}

func (ms *Controller) start() {
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
func (ms *Controller) Register(name string, s Scheduler, replayConfigs bool) {
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
		configStates := ms.ConfigStateStore.List()

		configs := make([]integration.Config, 0, len(configStates))
		for _, config := range configStates {
			if config.desiredState == Scheduled {
				configs = append(configs, *config.config)
			}
		}
		s.Schedule(configs)
	}
}

// Deregister a scheduler in the schedulerController to dispatch to
func (ms *Controller) Deregister(name string) {
	ms.m.Lock()
	defer ms.m.Unlock()
	if _, ok := ms.activeSchedulers[name]; !ok {
		log.Warnf("Scheduler %s no registered, skipping", name)
		return
	}
	delete(ms.activeSchedulers, name)
}

// ApplyChanges add configDigests to the workqueue
func (ms *Controller) ApplyChanges(digests []Digest) {
	for _, configDigest := range digests {
		ms.queue.Add(workItem{digest: configDigest})
	}
}

// Schedule for test only, actual scheduling should be done by the autoconfig.applyChanges
func (ms *Controller) Schedule(configs []integration.Config) {
	digests := ms.ConfigStateStore.UpdateDesiredState(integration.ConfigChanges{Schedule: configs})
	ms.ApplyChanges(digests)
}

// Unschedule for test only, actual unscheduling should be done by the autoconfig.applyChanges
func (ms *Controller) Unschedule(configs []integration.Config) {
	digests := ms.ConfigStateStore.UpdateDesiredState(integration.ConfigChanges{Unschedule: configs})
	ms.ApplyChanges(digests)
}

func (ms *Controller) worker() {
	for ms.processNextWorkItem() {
	}
}

// processNextWorkItem processes the next work item in the queue
// Action type will be calculated as following:
// Current State,   Desired State     Action
// Unscheduled,     Schedule,         Schedule
// Unscheduled,     Unschedule,       None
// Scheduled,       Schedule,         None
// Scheduled,       Unschedule,       Unschedule
func (ms *Controller) processNextWorkItem() bool {
	item, quit := ms.queue.Get()
	if quit {
		return false
	}
	configDigest := item.(workItem).digest
	configState, found := ms.ConfigStateStore.GetConfigState(configDigest)
	if !found {
		log.Warnf("config %s not found in ConfigStateStore", configDigest)
		ms.queue.Done(item)
		return true
	}

	currentState := Unscheduled
	desiredState := configState.desiredState
	configName := configState.config.Name
	if _, found := ms.scheduledConfigs[string(configDigest)]; found {
		currentState = Scheduled
	}
	if desiredState == currentState {
		ms.queue.Done(item) // no action needed
		return true
	}
	log.Tracef("Controller starts processing config %s: currentState: %d, desiredState: %d", configName, currentState, desiredState)
	ms.m.Lock() //lock on activeSchedulers
	for _, scheduler := range ms.activeSchedulers {
		if desiredState == Scheduled {
			//to be scheduled
			scheduler.Schedule(([]integration.Config{*configState.config})) // TODO: check status of action
			ms.scheduledConfigs[string(configDigest)] = configState.config
		} else {
			//to be unscheduled
			scheduler.Unschedule(([]integration.Config{*configState.config})) // TODO: check status of action
			delete(ms.scheduledConfigs, string(configDigest))
		}
	}
	ms.m.Unlock()
	ms.queue.Done(item)
	return true
}

// Stop handles clean stop of registered schedulers
func (ms *Controller) Stop() {
	ms.m.Lock()
	defer ms.m.Unlock()
	for _, scheduler := range ms.activeSchedulers {
		scheduler.Stop()
	}
	close(ms.stopChannel)
	ms.queue.ShutDown()
	ms.started = false
	ms.scheduledConfigs = make(map[string]*integration.Config)
}

// Purge removes all scheduled configs and desired states, testing only
func (ms *Controller) Purge() {
	ms.queue.ShutDown()
	ms.queue = workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Controller")
	ms.m.Lock()
	defer ms.m.Unlock()
	ms.scheduledConfigs = make(map[string]*integration.Config)
	ms.ConfigStateStore.PurgeConfigStates()
}
