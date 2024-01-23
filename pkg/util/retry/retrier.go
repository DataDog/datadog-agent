// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package retry implements a configurable retry mechanism that can be embedded
// in any class needing a retry-on-error system.
package retry

import (
	"sync"
	"time"
)

// Retrier implements a configurable retry mechanism than can be embedded
// in any class providing attempt logic as a `func() error` method.
// See the unit test for an example.
type Retrier struct {
	sync.RWMutex
	cfg          Config
	status       Status
	nextTry      time.Time
	tryCount     uint
	lastTryError error
}

// SetupRetrier must be called before calling other methods
func (r *Retrier) SetupRetrier(cfg *Config) error {
	panic("not called")
}

// RetryStatus allows users to query the status
func (r *Retrier) RetryStatus() Status {
	panic("not called")
}

// NextRetry allows users to know when the next retry can happened
func (r *Retrier) NextRetry() time.Time {
	panic("not called")
}

// LastError allows users to know what the last retry failure error was
func (r *Retrier) LastError() *Error {
	panic("not called")
}

// TriggerRetry triggers a new retry and returns the result
func (r *Retrier) TriggerRetry() *Error {
	panic("not called")
}

func (r *Retrier) doTry() *Error {
	panic("not called")
}

func (r *Retrier) errorf(format string, a ...interface{}) *Error {
	panic("not called")
}

func (r *Retrier) wrapError(err error) *Error {
	panic("not called")
}
