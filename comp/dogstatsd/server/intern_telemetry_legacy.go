// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !go1.23

package server

// Reset increments the reset counter and updates the size and bytes gauges.
func (si *stringInternerInstanceTelemetry) Reset(length int) {
	if si.enabled {
		si.resets.Inc()
		si.bytes.Sub(float64(si.curBytes))
		si.size.Sub(float64(length))
		si.curBytes = 0
	}
}

// Miss increments the miss counter and updates the size and bytes gauges.
func (si *stringInternerInstanceTelemetry) Miss(length int) {
	if si.enabled {
		si.miss.Inc()
		si.size.Inc()
		si.bytes.Add(float64(length))
		si.globaltlmSIRStrBytes.Observe(float64(length))
		si.curBytes += length
	}
}