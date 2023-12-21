// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package params implements function parameters for [e2e.Suite]
package params

// Params implements [e2e.Suite] options
type Params struct {
	StackName string

	// Setting DevMode allows to skip deletion regardless of test results
	// Unavailable in CI.
	DevMode bool

	SkipDeleteOnFailure bool

	// Setting LazyEnvironment allows to skip environment creation
	// until the first explicit call to suite.Env()
	LazyEnvironment bool
}

// Option is an optional function parameter type for e2e options
type Option = func(*Params)

// WithStackName overrides the default stack name.
// This function is useful only when using [Run].
func WithStackName(stackName string) func(*Params) {
	return func(options *Params) {
		options.StackName = stackName
	}
}

// WithDevMode enables dev mode.
// Dev mode doesn't destroy the environment when the test finished which can
// be useful when writing a new E2E test.
func WithDevMode() func(*Params) {
	return func(options *Params) {
		options.DevMode = true
	}
}

// WithSkipDeleteOnFailure doesn't destroy the environment when a test fail.
func WithSkipDeleteOnFailure() func(*Params) {
	return func(options *Params) {
		options.SkipDeleteOnFailure = true
	}
}

// WithLazyEnvironment skips environment creation until the first explicit call to suite.Env()
func WithLazyEnvironment() func(*Params) {
	return func(options *Params) {
		options.LazyEnvironment = true
	}
}
