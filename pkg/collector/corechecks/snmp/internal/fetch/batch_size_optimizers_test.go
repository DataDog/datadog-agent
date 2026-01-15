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
				lastRefreshTs: now.Add(-failuresWindowDuration / 2),
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
				lastRefreshTs: now.Add(-failuresWindowDuration / 2),
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
				lastRefreshTs: now.Add(-failuresWindowDuration * 2),
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
		batchSizeOptimizer         *oidBatchSizeOptimizer
		expectedBatchSizeOptimizer *oidBatchSizeOptimizer
		expectedBatchSizeChanged   bool
	}{
		{
			name: "batch size is 1",
			batchSizeOptimizer: &oidBatchSizeOptimizer{
				configBatchSize: 4,
				batchSize:       1,
				failuresByBatchSize: map[int]int{
					4: 1,
					2: 1,
				},
				lastSuccessfulBatchSize: 1,
			},
			expectedBatchSizeOptimizer: &oidBatchSizeOptimizer{
				configBatchSize: 4,
				batchSize:       1,
				failuresByBatchSize: map[int]int{
					4: 1,
					2: 1,
					1: 1,
				},
				lastSuccessfulBatchSize: 1,
			},
			expectedBatchSizeChanged: false,
		},
		{
			name: "new batch size is lesser than the last successful batch size",
			batchSizeOptimizer: &oidBatchSizeOptimizer{
				configBatchSize:         4,
				batchSize:               4,
				failuresByBatchSize:     map[int]int{},
				lastSuccessfulBatchSize: 3,
			},
			expectedBatchSizeOptimizer: &oidBatchSizeOptimizer{
				configBatchSize: 4,
				batchSize:       3,
				failuresByBatchSize: map[int]int{
					4: 1,
				},
				lastSuccessfulBatchSize: 3,
			},
			expectedBatchSizeChanged: true,
		},
		{
			name: "batch size is equal to the last successful batch size",
			batchSizeOptimizer: &oidBatchSizeOptimizer{
				configBatchSize:         4,
				batchSize:               3,
				failuresByBatchSize:     map[int]int{},
				lastSuccessfulBatchSize: 3,
			},
			expectedBatchSizeOptimizer: &oidBatchSizeOptimizer{
				configBatchSize: 4,
				batchSize:       3 / onFailureDecreaseFactor,
				failuresByBatchSize: map[int]int{
					3: 1,
				},
				lastSuccessfulBatchSize: 3,
			},
			expectedBatchSizeChanged: true,
		},
		{
			name: "batch size is lesser to the last successful batch size",
			batchSizeOptimizer: &oidBatchSizeOptimizer{
				configBatchSize:         4,
				batchSize:               2,
				failuresByBatchSize:     map[int]int{},
				lastSuccessfulBatchSize: 3,
			},
			expectedBatchSizeOptimizer: &oidBatchSizeOptimizer{
				configBatchSize: 4,
				batchSize:       2 / onFailureDecreaseFactor,
				failuresByBatchSize: map[int]int{
					2: 1,
				},
				lastSuccessfulBatchSize: 3,
			},
			expectedBatchSizeChanged: true,
		},
		{
			name: "batch size is greater than the last successful batch size",
			batchSizeOptimizer: &oidBatchSizeOptimizer{
				configBatchSize:         10,
				batchSize:               10,
				failuresByBatchSize:     map[int]int{},
				lastSuccessfulBatchSize: 3,
			},
			expectedBatchSizeOptimizer: &oidBatchSizeOptimizer{
				configBatchSize: 10,
				batchSize:       10 / onFailureDecreaseFactor,
				failuresByBatchSize: map[int]int{
					10: 1,
				},
				lastSuccessfulBatchSize: 3,
			},
			expectedBatchSizeChanged: true,
		},
		{
			name: "batch size should be decreased",
			batchSizeOptimizer: &oidBatchSizeOptimizer{
				configBatchSize: 4,
				batchSize:       4,
				failuresByBatchSize: map[int]int{
					4: 1,
				},
				lastSuccessfulBatchSize: 0,
			},
			expectedBatchSizeOptimizer: &oidBatchSizeOptimizer{
				configBatchSize: 4,
				batchSize:       4 / onFailureDecreaseFactor,
				failuresByBatchSize: map[int]int{
					4: 2,
				},
				lastSuccessfulBatchSize: 0,
			},
			expectedBatchSizeChanged: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			batchSizeChanged := tt.batchSizeOptimizer.onBatchSizeFailure()
			assert.Equal(t, tt.expectedBatchSizeOptimizer, tt.batchSizeOptimizer)
			assert.Equal(t, tt.expectedBatchSizeChanged, batchSizeChanged)
		})
	}
}

func Test_batchSizeOptimizer_onBatchSizeSuccess(t *testing.T) {
	tests := []struct {
		name                       string
		batchSizeOptimizer         *oidBatchSizeOptimizer
		expectedBatchSizeOptimizer *oidBatchSizeOptimizer
	}{
		{
			name: "new batch size has too much failures",
			batchSizeOptimizer: &oidBatchSizeOptimizer{
				configBatchSize: 10,
				batchSize:       6,
				failuresByBatchSize: map[int]int{
					7: maxFailuresPerWindow + 1,
				},
				lastSuccessfulBatchSize: 5,
			},
			expectedBatchSizeOptimizer: &oidBatchSizeOptimizer{
				configBatchSize: 10,
				batchSize:       6,
				failuresByBatchSize: map[int]int{
					7: maxFailuresPerWindow + 1,
				},
				lastSuccessfulBatchSize: 6,
			},
		},
		{
			name: "batch size is config batch size",
			batchSizeOptimizer: &oidBatchSizeOptimizer{
				configBatchSize: 10,
				batchSize:       10,
				failuresByBatchSize: map[int]int{
					6: 1,
				},
				lastSuccessfulBatchSize: 9,
			},
			expectedBatchSizeOptimizer: &oidBatchSizeOptimizer{
				configBatchSize: 10,
				batchSize:       10,
				failuresByBatchSize: map[int]int{
					6: 1,
				},
				lastSuccessfulBatchSize: 10,
			},
		},
		{
			name: "batch size should be increased",
			batchSizeOptimizer: &oidBatchSizeOptimizer{
				configBatchSize: 10,
				batchSize:       6,
				failuresByBatchSize: map[int]int{
					6: 1,
				},
				lastSuccessfulBatchSize: 9,
			},
			expectedBatchSizeOptimizer: &oidBatchSizeOptimizer{
				configBatchSize: 10,
				batchSize:       6 + onSuccessIncreaseValue,
				failuresByBatchSize: map[int]int{
					6: 1,
				},
				lastSuccessfulBatchSize: 6,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.batchSizeOptimizer.onBatchSizeSuccess()
			assert.Equal(t, tt.expectedBatchSizeOptimizer, tt.batchSizeOptimizer)
		})
	}
}
