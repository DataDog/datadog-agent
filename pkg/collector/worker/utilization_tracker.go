// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package worker

import (
	"time"

	"github.com/benbjohnson/clock"
)

type trackerEvent int

const (
	checkStarted trackerEvent = iota
	checkStopped
	trackerTick
)

//nolint:revive // TODO(AML) Fix revive linter
type UtilizationTracker struct {
	Output chan float64

	eventsChan chan trackerEvent

	// amount of busy time since the last tick
	busy time.Duration
	// value is the utilization fraction as observed at the last tick
	value float64
	// alpha is the ewma smoothing factor.
	alpha float64

	checkStarted time.Time
	nextTick     time.Time
	interval     time.Duration

	clock clock.Clock
}

// NewUtilizationTracker instantiates and configures a utilization tracker that
// calculates the values and publishes them to expvars
func NewUtilizationTracker(
	workerName string,
	interval time.Duration,
) *UtilizationTracker {
	return newUtilizationTrackerWithClock(
		workerName,
		interval,
		clock.New(),
	)
}

// newUtilizationTrackerWithClock is primarely used for testing.
//
// Does not start the background goroutines, so that the tests can call update() to get
// deterministic results.
//
//nolint:revive // TODO(AML) Fix revive linter
func newUtilizationTrackerWithClock(workerName string, interval time.Duration, clk clock.Clock) *UtilizationTracker {
	ut := &UtilizationTracker{
		clock: clk,

		eventsChan: make(chan trackerEvent),

		nextTick: clk.Now(),
		interval: interval,
		alpha:    0.25, // converges to 99.98% of constant input in 30 iterations.

		Output: make(chan float64, 1),
	}

	go ut.run()

	return ut
}

func (ut *UtilizationTracker) run() {
	defer close(ut.Output)

	for ev := range ut.eventsChan {
		now := ut.clock.Now()
		// handle all elapsed time intervals, if any
		ut.update(now)
		// invariant: ut.nextTick > now

		switch ev {
		case checkStarted:
			// invariant: ut.nextTick > ut.checkStarted
			ut.checkStarted = now
		case checkStopped:
			ut.busy += now.Sub(ut.checkStarted)
			ut.checkStarted = time.Time{}
		case trackerTick:
			// nothing, just tick
		}
	}
}

func (ut *UtilizationTracker) update(now time.Time) {
	for ut.nextTick.Before(now) {
		if !ut.checkStarted.IsZero() {
			// invariant: ut.nextTick > ut.checkStarted
			ut.busy += ut.nextTick.Sub(ut.checkStarted)
			ut.checkStarted = ut.nextTick
		}

		update := float64(ut.busy) / float64(ut.interval)
		ut.value = ut.value*(1.0-ut.alpha) + update*ut.alpha
		ut.busy = 0

		ut.nextTick = ut.nextTick.Add(ut.interval)
	}
	// invariant: ut.nextTick > now
	ut.Output <- ut.value
}

// Stop should be invoked when a worker is about to exit
// so that we can remove the instance's expvars
func (ut *UtilizationTracker) Stop() {
	// The user will not send anything anymore
	close(ut.eventsChan)
}

// Tick updates to the utilization during intervals where no check were started or stopped.
//
// Produces one value on the Output channel.
func (ut *UtilizationTracker) Tick() {
	ut.eventsChan <- trackerTick
}

// CheckStarted should be invoked when a worker's check is about to run so that we can track the
// start time and the utilization.
//
// Produces one value on the Output channel.
func (ut *UtilizationTracker) CheckStarted() {
	ut.eventsChan <- checkStarted
}

// CheckFinished should be invoked when a worker's check is complete so that we can calculate the
// utilization of the linked worker.
//
// Produces one value on the Output channel.
func (ut *UtilizationTracker) CheckFinished() {
	ut.eventsChan <- checkStopped
}
