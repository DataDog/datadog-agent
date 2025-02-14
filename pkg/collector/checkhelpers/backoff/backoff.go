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
	countdown = "countdown"
	maximum   = "maximum"
)

type RetryableOperation[T any] func() (T, error)

type Store struct {
	lock       sync.RWMutex
	strategies map[string]map[string]int // "operation_name": { "countdown": X, "maximum": Y }
}

func New() *Store {
	return &Store{
		strategies: make(map[string]map[string]int),
	}
}

func (s *Store) Delete(name string) {
	s.lock.Lock()
	delete(s.strategies, name)
	s.lock.Unlock()
}

// Get returns the current strategies strategy's countdown, i.e. how many checks to skip until retrying
func (s *Store) Get(name string) (int, bool) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	// Get the backoff strategy ex: "operation_name": { "countdown": 2, "maximum": 4 }
	strategy, ok := s.strategies[name]
	if !ok {
		return 0, false
	}

	// Check that the strategy's countdown key exists
	status, ok := strategy["countdown"]
	if !ok {
		return 0, false
	}

	return status, ok
}

// Init initializes both the "countdown" and "maximum" keys associated with an operation's retries
// We need to track two fields in the map because we need to:
// - keep track of the maximum backoff reached (so we can exponentially backoff)
// - keep track of the current strategy's countdown (so we can keep track of how many iterations have elapsed)
func (s *Store) Init(name string, init int) bool {
	s.lock.RLock()
	defer s.lock.RUnlock()

	// Guard against initializing a key that already exists
	if _, ok := s.strategies[name]; ok {
		return ok
	}

	// Initialize a new strategy map with given value
	s.strategies[name] = map[string]int{countdown: init, maximum: init}

	return true
}

// Decrement decreases the current strategies strategy's countdown by 1, called each time an attempt is skipped
func (s *Store) Decrement(name string) bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	// Get the backoff strategy e.g. "operation_name": { "countdown": 2, "maximum": 4 }
	strategy, ok := s.strategies[name]
	if !ok {
		return false
	}

	// Check that the strategy's countdown key exists
	if _, ok := strategy[countdown]; !ok {
		return false
	}

	strategy[countdown]--

	return true
}

// ExponentialIncrease increases the previous max strategies by a supplied multiplier and sets the current
// strategy countdown to the new max
//
// e.g. if the previous max was 2, this would set both max and countdown to 4
func (s *Store) ExponentialIncrease(name string, multiplier int, max int) bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	// Get the backoff strategy e.g. "operation_name": { "countdown": 2, "maximum": 4 }
	strategy, ok := s.strategies[name]
	if !ok {
		return false
	}

	// Check that the strategy's maximum key exists
	if _, ok := strategy[maximum]; !ok {
		return false
	}

	// Calculate new backoff
	newMaximum := strategy[maximum] * multiplier
	if newMaximum > max {
		newMaximum = max
	}

	strategy[maximum] = newMaximum
	strategy[countdown] = newMaximum

	return true
}

// Retry is a helper function to retry operations inside a Check, it relies on the fact that a
// check will be run at regular intervals, and "backs off" by skipping N number of check executions
func Retry[T any](backoff *Store, opName string, op RetryableOperation[T], multiplier int, maxBackoff int) (T, error) {
	var none T

	// Check if operation is already in backoff and throw error if so
	if counter, ok := backoff.Get(opName); ok {
		if counter > 0 {
			err := log.Errorf("In backoff, skipping %d check(s) runs before resuming: %s",
				counter,
				opName)

			// Decrement strategy by 1
			backoff.Decrement(opName)
			return none, err
		}

	}

	// Call the function
	result, err := op()

	// If operation failed, adjust backoff strategy
	if err != nil {
		_, ok := backoff.Get(opName)

		// backoff strategy wasn't found, default to 1
		if !ok {
			backoff.Init(opName, 1)
			return none, err
		}

		// Increase backoff strategy by a specified multiplication factor until max is reached
		backoff.ExponentialIncrease(opName, multiplier, maxBackoff)

		// throw err
		return none, err
	}

	// Operation was successful, clear backoff strategy entry
	backoff.Delete(opName)

	return result, nil
}
