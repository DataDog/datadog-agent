// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utilizationtracker provides a utility to track the utilization of a component.
package utilizationtracker

import (
	"time"

	"github.com/benbjohnson/clock"
)

type trackerEvent int

const (
	started trackerEvent = iota
	stopped
	trackerTick
)

// UtilizationTracker tracks the utilization of a component.
type UtilizationTracker struct {
	Output chan float64

	eventsChan chan trackerEvent

	// amount of busy time since the last tick
	busy time.Duration
	// value is the utilization fraction as observed at the last tick
	value float64
	// alpha is the ewma smoothing factor.
	alpha float64

	started  time.Time
	nextTick time.Time
	interval time.Duration

	clock clock.Clock
}

// NewUtilizationTracker instantiates and configures a utilization tracker that
// calculates the values and publishes them to expvars
func NewUtilizationTracker(
	interval time.Duration,
	alpha float64,
) *UtilizationTracker {
	return newUtilizationTrackerWithClock(
		interval,
		clock.New(),
		alpha,
	)
}

// newUtilizationTrackerWithClock is primarely used for testing.
// Does not start the background goroutines, so that the tests can call update() to get
// deterministic results.
func newUtilizationTrackerWithClock(interval time.Duration, clk clock.Clock, alpha float64) *UtilizationTracker {
	ut := &UtilizationTracker{
		clock: clk,

		eventsChan: make(chan trackerEvent),

		nextTick: clk.Now(),
		interval: interval,
		alpha:    alpha,
		Output:   make(chan float64, 1),
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
		case started:
			// invariant: ut.nextTick > ut.started
			ut.started = now
		case stopped:
			ut.busy += now.Sub(ut.started)
			ut.started = time.Time{}
		case trackerTick:
			// nothing, just tick
		}
	}
}

func (ut *UtilizationTracker) update(now time.Time) {
	for ut.nextTick.Before(now) {
		if !ut.started.IsZero() {
			// invariant: ut.nextTick > ut.started
			ut.busy += ut.nextTick.Sub(ut.started)
			ut.started = ut.nextTick
		}

		update := float64(ut.busy) / float64(ut.interval)
		ut.value = ut.value*(1.0-ut.alpha) + update*ut.alpha
		ut.busy = 0

		ut.nextTick = ut.nextTick.Add(ut.interval)
	}
	// invariant: ut.nextTick > now
	ut.Output <- ut.value
}

// Stop should be invoked when a component is about to exit
// so that we can clean up the instances resources.
func (ut *UtilizationTracker) Stop() {
	// The user will not send anything anymore
	close(ut.eventsChan)
}

// Tick updates to the utilization during intervals where no component were started or stopped.
//
// Produces one value on the Output channel.
func (ut *UtilizationTracker) Tick() {
	ut.eventsChan <- trackerTick
}

// Started should be invoked when a compnent's work is about to being so that we can track the
// start time and the utilization.
//
// Produces one value on the Output channel.
func (ut *UtilizationTracker) Started() {
	ut.eventsChan <- started
}

// Finished should be invoked when a compnent's work is complete so that we can calculate the
// utilization of the compoennt.
//
// Produces one value on the Output channel.
func (ut *UtilizationTracker) Finished() {
	ut.eventsChan <- stopped
}
