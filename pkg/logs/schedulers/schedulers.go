// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package schedulers

import (
	"sync"

	logsConfig "github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Schedulers manages a collection of schedulers.
type Schedulers struct {
	// schedulers is the set of running schedulers
	schedulers []Scheduler

	// started is true after Start
	started bool
}

// NewSchedulers creates a new, empty Schedulers instance
func NewSchedulers() *Schedulers {
	return &Schedulers{}
}

// AddScheduler adds a scheduler to the collection.  This must be called before Start.
func (ss *Schedulers) AddScheduler(scheduler Scheduler) {
	if ss.started {
		log.Error("Schedulers.AddScheduler called after Start()")
		return
	}
	ss.schedulers = append(ss.schedulers, scheduler)
}

// Start starts all schedulers in the collection.
func (ss *Schedulers) Start(sources *logsConfig.LogSources, services *service.Services) {
	mgr := &sourceManager{sources, services}
	ss.started = true
	for _, s := range ss.schedulers {
		s.Start(mgr)
	}
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
