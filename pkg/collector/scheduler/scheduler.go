package scheduler

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	log "github.com/cihub/seelog"
)

const defaultTimeout time.Duration = 5000 * time.Millisecond

// Scheduler keeps things rolling.
// More docs to come...
type Scheduler struct {
	checksPipe chan<- check.Check          // The pipe the Runner pops the checks from, initially set to nil
	done       chan bool                   // Guard for the main loop
	halted     chan bool                   // Used to internally communicate all queues are done
	started    chan bool                   // Used to internally communicate the queues are up
	jobQueues  map[time.Duration]*jobQueue // We have one scheduling queue for every interval
	mu         sync.Mutex                  // To protect critical sections in struct's fields
	running    uint32                      // Flag to see if the scheduler is running
}

// NewScheduler create a Scheduler and returns a pointer to it.
func NewScheduler() *Scheduler {
	return &Scheduler{
		done:      make(chan bool, 1),
		halted:    make(chan bool, 1),
		started:   make(chan bool, 1),
		jobQueues: make(map[time.Duration]*jobQueue),
		running:   0,
	}
}

// Enter schedules a `Check`s for execution accordingly to the `Check.Interval()` value.
// If the interval is 0, the check is supposed to run only once.
func (s *Scheduler) Enter(check check.Check) error {
	if check.Interval() < 0 {
		return fmt.Errorf("Schedule interval must be a positive integer or 0")
	}

	// send immediately to the checks Pipe if this is a one-time schedule
	// do not block, in case the runner has not started
	if check.Interval() == 0 {
		log.Info("Scheduling check for one-time execution")
		go func() {
			s.checksPipe <- check
		}()
		return nil
	}

	// sync when accessing `jobQueues`
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.jobQueues[check.Interval()]; !ok {
		s.jobQueues[check.Interval()] = newJobQueue(check.Interval())
	}
	s.jobQueues[check.Interval()].addJob(check)

	return nil
}

// Run is the Scheduler main loop.
// This doesn't block but waits for the queues to be ready before returning.
func (s *Scheduler) Run(checksPipe chan<- check.Check) {
	// Invoking Run does nothing if the Scheduler is already running
	if atomic.LoadUint32(&s.running) != 0 {
		log.Debug("Scheduler is already running")
		return
	}

	go func() {
		log.Debug("Starting scheduler loop...")

		// setup the output channel
		s.checksPipe = checksPipe

		s.startQueues()

		// set internal state
		atomic.StoreUint32(&s.running, 1)

		// notify queues are up, channel is buffered this doesn't block
		s.started <- true

		// wait here until we're done
		<-s.done

		// someone asked to stop
		log.Debug("Exited Scheduler loop, shutting down queues...")
		s.stopQueues()
		atomic.StoreUint32(&s.running, 0)

		// notify we're done, channel is buffered this doesn't block
		s.halted <- true
	}()

	// Wait until queues are up
	<-s.started
}

// Stop the scheduler, optionally pass an integer to specify
// the timeout in `time.Duration` format.
func (s *Scheduler) Stop(timeout ...time.Duration) error {
	// Stopping when the Scheduler is not running is a noop.
	if atomic.LoadUint32(&s.running) == 0 {
		log.Debug("Scheduler is already stopped")
		return nil
	}

	to := defaultTimeout
	if len(timeout) == 1 {
		to = timeout[0]
	}

	// Interrupt the main loop, proceeding to shut down all the queues
	// `done` is buffered so we can proceed and wait for shutdown (or timeout)
	s.done <- true
	log.Debugf("Waiting for the scheduler to shutdown, timeout after %dns.", to)

	select {
	case <-s.halted:
		return nil
	case <-time.After(to):
		return errors.New("Stop operation timed out")
	}
}

// Reload the scheduler
func (s *Scheduler) Reload(timeout ...time.Duration) error {
	log.Debug("Reloading scheduler loop...")
	if s.Stop(timeout...) == nil {
		log.Debug("Scheduler stopped, running again...")
		s.Run(s.checksPipe)
		return nil
	}

	return errors.New("Unable to perform reload")
}

// stopQueues shuts down the timers for each active queue
func (s *Scheduler) stopQueues() {
	for _, q := range s.jobQueues {
		// check that the queue is actually running or this blocks
		// while posting to the channel
		if q.running {
			q.stop <- true
			q.running = false
		}
	}
}

// startQueues loads the timer for each queue
func (s *Scheduler) startQueues() {
	for _, q := range s.jobQueues {
		q.run(s.checksPipe)
		q.running = true
	}
}
