// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/runner/expvars"
	"github.com/DataDog/datadog-agent/pkg/collector/runner/tracker"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	serviceCheckStatusKey = "datadog.agent.check_status"

	// Variables for the utilization expvars
	pollingInterval = 15 * time.Second
)

// Worker is an object that encapsulates the logic to manage a loop of processing
// checks over the provided `PendingCheckChan`
type Worker struct {
	ID   int
	Name string

	checksTracker           *tracker.RunningChecksTracker
	getDefaultSenderFunc    func() (aggregator.Sender, error)
	pendingChecksChan       chan check.Check
	runnerID                int
	shouldAddCheckStatsFunc func(id check.ID) bool
	utilizationTickInterval time.Duration
}

// NewWorker returns an instance of a `Worker` after parameter sanity checks are passed
func NewWorker(
	runnerID int,
	ID int,
	pendingChecksChan chan check.Check,
	checksTracker *tracker.RunningChecksTracker,
	shouldAddCheckStatsFunc func(id check.ID) bool,
) (*Worker, error) {

	if checksTracker == nil {
		return nil, fmt.Errorf("worker cannot initialize using a nil checksTracker")
	}

	if pendingChecksChan == nil {
		return nil, fmt.Errorf("worker cannot initialize using a nil pendingChecksChan")
	}

	if shouldAddCheckStatsFunc == nil {
		return nil, fmt.Errorf("worker cannot initialize using a nil shouldAddCheckStatsFunc")
	}

	return newWorkerWithOptions(
		runnerID,
		ID,
		pendingChecksChan,
		checksTracker,
		shouldAddCheckStatsFunc,
		aggregator.GetDefaultSender,
		pollingInterval,
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
	shouldAddCheckStatsFunc func(id check.ID) bool,
	getDefaultSenderFunc func() (aggregator.Sender, error),
	utilizationTickInterval time.Duration,
) (*Worker, error) {

	if getDefaultSenderFunc == nil {
		return nil, fmt.Errorf("worker cannot initialize using a nil getDefaultSenderFunc")
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
		utilizationTickInterval: utilizationTickInterval,
	}, nil
}

// Run waits for checks and run them as long as they arrive on the channel
func (w *Worker) Run() {
	log.Debugf("Runner %d, worker %d: Ready to process checks...", w.runnerID, w.ID)

	utilizationTracker := NewUtilizationTracker(w.Name, w.utilizationTickInterval)
	defer utilizationTracker.Stop()

	startExpvarUpdater(w.Name, utilizationTracker)
	cancel := startTrackerTicker(utilizationTracker, w.utilizationTickInterval)
	defer cancel()

	for check := range w.pendingChecksChan {
		checkLogger := CheckLogger{Check: check}
		longRunning := check.Interval() == 0

		// Add check to tracker if it's not already running
		if !w.checksTracker.AddCheck(check) {
			checkLogger.Debug("Check is already running, skipping execution...")
			continue
		}

		checkStartTime := time.Now()

		checkLogger.CheckStarted()

		expvars.AddRunningCheckCount(1)
		expvars.SetRunningStats(check.ID(), checkStartTime)

		utilizationTracker.CheckStarted()

		// Run the check
		var checkErr error
		checkErr = check.Run()

		utilizationTracker.CheckFinished()

		expvars.DeleteRunningStats(check.ID())

		checkWarnings := check.GetWarnings()

		// Use the default sender for the service checks
		sender, err := w.getDefaultSenderFunc()
		if err != nil {
			log.Errorf("Error getting default sender: %v. Not sending status check for %s", err, check)
		}
		serviceCheckTags := []string{fmt.Sprintf("check:%s", check.String())}
		serviceCheckStatus := metrics.ServiceCheckOK

		hname, _ := hostname.Get(context.TODO())

		if len(checkWarnings) != 0 {
			expvars.AddWarningsCount(len(checkWarnings))
			serviceCheckStatus = metrics.ServiceCheckWarning
		}

		if checkErr != nil {
			checkLogger.Error(checkErr)
			expvars.AddErrorsCount(1)
			serviceCheckStatus = metrics.ServiceCheckCritical
		}

		if sender != nil && !longRunning {
			sender.ServiceCheck(serviceCheckStatusKey, serviceCheckStatus, hname, serviceCheckTags, "")
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
				expvars.AddCheckStats(check, time.Since(checkStartTime), checkErr, checkWarnings, sStats)
			}
		}

		checkLogger.CheckFinished()
	}

	log.Debugf("Runner %d, worker %d: Finished processing checks.", w.runnerID, w.ID)
}

func startExpvarUpdater(name string, ut *UtilizationTracker) {
	expvars.SetWorkerStats(name, &expvars.WorkerStats{
		Utilization: 0.0,
	})

	go func() {
		for value := range ut.Output {
			expvars.SetWorkerStats(name, &expvars.WorkerStats{
				Utilization: value,
			})
		}
		expvars.DeleteWorkerStats(name)
	}()
}

func startTrackerTicker(ut *UtilizationTracker, interval time.Duration) func() {
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
