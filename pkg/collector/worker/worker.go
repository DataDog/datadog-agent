// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package worker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	haagent "github.com/DataDog/datadog-agent/comp/haagent/def"
	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/runner/expvars"
	"github.com/DataDog/datadog-agent/pkg/collector/runner/tracker"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/utilizationtracker"
)

const (
	serviceCheckStatusKey = "datadog.agent.check_status"

	// Variables for the utilization expvars
	pollingInterval = 15 * time.Second
)

// The worker utilization is also reported via expvars, but it emits one metric
// for each worker, which is a bit inconvenient to use because the number of
// workers might be different on every Agent. With telemetry, we can use a
// single metric and put the worker name in a tag.
var workerUtilization = telemetry.NewGauge(
	"collector",
	"worker_utilization",
	[]string{"worker_name"},
	"Worker utilization. It's a value between 0 and 1 that represents the share of time that the check runner worker is running checks",
)

// Worker is an object that encapsulates the logic to manage a loop of processing
// checks over the provided `PendingCheckChan`
type Worker struct {
	ID   int
	Name string

	checksTracker           *tracker.RunningChecksTracker
	getDefaultSenderFunc    func() (sender.Sender, error)
	pendingChecksChan       chan check.Check
	runnerID                int
	shouldAddCheckStatsFunc func(id checkid.ID) bool
	utilizationTickInterval time.Duration
	haAgent                 haagent.Component
	healthPlatform          healthplatform.Component
	watchdogWarningTimeout  time.Duration
}

// NewWorker returns an instance of a `Worker` after parameter sanity checks are passed
func NewWorker(
	senderManager sender.SenderManager,
	haAgent haagent.Component,
	healthPlatform healthplatform.Component,
	runnerID int,
	ID int,
	pendingChecksChan chan check.Check,
	checksTracker *tracker.RunningChecksTracker,
	shouldAddCheckStatsFunc func(id checkid.ID) bool,
	watchdogWarningTimeout time.Duration,
) (*Worker, error) {

	if checksTracker == nil {
		return nil, errors.New("worker cannot initialize using a nil checksTracker")
	}

	if pendingChecksChan == nil {
		return nil, errors.New("worker cannot initialize using a nil pendingChecksChan")
	}

	if shouldAddCheckStatsFunc == nil {
		return nil, errors.New("worker cannot initialize using a nil shouldAddCheckStatsFunc")
	}

	return newWorkerWithOptions(
		runnerID,
		ID,
		pendingChecksChan,
		checksTracker,
		shouldAddCheckStatsFunc,
		senderManager.GetDefaultSender,
		haAgent,
		healthPlatform,
		pollingInterval,
		watchdogWarningTimeout,
	)
}

// newWorkerWithOptions returns an instance of a `Worker` with an override for the
// `aggregator.GetDefaultSender()`. The purpose of this pass-through is to help
// test the aggregator logic.
func newWorkerWithOptions(
	runnerID int,
	ID int,
	pendingChecksChan chan check.Check,
	checksTracker *tracker.RunningChecksTracker,
	shouldAddCheckStatsFunc func(id checkid.ID) bool,
	getDefaultSenderFunc func() (sender.Sender, error),
	haAgent haagent.Component,
	healthPlatform healthplatform.Component,
	utilizationTickInterval time.Duration,
	watchdogWarningTimeout time.Duration,
) (*Worker, error) {

	if getDefaultSenderFunc == nil {
		return nil, errors.New("worker cannot initialize using a nil getDefaultSenderFunc")
	}

	workerName := fmt.Sprintf("worker_%d", ID)

	return &Worker{
		ID:                      ID,
		Name:                    workerName,
		checksTracker:           checksTracker,
		pendingChecksChan:       pendingChecksChan,
		runnerID:                runnerID,
		shouldAddCheckStatsFunc: shouldAddCheckStatsFunc,
		getDefaultSenderFunc:    getDefaultSenderFunc,
		haAgent:                 haAgent,
		healthPlatform:          healthPlatform,
		utilizationTickInterval: utilizationTickInterval,
		watchdogWarningTimeout:  watchdogWarningTimeout,
	}, nil
}

// Run waits for checks and run them as long as they arrive on the channel
func (w *Worker) Run() {
	log.Debugf("Runner %d, worker %d: Ready to process checks...", w.runnerID, w.ID)

	alpha := 0.25 // converges to 99.98% of constant input in 30 iterations.
	utilizationTracker := utilizationtracker.NewUtilizationTracker(w.utilizationTickInterval, alpha)
	defer utilizationTracker.Stop()

	startUtilizationUpdater(w.Name, utilizationTracker)
	cancel := startTrackerTicker(utilizationTracker, w.utilizationTickInterval)
	defer cancel()

	for check := range w.pendingChecksChan {
		checkLogger := CheckLogger{Check: check}
		longRunning := check.Interval() == 0

		if w.haAgent.Enabled() && check.IsHASupported() && !w.haAgent.IsActive() {
			checkLogger.Debug("Check is an HA integration and current agent is not leader, skipping execution...")
			continue
		}

		// Add check to tracker if it's not already running
		if !w.checksTracker.AddCheck(check) {
			checkLogger.Debug("Check is already running, skipping execution...")
			continue
		}

		var watchdogCancel chan struct{}
		var watchdogWG sync.WaitGroup
		if w.watchdogWarningTimeout > 0 {
			watchdogCancel = make(chan struct{})
			watchdogWG.Add(1)
			go func() {
				defer watchdogWG.Done()
				select {
				case <-time.After(w.watchdogWarningTimeout):
					log.Warnf("Check %s is running for longer than the watchdog warning timeout of %s", check.ID(), w.watchdogWarningTimeout)
				case <-watchdogCancel:
				}
			}()
		}

		checkStartTime := time.Now()

		checkLogger.CheckStarted()

		expvars.AddRunningCheckCount(1)
		expvars.SetRunningStats(check.ID(), checkStartTime)

		utilizationTracker.Started()

		// Run the check
		checkErr := check.Run()

		utilizationTracker.Finished()

		expvars.DeleteRunningStats(check.ID())

		checkWarnings := check.GetWarnings()

		// Use the default sender for the service checks
		sender, err := w.getDefaultSenderFunc()
		if err != nil {
			log.Errorf("Error getting default sender: %v. Not sending status check for %s", err, check)
		}
		serviceCheckTags := []string{"check:" + check.String(), "dd_enable_check_intake:true"}
		serviceCheckStatus := servicecheck.ServiceCheckOK

		hname, _ := hostname.Get(context.TODO())

		if len(checkWarnings) != 0 {
			expvars.AddWarningsCount(len(checkWarnings))
			serviceCheckStatus = servicecheck.ServiceCheckWarning
		}

		if checkErr != nil {
			checkLogger.Error(checkErr)
			expvars.AddErrorsCount(1)
			serviceCheckStatus = servicecheck.ServiceCheckCritical
		}

		if sender != nil && !longRunning {
			if pkgconfigsetup.Datadog().GetBool("integration_check_status_enabled") {
				sender.ServiceCheck(serviceCheckStatusKey, serviceCheckStatus, hname, serviceCheckTags, "")
			}
			// FIXME(remy): this `Commit()` should be part of the `if` above, we keep
			// it here for now to make sure it's not breaking any historical behavior
			// with the shared default sender.
			sender.Commit()
		}

		// Remove the check from the running list
		w.checksTracker.DeleteCheck(check.ID())

		// Publish statistics about this run
		expvars.AddRunningCheckCount(-1)
		expvars.AddRunsCount(1)

		if !longRunning || len(checkWarnings) != 0 || checkErr != nil {
			// If the scheduler isn't assigned (it should), just add stats
			// otherwise only do so if the check is in the scheduler
			if w.shouldAddCheckStatsFunc(check.ID()) {
				sStats, _ := check.GetSenderStats()
				expvars.AddCheckStats(check, time.Since(checkStartTime), checkErr, checkWarnings, sStats, w.haAgent, w.healthPlatform)
			}
		}

		checkLogger.CheckFinished()

		if watchdogCancel != nil {
			close(watchdogCancel)
			watchdogWG.Wait()
		}
	}

	log.Debugf("Runner %d, worker %d: Finished processing checks.", w.runnerID, w.ID)
}

func startUtilizationUpdater(name string, ut *utilizationtracker.UtilizationTracker) {
	expvars.SetWorkerStats(name, &expvars.WorkerStats{
		Utilization: 0.0,
	})

	workerUtilization.Set(0, name)

	go func() {
		for value := range ut.Output {
			expvars.SetWorkerStats(name, &expvars.WorkerStats{
				Utilization: value,
			})

			workerUtilization.Set(value, name)
		}
		expvars.DeleteWorkerStats(name)
	}()
}

func startTrackerTicker(ut *utilizationtracker.UtilizationTracker, interval time.Duration) func() {
	ticker := time.NewTicker(interval)
	cancel := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		defer ticker.Stop()
		defer close(done)
		for {
			select {
			case <-ticker.C:
				ut.Tick()
			case <-cancel:
				return
			}
		}
	}()

	return func() {
		cancel <- struct{}{}
		<-done // make sure Tick will not be called after we return.
	}
}
