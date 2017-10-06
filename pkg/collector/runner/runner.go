// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package runner

import (
	"expvar"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util"
	log "github.com/cihub/seelog"
)

const stopCheckTimeout time.Duration = 500 * time.Millisecond // Time to wait for a check to stop

// checkStats holds the stats from the running checks
type runnerCheckStats struct {
	Stats map[check.ID]*check.Stats
	M     sync.RWMutex
}

var (
	runnerStats *expvar.Map
	checkStats  *runnerCheckStats
)

func init() {
	runnerStats = expvar.NewMap("runner")
	runnerStats.Set("Checks", expvar.Func(expCheckStats))
	checkStats = &runnerCheckStats{
		Stats: make(map[check.ID]*check.Stats),
	}
}

// Runner ...
type Runner struct {
	pending       chan check.Check         // The channel where checks come from
	done          chan bool                // Guard for the main loop
	runningChecks map[check.ID]check.Check // the list of checks running
	m             sync.Mutex               // to control races on runningChecks
	running       uint32                   // Flag to see if the Runner is, well, running
}

// NewRunner takes the number of desired goroutines processing incoming checks.
func NewRunner(numWorkers int) *Runner {
	r := &Runner{
		// initialize the channel
		pending:       make(chan check.Check),
		runningChecks: make(map[check.ID]check.Check),
		running:       1,
	}

	// start the workers
	for i := 0; i < numWorkers; i++ {
		go r.work()
	}

	log.Infof("Runner started with %d workers.", numWorkers)
	runnerStats.Add("Workers", int64(numWorkers))
	return r
}

// Stop closes the pending channel so all workers will exit their loop and terminate
func (r *Runner) Stop() {
	if atomic.LoadUint32(&r.running) == 0 {
		log.Debug("Runner already stopped, nothing to do here...")
		return
	}

	log.Info("Runner is shutting down...")

	close(r.pending)
	atomic.StoreUint32(&r.running, 0)

	// stop checks that are still running
	r.m.Lock()
	for _, check := range r.runningChecks {
		log.Infof("Stopping Check %v that is still running...", check)
		done := make(chan struct{})
		go func() {
			check.Stop()
			close(done)
		}()

		select {
		case <-done:
			// all good
		case <-time.After(stopCheckTimeout):
			// check is not responding
			log.Errorf("Check %v not responding, timing out...", check)
		}
	}
	r.m.Unlock()
}

// GetChan returns a write-only version of the pending channel
func (r *Runner) GetChan() chan<- check.Check {
	return r.pending
}

// StopCheck invokes the `Stop` method on a check if it's running. If the check
// is not running, this is a noop
func (r *Runner) StopCheck(id check.ID) error {
	done := make(chan bool)

	r.m.Lock()
	defer r.m.Unlock()

	if c, isRunning := r.runningChecks[id]; isRunning {
		log.Debugf("Stopping check %s", c)
		go func() {
			c.Stop()
			close(done)
		}()
	} else {
		return nil
	}

	select {
	case <-done:
		return nil
	case <-time.After(stopCheckTimeout):
		return fmt.Errorf("timeout during stop operation on check id %s", id)
	}
}

// work waits for checks and run them as long as they arrive on the channel
func (r *Runner) work() {
	log.Debug("Ready to process checks...")

	for check := range r.pending {
		// see if the check is already running
		r.m.Lock()
		if _, isRunning := r.runningChecks[check.ID()]; isRunning {
			log.Debugf("Check %s is already running, skip execution...", check)
			r.m.Unlock()
			continue
		} else {
			r.runningChecks[check.ID()] = check
			runnerStats.Add("RunningChecks", 1)
		}
		r.m.Unlock()

		doLog, lastLog := shouldLog(check.ID())

		if doLog {
			log.Infof("Running check %s", check)
		} else {
			log.Debugf("Running check %s", check)
		}

		// run the check
		var err error
		t0 := time.Now()

		if check.Interval() == 0 {
			// retry long running checks, bail out if they return an error 3 times
			// in a row without running for at least 5 seconds
			// TODO: this should be check-configurable, with meaningful default values
			err = retry(5*time.Second, 3, check.Run)
		} else {
			// normal check run
			err = check.Run()
		}

		warnings := check.GetWarnings()

		// use the default sender for the service checks
		sender, e := aggregator.GetDefaultSender()
		if e != nil {
			log.Errorf("Error getting default sender: %v. Not sending status check for %s", e, check)
		}
		serviceCheckTags := []string{fmt.Sprintf("check:%s", check.String())}
		serviceCheckStatus := metrics.ServiceCheckOK

		hostname := getHostname()

		if len(warnings) != 0 {
			// len returns int, and this expect int64, so it has to be converted
			runnerStats.Add("Warnings", int64(len(warnings)))
			serviceCheckStatus = metrics.ServiceCheckWarning
		}

		if err != nil {
			log.Errorf("Error running check %s: %s", check, err)
			runnerStats.Add("Errors", 1)
			serviceCheckStatus = metrics.ServiceCheckCritical
		}

		if sender != nil {
			sender.ServiceCheck("datadog.agent.check_status", serviceCheckStatus, hostname, serviceCheckTags, "")
			sender.Commit()
		}

		// remove the check from the running list
		r.m.Lock()
		delete(r.runningChecks, check.ID())
		r.m.Unlock()

		// publish statistics about this run
		runnerStats.Add("RunningChecks", -1)
		runnerStats.Add("Runs", 1)
		mStats, _ := check.GetMetricStats()
		addWorkStats(check, time.Since(t0), err, warnings, mStats)

		l := "Done running check %s"
		if doLog {
			if lastLog {
				l = l + fmt.Sprintf(" first runs done, next runs will be logged every %v runs", config.Datadog.GetInt64("logging_frequency"))
			}
			log.Infof(l, check)
		} else {
			log.Debugf(l, check)
		}
	}

	log.Debug("Finished processing checks.")
}

func shouldLog(id check.ID) (doLog bool, lastLog bool) {
	checkStats.M.RLock()
	defer checkStats.M.RUnlock()

	loggingFrequency := uint64(config.Datadog.GetInt64("logging_frequency"))

	s, found := checkStats.Stats[id]
	if found {
		if s.TotalRuns <= 5 {
			doLog = true
			if s.TotalRuns == 5 {
				lastLog = true
			}
		} else if s.TotalRuns%loggingFrequency == 0 {
			doLog = true
		}
	} else {
		doLog = true
	}

	return
}

func addWorkStats(c check.Check, execTime time.Duration, err error, warnings []error, mStats map[string]int64) {
	var s *check.Stats
	var found bool

	checkStats.M.Lock()
	s, found = checkStats.Stats[c.ID()]
	if !found {
		s = check.NewStats(c)
		checkStats.Stats[c.ID()] = s
	}
	checkStats.M.Unlock()

	s.Add(execTime, err, warnings, mStats)
}

func expCheckStats() interface{} {
	checkStats.M.RLock()
	defer checkStats.M.RUnlock()

	return checkStats.Stats
}

// GetCheckStats returns the check stats map
func GetCheckStats() map[check.ID]*check.Stats {
	checkStats.M.RLock()
	defer checkStats.M.RUnlock()

	return checkStats.Stats
}

func getHostname() string {
	hostname, _ := util.GetHostname()
	return hostname
}

func retry(retryDuration time.Duration, retries int, callback func() error) (err error) {
	attempts := 0

	for {
		t0 := time.Now()
		err = callback()
		if err == nil {
			return nil
		}

		// how much did the callback run?
		execDuration := time.Now().Sub(t0)
		if execDuration < retryDuration {
			// the callback failed too soon, retry but increment the counter
			attempts++
		} else {
			// the callback failed after the retryDuration, reset the counter
			attempts = 0
		}

		if attempts == retries {
			// give up
			return fmt.Errorf("bail out, last error: %v", err)
		}

		log.Warnf("Retrying, got an error executing the callback: %v", err)
	}
}
