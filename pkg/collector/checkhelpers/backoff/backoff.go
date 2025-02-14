// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package backoff

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"sync"
)

const (
	status  = "status"
	maximum = "maximum"
)

type RetryableOperation[T any] func() (T, error)

type Store struct {
	lock    sync.RWMutex
	backoff map[string]map[string]int // operation name -> backoffStatus/backoffMax -> int
}

func New() *Store {
	return &Store{
		backoff: make(map[string]map[string]int),
	}
}

// Get returns the current backoff strategy's status, i.e. how many checks to skip until retrying
func (s *Store) Get(name string) (int, bool) {
	s.lock.RLock()
	strategy, ok := s.backoff[name]
	if !ok {
		return 0, ok
	}

	status, ok := strategy["status"]
	if !ok {
		return 0, ok
	}

	s.lock.RUnlock()
	return status, ok
}

// Init initializes both the "status" and "max" counters associated with an operation's retries
// We need to track two fields in the map because we need to both:
// - keep track of the maximum backoff reached (otherwise we couldn't exponentially backoff)
// - keep track of the current backoff strategy's status, decrementing by 1 each attempt
func (s *Store) Init(name string, init int) bool {
	s.lock.RLock()

	// Guard against initializing a key that already exists
	if _, ok := s.backoff[name]; ok {
		s.lock.RUnlock()
		return ok
	}

	s.backoff[name] = map[string]int{status: init, maximum: init}
	s.lock.RUnlock()

	return true
}

// Decrement decreases the current backoff strategy's status by 1, called each time an attempt is skipped
func (s *Store) Decrement(name string) bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	strategy, ok := s.backoff[name]
	if !ok {
		return false
	}

	if _, ok := strategy[status]; !ok {
		return false
	}

	strategy[status]--

	return true
}

// ExponentialIncrease increases the previous max backoff by a supplied multiplier and sets the current
// backoff status to the new max
// e.g. if the previous max was 2, this would set both max and status to 4
func (s *Store) ExponentialIncrease(name string, multiplier int, max int) bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	strategy, ok := s.backoff[name]
	if !ok {
		return false
	}

	if _, ok := strategy[maximum]; !ok {
		return false
	}

	multiplied := strategy[maximum] * multiplier
	if multiplied > max {
		multiplied = max
	}

	strategy[maximum] = multiplied
	strategy[status] = multiplied

	return true
}

// Retry is a helper function to retry operations inside a Check, it relies on the fact that a
// check will be run at regular intervals, and "backs off" by skipping N number of check executions
func Retry[T any](backoff *Store, opName string, op RetryableOperation[T], multiplier int, maxBackoff int) (T, error) {
	var none T

	// Check if operation is already in backoff and throw error if so
	if status, ok := backoff.Get(opName); ok {
		if status > 0 {
			err := log.Errorf("In backoff, skipping %d check(s) runs before resuming: %s",
				status,
				opName)

			// Decrement backoff by 1
			backoff.Decrement(opName)
			return none, err
		}

	}

	result, err := op()

	// If operation failed, set backoff
	if err != nil {
		_, ok := backoff.Get(opName)
		// backoff wasn't found, default to 1
		if !ok {
			backoff.Init(opName, 1)
			return none, err
		}

		// Increase backoff by a specified factor until max is reached
		backoff.ExponentialIncrease(opName, multiplier, maxBackoff)

		// throw err
		return none, err
	}

	return result, nil
}
