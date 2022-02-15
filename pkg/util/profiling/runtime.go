// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profiling

import (
	"runtime"
)

var (
	// Go runtime does not provide a method to read current value of block profiling rate,
	// but do need it in a few places: to restore state after collecting flare profile, and
	// to pass to continuous profiler so it doesn't overwrite it with built-in default.
	blockProfileRate int
)

// SetBlockProfileRate sets goroutine blocking profile rate.
func SetBlockProfileRate(rate int) {
	mu.Lock()
	defer mu.Unlock()
	blockProfileRate = rate
	runtime.SetBlockProfileRate(rate)
}

// GetBlockProfileRate returns the current rate of goroutine blocking profile.
func GetBlockProfileRate() int {
	mu.RLock()
	defer mu.RUnlock()
	return blockProfileRate
}

// SetMutexProfileFraction sets fraction of mutexes that generate profiling data.
func SetMutexProfileFraction(fraction int) {
	mu.Lock()
	defer mu.Unlock()
	runtime.SetMutexProfileFraction(fraction)
}

// GetMutexProfileFraction returns the current fraction of mutexes that generate profile data.
func GetMutexProfileFraction() int {
	mu.RLock()
	defer mu.RUnlock()
	return runtime.SetMutexProfileFraction(-1)
}
