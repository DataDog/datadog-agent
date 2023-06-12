// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

// SimpleGauge tracks how many times something is happening.
type SimpleGauge interface {
	// Inc increments the gaguge.
	Inc()
	// Dec decrements the gauge.
	Dec()
	// Add increments the gauge by given amount.
	Add(float64)
	// Sub decrements the gauge by given amount.
	Sub(float64)
	// Set sets the value of the gauge.
	Set(float64)
	// Get gets the value of the gauge.
	Get() float64
}
