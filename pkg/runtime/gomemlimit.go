// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runtime

import (
	"context"
	"time"
)

// MemoryLimiter allows to set GOMEMLIMIT based on different scenarios
type MemoryLimiter interface {
	Run(ctx context.Context) error
}

// MemoryLimiterArgs are the arguments to create a MemoryLimiter
type MemoryLimiterArgs struct {
	// LimitPct is the percentage of the memory limit (or MaxMemory) to set as GOMEMLIMIT (0.0 - 1.0)
	LimitPct float64
	// MaxMemory allows to provide a custom memory limit without relying cgroups (bytes)
	MaxMemory uint64
	// Interval is the interval at which the memory limiter will check the memory usage (only for dynamic memory limiter)
	Interval time.Duration
}
