// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package fetch

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/snmp/batchsize"
)

func Test_batchSizeOptimizers_refreshIfOutdated(t *testing.T) {
	now := time.Now()

	makeOptimizers := func(refreshTs time.Time, getFailures, getBulkFailures, getNextFailures map[int]int) *OidBatchSizeOptimizers {
		return &OidBatchSizeOptimizers{
			snmpGetOptimizer:     batchsize.NewOptimizerForTest(10, 6, getFailures, 0),
			snmpGetBulkOptimizer: batchsize.NewOptimizerForTest(2, 1, getBulkFailures, 0),
			snmpGetNextOptimizer: batchsize.NewOptimizerForTest(12, 2, getNextFailures, 0),
			lastRefreshTs:        refreshTs,
		}
	}

	tests := []struct {
		name     string
		input    *OidBatchSizeOptimizers
		expected *OidBatchSizeOptimizers
	}{
		{
			name: "batch size is not outdated",
			input: makeOptimizers(
				now.Add(-failuresWindowDuration/2),
				map[int]int{1: 2, 6: 10},
				map[int]int{},
				map[int]int{10: 6, 12: 10},
			),
			expected: makeOptimizers(
				now.Add(-failuresWindowDuration/2),
				map[int]int{1: 2, 6: 10},
				map[int]int{},
				map[int]int{10: 6, 12: 10},
			),
		},
		{
			name: "batch size is outdated",
			input: makeOptimizers(
				now.Add(-failuresWindowDuration*2),
				map[int]int{1: 2, 6: 10},
				map[int]int{},
				map[int]int{10: 6, 12: 10},
			),
			expected: makeOptimizers(
				now,
				map[int]int{},
				map[int]int{},
				map[int]int{},
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.input.refreshIfOutdated(now)
			assert.Equal(t, tt.expected, tt.input)
		})
	}
}
