// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runner

import (
	"fmt"

	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/runner/tracker"
	"github.com/DataDog/datadog-agent/pkg/collector/scheduler"
	"github.com/DataDog/datadog-agent/pkg/collector/worker"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// Time to wait for a check to stop
	stopCheckTimeout time.Duration = 500 * time.Millisecond
	// Time to wait for all checks to stop
	stopAllChecksTimeout time.Duration = 2 * time.Second
)

var (
	// Atomic incrementing variables for generating globally unique runner and worker object IDs
	runnerIDGenerator = atomic.NewUint64(0)
	workerIDGenerator = atomic.NewUint64(0)
)

// Runner is the object in charge of running all the checks
type Runner struct {
	senderManager       sender.SenderManager
	isRunning           *atomic.Bool
	id                  int                           // Globally unique identifier for the Runner
	workers             map[int]*worker.Worker        // Workers currrently under this Runner's management
	workersLock         sync.Mutex                    // Lock to prevent concurrent worker changes
	isStaticWorkerCount bool                          // Flag indicating if numWorkers is dynamically updated
	pendingChecksChan   chan check.Check              // The channel where checks come from
	checksTracker       *tracker.RunningChecksTracker // Tracker in charge of maintaining the running check list
	scheduler           *scheduler.Scheduler          // Scheduler runner operates on
	schedulerLock       sync.RWMutex                  // Lock around operations on the scheduler
}

// NewRunner takes the number of desired goroutines processing incoming checks.
func NewRunner(senderManager sender.SenderManager) *Runner {
	numWorkers := config.Datadog.GetInt("check_runners")

	r := &Runner{
		senderManager:       senderManager,
		id:                  int(runnerIDGenerator.Inc()),
		isRunning:           atomic.NewBool(true),
		workers:             make(map[int]*worker.Worker),
		isStaticWorkerCount: numWorkers != 0,
		pendingChecksChan:   make(chan check.Check),
		checksTracker:       tracker.NewRunningChecksTracker(),
	}

	if !r.isStaticWorkerCount {
		numWorkers = config.DefaultNumWorkers
	}

	r.ensureMinWorkers(numWorkers)

	return r
}

// EnsureMinWorkers increases the number of workers to match the
// `desiredNumWorkers` parameter
func (r *Runner) ensureMinWorkers(desiredNumWorkers int) {
	r.workersLock.Lock()
	defer r.workersLock.Unlock()

	currentWorkers := len(r.workers)

	if desiredNumWorkers <= currentWorkers {
		return
	}

	workersToAdd := desiredNumWorkers - currentWorkers
	for idx := 0; idx < workersToAdd; idx++ {
		worker, err := r.newWorker()
		if err == nil {
			r.workers[worker.ID] = worker
		}
	}

	log.Infof(
		"Runner %d added %d workers (total: %d)",
		r.id,
		workersToAdd,
		len(r.workers),
	)
}

// AddWorker adds a single worker to the runner.
func (r *Runner) AddWorker() {
	r.workersLock.Lock()
	defer r.workersLock.Unlock()

	worker, err := r.newWorker()
	if err == nil {
		r.workers[worker.ID] = worker
	}
}

// addWorker adds a new worker running in a separate goroutine
func (r *Runner) newWorker() (*worker.Worker, error) {
	worker, err := worker.NewWorker(
		r.senderManager,
		r.id,
		int(workerIDGenerator.Inc()),
		r.pendingChecksChan,
		r.checksTracker,
		r.ShouldAddCheckStats,
	)
	if err != nil {
		log.Errorf("Runner %d was unable to instantiate a worker: %s", r.id, err)
		return nil, err
	}

	go func() {
		defer r.removeWorker(worker.ID)

		worker.Run()
	}()

	return worker, nil
}

func (r *Runner) removeWorker(id int) {
	r.workersLock.Lock()
	defer r.workersLock.Unlock()

	delete(r.workers, id)
}

// UpdateNumWorkers checks if the current number of workers is reasonable,
// and adds more if needed
func (r *Runner) UpdateNumWorkers(numChecks int64) {
	// We don't want to update the worker count when we have a static number defined
	if r.isStaticWorkerCount {
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
		desiredNumWorkers = config.MaxNumWorkers
	}

	r.ensureMinWorkers(desiredNumWorkers)
}

// Stop closes the pending channel so all workers will exit their loop and terminate
// All publishers to the pending channel need to have stopped before Stop is called
func (r *Runner) Stop() {
	if !r.isRunning.CompareAndSwap(true, false) {
		log.Debugf("Runner %d already stopped, nothing to do here...", r.id)
		return
	}

	log.Infof("Runner %d is shutting down...", r.id)
	close(r.pendingChecksChan)

	wg := sync.WaitGroup{}

	// Stop running checks
	r.checksTracker.WithRunningChecks(func(runningChecks map[checkid.ID]check.Check) {
		// Stop all python subprocesses
		terminateChecksRunningProcesses()

		for _, c := range runningChecks {
			wg.Add(1)
			go func(ch check.Check) {
				err := r.StopCheck(ch.ID())
				if err != nil {
					log.Warnf("Check %v not responding after %v: %s", ch, stopCheckTimeout, err)
				}

				wg.Done()
			}(c)
		}
	})

	globalDone := make(chan struct{})
	go func() {
		log.Debugf("Runner %d waiting for all the workers to exit...", r.id)
		wg.Wait()

		log.Debugf("All runner %d workers have been shut down", r.id)
		close(globalDone)
	}()

	select {
	case <-globalDone:
		log.Infof("Runner %d shut down", r.id)
	case <-time.After(stopAllChecksTimeout):
		log.Errorf(
			"Some checks on runner %d not responding after %v, timing out...",
			r.id,
			stopAllChecksTimeout,
		)
	}
}

// GetChan returns a write-only version of the pending channel
func (r *Runner) GetChan() chan<- check.Check {
	return r.pendingChecksChan
}

// SetScheduler sets the scheduler for the runner
func (r *Runner) SetScheduler(s *scheduler.Scheduler) {
	r.schedulerLock.Lock()
	r.scheduler = s
	r.schedulerLock.Unlock()
}

// getScheduler gets the scheduler set on the runner
func (r *Runner) getScheduler() *scheduler.Scheduler {
	r.schedulerLock.RLock()
	defer r.schedulerLock.RUnlock()

	return r.scheduler
}

// ShouldAddCheckStats returns true if check stats should be preserved or not
func (r *Runner) ShouldAddCheckStats(id checkid.ID) bool {
	r.schedulerLock.RLock()
	defer r.schedulerLock.RUnlock()

	sc := r.getScheduler()
	if sc == nil || sc.IsCheckScheduled(id) {
		return true
	}

	return false
}

// StopCheck invokes the `Stop` method on a check if it's running. If the check
// is not running, this is a noop
func (r *Runner) StopCheck(id checkid.ID) error {
	done := make(chan bool)

	stopFunc := func(c check.Check) {
		log.Debugf("Stopping running check %s...", c.ID())
		go func() {
			// Remember that the check was stopped so that even if it runs we can discard its stats
			c.Stop()
			close(done)
		}()
	}

	if !r.checksTracker.WithCheck(id, stopFunc) {
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
