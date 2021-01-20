package flush

import (
	"time"
)

// Strategy is deciding whether the data should be flushed or not at the given moment.
type Strategy interface {
	String() string
	ShouldFlush(moment FlushMoment, t time.Time) bool
}

// FlushMoment represents at which moment we're asking the flush Strategy if we
// should flush or not.
// Note that there is no entry for the shutdown of the environment because we always
// flush in this situation.
type FlushMoment string

const (
	// Starting is used to represent the moment the function is starting because
	// it has been invoked.
	Starting FlushMoment = "starting"
	// Stopping is used to represent the moment right after the function has finished
	// its execution.
	Stopping FlushMoment = "stopping"
	// Running is used to indicate that the function is still running.
	// Running FlushMoment = "running"
)

// -----

// AtTheEnd strategy is the simply flushing the data at the end of the execution of the function.
// FIXME(remy): in its own file?
type AtTheEnd struct{}

func (s *AtTheEnd) String() string { return "end" }

func (s *AtTheEnd) ShouldFlush(moment FlushMoment, t time.Time) bool {
	return moment == Stopping
}

// -----

// AtTheStart is the strategy flushing at the start of the execution of the function.
// FIXME(remy): in its own file?
type AtTheStart struct{}

func (s *AtTheStart) String() string { return "start" }

func (s *AtTheStart) ShouldFlush(moment FlushMoment, t time.Time) bool {
	return moment == Starting
}

// -----

// AtLeast is the strategy flushing at least every N [nano/micro/milli]seconds
// at the start of the function.
type AtLeast struct {
	// FIXME(remy): comment me
	N time.Duration
	// lastFlush
	lastFlush time.Time
}

func (s *AtLeast) String() string { return "at least" }

func (s *AtLeast) ShouldFlush(moment FlushMoment, t time.Time) bool {
	if moment == Starting {
		now := time.Now()
		if s.lastFlush.Add(s.N).Before(now) {
			s.lastFlush = now
			return true
		}
	}
	return false
}

// -----

// EveryNInvoke is the strategy flushing at the start of the function but only every N invocations.
type EveryNInvoke struct {
	// The flush will happen every N invocations.
	// In other words: N-1 is the amount of function invocation for which this strategy won't flush
	N int
	// cnt is the internal counter used to decide whether or not the flush should be executed.
	cnt int
}

func (s *EveryNInvoke) String() string { return "every n invoke" }

func (s *EveryNInvoke) ShouldFlush(moment FlushMoment, t time.Time) bool {
	if moment == Starting {
		s.cnt += 1
		if s.cnt%s.N == 0 {
			s.cnt = 0
			return true
		}
	}
	return false
}
