// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package params TODO comment
package params

// Params exported type should have comment or be unexported
type Params struct {
	StackName string

	// Setting DevMode allows to skip deletion regardless of test results
	// Unavailable in CI.
	DevMode bool

	SkipDeleteOnFailure bool
}

// Option exported type should have comment or be unexported
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
