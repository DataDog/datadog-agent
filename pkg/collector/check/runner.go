package check

import (
	"expvar"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/cihub/seelog"
)

const stopCheckTimeoutMs = 500 // Time to wait for a check to stop in milliseconds

var (
	runnerStats *expvar.Map
	checkStats  map[string]*Stats
	checkStatsM sync.Mutex
)

func init() {
	runnerStats = expvar.NewMap("runner")
	runnerStats.Set("Checks", expvar.Func(expCheckStats))
	checkStats = make(map[string]*Stats)
}

// Runner ...
type Runner struct {
	pending       chan Check       // The channel where checks come from
	done          chan bool        // Guard for the main loop
	runningChecks map[string]Check // the list of checks running
	m             sync.Mutex       // to control races on runningChecks
	running       uint32           // Flag to see if the Runner is, well, running
}

// NewRunner takes the number of desired goroutines processing incoming checks.
func NewRunner(numWorkers int) *Runner {
	r := &Runner{
		// initialize the channel
		pending:       make(chan Check),
		runningChecks: make(map[string]Check),
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
		case <-time.After(stopCheckTimeoutMs * time.Millisecond):
			// check is not responding
			log.Errorf("Check %v not responding, timing out...", check)
		}
	}
	r.m.Unlock()
}

// GetChan returns a write-only version of the pending channel
func (r *Runner) GetChan() chan<- Check {
	return r.pending
}

// Run waits for checks and run them as long as they arrive on the channel
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

		log.Infof("Running check %s", check)

		// run the check
		t0 := time.Now()
		err := check.Run()
		if err != nil {
			log.Errorf("Error running check %s: %s", check, err)
			runnerStats.Add("Errors", 1)
		}

		// remove the check from the running list
		r.m.Lock()
		delete(r.runningChecks, check.ID())
		r.m.Unlock()

		// publish statistics about this run
		runnerStats.Add("RunningChecks", -1)
		runnerStats.Add("Runs", 1)
		addWorkStats(check, time.Since(t0), err)

		log.Infof("Done running check %s", check)
	}

	log.Debug("Finished to process checks.")
}

func addWorkStats(c Check, execTime time.Duration, err error) {
	var s *Stats
	var found bool

	checkStatsM.Lock()
	s, found = checkStats[c.ID()]
	if !found {
		s = newStats(c)
		checkStats[c.ID()] = s
	}
	checkStatsM.Unlock()

	s.add(execTime, err == nil)
}

func expCheckStats() interface{} {
	return checkStats
}
