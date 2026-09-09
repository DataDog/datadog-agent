// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package eks

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewParamsAutoModeRejectsNodeGroups(t *testing.T) {
	tests := []struct {
		name         string
		options      []Option
		wantContains string
	}{
		{
			name:         "windows node group",
			options:      []Option{WithAutoMode(), WithWindowsNodeGroup()},
			wantContains: "WithWindowsNodeGroup",
		},
		{
			name:         "gpu node group",
			options:      []Option{WithAutoMode(), WithGPUNodeGroup("")},
			wantContains: "WithGPUNodeGroup",
		},
		{
			name:         "linux node group, option order reversed",
			options:      []Option{WithLinuxNodeGroup(), WithAutoMode()},
			wantContains: "WithLinuxNodeGroup",
		},
		{
			name:         "every node group is reported",
			options:      []Option{WithAutoMode(), WithLinuxARMNodeGroup(), WithBottlerocketNodeGroup()},
			wantContains: "WithLinuxARMNodeGroup, WithBottlerocketNodeGroup",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, err := NewParams(tt.options...)
			require.Error(t, err)
			assert.Nil(t, params)
			assert.Contains(t, err.Error(), tt.wantContains)
		})
	}
}

func TestNewParamsAutoModeAlone(t *testing.T) {
	params, err := NewParams(WithAutoMode())
	require.NoError(t, err)
	assert.True(t, params.AutoMode)
	// Auto Mode owns the data plane, so no managed node group is requested.
	assert.False(t, params.LinuxNodeGroup)
	assert.False(t, params.LinuxARMNodeGroup)
	assert.False(t, params.BottleRocketNodeGroup)
	assert.False(t, params.WindowsNodeGroup)
	assert.False(t, params.GPUNodeGroup)
}

func TestNewParamsNodeGroupsWithoutAutoMode(t *testing.T) {
	params, err := NewParams(WithLinuxNodeGroup(), WithWindowsNodeGroup())
	require.NoError(t, err)
	assert.False(t, params.AutoMode)
	assert.True(t, params.LinuxNodeGroup)
	assert.True(t, params.WindowsNodeGroup)
}
