// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package syntheticstestschedulerimpl

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
)

func TestNewSchedulerConfigs_Namespace(t *testing.T) {
	tests := []struct {
		name      string
		overrides map[string]interface{}
		expected  string
	}{
		{
			name:      "falls back to network_devices.namespace default",
			overrides: map[string]interface{}{},
			expected:  "default",
		},
		{
			name:      "uses configured network_devices.namespace",
			overrides: map[string]interface{}{"network_devices.namespace": "ndm-ns"},
			expected:  "ndm-ns",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.NewMockWithOverrides(t, tt.overrides)
			configs := newSchedulerConfigs(cfg)
			require.Equal(t, tt.expected, configs.namespace)
		})
	}
}
