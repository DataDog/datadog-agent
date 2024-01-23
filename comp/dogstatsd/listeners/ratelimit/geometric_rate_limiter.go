// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package ratelimit

type geometricRateLimiterConfig struct {
	minRate float64
	maxRate float64
	factor  float64
}

// geometricRateLimiter is a rate limiter where the rate is increased or decreased
// by multiplying or dividing by a constant factor.
// The rate defines how often `keep` returns true. For example for a rate of `0.25`
// the first 3 calls of `keep` return `false` and the 4th call returns `true`.
// The initial value of rate is set to `config.minRate`
type geometricRateLimiter struct {
	tick             int
	currentRateLimit float64
	minRate          float64
	maxRate          float64
	factor           float64
}

func newGeometricRateLimiter(config geometricRateLimiterConfig) *geometricRateLimiter {
	panic("not called")
}

func (r *geometricRateLimiter) keep() bool {
	panic("not called")
}

func (r *geometricRateLimiter) currentRate() float64 {
	panic("not called")
}

func (r *geometricRateLimiter) increaseRate() {
	panic("not called")
}

func (r *geometricRateLimiter) decreaseRate() {
	panic("not called")
}

func (r *geometricRateLimiter) normalizeRate() {
	panic("not called")
}
