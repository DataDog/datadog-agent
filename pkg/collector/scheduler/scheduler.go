package scheduler

import (
	"errors"
	"expvar"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	log "github.com/cihub/seelog"
)

const defaultTimeout time.Duration = 5 * time.Second
const minAllowedInterval time.Duration = 1 * time.Second

var (
	schedulerStats *expvar.Map
)

func init() {
	schedulerStats = expvar.NewMap("scheduler")
}

// Scheduler keeps things rolling.
// More docs to come...
type Scheduler struct {
	checksPipe   chan<- check.Check          // The pipe the Runner pops the checks from, initially set to nil
	done         chan bool                   // Guard for the main loop
	halted       chan bool                   // Used to internally communicate all queues are done
	started      chan bool                   // Used to internally communicate the queues are up
	jobQueues    map[time.Duration]*jobQueue // We have one scheduling queue for every interval
	checkToQueue map[check.ID]*jobQueue      // Keep track of what is the queue for any Check
	mu           sync.Mutex                  // To protect critical sections in struct's fields
	running      uint32                      // Flag to see if the scheduler is running
}

// NewScheduler create a Scheduler and returns a pointer to it.
func NewScheduler(checksPipe chan<- check.Check) *Scheduler {
	return &Scheduler{
		checksPipe:   checksPipe,
		done:         make(chan bool, 1),
		halted:       make(chan bool, 1),
		started:      make(chan bool, 1),
		jobQueues:    make(map[time.Duration]*jobQueue),
		checkToQueue: make(map[check.ID]*jobQueue),
		running:      0,
	}
}

// Enter schedules a `Check`s for execution accordingly to the `Check.Interval()` value.
// If the interval is 0, the check is supposed to run only once.
func (s *Scheduler) Enter(check check.Check) error {
	// send immediately to the checks Pipe if this is a one-time schedule
	// do not block, in case the runner has not started
	if check.Interval() == 0 {
		log.Infof("Scheduling check %v for one-time execution", check)
		go func() {
			s.checksPipe <- check
		}()
		schedulerStats.Add("ChecksEntered", 1)
		return nil
	}

	if check.Interval() < minAllowedInterval {
		return fmt.Errorf("Schedule interval must be greater than %v or 0", minAllowedInterval)
	}

	log.Infof("Scheduling check %v with an interval of %v", check, check.Interval())

	// sync when accessing `jobQueues` and `check2queue`
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.jobQueues[check.Interval()]; !ok {
		s.jobQueues[check.Interval()] = newJobQueue(check.Interval())
		s.startQueue(s.jobQueues[check.Interval()])
		schedulerStats.Add("QueuesCount", 1)
	}
	s.jobQueues[check.Interval()].addJob(check)
	// map each check to the Job Queue it was assigned to
	s.checkToQueue[check.ID()] = s.jobQueues[check.Interval()]

	schedulerStats.Add("ChecksEntered", 1)
	schedulerStats.Set("Queues", expvar.Func(expQueues(s)))
	return nil
}

// Cancel remove a Check from the scheduled queue. If the check is not
// in the scheduler, this is a noop.
func (s *Scheduler) Cancel(id check.ID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.checkToQueue[id]; !ok {
		return nil
	}

	// remove it from the queue
	err := s.checkToQueue[id].removeJob(id)
	if err != nil {
		return fmt.Errorf("unable to remove the Job from the queue: %s", err)
	}

	schedulerStats.Add("ChecksEntered", -1)
	schedulerStats.Set("Queues", expvar.Func(expQueues(s)))
	return nil
}

// Run is the Scheduler main loop.
// This doesn't block but waits for the queues to be ready before returning.
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
	log.Debugf("Waiting for the scheduler to shutdown, timeout after %v", to)

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
		s.Run()
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
		s.startQueue(q)
	}
}

// simple wrapper to have this in just one place
func (s *Scheduler) startQueue(q *jobQueue) {
	q.run(s.checksPipe)
	q.running = true
}

// expQueues return a function to get the stats for the queues
func expQueues(s *Scheduler) func() interface{} {
	return func() interface{} {
		queues := make([]map[string]interface{}, 0)

		for interval, queue := range s.jobQueues {
			queueStats := map[string]interface{}{
				"Interval": interval / time.Second,
				"Size":     len(queue.jobs),
			}
			queues = append(queues, queueStats)
		}
		return queues
	}
}
