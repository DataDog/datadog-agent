// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package configfilesdiscoveryimpl

import (
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type processRetry struct {
	work                 configCollectionWork
	collecting           bool
	retryAfterCollection bool
	processEventSeen     bool
}

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
	s.processRetryMu.Lock()
	defer s.processRetryMu.Unlock()

	for _, retry := range s.processRetries {
		if retry.work.target.entityID != containerID || !retry.work.collector.MatchesCommandline(args) {
			continue
		}
		if retry.processEventSeen {
			continue
		}
		retry.processEventSeen = true
		if retry.collecting {
			retry.retryAfterCollection = true
			continue
		}
		retry.collecting = true
		s.enqueueProcessRetryLocked(retry)
	}
}

func (s *adScheduler) trackProcessRetry(work configCollectionWork) *processRetry {
	if work.target.runtime != RuntimeDocker && work.target.runtime != RuntimeKubernetes {
		return nil
	}

	retry := &processRetry{
		work:       work,
		collecting: true,
	}
	s.processRetryMu.Lock()
	s.processRetries[work.config.Digest()] = retry
	s.processRetryMu.Unlock()
	return retry
}

func (s *adScheduler) isCurrentCollection(work configCollectionWork) bool {
	if work.processRetry == nil {
		return true
	}
	s.processRetryMu.Lock()
	ok := s.processRetries[work.config.Digest()] == work.processRetry
	s.processRetryMu.Unlock()
	return ok
}

func (s *adScheduler) finishProcessRetryWithoutConfigFiles(work configCollectionWork) {
	if work.processRetry == nil {
		return
	}

	s.processRetryMu.Lock()
	retry := s.processRetries[work.config.Digest()]
	if retry != work.processRetry {
		s.processRetryMu.Unlock()
		return
	}
	if retry.retryAfterCollection {
		retry.retryAfterCollection = false
		s.enqueueProcessRetryLocked(retry)
	} else if retry.processEventSeen {
		delete(s.processRetries, work.config.Digest())
	} else {
		retry.collecting = false
	}
	s.processRetryMu.Unlock()
}

func (s *adScheduler) enqueueProcessRetryLocked(retry *processRetry) {
	work := retry.work
	work.processRetry = retry
	select {
	case <-s.ctx.Done():
		delete(s.processRetries, work.config.Digest())
	case s.collectionQueue <- work:
	default:
		delete(s.processRetries, work.config.Digest())
		log.Warnf("config files discovery collection queue is full, dropping process-triggered retry for integration %q service %q runtime %q", work.config.Name, work.config.ServiceID, work.target.runtime)
	}
}

func (s *adScheduler) removeProcessRetry(work configCollectionWork) bool {
	if work.processRetry == nil {
		return true
	}
	s.processRetryMu.Lock()
	defer s.processRetryMu.Unlock()
	if s.processRetries[work.config.Digest()] == work.processRetry {
		delete(s.processRetries, work.config.Digest())
		return true
	}
	return false
}
