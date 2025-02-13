// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checkhelpers

import "github.com/DataDog/datadog-agent/pkg/util/log"

type BackoffStore interface {
	Get(string) (int, bool)
	Init(string, int)
	Decrement(string) bool
	ExponentialIncrease(string, int, int) bool
}
type RetryableOperation[T any] func() (T, error)

// Retry is a helper function to retry operations inside a Check, it relies on the fact that a
// check will be run at regular intervals, and "backs off" by skipping N number of check executions
func Retry[B BackoffStore, T any](backoff B, opName string, op RetryableOperation[T], factor int, maxBackoff int) (T, error) {
	var none T

	// Check if operation is already in backoff and throw error if so
	if b, ok := backoff.Get(opName); ok {
		if b > 0 {
			err := log.Errorf("In backoff, skipping %d check(s) runs before resuming: %s",
				backoff,
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
		backoff.ExponentialIncrease(opName, factor, maxBackoff)

		// throw err
		return none, err
	}

	return result, nil
}
