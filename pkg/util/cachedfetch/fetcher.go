// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package cachedfetch provides a read-through cache for fetched values.
package cachedfetch

import (
	"context"
	"sync"
)

// Fetcher supports fetching a value, such as from a cloud service API.  An
// attempt is made to fetch the value on each call to Fetch, but if that
// attempt fails then a cached value from the last successful attempt is
// returned, if such a value exists.  This helps the agent to "ride out"
// temporary failures in cloud APIs while still fetching fresh data when those
// APIs are functioning properly.  Cached values do not expire.
//
// Callers should instantiate one fetcher per piece of data required.
type Fetcher struct {
	// function that attempts to fetch the value
	Attempt func(context.Context) (interface{}, error)

	// the name of the thing being fetched, used in the default log message.  At
	// least one of Name and LogFailure must be non-nil.
	Name string

	// function to log a fetch failure, given the error and the last successful
	// value.  This function is not called if there is no last successful value.
	// If left at its zero state, a default log message will be generated, using
	// Name.
	LogFailure func(error, interface{})

	// previous successfully fetched value
	lastValue interface{}

	// mutex to protect access to lastValue
	sync.Mutex
}

// Fetch attempts to fetch the value, returning the result or the last successful
// value, or an error if no attempt has ever been successful.  No special handling
// is included for the Context: both context.Cancelled and context.DeadlineExceeded
// are handled like any other error by returning the cached value.
//
// This can be called from multiple goroutines, in which case it will call Attempt
// concurrently.
func (f *Fetcher) Fetch(ctx context.Context) (interface{}, error) {
	panic("not called")
}

// FetchString is a convenience wrapper around Fetch that returns a string
func (f *Fetcher) FetchString(ctx context.Context) (string, error) {
	panic("not called")
}

// FetchStringSlice is a convenience wrapper around Fetch that returns a string
func (f *Fetcher) FetchStringSlice(ctx context.Context) ([]string, error) {
	panic("not called")
}

// Reset resets the cached value (used for testing)
func (f *Fetcher) Reset() {
	panic("not called")
}
