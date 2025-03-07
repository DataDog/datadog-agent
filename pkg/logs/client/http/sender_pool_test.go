// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package http

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/client"
)

var defaultMinWorkers = 4
var defaultMaxWorkers = defaultMinWorkers * 10

func defaultPool() *senderPool {
	return newSenderPool(0, ewmaAlpha, defaultMinWorkers, defaultMaxWorkers, targetLatency, client.NewNoopDestinationMetadata())
}

/*func newPool(min int, max int) *senderPool {
	return newSenderPool(0, ewmaAlpha, min, max, targetLatency, client.NewNoopDestinationMetadata())
}
func TestDestinationError(t *testing.T) {
	// Get 429, see worker rollback
	// Get 500, perform no rollback
}*/

func TestWorkerCounts(t *testing.T) {
	scenarios := []struct {
		name                string
		latency             time.Duration
		expectedWorkerCount int
	}{
		{
			name:                "Mininum Workers chosen if latency below target",
			latency:             0,
			expectedWorkerCount: defaultMinWorkers,
		},
		{
			name:                "Reasonable number of workers added at higher than target latency",
			latency:             targetLatency * 2,
			expectedWorkerCount: defaultMinWorkers * 2,
		},
		{
			name:                "Maximum number of workers not exceeded",
			latency:             targetLatency * 20,
			expectedWorkerCount: defaultMaxWorkers,
		},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			pool := defaultPool()

			for i := 0; i < 100; i++ {
				pool.Run(func() destinationResult {
					return destinationResult{latency: s.latency}
				})
			}

			assert.Equal(t, s.expectedWorkerCount, pool.inUseWorkers)
		})
	}
}

/*func TestVirtualLatencyCalculations(t *testing.T) {
	// Condense to 1 number
	// Condense to 2 number
	pool := defaultPool()

	for i := 0; i < 100; i++ {
		pool.Run(func() destinationResult {
			return destinationResult{latency: targetLatency}
		})
	}

	// Should converge on a virtual latency of 10ms since there is only 1 worker, so virtual latency == real latency.
	require.InDelta(t, 10*time.Millisecond, pool.virtualLatency, float64(3*time.Millisecond))
}*/
