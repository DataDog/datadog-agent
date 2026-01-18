// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package flowaggregator

import (
	"math/rand"
	"time"

	"github.com/DataDog/datadog-agent/comp/netflow/common"
)

// FlowScheduler is responsible for determining when flows should be flushed
type FlowScheduler interface {
	// ScheduleNewFlowFlush returns the time when a flow should next be flushed
	ScheduleNewFlowFlush(currentTime time.Time) time.Time
	RefreshFlushTime(flow flowContext) time.Time
}

// JitterFlowScheduler implements a scheduler that adds random jitter to flush times
// This is used in production to distribute flow flushes over time
type JitterFlowScheduler struct {
	flushConfig common.FlushConfig
}

// ScheduleNewFlowFlush assigns the flush time for a given flow. It will choose a random time between currentTime
// and currentTime + FlowCollectionDuration.
func (s JitterFlowScheduler) ScheduleNewFlowFlush(currentTime time.Time) time.Time {
	jitter := time.Duration(rand.Intn(int(s.flushConfig.FlowCollectionDuration)))
	return currentTime.Add(jitter)
}

// RefreshFlushTime updates the flush time for a given flow. It will add FlowCollectionDuration to the current flush time.
// There is no jitter after a flow is first assigned a time.
func (s JitterFlowScheduler) RefreshFlushTime(flow flowContext) time.Time {
	return flow.nextFlush.Add(s.flushConfig.FlowCollectionDuration)
}

// ImmediateFlowScheduler implements a scheduler that prefers to flush as soon as possible
// This is primarily used in tests for deterministic behavior
type ImmediateFlowScheduler struct {
	flushConfig common.FlushConfig
}

// ScheduleNewFlowFlush implements FlowScheduler interface with immediate bias
func (s ImmediateFlowScheduler) ScheduleNewFlowFlush(currentTime time.Time) time.Time {
	// For testing, we prefer to flush flows as soon as possible
	// This ensures predictable behavior in test cases
	return currentTime
}

// RefreshFlushTime updates the flush time for a given flow. It will add FlowCollectionDuration to the current flush time.
func (s ImmediateFlowScheduler) RefreshFlushTime(flow flowContext) time.Time {
	return flow.nextFlush.Add(s.flushConfig.FlowCollectionDuration)
}
