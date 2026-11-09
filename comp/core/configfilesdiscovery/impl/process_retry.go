// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build docker || (cri && containerd) || test

package configfilesdiscoveryimpl

import (
	"time"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// processFallbackRegistry tracks container-backed AD configs that are still
// eligible for their single process-triggered recollection. Each key identifies
// a watchedConfig, the scheduler state retained from the config's first Schedule
// call until Unschedule. An entry is added for each newly watched Docker or
// Kubernetes config, remains while process events are unrelated or unusable,
// and is removed when the first usable matching event is consumed or the config
// is unscheduled.
type processFallbackRegistry map[string]struct{}

// startProcessFallbackListener subscribes to process events used to trigger
// the one-shot recollection for container-backed configs.
func (c *component) startProcessFallbackListener() {
	if c.store == nil {
		return
	}

	filter := workloadmeta.NewFilterBuilder().
		SetEventType(workloadmeta.EventTypeSet).
		AddKindWithEntityFilter(workloadmeta.KindProcess, func(entity workloadmeta.Entity) bool {
			process, ok := entity.(*workloadmeta.Process)
			return ok && process.ContainerID != "" && len(process.Cmdline) > 0
		}).
		Build()
	c.processEvents = c.store.Subscribe(schedulerName, workloadmeta.NormalPriority, filter)
	c.scheduler.startProcessEventListener(c.processEvents)
}

// stopProcessFallbackListener removes the process-event subscription, if one
// was created when the component started.
func (c *component) stopProcessFallbackListener() {
	if c.processEvents == nil {
		return
	}
	c.store.Unsubscribe(c.processEvents)
	c.processEvents = nil
}

// registerProcessFallbackLocked adds process fallback work for a newly
// scheduled container watch. The caller must hold s.mu.
func (s *adScheduler) registerProcessFallbackLocked(watch *watchedConfig) {
	if watch.target.runtime != RuntimeDocker && watch.target.runtime != RuntimeKubernetes {
		return
	}
	if s.pendingProcessFallbacks == nil {
		s.pendingProcessFallbacks = make(processFallbackRegistry)
	}
	s.pendingProcessFallbacks[watch.key] = struct{}{}
}

// removeProcessFallbackLocked discards any process fallback work associated
// with an unscheduled watch. The caller must hold s.mu.
func (s *adScheduler) removeProcessFallbackLocked(watch *watchedConfig) {
	delete(s.pendingProcessFallbacks, watch.key)
}

// startProcessEventListener consumes workloadmeta process events until the
// scheduler stops or the subscription closes.
func (s *adScheduler) startProcessEventListener(events <-chan workloadmeta.EventBundle) {
	s.workerDone.Add(1)
	go func() {
		defer s.workerDone.Done()
		for {
			select {
			case <-s.ctx.Done():
				return
			case bundle, ok := <-events:
				if !ok {
					return
				}
				s.handleProcessEventBundle(bundle)
			}
		}
	}()
}

// handleProcessEventBundle acknowledges one event bundle and forwards container
// process command lines to matching config watches.
func (s *adScheduler) handleProcessEventBundle(bundle workloadmeta.EventBundle) {
	defer bundle.Acknowledge()

	for _, event := range bundle.Events {
		process, ok := event.Entity.(*workloadmeta.Process)
		if !ok || process.ContainerID == "" || len(process.Cmdline) == 0 {
			continue
		}
		s.requestProcessFallback(process.ContainerID, TargetCommandline{
			Args:       process.Cmdline,
			WorkingDir: process.Cwd,
		})
	}
}

// requestProcessFallback consumes the first usable process command line for
// each matching watch and asks the collection state machine for one follow-up.
func (s *adScheduler) requestProcessFallback(containerID string, commandline TargetCommandline) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, watch := range s.watches {
		if watch.target.runtime != RuntimeDocker && watch.target.runtime != RuntimeKubernetes {
			continue
		}
		if watch.target.entityID != containerID {
			continue
		}
		if _, pending := s.pendingProcessFallbacks[watch.key]; !pending {
			continue
		}
		collector, found := s.collectors[watch.integration]
		if !found || !collector.CanCollectFromProcess(commandline) {
			continue
		}
		// Consume the one-shot fallback only after the state machine records the
		// recollection. In particular, a process seen during startup jitter must
		// not prevent a later process event from triggering the fallback after the
		// initial collection.
		if s.transitionWatchLocked(watch, recollectionRequested, time.Time{}) {
			delete(s.pendingProcessFallbacks, watch.key)
		}
	}
}
