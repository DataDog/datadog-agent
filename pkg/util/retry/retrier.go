// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package retry

import (
	"errors"
	"fmt"
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
	if cfg == nil {
		return errors.New("nil configuration object")
	}

	switch cfg.Strategy {
	case RetryCount:
		if cfg.RetryCount == 0 {
			return errors.New("RetryCount strategy needs a non-zero RetryCount")
		}
		if cfg.RetryDelay.Nanoseconds() == 0 {
			return errors.New("RetryCount strategy needs a non-zero RetryDelay")
		}
	case Backoff:
		if cfg.InitialRetryDelay == 0 {
			return errors.New("Backoff strategy needs a non-zero InitialRetryDelay")
		}
		if cfg.MaxRetryDelay == 0 {
			return errors.New("Backoff strategy needs a non-zero MaxRetryDelay")
		}
	}

	r.Lock()
	r.cfg = *cfg
	if cfg.Strategy == JustTesting {
		r.status = OK
	} else {
		r.status = Idle
	}

	if r.cfg.now == nil {
		r.cfg.now = time.Now
	}
	r.Unlock()

	return nil
}

// RetryStatus allows users to query the status
func (r *Retrier) RetryStatus() Status {
	r.RLock()
	defer r.RUnlock()

	return r.status
}

// NextRetry allows users to know when the next retry can happened
func (r *Retrier) NextRetry() time.Time {
	r.RLock()
	defer r.RUnlock()

	return r.nextTry
}

// TriggerRetry triggers a new retry and returns the result
func (r *Retrier) TriggerRetry() *Error {
	r.RLock()
	status := r.status
	r.RUnlock()

	switch status {
	case OK:
		return nil
	case NeedSetup:
		return r.errorf("retryer not initialised")
	case PermaFail:
		return r.errorf("retry number exceeded")
	default:
		return r.doTry()
	}
}

func (r *Retrier) doTry() *Error {
	r.RLock()
	if !r.nextTry.IsZero() && r.cfg.now().Before(r.nextTry) {
		r.RUnlock()
		return r.errorf("try delay not elapsed yet")
	}
	method := r.cfg.AttemptMethod
	r.RUnlock()
	err := method()

	r.Lock()
	r.lastTryError = err
	if err == nil {
		r.status = OK
	} else {
		switch r.cfg.Strategy {
		case OneTry:
			r.status = PermaFail
		case RetryCount:
			r.tryCount++
			if r.tryCount >= r.cfg.RetryCount {
				r.status = PermaFail
			} else {
				r.status = FailWillRetry
				r.nextTry = r.cfg.now().Add(r.cfg.RetryDelay - 100*time.Millisecond)
			}
		case Backoff:
			sleep := r.cfg.InitialRetryDelay * 1 << r.tryCount
			if sleep > r.cfg.MaxRetryDelay {
				sleep = r.cfg.MaxRetryDelay
			} else {
				r.tryCount++
			}
			r.status = FailWillRetry
			r.nextTry = r.cfg.now().Add(sleep)
		}
	}
	r.Unlock()

	return r.wrapError(err)
}

func (r *Retrier) errorf(format string, a ...interface{}) *Error {
	return r.wrapError(fmt.Errorf(format, a...))
}

func (r *Retrier) wrapError(err error) *Error {
	if err == nil {
		return nil
	}

	r.RLock()
	defer r.RUnlock()

	return &Error{
		RessourceName: r.cfg.Name,
		RetryStatus:   r.status,
		LogicError:    err,
		LastTryError:  r.lastTryError,
	}
}
