package scheduler

import (
	"errors"
	"sync"
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
	jobQueues  map[time.Duration]*jobQueue // We have one scheduling queue for every interval
	mu         sync.Mutex                  // to protect critical sections in struct's fields
}

// jobQueue contains a list of checks (called jobs) that need to be
// scheduled at a certain interval.
type jobQueue struct {
	interval time.Duration
	stop     chan bool
	ticker   *time.Ticker
	jobs     []check.Check
	started  bool
	mu       sync.Mutex // to protect critical sections in struct's fields
}

// NewScheduler create a Scheduler and returns a pointer to it.
func NewScheduler(out chan<- check.Check) *Scheduler {
	return &Scheduler{
		checksPipe: out,
		done:       make(chan bool),
		jobQueues:  make(map[time.Duration]*jobQueue),
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

// Run is the Scheduler main loop
func (s *Scheduler) Run() {
	go func() {
		log.Debug("Starting scheduler loop...")
		s.startQueues()

		<-s.done

		log.Debug("Scheduler loop done, shutting down queues...")
		s.stopQueues()
	}()
}

// Stop the scheduler, optionally pass an integer to specify
// the timeout in milliseconds
func (s *Scheduler) Stop(timeout ...int) error {
	to := time.Duration(defaultTimeout)
	if len(timeout) == 1 {
		to = time.Duration(timeout[0])
	}

	log.Debugf("Stopping scheduler loop, timeout after %dms.", to)
	select {
	case s.done <- true:
		return nil
	case <-time.After(to * time.Millisecond):
		return errors.New("Stop operation timed out.")
	}
}

// Reload the scheduler
func (s *Scheduler) Reload(timeout ...int) error {
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
		if q.started {
			q.stop <- true
			q.started = false
		}
	}
}

// startQueues loads the timer for each queue
func (s *Scheduler) startQueues() {
	for _, q := range s.jobQueues {
		go q.run(s.checksPipe)
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
// execution pipeline
func (jq *jobQueue) run(out chan<- check.Check) {
	jq.started = true
	for {
		select {
		case <-jq.stop:
			// someone asked to stop this queue
			jq.ticker.Stop()
		case <-jq.ticker.C:
			// normal case, (re)schedule the queue
			for _, check := range jq.jobs {
				log.Debugf("Enqueuing check %s for queue %d", check, jq.interval)
				out <- check
			}
		}
	}
}
