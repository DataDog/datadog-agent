// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package topn

import (
	"testing"
	"time"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/netflow/common"
	"github.com/stretchr/testify/assert"
)

func TestTopNScheduling(t *testing.T) {
	config := newThrottler(630, common.FlushConfig{
		// 60 buckets, make tests easier to set up + run
		FlowCollectionDuration: 1 * time.Hour,
		FlushTickFrequency:     1 * time.Minute,
	}, logmock.New(t))
	// this will be effectively 00:00 for tests
	frameOfReference, _ := time.Parse(time.RFC3339, "2025-11-01T12:00:00Z")
	frameOfReference = frameOfReference.Truncate(5 * time.Minute).UTC()

	timeAt := func(hhmm int) time.Time {
		hours := hhmm / 100
		minutes := hhmm % 100
		if minutes >= 60 {
			panic("illegal test parameters, hhmm should be military time, e.g. 00_59, 23_59, ...")
		}
		if hours >= 24 {
			panic("illegal test parameters, hhmm should be military time, e.g. 00_59, 23_59, ...")
		}

		return frameOfReference.Add(time.Duration(hours) * time.Hour).Add(time.Duration(minutes) * time.Minute)
	}

	tests := []struct {
		name   string
		ctx    common.FlushContext
		expect int
	}{
		{
			name: "the first bucket should have an extra",
			ctx: common.FlushContext{
				FlushTime:     timeAt(2_00),
				LastFlushedAt: timeAt(1_59),
				NumFlushes:    1,
			},
			expect: 11,
		},
		{
			name: "the 30th bucket should have an extra",
			ctx: common.FlushContext{
				FlushTime:     timeAt(2_29),
				LastFlushedAt: timeAt(2_28),
				NumFlushes:    1,
			},
			expect: 11,
		},
		{
			name: "the 31st bucket should not have an extra",
			ctx: common.FlushContext{
				FlushTime:     timeAt(2_30),
				LastFlushedAt: timeAt(2_29),
				NumFlushes:    1,
			},
			expect: 10,
		},
		{
			name: "handles when flushes occur over the reset period",
			ctx: common.FlushContext{
				FlushTime:     timeAt(2_01),
				LastFlushedAt: timeAt(1_58),
				NumFlushes:    3,
			},
			expect: 32, // 1:59 = 10 rows, 2:00 = 11 rows, 2:01 = 11 rows
		},
		{
			name: "it behaves weirdly when num flushes passed in by caller is 0 (should we derive this ourselves instead?)",
			ctx: common.FlushContext{
				FlushTime:     timeAt(2_01),
				LastFlushedAt: timeAt(1_58),
				NumFlushes:    0,
			},
			expect: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := config.GetNumRowsToFlushFor(test.ctx)
			assert.Equal(t, test.expect, result)
		})
	}
}
