// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package configfilesdiscoveryimpl

import (
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

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

func (s *adScheduler) handleProcessEventBundle(bundle workloadmeta.EventBundle) {
	defer bundle.Acknowledge()

	for _, event := range bundle.Events {
		process, ok := event.Entity.(*workloadmeta.Process)
		if !ok || process.ContainerID == "" || len(process.Cmdline) == 0 {
			continue
		}
		s.retryProcessCollections(process.ContainerID, process.Cmdline)
	}
}

func (s *adScheduler) retryProcessCollections(containerID string, args []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, watch := range s.watches {
		if watch.target.runtime != RuntimeDocker && watch.target.runtime != RuntimeKubernetes {
			continue
		}
		if watch.target.entityID != containerID || watch.processRetryDisabled || watch.processEventSeen {
			continue
		}
		collector, found := s.collectors[watch.integration]
		if !found || !collector.MatchesCommandline(args) {
			continue
		}

		watch.processEventSeen = true
		if !watch.processRetryReady || watch.inFlight {
			watch.processRetryPending = true
			continue
		}
		s.enqueueCollectionLocked(watch)
	}
}
