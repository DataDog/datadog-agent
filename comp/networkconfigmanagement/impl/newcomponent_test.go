// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ncm

package networkconfigmanagementimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
)

func TestResolveConfigsPerDeviceLimits(t *testing.T) {
	tests := []struct {
		name    string
		min     int
		max     int
		wantMin int
		wantMax int
	}{
		{
			name:    "within bounds, no adjustment",
			min:     3,
			max:     50,
			wantMin: 3,
			wantMax: 50,
		},
		{
			name:    "min below floor is raised to floor",
			min:     0,
			max:     50,
			wantMin: minConfigsPerDeviceFloor,
			wantMax: 50,
		},
		{
			name:    "min above max is lowered to max",
			min:     5,
			max:     2,
			wantMin: 2,
			wantMax: 2,
		},
		{
			name:    "max below floor is raised to floor",
			min:     3,
			max:     0,
			wantMin: minConfigsPerDeviceFloor,
			wantMax: minConfigsPerDeviceFloor,
		},
		{
			name:    "min above max is lowered to max, after max is floored",
			min:     5,
			max:     1,
			wantMin: minConfigsPerDeviceFloor,
			wantMax: minConfigsPerDeviceFloor,
		},
		{
			name:    "min and max equal, no adjustment",
			min:     4,
			max:     4,
			wantMin: 4,
			wantMax: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logmock.New(t)
			gotMin, gotMax := resolveConfigsPerDeviceLimits(tt.min, tt.max, logger)
			assert.Equal(t, tt.wantMin, gotMin)
			assert.Equal(t, tt.wantMax, gotMax)
		})
	}
}
