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
	lock       sync.RWMutex
	strategies map[string]map[string]int // operation name -> backoffStatus/backoffMax -> int
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

// Get returns the current strategies strategy's status, i.e. how many checks to skip until retrying
func (s *Store) Get(name string) (int, bool) {
	s.lock.RLock()

	// Get the backoff strategy
	strategy, ok := s.strategies[name]
	if !ok {
		s.lock.RUnlock()
		return 0, false
	}

	// Check that the strategy's status exists
	status, ok := strategy["status"]
	if !ok {
		s.lock.RUnlock()
		return 0, false
	}

	s.lock.RUnlock()

	return status, ok
}

// Init initializes both the "status" and "max" counters associated with an operation's retries
// We need to track two fields in the map because we need to both:
// - keep track of the maximum strategies reached (otherwise we couldn't exponentially strategies)
// - keep track of the current strategies strategy's status, decrementing by 1 each attempt
func (s *Store) Init(name string, init int) bool {
	s.lock.RLock()

	// Guard against initializing a key that already exists
	if _, ok := s.strategies[name]; ok {
		s.lock.RUnlock()
		return ok
	}

	// Initialize map with given value
	s.strategies[name] = map[string]int{status: init, maximum: init}

	s.lock.RUnlock()

	return true
}

// Decrement decreases the current strategies strategy's status by 1, called each time an attempt is skipped
func (s *Store) Decrement(name string) bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	// Get the backoff strategy
	strategy, ok := s.strategies[name]
	if !ok {
		return false
	}

	// Check that the strategy's status exists
	if _, ok := strategy[status]; !ok {
		return false
	}

	strategy[status]--

	return true
}

// ExponentialIncrease increases the previous max strategies by a supplied multiplier and sets the current
// strategies status to the new max
// e.g. if the previous max was 2, this would set both max and status to 4
func (s *Store) ExponentialIncrease(name string, multiplier int, max int) bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	// Get the strategies strategy
	strategy, ok := s.strategies[name]
	if !ok {
		return false
	}

	// Check that the strategy's maximum key exists
	if _, ok := strategy[maximum]; !ok {
		return false
	}

	// Calculate new backoff
	newBackoff := strategy[maximum] * multiplier
	if newBackoff > max {
		newBackoff = max
	}

	strategy[maximum] = newBackoff
	strategy[status] = newBackoff

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

			// Decrement strategy by 1
			backoff.Decrement(opName)
			return none, err
		}

	}

	result, err := op()

	// If operation failed, set backoff strategy
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
