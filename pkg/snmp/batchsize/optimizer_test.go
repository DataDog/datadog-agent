// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package batchsize

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Optimizer_OnFailure(t *testing.T) {
	tests := []struct {
		name              string
		optimizer         *Optimizer
		expectedOptimizer *Optimizer
		expectedChanged   bool
	}{
		{
			name: "batch size is 1",
			optimizer: &Optimizer{
				configBatchSize: 4,
				batchSize:       1,
				failuresByBatchSize: map[int]int{
					4: 1,
					2: 1,
				},
				lastSuccessfulBatchSize: 1,
			},
			expectedOptimizer: &Optimizer{
				configBatchSize: 4,
				batchSize:       1,
				failuresByBatchSize: map[int]int{
					4: 1,
					2: 1,
					1: 1,
				},
				lastSuccessfulBatchSize: 1,
			},
			expectedChanged: false,
		},
		{
			name: "new batch size is lesser than the last successful batch size",
			optimizer: &Optimizer{
				configBatchSize:         4,
				batchSize:               4,
				failuresByBatchSize:     map[int]int{},
				lastSuccessfulBatchSize: 3,
			},
			expectedOptimizer: &Optimizer{
				configBatchSize: 4,
				batchSize:       3,
				failuresByBatchSize: map[int]int{
					4: 1,
				},
				lastSuccessfulBatchSize: 3,
			},
			expectedChanged: true,
		},
		{
			name: "batch size is equal to the last successful batch size",
			optimizer: &Optimizer{
				configBatchSize:         4,
				batchSize:               3,
				failuresByBatchSize:     map[int]int{},
				lastSuccessfulBatchSize: 3,
			},
			expectedOptimizer: &Optimizer{
				configBatchSize: 4,
				batchSize:       3 / onFailureDecreaseFactor,
				failuresByBatchSize: map[int]int{
					3: 1,
				},
				lastSuccessfulBatchSize: 3,
			},
			expectedChanged: true,
		},
		{
			name: "batch size is lesser to the last successful batch size",
			optimizer: &Optimizer{
				configBatchSize:         4,
				batchSize:               2,
				failuresByBatchSize:     map[int]int{},
				lastSuccessfulBatchSize: 3,
			},
			expectedOptimizer: &Optimizer{
				configBatchSize: 4,
				batchSize:       2 / onFailureDecreaseFactor,
				failuresByBatchSize: map[int]int{
					2: 1,
				},
				lastSuccessfulBatchSize: 3,
			},
			expectedChanged: true,
		},
		{
			name: "batch size is greater than the last successful batch size",
			optimizer: &Optimizer{
				configBatchSize:         10,
				batchSize:               10,
				failuresByBatchSize:     map[int]int{},
				lastSuccessfulBatchSize: 3,
			},
			expectedOptimizer: &Optimizer{
				configBatchSize: 10,
				batchSize:       10 / onFailureDecreaseFactor,
				failuresByBatchSize: map[int]int{
					10: 1,
				},
				lastSuccessfulBatchSize: 3,
			},
			expectedChanged: true,
		},
		{
			name: "batch size should be decreased",
			optimizer: &Optimizer{
				configBatchSize: 4,
				batchSize:       4,
				failuresByBatchSize: map[int]int{
					4: 1,
				},
				lastSuccessfulBatchSize: 0,
			},
			expectedOptimizer: &Optimizer{
				configBatchSize: 4,
				batchSize:       4 / onFailureDecreaseFactor,
				failuresByBatchSize: map[int]int{
					4: 2,
				},
				lastSuccessfulBatchSize: 0,
			},
			expectedChanged: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changed := tt.optimizer.OnFailure()
			assert.Equal(t, tt.expectedOptimizer, tt.optimizer)
			assert.Equal(t, tt.expectedChanged, changed)
		})
	}
}

func Test_Optimizer_OnSuccess(t *testing.T) {
	tests := []struct {
		name              string
		optimizer         *Optimizer
		expectedOptimizer *Optimizer
	}{
		{
			name: "new batch size has too much failures",
			optimizer: &Optimizer{
				configBatchSize: 10,
				batchSize:       6,
				failuresByBatchSize: map[int]int{
					7: maxFailuresPerWindow + 1,
				},
				lastSuccessfulBatchSize: 5,
			},
			expectedOptimizer: &Optimizer{
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
			optimizer: &Optimizer{
				configBatchSize: 10,
				batchSize:       10,
				failuresByBatchSize: map[int]int{
					6: 1,
				},
				lastSuccessfulBatchSize: 9,
			},
			expectedOptimizer: &Optimizer{
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
			optimizer: &Optimizer{
				configBatchSize: 10,
				batchSize:       6,
				failuresByBatchSize: map[int]int{
					6: 1,
				},
				lastSuccessfulBatchSize: 9,
			},
			expectedOptimizer: &Optimizer{
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
			tt.optimizer.OnSuccess()
			assert.Equal(t, tt.expectedOptimizer, tt.optimizer)
		})
	}
}

func Test_Optimizer_BatchSize(t *testing.T) {
	o := NewOptimizer(5, "test")
	assert.Equal(t, 5, o.BatchSize())
	o.OnFailure()
	assert.Equal(t, 2, o.BatchSize())
}

func Test_Optimizer_RefreshFailures(t *testing.T) {
	o := &Optimizer{
		configBatchSize:     4,
		batchSize:           2,
		failuresByBatchSize: map[int]int{4: 3, 2: 1},
	}
	o.RefreshFailures()
	assert.Equal(t, map[int]int{}, o.failuresByBatchSize)
}
