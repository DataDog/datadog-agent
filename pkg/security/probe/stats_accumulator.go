// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

// StatsAccumulator calculates the online statistics based on Welford's online algorithm:
// https://en.wikipedia.org/wiki/Algorithms_for_calculating_variance
type StatsAccumulator struct {
	count uint64
	mean  float64
	m2    float64
	max   float64
}

// Update adds a value to the accumulator
func (a *StatsAccumulator) Update(val float64) {
	a.count++
	delta := val - a.mean
	a.mean += delta / float64(a.count)
	delta2 := val - a.mean
	a.m2 += delta * delta2
	if val > a.max {
		a.max = val
	}
}

// Finalize returns the mean, variance and maximum
func (a *StatsAccumulator) Finalize() (float64, float64, float64) {
	return a.mean, a.m2 / float64(a.count), a.max
}
