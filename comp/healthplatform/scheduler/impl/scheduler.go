// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package schedulerimpl implements the health platform scheduler component.
package schedulerimpl

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	schedulerdef "github.com/DataDog/datadog-agent/comp/healthplatform/scheduler/def"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
)

const defaultCheckInterval = 15 * time.Minute

// registeredHealthCheck holds the state for a single scheduled health check.
type registeredHealthCheck struct {
	source       string
	fn           runnerdef.HealthCheckFunc
	interval     time.Duration
	lastIssueIDs map[string]struct{}
	stopCh       chan struct{}
}

// scheduler manages periodic health checks.
// Each registered check runs in its own goroutine with an independent ticker.
type scheduler struct {
	log    log.Component
	runner runnerdef.Component
	store  storedef.Component

	checks map[string]*registeredHealthCheck
	// checkMux guards checks, started, and each check's lastIssueIDs.
	checkMux sync.RWMutex

	started bool
	wg      sync.WaitGroup
}

// Requires defines the dependencies for the scheduler.
type Requires struct {
	Log       log.Component
	Lifecycle compdef.Lifecycle
	Runner    runnerdef.Component
	Store     storedef.Component
}

// NewComponent creates a new scheduler instance and registers its lifecycle hooks.
func NewComponent(reqs Requires) schedulerdef.Component {
	s := &scheduler{
		log:    reqs.Log,
		runner: reqs.Runner,
		store:  reqs.Store,
		checks: make(map[string]*registeredHealthCheck),
	}
	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: s.start,
		OnStop:  s.stop,
	})
	return s
}

func (s *scheduler) start(_ context.Context) error {
	s.log.Info("Starting health platform scheduler")
	s.checkMux.Lock()
	defer s.checkMux.Unlock()
	s.started = true
	for _, check := range s.checks {
		s.startCheck(check)
	}
	return nil
}

func (s *scheduler) stop(_ context.Context) error {
	s.log.Info("Stopping health platform scheduler")
	s.checkMux.Lock()
	s.started = false
	for _, check := range s.checks {
		close(check.stopCh)
	}
	s.checkMux.Unlock()
	s.wg.Wait()
	return nil
}

// Schedule registers fn to run at the given interval.
func (s *scheduler) Schedule(source string, fn runnerdef.HealthCheckFunc, interval time.Duration, initialIssueIDs []string) error {
	if source == "" {
		return errors.New("source cannot be empty")
	}
	if fn == nil {
		return errors.New("health check function cannot be nil")
	}
	if interval <= 0 {
		interval = defaultCheckInterval
	}

	s.checkMux.Lock()
	defer s.checkMux.Unlock()

	if _, exists := s.checks[source]; exists {
		return fmt.Errorf("health check for source %q is already registered", source)
	}

	lastIssueIDs := make(map[string]struct{}, len(initialIssueIDs))
	for _, id := range initialIssueIDs {
		lastIssueIDs[id] = struct{}{}
	}
	check := &registeredHealthCheck{
		source:       source,
		fn:           fn,
		interval:     interval,
		lastIssueIDs: lastIssueIDs,
		stopCh:       make(chan struct{}),
	}
	s.checks[source] = check

	if s.started {
		s.startCheck(check)
	}

	s.log.Debugf("Registered health check for source %q (interval: %v)", source, interval)
	return nil
}

func (s *scheduler) startCheck(check *registeredHealthCheck) {
	s.wg.Add(1)
	go s.runAndSchedule(check)
}

func (s *scheduler) runAndSchedule(check *registeredHealthCheck) {
	defer s.wg.Done()

	s.tick(check)

	ticker := time.NewTicker(check.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.tick(check)
		case <-check.stopCh:
			return
		}
	}
}

// tick runs the check once and resolves any issue ids that disappeared.
// If the run returns an error, lastIssueIDs is left unchanged so that a
// transient probe failure does not resolve still-active issues.
func (s *scheduler) tick(check *registeredHealthCheck) {
	newIDs, err := s.runner.Run(check.source, check.fn)
	if err != nil {
		s.log.Warnf("health check %q returned error: %v", check.source, err)
		return
	}

	newSet := make(map[string]struct{}, len(newIDs))
	for _, id := range newIDs {
		newSet[id] = struct{}{}
	}

	// Collect IDs to resolve before releasing the lock to avoid holding checkMux
	// while calling into the store (which acquires its own lock).
	s.checkMux.Lock()
	var toResolve []string
	for id := range check.lastIssueIDs {
		if _, still := newSet[id]; !still {
			toResolve = append(toResolve, id)
		}
	}
	check.lastIssueIDs = newSet
	s.checkMux.Unlock()
	for _, id := range toResolve {
		s.store.ResolveIssue(id)
	}
}
