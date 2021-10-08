// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package retry

import "time"

// Status is returned by Retrier object to inform user classes
type Status int

const (
	// NeedSetup is the default value: SetupRetrier must be called
	NeedSetup Status = iota // Default zero value
	// Idle means the Retrier is ready for Try to be called
	Idle
	// OK means the object is available
	OK
	// FailWillRetry informs users the object is not available yet,
	// but they should retry later
	FailWillRetry
	// PermaFail informs the user the object will not be available.
	PermaFail
)

// Strategy sets how the Retrier should handle failure
type Strategy int

const (
	// OneTry is the default value: only try one, then permafail
	OneTry Strategy = iota // Default zero value
	// RetryCount sets the Retrier to try a fixed number of times
	RetryCount
	// Backoff retries often at the beginning and then, less often
	Backoff
	// RetryDuration sets the Retrier to try for a fixed duration
	// RetryDuration // FIXME: implement

	// JustTesting forces an OK status for unit tests that require a
	// non-functional object but no failure on init (eg. docker)
	JustTesting
)

// Config contains all the required parameters for Retrier
type Config struct {
	Name              string
	AttemptMethod     func() error
	Strategy          Strategy
	RetryCount        uint
	RetryDelay        time.Duration
	InitialRetryDelay time.Duration
	MaxRetryDelay     time.Duration
	// now function is used in unit tests only and should be left to nil otherwise
	now func() time.Time
}
