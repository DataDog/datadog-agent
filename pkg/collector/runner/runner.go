// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package runner

import (
	"expvar"
	"fmt"
	"strings"

	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/py"
	"github.com/DataDog/datadog-agent/pkg/collector/scheduler"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// Time to wait for a check to stop
	stopCheckTimeout time.Duration = 500 * time.Millisecond
	// Time to wait for all checks to stop
	stopAllChecksTimeout time.Duration = 2 * time.Second
	// How long is the first series of check runs we want to log
	firstRunSeries uint64 = 5
)

var (
	// TestWg is used for testing the number of check workers
	TestWg            sync.WaitGroup
	defaultNumWorkers = 4
	maxNumWorkers     = 25
	runnerStats       *expvar.Map
	checkStats        *runnerCheckStats
)

func init() {
	runnerStats = expvar.NewMap("runner")
	runnerStats.Set("Checks", expvar.Func(expCheckStats))
	checkStats = &runnerCheckStats{
		Stats: make(map[string]map[check.ID]*check.Stats),
	}
}

// checkStats holds the stats from the running checks
type runnerCheckStats struct {
	Stats map[string]map[check.ID]*check.Stats
	M     sync.RWMutex
}

// Runner ...
type Runner struct {
	pending          chan check.Check         // The channel where checks come from
	runningChecks    map[check.ID]check.Check // The list of checks running
	scheduler        *scheduler.Scheduler     // Scheduler runner operates on
	m                sync.Mutex               // To control races on runningChecks
	running          uint32                   // Flag to see if the Runner is, well, running
	staticNumWorkers bool                     // Flag indicating if numWorkers is dynamically updated
}

// NewRunner takes the number of desired goroutines processing incoming checks.
func NewRunner() *Runner {
	numWorkers := config.Datadog.GetInt("check_runners")
	if numWorkers > maxNumWorkers {
		numWorkers = maxNumWorkers
		log.Warnf("Configured number of checks workers (%v) is too high: %v will be used", numWorkers, maxNumWorkers)
	}

	r := &Runner{
		// initialize the channel
		pending:          make(chan check.Check),
		runningChecks:    make(map[check.ID]check.Check),
		running:          1,
		staticNumWorkers: numWorkers != 0,
	}

	if !r.staticNumWorkers {
		numWorkers = defaultNumWorkers
	}

	// start the workers
	for i := 0; i < numWorkers; i++ {
		r.AddWorker()
	}

	log.Infof("Runner started with %d workers.", numWorkers)
	return r
}

// AddWorker adds a new worker to the worker pull
func (r *Runner) AddWorker() {
	runnerStats.Add("Workers", 1)
	TestWg.Add(1)
	go r.work()
}

// UpdateNumWorkers checks if the current number of workers is reasonable, and adds more if needed
func (r *Runner) UpdateNumWorkers(numChecks int64) {
	numWorkers, _ := strconv.Atoi(runnerStats.Get("Workers").String())

	if r.staticNumWorkers {
		return
	}

	// Find which range the number of checks we're running falls in
	var desiredNumWorkers int
	switch {
	case numChecks <= 10:
		desiredNumWorkers = 4
	case numChecks <= 15:
		desiredNumWorkers = 10
	case numChecks <= 20:
		desiredNumWorkers = 15
	case numChecks <= 25:
		desiredNumWorkers = 20
	default:
		desiredNumWorkers = maxNumWorkers
	}

	// Add workers if we don't have enough for this range
	added := 0
	for {
		if numWorkers >= desiredNumWorkers {
			break
		}
		r.AddWorker()
		numWorkers++
		added++
	}
	if added > 0 {
		log.Infof("Added %d workers to runner: now at "+runnerStats.Get("Workers").String()+" workers.", added)
	}
}

// Stop closes the pending channel so all workers will exit their loop and terminate
// All publishers to the pending channel need to have stopped before Stop is called
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
	globalDone := make(chan struct{})
	wg := sync.WaitGroup{}

	// stop all python subprocesses
	err := py.TerminateRunningProcesses()
	if err != nil {
		log.Warnf("Problem termination python processes: %v", err)
	}

	// stop running checks
	for _, c := range r.runningChecks {
		wg.Add(1)
		go func(c check.Check) {
			log.Infof("Stopping Check %v that is still running...", c)
			done := make(chan struct{})
			go func() {
				c.Stop()
				close(done)
				wg.Done()
			}()

			select {
			case <-done:
				// all good
			case <-time.After(stopCheckTimeout):
				// check is not responding
				log.Warnf("Check %v not responding after %v", c, stopCheckTimeout)
			}
		}(c)
	}
	r.m.Unlock()

	go func() {
		wg.Wait()
		close(globalDone)
	}()
	select {
	case <-globalDone:
		// all good
	case <-time.After(stopAllChecksTimeout):
		// some checks are not responding
		log.Errorf("Some checks not responding after %v, timing out...", stopAllChecksTimeout)
	}
}

// GetChan returns a write-only version of the pending channel
func (r *Runner) GetChan() chan<- check.Check {
	return r.pending
}

// SetScheduler sets the scheduler for the runner
func (r *Runner) SetScheduler(s *scheduler.Scheduler) {
	r.m.Lock()
	defer r.m.Unlock()

	r.scheduler = s
}

// StopCheck invokes the `Stop` method on a check if it's running. If the check
// is not running, this is a noop
func (r *Runner) StopCheck(id check.ID) error {
	done := make(chan bool)

	r.m.Lock()
	defer r.m.Unlock()

	if c, isRunning := r.runningChecks[id]; isRunning {
		log.Debugf("Stopping check %s", c.ID())
		go func() {
			// Remember that the check was stopped so that even if it runs we can discard its stats
			c.Stop()
			close(done)
		}()
	} else {
		log.Debugf("Check %s is not running, not stopping it", id)
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
	defer TestWg.Done()
	defer runnerStats.Add("Workers", -1)

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

		err = check.Run()
		longRunning := check.Interval() == 0

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

		if sender != nil && !longRunning {
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

		r.m.Lock()
		if !longRunning || len(warnings) != 0 || err != nil {
			// If the scheduler isn't assigned (it should), just add stats
			// otherwise only do so if the check is in the scheduler
			if r.scheduler == nil || r.scheduler.IsCheckScheduled(check.ID()) {
				mStats, _ := check.GetMetricStats()
				addWorkStats(check, time.Since(t0), err, warnings, mStats)
			}
		}
		r.m.Unlock()

		l := "Done running check %s"
		if doLog {
			if lastLog {
				l = l + fmt.Sprintf(", next runs will be logged every %v runs", config.Datadog.GetInt64("logging_frequency"))
			}
			log.Infof(l, check)
		} else {
			log.Debugf(l, check)
		}

		if check.Interval() == 0 {
			log.Infof("Check %v one-time's execution has finished", check)
			return
		}
	}

	log.Debug("Finished processing checks.")
}

func shouldLog(id check.ID) (doLog bool, lastLog bool) {
	checkStats.M.RLock()
	defer checkStats.M.RUnlock()

	var nameFound, idFound bool
	var s *check.Stats

	loggingFrequency := uint64(config.Datadog.GetInt64("logging_frequency"))
	name := strings.Split(string(id), ":")[0]

	stats, nameFound := checkStats.Stats[name]
	if nameFound {
		s, idFound = stats[id]
	}
	// this is the first time we see the check, log it
	if !idFound {
		doLog = true
		lastLog = false
		return
	}

	// we log the first firstRunSeries times, then every loggingFrequency times
	doLog = s.TotalRuns <= firstRunSeries || s.TotalRuns%loggingFrequency == 0
	// we print a special message when we change logging frequency
	lastLog = s.TotalRuns == firstRunSeries
	return
}

func addWorkStats(c check.Check, execTime time.Duration, err error, warnings []error, mStats map[string]int64) {
	var s *check.Stats
	var found bool

	checkStats.M.Lock()
	log.Debugf("Add stats for %s", string(c.ID()))
	stats, found := checkStats.Stats[c.String()]
	if !found {
		stats = make(map[check.ID]*check.Stats)
		checkStats.Stats[c.String()] = stats
	}
	s, found = stats[c.ID()]
	if !found {
		s = check.NewStats(c)
		stats[c.ID()] = s
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
func GetCheckStats() map[string]map[check.ID]*check.Stats {
	checkStats.M.RLock()
	defer checkStats.M.RUnlock()

	return checkStats.Stats
}

// RemoveCheckStats removes a check from the check stats map
func RemoveCheckStats(checkID check.ID) {
	checkStats.M.Lock()
	defer checkStats.M.Unlock()
	log.Debugf("Remove stats for %s", string(checkID))

	checkName := strings.Split(string(checkID), ":")[0]
	stats, found := checkStats.Stats[checkName]
	if found {
		delete(stats, checkID)
		if len(stats) == 0 {
			delete(checkStats.Stats, checkName)
		}
	}
}

func getHostname() string {
	hostname, _ := util.GetHostname()
	return hostname
}
