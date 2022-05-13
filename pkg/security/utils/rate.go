// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package utils

import (
	"time"
)

// RateLimiter defines a rate limiter
type RateLimiter struct {
	Rate  uint64
	Burst int

	bucketSize int
	nextBucket uint64
}

// NewRateLimiter return a new RateLimiter
func NewRateLimiter(rate time.Duration, burst int) *RateLimiter {
	return &RateLimiter{
		Rate:  uint64(rate.Nanoseconds()),
		Burst: burst,
	}
}

// Allow return whether allowed or not
func (r *RateLimiter) Allow(ns uint64) bool {
	if r.bucketSize < r.Burst {
		r.bucketSize++

		if r.nextBucket == 0 {
			r.nextBucket = ns + r.Rate
		}
		return true
	} else if ns > r.nextBucket {
		r.bucketSize = 1
		r.nextBucket = ns + r.Rate

		return true
	}

	return false
}
