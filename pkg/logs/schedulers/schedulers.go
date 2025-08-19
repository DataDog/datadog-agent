// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package schedulers

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// Schedulers manages a collection of schedulers.
type Schedulers struct {
	// mgr is the SourceManager that will be given to schedulers
	mgr SourceManager

	// schedulers is the set of running schedulers
	schedulers []Scheduler

	// started is true after Start
	started bool
}

// NewSchedulers creates a new, empty Schedulers instance
func NewSchedulers(sources *sources.LogSources, services *service.Services) *Schedulers {
	return &Schedulers{
		mgr: &sourceManager{sources, services},
	}
}

// AddScheduler adds a scheduler to the collection.  If called after Start(), then the
// scheduler will be started immediately.
func (ss *Schedulers) AddScheduler(scheduler Scheduler) {
	ss.schedulers = append(ss.schedulers, scheduler)
	if ss.started {
		scheduler.Start(ss.mgr)
	}
}

// GetSources returns all the log source from the source manager.
func (ss *Schedulers) GetSources() []*sources.LogSource {
	return ss.mgr.GetSources()
}

// Start starts all schedulers in the collection.
func (ss *Schedulers) Start() {
	for _, s := range ss.schedulers {
		s.Start(ss.mgr)
	}
	ss.started = true
}

// Stop all schedulers and wait until they are complete.
func (ss *Schedulers) Stop() {
	var wg sync.WaitGroup
	for _, s := range ss.schedulers {
		wg.Add(1)
		go func(s Scheduler) {
			defer wg.Done()
			s.Stop()
		}(s)
	}
	wg.Wait()
}
