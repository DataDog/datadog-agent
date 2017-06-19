package runner

import (
	"expvar"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	log "github.com/cihub/seelog"
)

const stopCheckTimeout time.Duration = 500 * time.Millisecond // Time to wait for a check to stop

var (
	runnerStats *expvar.Map
	checkStats  map[check.ID]*check.Stats
	checkStatsM sync.RWMutex
)

func init() {
	runnerStats = expvar.NewMap("runner")
	runnerStats.Set("Checks", expvar.Func(expCheckStats))
	checkStats = make(map[check.ID]*check.Stats)
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

func addWorkStats(c check.Check, execTime time.Duration, err error) {
	var s *check.Stats
	var found bool

	checkStatsM.Lock()
	s, found = checkStats[c.ID()]
	if !found {
		s = check.NewStats(c)
		checkStats[c.ID()] = s
	}
	checkStatsM.Unlock()

	s.Add(execTime, err)
}

func expCheckStats() interface{} {
	checkStatsM.RLock()
	defer checkStatsM.RUnlock()

	return checkStats
}
