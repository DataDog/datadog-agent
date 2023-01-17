// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scheduler

import (
	"expvar"
	"fmt"
	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

var (
	minAllowedInterval     = 1 * time.Second
	schedulerExpvars       *expvar.Map
	schedulerQueuesCount   = expvar.Int{}
	schedulerChecksEntered = expvar.Int{}

	tlmChecksEntered = telemetry.NewGauge("scheduler", "checks_entered",
		[]string{"check_name"}, "How many checks are currently tracked by the scheduler")
	tlmQueuesCount = telemetry.NewCounter("scheduler", "queues_count",
		nil, "How many queues were opened")
)

func init() {
	schedulerExpvars = expvar.NewMap("scheduler")
	schedulerExpvars.Set("QueuesCount", &schedulerQueuesCount)
	schedulerExpvars.Set("ChecksEntered", &schedulerChecksEntered)
}

// Scheduler keeps things rolling.
// More docs to come...
type Scheduler struct {
	running          *atomic.Bool                // Flag to see if the scheduler is running
	checksPipe       chan<- check.Check          // The pipe the Runner pops the checks from, initially set to nil
	done             chan bool                   // Guard for the main loop
	halted           chan bool                   // Used to internally communicate all queues are done
	started          chan bool                   // Used to internally communicate the queues are up
	jobQueues        map[time.Duration]*jobQueue // We have one scheduling queue for every interval
	tlmTrackedChecks map[check.ID]string         // Keep track of the checks that are tracked with telemetry
	mu               sync.Mutex                  // To protect critical sections in struct's fields

	checkToQueue map[check.ID]*jobQueue // Keep track of what is the queue for any Check
	// To protect checkToQueue. Using mu would create a deadlock when stopping the Scheduler. 'jobQueue' is calling
	// 'IsCheckScheduled' right when then 'Stop' function is called and mu is already lock. for this reason we have
	// to lock: one for the Scheduler and a dedicated one for the 'IsCheckScheduled' method. This way 'jobQueue' and
	// metadata provider can call 'IsCheckScheduled' without creating a deadlock.
	checkToQueueMutex sync.RWMutex

	cancelOneTime chan bool      // Used to internally communicate a cancel signal to one-time schedule goroutines
	wgOneTime     sync.WaitGroup // WaitGroup to track the exit of one-time schedule goroutines
}

// NewScheduler create a Scheduler and returns a pointer to it.
func NewScheduler(checksPipe chan<- check.Check) *Scheduler {
	return &Scheduler{
		checksPipe:       checksPipe,
		done:             make(chan bool),
		halted:           make(chan bool),
		started:          make(chan bool),
		jobQueues:        make(map[time.Duration]*jobQueue),
		checkToQueue:     make(map[check.ID]*jobQueue),
		tlmTrackedChecks: make(map[check.ID]string),
		running:          atomic.NewBool(false),
		cancelOneTime:    make(chan bool),
		wgOneTime:        sync.WaitGroup{},
	}
}

// Enter schedules a `Check`s for execution accordingly to the `Check.Interval()` value.
// If the interval is 0, the check is supposed to run only once.
func (s *Scheduler) Enter(check check.Check) error {
	// enqueue immediately if this is a one-time schedule
	if check.Interval() == 0 {
		s.enqueueOnce(check)
		return nil
	}

	if check.Interval() < minAllowedInterval {
		return fmt.Errorf("schedule interval must be greater than %v or 0", minAllowedInterval)
	}

	log.Infof("Scheduling check %s with an interval of %v", check.ID(), check.Interval())

	// sync when accessing `jobQueues` and `check2queue`
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.jobQueues[check.Interval()]; !ok {
		s.jobQueues[check.Interval()] = newJobQueue(check.Interval())
		s.startQueue(s.jobQueues[check.Interval()])
		if check.IsTelemetryEnabled() {
			tlmQueuesCount.Inc()
		}
		schedulerQueuesCount.Add(1)
	}
	s.jobQueues[check.Interval()].addJob(check)

	// map each check to the Job Queue it was assigned to
	s.checkToQueueMutex.Lock()
	s.checkToQueue[check.ID()] = s.jobQueues[check.Interval()]
	s.checkToQueueMutex.Unlock()

	schedulerChecksEntered.Add(1)
	if check.IsTelemetryEnabled() {
		checkName := check.String()
		s.tlmTrackedChecks[check.ID()] = checkName
		tlmChecksEntered.Inc(checkName)
	}
	schedulerExpvars.Set("Queues", expvar.Func(expQueues(s)))
	return nil
}

// Cancel remove a Check from the scheduled queue. If the check is not
// in the scheduler, this is a noop.
func (s *Scheduler) Cancel(id check.ID) error {
	s.mu.Lock()
	s.checkToQueueMutex.Lock()
	defer s.mu.Unlock()
	defer s.checkToQueueMutex.Unlock()

	log.Infof("Unscheduling check %s", string(id))

	if _, ok := s.checkToQueue[id]; !ok {
		return nil
	}

	// remove it from the queue
	err := s.checkToQueue[id].removeJob(id)
	if err != nil {
		return fmt.Errorf("unable to remove the Job from the queue: %s", err)
	}
	delete(s.checkToQueue, id)

	schedulerChecksEntered.Add(-1)
	if checkName, ok := s.tlmTrackedChecks[id]; ok {
		delete(s.tlmTrackedChecks, id)
		tlmChecksEntered.Dec(checkName)
	}
	schedulerExpvars.Set("Queues", expvar.Func(expQueues(s)))
	return nil
}

// Run is the Scheduler main loop.
// This doesn't block but waits for the queues to be ready before returning.
func (s *Scheduler) Run() {
	// Invoking Run does nothing if the Scheduler is already running
	if s.running.Load() {
		log.Debug("Scheduler is already running")
		return
	}

	go func() {
		log.Debug("Starting scheduler loop...")

		s.startQueues()

		// set internal state
		s.running.Store(true)

		// notify queues are up
		s.started <- true

		// wait here until we're done
		<-s.done

		// someone asked to stop
		s.running.Store(false)
		log.Debug("Exited Scheduler loop, shutting down queues...")
		s.stopQueues()

		// notify we're done
		s.halted <- true
	}()

	// Wait until queues are up
	<-s.started
}

// Stop the scheduler, blocks until the scheduler is fully stopped.
func (s *Scheduler) Stop() error {
	// Stopping when the Scheduler is not running is a noop.
	if !s.running.Load() {
		log.Debug("Scheduler is already stopped")
		return nil
	}

	// Interrupt the main loop, proceeding to shut down all the queues
	s.done <- true

	// Signal an exit to any remaining goroutine still trying to enqueue one-time checks,
	// and wait for them to exit
	close(s.cancelOneTime)
	s.wgOneTime.Wait()

	log.Debugf("Waiting for the scheduler to shutdown")

	select {
	case <-s.halted:
		return nil
	}
}

// IsCheckScheduled returns whether a check is in the schedule or not
func (s *Scheduler) IsCheckScheduled(id check.ID) bool {
	s.checkToQueueMutex.RLock()
	defer s.checkToQueueMutex.RUnlock()

	_, found := s.checkToQueue[id]
	return found
}

// stopQueues shuts down the timers for each active queue
// Blocks until all the queues have fully stopped
func (s *Scheduler) stopQueues() {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Debugf("Stopping %v queue(s)", len(s.jobQueues))
	for _, q := range s.jobQueues {
		// check that the queue is actually running or this blocks
		// while posting to the channel
		if q.running {
			q.stop <- true
			<-q.stopped
			log.Debugf("Stopped queue %v", q.interval)
			q.running = false
		}
	}
}

// startQueues loads the timer for each queue
// Should not block, unless there's contention on the internal mutex
func (s *Scheduler) startQueues() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, q := range s.jobQueues {
		s.startQueue(q)
	}
}

// startQueue starts a queue (non-blocking operation) if it's not running yet
func (s *Scheduler) startQueue(q *jobQueue) {
	if !q.running {
		q.run(s)
		q.running = true
	}
}

// enqueueOnce enqueues a check once to the checksPipe.
// Do not block, in case the runner has not started yet.
// The queuing can be cancelled by closing the `cancelOneTime` channel.
func (s *Scheduler) enqueueOnce(check check.Check) {
	log.Infof("Scheduling check %v for one-time execution", check)
	s.wgOneTime.Add(1)

	go func(cancelOneTime <-chan bool) {
		defer s.wgOneTime.Done()
		select {
		case s.checksPipe <- check:
		case <-cancelOneTime:
		}
	}(s.cancelOneTime)

	schedulerChecksEntered.Add(1)
}

// expQueues return a function to get the stats for the queues
func expQueues(s *Scheduler) func() interface{} {
	return func() interface{} {
		queues := make([]map[string]interface{}, 0)

		for _, queue := range s.jobQueues {
			queues = append(queues, queue.stats())
		}
		return queues
	}
}
