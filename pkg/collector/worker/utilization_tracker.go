// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package worker

import (
	"fmt"
	"sync"
	"time"

	"github.com/benbjohnson/clock"

	"github.com/DataDog/datadog-agent/pkg/collector/runner/expvars"
	"github.com/DataDog/datadog-agent/pkg/util"
)

// utilizationTracker is the object that polls and evaluates utilization
// statistics which then get published to expvars.
type utilizationTracker struct {
	sync.RWMutex

	workerName string
	started    bool
	stopped    bool

	utilizationStats util.SlidingWindow
	pollingFunc      util.PollingFunc
	statsUpdateFunc  util.StatsUpdateFunc

	clock              clock.Clock
	busyDuration       time.Duration
	checkStart         time.Time
	windowStart        time.Time
	isRunningLongCheck bool
}

// UtilizationTracker is the interface that encapsulates the API around the
// utilizationTracker object.
type UtilizationTracker interface {
	// Start activates the polling ticker for collection of data.
	Start() error

	// Stop is invoked when a worker is about to exit to remove the instance's
	// expvars.
	Stop() error

	// CheckStarted starts tracking the start time of a check.
	CheckStarted(bool)

	// CheckFinished ends tracking the the runtime of a check previously registered
	// with `CheckStarted()`.
	CheckFinished()

	// IsRunningLongCheck returns true if we are currently running a check
	// that is meant to be of indefinite duration (e.g. jmxfetch checks).
	IsRunningLongCheck() bool
}

// NewUtilizationTracker instantiates and configures a utilization tracker that
// calculates the values and publishes them to expvars
func NewUtilizationTracker(
	workerName string,
	windowSize time.Duration,
	pollingInterval time.Duration,
) (UtilizationTracker, error) {
	return newUtilizationTrackerWithClock(
		workerName,
		windowSize,
		pollingInterval,
		clock.New(),
	)
}

// newUtilizationTrackerWithClock is primarely used for testing
func newUtilizationTrackerWithClock(
	workerName string,
	windowSize time.Duration,
	pollingInterval time.Duration,
	clk clock.Clock,
) (UtilizationTracker, error) {

	sw, err := util.NewSlidingWindowWithClock(windowSize, pollingInterval, clk)
	if err != nil {
		return nil, err
	}

	ut := &utilizationTracker{
		workerName:       workerName,
		utilizationStats: sw,

		clock:        clk,
		busyDuration: time.Duration(0),
		checkStart:   time.Time{},
		windowStart:  clk.Now(),
	}

	ut.pollingFunc = func() float64 {
		ut.Lock()
		defer ut.Unlock()

		currentTime := clk.Now()

		if !ut.checkStart.IsZero() {
			duration := currentTime.Sub(ut.checkStart)
			ut.busyDuration += duration
			ut.checkStart = currentTime
		}

		pollingWindowDuration := currentTime.Sub(ut.windowStart)
		if pollingWindowDuration == 0 {
			return 0.0
		}

		ut.windowStart = currentTime

		utilization := float64(ut.busyDuration) / float64(pollingWindowDuration)
		ut.busyDuration = time.Duration(0)

		return utilization
	}

	ut.statsUpdateFunc = func(utilization float64) {
		expvars.SetWorkerStats(workerName, &expvars.WorkerStats{
			Utilization: utilization,
		})
	}

	return ut, nil
}

// Start activates the polling ticker for collection of data
func (ut *utilizationTracker) Start() error {
	ut.Lock()
	defer ut.Unlock()

	if ut.started {
		return fmt.Errorf("Attempted to use UtilizationTracker.Start() when the tracker was already started")
	}

	if ut.stopped {
		return fmt.Errorf("Attempted to use UtilizationTracker.Start() after the tracker was stopped")
	}

	// Initialize the worker expvar
	expvars.SetWorkerStats(
		ut.workerName,
		&expvars.WorkerStats{
			Utilization: 0,
		},
	)

	// Start the ticker
	err := ut.utilizationStats.Start(ut.pollingFunc, ut.statsUpdateFunc)
	if err != nil {
		return err
	}

	ut.started = true

	return nil
}

// Stop should be invoked when a worker is about to exit
// so that we can remove the instance's expvars
func (ut *utilizationTracker) Stop() error {
	ut.Lock()
	defer ut.Unlock()

	if !ut.started {
		return fmt.Errorf("Attempted to use UtilizationTracker.Stop() when the tracker was never started")
	}

	if ut.stopped {
		return fmt.Errorf("Attempted to use UtilizationTracker.Stop() when the tracker was already stopped")
	}

	ut.stopped = true

	ut.utilizationStats.Stop()
	expvars.DeleteWorkerStats(ut.workerName)

	return nil
}

// CheckStarted should be invoked when a worker's check is about to
// run so that we can track the start time and the utilization. Long-running
// flag is to indicate if we should be worried about showing warnings
// when utilization raises above the threshold.
func (ut *utilizationTracker) CheckStarted(longRunning bool) {
	ut.Lock()

	ut.isRunningLongCheck = longRunning
	ut.checkStart = ut.clock.Now()

	ut.Unlock()
}

// CheckFinished should be invoked when a worker's check is complete
// so that we can calculate the utilization of the linked worker
func (ut *utilizationTracker) CheckFinished() {
	ut.Lock()

	duration := ut.clock.Now().Sub(ut.checkStart)
	ut.busyDuration += duration

	ut.isRunningLongCheck = false
	ut.checkStart = time.Time{}

	ut.Unlock()
}

// IsRunningLongCheck returns true if we are currently running a check
// that is meant to be of indefinite duration (e.g. jmxfetch checks).
func (ut *utilizationTracker) IsRunningLongCheck() bool {
	ut.RLock()
	defer ut.RUnlock()

	return ut.isRunningLongCheck
}
