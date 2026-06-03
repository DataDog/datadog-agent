// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultscript

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/common"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/config"
)

func TestSetConfigProcessAgent(t *testing.T) {
	tests := []struct {
		name                string
		env                 map[string]string
		processCollection   *bool
		containerCollection *bool
		processDiscovery    *bool
		expectErr           bool
	}{
		{
			name: "unset leaves all nil",
			env:  map[string]string{},
		},
		{
			name:              "process collection enabled",
			env:               map[string]string{"DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED": "true"},
			processCollection: config.BoolToPtr(true),
		},
		{
			name:              "process collection enabled with 1",
			env:               map[string]string{"DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED": "1"},
			processCollection: config.BoolToPtr(true),
		},
		{
			name:              "process collection disabled with 0",
			env:               map[string]string{"DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED": "0"},
			processCollection: config.BoolToPtr(false),
		},
		{
			name:              "process collection explicitly disabled",
			env:               map[string]string{"DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED": "false"},
			processCollection: config.BoolToPtr(false),
		},
		{
			name:      "invalid boolean value is rejected",
			env:       map[string]string{"DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED": "yes"},
			expectErr: true,
		},
		{
			name:                "container collection via process_agent alias",
			env:                 map[string]string{"DD_PROCESS_AGENT_CONTAINER_COLLECTION_ENABLED": "true"},
			containerCollection: config.BoolToPtr(true),
		},
		{
			name:             "process discovery disabled",
			env:              map[string]string{"DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED": "false"},
			processDiscovery: config.BoolToPtr(false),
		},
		{
			name: "canonical name wins over alias",
			env: map[string]string{
				"DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED": "true",
				"DD_PROCESS_AGENT_PROCESS_COLLECTION_ENABLED":  "false",
			},
			processCollection: config.BoolToPtr(true),
		},
		{
			name: "all three set",
			env: map[string]string{
				"DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED":   "true",
				"DD_PROCESS_CONFIG_CONTAINER_COLLECTION_ENABLED": "false",
				"DD_PROCESS_CONFIG_PROCESS_DISCOVERY_ENABLED":    "false",
			},
			processCollection:   config.BoolToPtr(true),
			containerCollection: config.BoolToPtr(false),
			processDiscovery:    config.BoolToPtr(false),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}

			s := &common.Setup{}
			err := setConfigProcessAgent(s)
			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			pc := s.Config.DatadogYAML.ProcessConfig
			assert.Equal(t, tc.processCollection, pc.ProcessCollection.Enabled, "process_collection.enabled")
			assert.Equal(t, tc.containerCollection, pc.ContainerCollection.Enabled, "container_collection.enabled")
			assert.Equal(t, tc.processDiscovery, pc.ProcessDiscovery.Enabled, "process_discovery.enabled")
		})
	}
}
