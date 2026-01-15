// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package flowaggregator

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/netflow/common"
	"github.com/stretchr/testify/assert"
)

func TestFlowScheduler_Immediate(t *testing.T) {
	sched := ImmediateFlowScheduler{
		flushConfig: common.FlushConfig{
			FlowCollectionDuration: 5 * time.Minute,
			FlushTickFrequency:     10 * time.Second,
		},
	}
	initialTime, _ := time.Parse("2006-01-02 15:04:05", "2000-01-01 00:00:00")

	t.Run("it schedules work based on the incoming timestamp", func(t *testing.T) {
		assert.Equal(t, sched.ScheduleNewFlowFlush(initialTime), initialTime)
	})

	t.Run("it adds FlowCollectionDuration when refreshing a flow", func(t *testing.T) {
		expectedTime, _ := time.Parse("2006-01-02 15:04:05", "2000-01-01 00:05:00")

		assert.Equal(t, sched.RefreshFlushTime(flowContext{
			nextFlush: initialTime,
		}), expectedTime)
	})
}

func TestFlowScheduler_Jitterer(t *testing.T) {
	sched := JitterFlowScheduler{
		flushConfig: common.FlushConfig{
			FlowCollectionDuration: 5 * time.Minute,
			FlushTickFrequency:     10 * time.Second,
		},
	}
	initialTime, _ := time.Parse("2006-01-02 15:04:05", "2000-01-01 00:00:00")

	t.Run("it schedules work based on the incoming timestamp", func(t *testing.T) {
		for range 5 {
			assert.WithinRange(t, sched.ScheduleNewFlowFlush(initialTime), initialTime, initialTime.Add(5*time.Minute))
		}
	})

	t.Run("it adds FlowCollectionDuration when refreshing a flow, no more jitter", func(t *testing.T) {
		expectedTime, _ := time.Parse("2006-01-02 15:04:05", "2000-01-01 00:05:00")

		assert.Equal(t, sched.RefreshFlushTime(flowContext{
			nextFlush: initialTime,
		}), expectedTime)
	})
}
