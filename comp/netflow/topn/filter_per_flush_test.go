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
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestPerFlushFilter(t *testing.T) {
	type TestCase struct {
		name   string
		inputs []*common.Flow
		expect []*common.Flow
		ctx    common.FlushContext
	}
	// this will be effectively 00:00 for tests. Because we truncate to 5 minutes, it may who different values, but all the
	// math is relative to each other so it should be OK.
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

	testCases := []TestCase{
		{
			name: "it should sort before filtering",
			ctx: common.FlushContext{
				FlushTime:     timeAt(12_00),
				LastFlushedAt: timeAt(11_59),
				NumFlushes:    1,
			},
			inputs: sampleFlows(4, 2, 1, 3),
			expect: sampleFlows(4, 3),
		},
		{
			name: "it should handle empty slices",
			ctx: common.FlushContext{
				FlushTime:     timeAt(12_00),
				LastFlushedAt: timeAt(11_59),
				NumFlushes:    1,
			},
			inputs: []*common.Flow{},
			expect: []*common.Flow{},
		},
		{
			name: "it should not sort when n and list length are the same",
			ctx: common.FlushContext{
				FlushTime:     timeAt(12_00),
				LastFlushedAt: timeAt(11_59),
				NumFlushes:    1,
			},
			inputs: sampleFlows(100, 200),
			expect: sampleFlows(100, 200),
		},
		{
			name: "it should not sort the result if n is greater than the total number of flows",
			ctx: common.FlushContext{
				FlushTime:     timeAt(12_05),
				LastFlushedAt: timeAt(12_00),
				NumFlushes:    5,
			},
			inputs: sampleFlows(100, 200, 300),
			expect: sampleFlows(100, 200, 300),
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			metrics := mocksender.NewMockSender("")
			metrics.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
			metrics.On("Count", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
			metrics.On("Histogram", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

			filter := NewPerFlushFilter(120, common.FlushConfig{
				// 60 buckets, make tests easy to set up + run. 2 per tick
				FlowCollectionDuration: 1 * time.Hour,
				FlushTickFrequency:     1 * time.Minute,
			}, metrics, logmock.New(t))

			outputs := filter.Filter(test.ctx, test.inputs)

			assert.EqualValues(t, outputs, test.expect)
		})
	}
}

func sampleFlows(numBytes ...uint64) []*common.Flow {
	flows := make([]*common.Flow, len(numBytes))
	for idx, bytes := range numBytes {
		flows[idx] = &common.Flow{
			Bytes: bytes,
		}
	}
	return flows
}
