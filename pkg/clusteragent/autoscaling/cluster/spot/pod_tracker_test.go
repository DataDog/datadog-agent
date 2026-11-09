// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOwnerPodSetExcess(t *testing.T) {
	tests := []struct {
		name                               string
		percentage, minOnDemand            int
		spot, onDemand                     int
		wantExcessSpot, wantExcessOnDemand int
	}{
		{
			name:       "converged: 50% spot with both sides matching",
			percentage: 50, minOnDemand: 0,
			spot: 5, onDemand: 5,
		},
		{
			name:       "minOnDemand deficit: spot pods are excess by the deficit",
			percentage: 100, minOnDemand: 2,
			spot: 5, onDemand: 0,
			wantExcessSpot: 2,
		},
		{
			name:       "minOnDemand clamps spot target: 100% requested but minOnDemand=1 forces 1 on-demand",
			percentage: 100, minOnDemand: 1,
			spot: 3, onDemand: 0,
			wantExcessSpot: 1,
		},
		{
			name:       "spot above percentage target: evict spot",
			percentage: 50, minOnDemand: 0,
			spot: 7, onDemand: 3,
			wantExcessSpot: 2,
		},
		{
			name:       "on-demand above target with minOnDemand satisfied: evict on-demand",
			percentage: 50, minOnDemand: 1,
			spot: 3, onDemand: 7,
			wantExcessOnDemand: 2,
		},
		{
			name:       "empty workload: no excess",
			percentage: 100, minOnDemand: 0,
		},
		{
			name:       "below minOnDemand with no spot pods: no excess (rebalancer can't fix)",
			percentage: 100, minOnDemand: 5,
			spot: 0, onDemand: 2,
			wantExcessSpot: 3, // minOnDemand-onDemand; rebalancer would still try to evict spot, but there are none
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := &ownerPodSet{
				config:       workloadSpotConfig{percentage: tt.percentage, minOnDemand: tt.minOnDemand},
				spotUIDs:     make(map[string]podInfo),
				onDemandUIDs: make(map[string]podInfo),
			}
			for i := range tt.spot {
				ps.spotUIDs["s"+strconv.Itoa(i)] = podInfo{}
			}
			for i := range tt.onDemand {
				ps.onDemandUIDs["o"+strconv.Itoa(i)] = podInfo{}
			}
			gotSpot, gotOnDemand := ps.excess()
			assert.Equal(t, tt.wantExcessSpot, gotSpot, "excess spot")
			assert.Equal(t, tt.wantExcessOnDemand, gotOnDemand, "excess on-demand")
			// At most one capacity type can have excess at any time.
			assert.False(t, gotSpot > 0 && gotOnDemand > 0, "excess should be unidirectional")
		})
	}
}
