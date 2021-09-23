// This package implements an API very similar to that of the built-in 'time'
// package, but in such a way that it can be "faked" in tests.
//
// Specifically, this package implements "accelerated" time, which skips the
// fake clock forward during times when all goroutines might otherwise be
// sleeping.  It does this by sleeping for a configurable interval, which
// should be long enough to allow any active cpu usage to finish and leave
// only pending timers.  When the interval expires, the fake time is skipped
// ahead to the earliest of those timers, which fires immediately.  The result
// is that tests can use "real" durations, such as a 30-second connection
// timeout, and this package will avoid waiting that long in terms of
// wall-clock time.
package time

import (
	"container/heap"
	"sync"
	gotime "time"
)

var faker *accelFaker

type Event struct {
	when    Time
	trigger chan<- struct{}
}

type EventQueue []*Event

func (eq EventQueue) Len() int { return len(eq) }

func (eq EventQueue) Less(i, j int) bool {
	return eq[i].when.Before(eq[j].when)
}

func (eq EventQueue) Swap(i, j int) {
	eq[i], eq[j] = eq[j], eq[i]
}

func (eq *EventQueue) Push(x interface{}) {
	*eq = append(*eq, x.(*Event))
}

func (eq *EventQueue) Pop() interface{} {
	old := *eq
	n := len(old)
	evt := old[n-1]
	old[n-1] = nil // avoid memory leak
	*eq = old[0 : n-1]
	return evt
}

// A Faker exists while time is being faked, and allows returning to "real"
// time with its Stop method.  Typically a Faker is created in a test case
// with something like `defer time.StartAcceleratedFake().Stop()`.
type Faker interface {
	// Stop the faker.  This will block until the faker is fully stopped.
	Stop()
}

type accelFaker struct {
	// This is the time interval after which we assume all blocking operations
	// have completed, and we can skip to the next interesting point in time.
	// Increasing this value will make tests run slower, but too small a value
	// may result in timers firing in unexpected orders.  The default value is
	// 10ms.
	interval Duration

	// indicate that the faker should stop
	stop chan<- struct{}

	// indicate that the faker *has* stopped
	done <-chan struct{}

	// remainder of the fields are protected by this mutex
	sync.Mutex

	// priority queue of future events and the time they should
	// occur
	futureEvents EventQueue

	// the current time
	now Time
}

// Start faking time.  Between this call and the subsequent faker.Stop() call,
// any delays for sleeping or tickers will be shortened.  The interval
// parameter should be longer than the time required to accomplish any
// non-sleeping operations, but no longer.  Increasing the interval will make
// tests take longer in a linear fashion (that is, doubling interval will
// double the time the test takes).
func StartAcceleratedFake(interval Duration) Faker {
	stop := make(chan struct{})
	done := make(chan struct{})
	futureEvents := make(EventQueue, 0)
	faker = &accelFaker{
		interval:     interval,
		stop:         stop,
		done:         done,
		futureEvents: futureEvents,
		now:          gotime.Now(),
	}
	heap.Init(&faker.futureEvents)
	faker.start(stop, done)
	return faker
}

// Run an event loop to simulate time.  This uses a ticker to advance time to
// the next moment something will happen every interval.  The idea is that
// interval should be enough time for any non-blocking test operations to
// complete, so we are merely skipping the blocking (sleeping) operations.
func (fkr *accelFaker) start(stop <-chan struct{}, done chan<- struct{}) {
	go func() {
		for {
			// If there was a lengthy GC operation, then the tick invocation
			// may have taken longer than fkr.interval, which would leave us
			// performing two ticks back-to-back.  To avoid this, we use a
			// timer that is reset at the beginning of each loop.
			tick := gotime.After(fkr.interval)

			select {
			case <-stop:
				close(done)
				return
			case <-tick:
				fkr.tick()
			}
		}
	}()
}

// Tick along to the next event
func (fkr *accelFaker) tick() {
	fkr.Lock()

	if len(fkr.futureEvents) == 0 {
		// no further events, so nothing to do ("now" remains unchanged)
		fkr.Unlock()
		return
	}

	// peek at the next event to see how far to advance
	nextEvt := fkr.futureEvents[0]
	toTrigger := fkr.advance(nextEvt.when.Sub(fkr.now))

	// unlock the faker before sending events, to avoid unnecessary
	// churn waiting on the mutex
	fkr.Unlock()

	for _, evt := range toTrigger {
		close(evt.trigger)
	}
}

// Advance the fake clock by the given duration, returning the
// events that should be triggered
//
// NOTE: this method assumes fkr is locked
func (fkr *accelFaker) advance(d Duration) []*Event {
	fkr.now = fkr.now.Add(d)

	// we must trigger the events with the faker unlocked, so
	// make a list of them first
	toTrigger := []*Event{}
	for len(fkr.futureEvents) > 0 && !fkr.futureEvents[0].when.After(fkr.now) {
		toTrigger = append(toTrigger, heap.Pop(&fkr.futureEvents).(*Event))
	}

	return toTrigger
}

// Get the current time
func (fkr *accelFaker) Now() Time {
	fkr.Lock()
	defer fkr.Unlock()

	return fkr.now
}

// Create a channel that will be closed at the given (fake) time.
func (fkr *accelFaker) at(when Time) <-chan struct{} {
	fkr.Lock()
	defer fkr.Unlock()

	c := make(chan struct{})

	// for times in the past, or now, just trigger immediately
	if !when.After(fkr.now) {
		close(c)
		return c
	}

	heap.Push(&fkr.futureEvents, &Event{
		when:    when,
		trigger: c,
	})

	return c
}

// Create a channel that will be closed after the given (fake) // duration has passed.
func (fkr *accelFaker) after(d Duration) <-chan struct{} {
	fkr.Lock()
	defer fkr.Unlock()

	c := make(chan struct{})

	heap.Push(&fkr.futureEvents, &Event{
		when:    fkr.now.Add(d),
		trigger: c,
	})

	return c
}

// Stop faking time.  This will restore all functions to their normal,
// production behavior.
func (fkr *accelFaker) Stop() {
	if faker != fkr {
		panic("method call on stopped faker")
	}

	// stop the goroutine and wait for it
	close(fkr.stop)
	<-fkr.done

	faker = nil
}
