package scheduler

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/op/go-logging"
)

var log = logging.MustGetLogger("datadog-agent")

const defaultTimeout time.Duration = 5000 * time.Millisecond

// Scheduler keeps things rolling.
// More docs to come...
type Scheduler struct {
	checksPipe chan<- check.Check          // The pipe the Runner pops the checks from
	done       chan bool                   // Guard for the main loop
	halted     chan bool                   // Used to internally communicate all queues are done
	started    chan bool                   // Used to internally communicate the queues are up
	jobQueues  map[time.Duration]*jobQueue // We have one scheduling queue for every interval
	mu         sync.Mutex                  // To protect critical sections in struct's fields
	running    uint32                      // Flag to see if the scheduler is running
}

// jobQueue contains a list of checks (called jobs) that need to be
// scheduled at a certain interval.
type jobQueue struct {
	interval time.Duration
	stop     chan bool
	ticker   *time.Ticker
	jobs     []check.Check
	running  bool
	mu       sync.Mutex // to protect critical sections in struct's fields
}

// NewScheduler create a Scheduler and returns a pointer to it.
func NewScheduler(out chan<- check.Check) *Scheduler {
	return &Scheduler{
		checksPipe: out,
		done:       make(chan bool, 1),
		halted:     make(chan bool, 1),
		started:    make(chan bool, 1),
		jobQueues:  make(map[time.Duration]*jobQueue),
		running:    0,
	}
}

// Enter schedules a list of `Check`s for execution.
func (s *Scheduler) Enter(checks []check.Check) {
	s.mu.Lock() // sync when accessing `jobQueues`
	for _, c := range checks {
		interval := c.Interval()
		_, ok := s.jobQueues[interval]
		if !ok {
			s.jobQueues[interval] = newJobQueue(interval)
		}
		s.jobQueues[interval].addJob(c)
	}
	s.mu.Unlock()
}

// Run is the Scheduler main loop.
// NOTE: it doesn't block but waits for the queues to be ready before returning.
func (s *Scheduler) Run() {
	// Invoking Run does nothing if the Scheduler is already running
	if atomic.LoadUint32(&s.running) != 0 {
		log.Debug("Scheduler is already running")
		return
	}

	go func() {
		log.Debug("Starting scheduler loop...")
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
		return errors.New("Stop operation timed out.")
	}
}

// Reload the scheduler
func (s *Scheduler) Reload(timeout ...time.Duration) error {
	log.Debug("Reloading scheduler loop...")
	if s.Stop(timeout...) == nil {
		log.Debug("Scheduler stopped, running again...")
		s.Run()
		return nil
	}

	return errors.New("Unable to perform reload.")
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
	}
}

// newJobQueue creates a new jobQueue instance
// the stop channel is buffered so the scheduler loop can send a message to stop
// without blocking
func newJobQueue(interval time.Duration) *jobQueue {
	return &jobQueue{
		interval: interval,
		ticker:   time.NewTicker(time.Second * time.Duration(interval)),
		stop:     make(chan bool, 1),
	}
}

// addJob is a convenience method to add a check to a queue
func (jq *jobQueue) addJob(c check.Check) {
	jq.mu.Lock()
	jq.jobs = append(jq.jobs, c)
	jq.mu.Unlock()
}

// run schedules the checks in the queue by posting them to the
// execution pipeline.
// This doesn't block.
func (jq *jobQueue) run(out chan<- check.Check) {
	jq.running = true
	go func() {
		for {
			select {
			case <-jq.stop:
				// someone asked to stop this queue
				jq.ticker.Stop()
				jq.running = false
			case <-jq.ticker.C:
				// normal case, (re)schedule the queue
				for _, check := range jq.jobs {
					log.Debugf("Enqueuing check %s for queue %d", check, jq.interval)
					out <- check
				}
			}
		}
	}()
}
