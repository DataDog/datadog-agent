// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

// SimpleCounter tracks how many times something is happening.
type SimpleCounter interface {
	// Inc increments the counter.
	Inc()
	// Add increments the counter by given amount.
	Add(float64)
	// Get gets the current counter value
	Get() float64
}
