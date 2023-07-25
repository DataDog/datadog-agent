// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flush

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"
)

const maxBackoffRetrySeconds = 5 * 60

// Strategy is deciding whether the data should be flushed or not at the given moment.
type Strategy interface {
	String() string
	ShouldFlush(moment Moment, t time.Time) bool
	Failure(t time.Time)
	Success()
}

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
	switch str {
	case "end":
		return &AtTheEnd{}, nil
	case "periodically":
		return NewPeriodically(10 * time.Second), nil
	}

	if strings.HasPrefix(str, "periodically") && strings.Count(str, ",") == 1 {
		parts := strings.Split(str, ",")

		msecs, err := strconv.Atoi(parts[1])
		if err != nil {
			return &AtTheEnd{}, fmt.Errorf("StrategyFromString: can't parse flush strategy: %s", str)
		}

		return NewPeriodically(time.Duration(msecs) * time.Millisecond), nil
	}

	return &AtTheEnd{}, fmt.Errorf("StrategyFromString: can't parse flush strategy: %s", str)
}

// -----

// AtTheEnd strategy is the simply flushing the data at the end of the execution of the function.
type AtTheEnd struct {
	lastFailure failedAttempt
}

func (s *AtTheEnd) String() string { return "end" }

// ShouldFlush returns true if this strategy want to flush at the given moment.
func (s *AtTheEnd) ShouldFlush(moment Moment, t time.Time) bool {
	if s.lastFailure.tooEarly(t) {
		return false
	} else {
		return moment == Stopping
	}
}

func (s *AtTheEnd) Failure(t time.Time) {
	s.lastFailure.incrementFailure(t)
}
func (s *AtTheEnd) Success() {
	s.lastFailure.reset()
}

// Periodically is the strategy flushing at least every N [nano/micro/milli]seconds
// at the start of the function.
type Periodically struct {
	interval    time.Duration
	lastFlush   time.Time
	lastFailure failedAttempt
}

// NewPeriodically returns an initialized Periodically flush strategy.
func NewPeriodically(interval time.Duration) *Periodically {
	return &Periodically{interval: interval}
}

func (s *Periodically) String() string {
	return fmt.Sprintf("periodically,%d", s.interval/time.Millisecond)
}

// ShouldFlush returns true if this strategy want to flush at the given moment.
func (s *Periodically) ShouldFlush(moment Moment, t time.Time) bool {
	if moment == Starting && !s.lastFailure.tooEarly(t) {
		if s.lastFlush.Add(s.interval).Before(t) {
			s.lastFlush = t
			return true
		}
	}
	return false
}

func (s *Periodically) Failure(t time.Time) {
	s.lastFailure.incrementFailure(t)
}

func (s *Periodically) Success() {
	s.lastFailure.reset()
}

type failedAttempt struct {
	lastFail time.Time
	retries  int
}

func (f *failedAttempt) tooEarly(now time.Time) bool {
	if f.retries > 0 {
		spreadRetrySeconds := float64(rand.Int31n(1_000)) / 1_000
		ignoreWindowSeconds := int(math.Min(math.Pow(2, float64(f.retries))+spreadRetrySeconds, maxBackoffRetrySeconds))
		log.Debug("Flush failed, retry number %s will happen in %s seconds", f.retries, ignoreWindowSeconds)
		return now.Before(f.lastFail.Add(time.Duration(ignoreWindowSeconds * 1e9)))
	} else {
		return false
	}
}

func (f *failedAttempt) incrementFailure(t time.Time) {
	if f.retries < 10 { // no need to go higher and risk overflow in the power op when calculating backoff
		f.retries += 1
	}
	f.lastFail = t
}

func (f *failedAttempt) reset() {
	f.retries = 0
}
