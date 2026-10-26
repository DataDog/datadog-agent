// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package kubeapiserver

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
)

func TestShouldHaveAutoscalerStores(t *testing.T) {
	tests := []struct {
		name     string
		cfg      map[string]interface{}
		expected bool
	}{
		{
			name:     "disabled by default",
			cfg:      map[string]interface{}{},
			expected: false,
		},
		{
			name: "enabled",
			cfg: map[string]interface{}{
				"cluster_agent.autoscaler_tags.enabled": true,
			},
			expected: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := config.NewMockWithOverrides(t, test.cfg)
			assert.Equal(t, test.expected, shouldHaveAutoscalerStores(cfg))
		})
	}
}
