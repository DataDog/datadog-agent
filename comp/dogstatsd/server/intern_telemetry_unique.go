// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build go1.23

package server

// For the unique.Handle implementation, we override specific telemetry methods
// that behave differently. The Reset method becomes a no-op since unique.Handle
// manages memory automatically, and Miss doesn't track size/bytes since we can't
// access the internal cache state.

// Reset is a no-op for unique.Handle implementation as it manages memory automatically.
func (si *stringInternerInstanceTelemetry) Reset(_ int) {
	// No-op: unique.Handle manages memory automatically via GC
	// We cannot track resets since there are no manual resets with unique.Handle
}

// Miss increments the miss counter and observes string length.
// Note: We cannot track size or bytes with unique.Handle since the internal
// cache is not accessible.
func (si *stringInternerInstanceTelemetry) Miss(length int) {
	if si.enabled {
		si.miss.Inc()
		// We can still track string length distribution
		si.globaltlmSIRStrBytes.Observe(float64(length))
		// Note: We cannot update size or bytes gauges as unique.Handle
		// doesn't expose internal cache information
	}
}