package check

import (
	"sync"
	"sync/atomic"

	log "github.com/cihub/seelog"
)

// Runner ...
type Runner struct {
	pending       chan Check       // The channel where checks come from
	done          chan bool        // Guard for the main loop
	runningChecks map[string]Check // the list of checks running
	m             sync.Mutex       // to control races on runningChecks
	running       uint32           // Flag to see if the Runner is, well, running
}

// NewRunner ...
func NewRunner() *Runner {
	return &Runner{}
}

// Run takes the number of desired goroutines processing incoming checks.
// It's designed to be stopped and restarted, that's why some of the initialization is
// done here.
func (r *Runner) Run(numWorkers int) {
	if atomic.LoadUint32(&r.running) != 0 {
		log.Debug("Runner was already started, nothing to do here...")
		return
	}

	// initialize the channel
	r.pending = make(chan Check)

	// initialize the running list
	r.runningChecks = make(map[string]Check)

	for i := 0; i < numWorkers; i++ {
		go r.work()
	}

	log.Infof("Runner started with %d workers.", numWorkers)
	atomic.StoreUint32(&r.running, 1)
}

// Stop closes the pending channel so all workers will exit their loop and terminate
func (r *Runner) Stop() {
	if atomic.LoadUint32(&r.running) == 0 {
		log.Debug("Runner already stopped, nothing to do here...")
		return
	}

	close(r.pending)
	atomic.StoreUint32(&r.running, 0)

	// stop checks that are still running
	r.m.Lock()
	for _, check := range r.runningChecks {
		check.Stop()
	}
	r.m.Unlock()
}

// StopCheck stops a specific check if it's currently running
func (r *Runner) StopCheck(checkID string) {
	r.m.Lock()
	defer r.m.Unlock()

	if _, isRunning := r.runningChecks[checkID]; isRunning {
		r.runningChecks[checkID].Stop()
	}
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
			continue
		} else {
			r.runningChecks[check.ID()] = check
		}
		r.m.Unlock()

		log.Infof("Running check %s", check)
		// run the check
		err := check.Run()
		if err != nil {
			log.Errorf("Error running check %s: %s", check, err)
		}

		// remove the check from the running list
		r.m.Lock()
		delete(r.runningChecks, check.ID())
		r.m.Unlock()

		log.Infof("Done running check %s", check)
	}

	log.Debug("Finished to process checks.")
}
