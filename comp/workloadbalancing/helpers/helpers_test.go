// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package workloadbalancinghelpers

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
)

func TestIsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		configs  map[string]interface{}
		expected bool
	}{
		{
			name:     "not set",
			configs:  map[string]interface{}{},
			expected: false,
		},
		{
			name:     "disabled",
			configs:  map[string]interface{}{"agent_workload_balancing.enabled": false},
			expected: false,
		},
		{
			name:     "enabled",
			configs:  map[string]interface{}{"agent_workload_balancing.enabled": true},
			expected: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsEnabled(config.NewMockWithOverrides(t, tt.configs)))
		})
	}
}
