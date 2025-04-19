// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package diskv2 provides Disk Check.
package diskv2

import "time"

// Clock abstracts time-related functions used in the package.
type Clock interface {
	// After returns a channel that will send the current time after the specified duration.
	After(d time.Duration) <-chan time.Time
}

// RealClock is the production implementation that wraps time.After.
type RealClock struct{}

// After returns the channel from time.After.
func (rc *RealClock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}

// FakeClock is used in unit tests.
type FakeClock struct {
	AfterCh chan time.Time
}

// After returns the controlled channel.
func (fc *FakeClock) After(_ time.Duration) <-chan time.Time {
	return fc.AfterCh
}
