// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package fetch

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_batchSizeOptimizers_refreshIfOutdated(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name                        string
		batchSizeOptimizers         *OidBatchSizeOptimizers
		expectedBatchSizeOptimizers *OidBatchSizeOptimizers
	}{
		{
			name: "batch size is not outdated",
			batchSizeOptimizers: &OidBatchSizeOptimizers{
				snmpGetOptimizer: &oidBatchSizeOptimizer{
					configBatchSize:     10,
					batchSize:           6,
					failuresByBatchSize: map[int]int{1: 2, 6: 10},
				},
				snmpGetBulkOptimizer: &oidBatchSizeOptimizer{
					configBatchSize:     2,
					batchSize:           1,
					failuresByBatchSize: map[int]int{},
				},
				snmpGetNextOptimizer: &oidBatchSizeOptimizer{
					configBatchSize:     12,
					batchSize:           2,
					failuresByBatchSize: map[int]int{10: 6, 12: 10},
				},
				lastRefreshTs: now.Add(-failuresTimeInterval / 2),
			},
			expectedBatchSizeOptimizers: &OidBatchSizeOptimizers{
				snmpGetOptimizer: &oidBatchSizeOptimizer{
					configBatchSize:     10,
					batchSize:           6,
					failuresByBatchSize: map[int]int{1: 2, 6: 10},
				},
				snmpGetBulkOptimizer: &oidBatchSizeOptimizer{
					configBatchSize:     2,
					batchSize:           1,
					failuresByBatchSize: map[int]int{},
				},
				snmpGetNextOptimizer: &oidBatchSizeOptimizer{
					configBatchSize:     12,
					batchSize:           2,
					failuresByBatchSize: map[int]int{10: 6, 12: 10},
				},
				lastRefreshTs: now.Add(-failuresTimeInterval / 2),
			},
		},
		{
			name: "batch size is outdated",
			batchSizeOptimizers: &OidBatchSizeOptimizers{
				snmpGetOptimizer: &oidBatchSizeOptimizer{
					configBatchSize:     10,
					batchSize:           6,
					failuresByBatchSize: map[int]int{1: 2, 6: 10},
				},
				snmpGetBulkOptimizer: &oidBatchSizeOptimizer{
					configBatchSize:     2,
					batchSize:           1,
					failuresByBatchSize: map[int]int{},
				},
				snmpGetNextOptimizer: &oidBatchSizeOptimizer{
					configBatchSize:     12,
					batchSize:           2,
					failuresByBatchSize: map[int]int{10: 6, 12: 10},
				},
				lastRefreshTs: now.Add(-failuresTimeInterval * 2),
			},
			expectedBatchSizeOptimizers: &OidBatchSizeOptimizers{
				snmpGetOptimizer: &oidBatchSizeOptimizer{
					configBatchSize:     10,
					batchSize:           6,
					failuresByBatchSize: map[int]int{},
				},
				snmpGetBulkOptimizer: &oidBatchSizeOptimizer{
					configBatchSize:     2,
					batchSize:           1,
					failuresByBatchSize: map[int]int{},
				},
				snmpGetNextOptimizer: &oidBatchSizeOptimizer{
					configBatchSize:     12,
					batchSize:           2,
					failuresByBatchSize: map[int]int{},
				},
				lastRefreshTs: now,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.batchSizeOptimizers.refreshIfOutdated(now)
			assert.Equal(t, tt.expectedBatchSizeOptimizers, tt.batchSizeOptimizers)
		})
	}
}

func Test_batchSizeOptimizer_onBatchSizeFailure(t *testing.T) {
	tests := []struct {
		name                       string
		configBatchSize            int
		batchSize                  int
		failuresByBatchSize        map[int]int
		expectedBatchSizeOptimizer *oidBatchSizeOptimizer
		expectedBatchSizeChanged   bool
	}{
		{
			name:            "batch size is 1",
			configBatchSize: 4,
			batchSize:       1,
			failuresByBatchSize: map[int]int{
				4: 1,
				2: 1,
			},
			expectedBatchSizeOptimizer: &oidBatchSizeOptimizer{
				configBatchSize: 4,
				batchSize:       1,
				failuresByBatchSize: map[int]int{
					4: 1,
					2: 1,
					1: 1,
				},
			},
			expectedBatchSizeChanged: false,
		},
		{
			name:            "batch size should be decreased",
			configBatchSize: 4,
			batchSize:       4,
			failuresByBatchSize: map[int]int{
				4: 1,
			},
			expectedBatchSizeOptimizer: &oidBatchSizeOptimizer{
				configBatchSize: 4,
				batchSize:       4 / decreaseFactor,
				failuresByBatchSize: map[int]int{
					4: 2,
				},
			},
			expectedBatchSizeChanged: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			batchSizeOptimizer := &oidBatchSizeOptimizer{
				configBatchSize:     tt.configBatchSize,
				batchSize:           tt.batchSize,
				failuresByBatchSize: tt.failuresByBatchSize,
			}
			batchSizeChanged := batchSizeOptimizer.onBatchSizeFailure()
			assert.Equal(t, tt.expectedBatchSizeOptimizer, batchSizeOptimizer)
			assert.Equal(t, tt.expectedBatchSizeChanged, batchSizeChanged)
		})
	}
}

func Test_batchSizeOptimizer_onBatchSizeSuccess(t *testing.T) {
	tests := []struct {
		name                       string
		configBatchSize            int
		batchSize                  int
		failuresByBatchSize        map[int]int
		expectedBatchSizeOptimizer *oidBatchSizeOptimizer
	}{
		{
			name:            "new batch size has too much failures",
			configBatchSize: 10,
			batchSize:       6,
			failuresByBatchSize: map[int]int{
				7: maxFailuresPerTimeInterval + 1,
			},
			expectedBatchSizeOptimizer: &oidBatchSizeOptimizer{
				configBatchSize: 10,
				batchSize:       6,
				failuresByBatchSize: map[int]int{
					7: maxFailuresPerTimeInterval + 1,
				},
			},
		},
		{
			name:            "batch size is config batch size",
			configBatchSize: 10,
			batchSize:       10,
			failuresByBatchSize: map[int]int{
				6: 1,
			},
			expectedBatchSizeOptimizer: &oidBatchSizeOptimizer{
				configBatchSize: 10,
				batchSize:       10,
				failuresByBatchSize: map[int]int{
					6: 1,
				},
			},
		},
		{
			name:            "batch size should be increased",
			configBatchSize: 10,
			batchSize:       6,
			failuresByBatchSize: map[int]int{
				6: 1,
			},
			expectedBatchSizeOptimizer: &oidBatchSizeOptimizer{
				configBatchSize: 10,
				batchSize:       6 + increaseValue,
				failuresByBatchSize: map[int]int{
					6: 1,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			batchSizeOptimizer := &oidBatchSizeOptimizer{
				configBatchSize:     tt.configBatchSize,
				batchSize:           tt.batchSize,
				failuresByBatchSize: tt.failuresByBatchSize,
			}
			batchSizeOptimizer.onBatchSizeSuccess()
			assert.Equal(t, tt.expectedBatchSizeOptimizer, batchSizeOptimizer)
		})
	}
}
