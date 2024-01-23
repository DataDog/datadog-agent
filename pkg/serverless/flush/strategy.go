// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(SERV) Fix revive linter
package flush

import (
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const maxBackoffRetrySeconds = 5 * 60

// Strategy is deciding whether the data should be flushed or not at the given moment.
type Strategy interface {
	String() string
	ShouldFlush(moment Moment, t time.Time) bool
	Failure(t time.Time)
	Success()
}

type retryState struct {
	lastFail time.Time
	retries  uint64
	lock     sync.Mutex
}

var globalRetryState = retryState{}

// Moment represents at which moment we're asking the flush strategy if we
// should flush or not.
// Note that there is no entry for the shutdown of the environment because we always
// flush in this situation.
type Moment string

const (
	// Starting is used to represent the moment the function is starting because
	// it has been invoked.
	Starting Moment = "starting"
	// Stopping is used to represent the moment right after the function has finished
	// its execution.
	Stopping Moment = "stopping"
)

// StrategyFromString returns a flush strategy from the given string.
// Possible values:
//   - end
//   - periodically[,milliseconds]
func StrategyFromString(str string) (Strategy, error) {
	panic("not called")
}

// -----

// AtTheEnd strategy is the simply flushing the data at the end of the execution of the function.
type AtTheEnd struct {
}

func (s *AtTheEnd) String() string { return "end" }

// ShouldFlush returns true if this strategy want to flush at the given moment.
func (s *AtTheEnd) ShouldFlush(moment Moment, t time.Time) bool {
	if globalRetryState.shouldWaitBackoff(t) {
		return false
	}
	return moment == Stopping
}

// Failure modify state to keep track of failure
func (s *AtTheEnd) Failure(t time.Time) {
	panic("not called")
}

// Success reset the state when a flush is successful
func (s *AtTheEnd) Success() {
	globalRetryState.reset()
}

// Periodically is the strategy flushing at least every N [nano/micro/milli]seconds
// at the start of the function.
type Periodically struct {
	interval  time.Duration
	lastFlush time.Time
}

// NewPeriodically returns an initialized Periodically flush strategy.
func NewPeriodically(interval time.Duration) *Periodically {
	panic("not called")
}

func (s *Periodically) String() string {
	panic("not called")
}

// ShouldFlush returns true if this strategy want to flush at the given moment.
func (s *Periodically) ShouldFlush(moment Moment, t time.Time) bool {
	panic("not called")
}

// Failure modify state to keep track of failure
func (s *Periodically) Failure(t time.Time) {
	panic("not called")
}

// Success reset the state when a flush is successful
func (s *Periodically) Success() {
	panic("not called")
}

func (r *retryState) shouldWaitBackoff(now time.Time) bool {
	r.lock.Lock()
	defer r.lock.Unlock()
	if r.retries > 0 {
		maxRetryBackoff := math.Min(float64(r.retries), 10) // no need to go higher and risk overflow in the power op
		spreadRetrySeconds := float64(rand.Int31n(1_000)) / 1_000
		ignoreWindowSeconds := int(math.Min(math.Pow(2, maxRetryBackoff)+spreadRetrySeconds, maxBackoffRetrySeconds))

		whenAcceptingFlush := r.lastFail.Add(time.Duration(ignoreWindowSeconds * 1e9))

		timeLeft := int(math.Max(float64(whenAcceptingFlush.Second()-now.Second()), 0))

		log.Debugf("Flush failed %d times, flushes will be prevented for %d seconds (%d left)", r.retries, ignoreWindowSeconds, timeLeft)
		return now.Before(whenAcceptingFlush)
	}
	return false
}

func (r *retryState) incrementFailure(t time.Time) {
	panic("not called")
}

func (r *retryState) reset() {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.retries = 0
}
