// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package scheduler

import "time"

// SetMinAllowedInterval overrides the minimum scheduling interval
// allowed by Enter. Returns the previous value so callers can restore it.
// Intended for tests in other packages that need to drive the scheduler at
// sub-second cadences.
func SetMinAllowedInterval(d time.Duration) time.Duration {
	prev := minAllowedInterval
	minAllowedInterval = d
	return prev
}
