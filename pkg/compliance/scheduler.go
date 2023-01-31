// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compliance

import (
	"context"
	"math/rand"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Scheduler is a simple scheduling interface for starting/stopping rule evaluation scheduling.
type Scheduler interface {
	StartScheduling(checks []Check)
	StopScheduling(ctx context.Context)
}

type periodicScheduler struct {
	started bool
	stop    chan struct{}
	done    chan struct{}
}

// NewPeriodicScheduler returns a default scheduler used for scheduling rule evaluation,
// for this compliance module. It is a bare minimum setup that schedules checks at constant
// and periodic and constant intervals, depending on their period.
//
// Checks with identical time periods are groupped together and scheduled in the same
// goroutine. In other words, we schedule as many goroutine as defined periods.
func NewPeriodicScheduler() Scheduler {
	return &periodicScheduler{
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
}

// Start will start the scheduling of our checks.
func (s *periodicScheduler) StartScheduling(checks []Check) {
	if s.started {
		panic("(programmer error) compliance: scheduler already started")
	}
	s.started = true
	// We group checks per time period, and run one goroutine for each period.
	// A jittered sleep time is added before starting the first rule to avoid
	// synchronized run at the beginning.
	buckets := make(map[time.Duration][]Check, len(checks))
	for _, check := range checks {
		period := check.Period()
		buckets[period] = append(buckets[period], check)
	}
	finish := make(chan struct{}, len(buckets))
	for period, checks := range buckets {
		go s.runPeriodicChecks(checks, period, finish)
	}
	go func() {
		for i := 0; i < len(buckets); i++ {
			<-finish
		}
		close(s.done)
	}()
}

func (s *periodicScheduler) runPeriodicChecks(
	checks []Check,
	period time.Duration,
	finish chan struct{},
) {
	jitterRng := rand.New(rand.NewSource(period.Nanoseconds()))

	checkIndex := 0
	runCheck := func() {
		check := checks[checkIndex]
		log.Infof("compliance: running check %q", check)
		if err := check.Run(); err != nil {
			log.Errorf("compliance: error running check %q: %v", check, err)
		}
		checkIndex = (checkIndex + 1) % len(checks)
	}

	interval := time.Duration(int(period.Milliseconds())/len(checks)) * time.Millisecond
	jitter := time.Duration(jitterRng.Int63n(interval.Milliseconds())) * time.Millisecond
	time.Sleep(jitter)
	runCheck()
	ticker := time.NewTicker(interval)
	for {
		select {
		case <-ticker.C:
			runCheck()
		case <-s.stop:
			ticker.Stop()
			finish <- struct{}{}
			return
		}
	}
}

func (s *periodicScheduler) StopScheduling(parentCtx context.Context) {
	ctx, cancel := context.WithDeadline(parentCtx, time.Now().Add(5*time.Second))
	defer cancel()
	close(s.stop)
	select {
	case <-s.done:
	case <-ctx.Done():
	}
}
