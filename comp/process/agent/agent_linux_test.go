// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/process/types"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

func TestEnabledHelper(t *testing.T) {
	tests := []struct {
		name        string
		agentFlavor string
		isCLCRunner bool
		checks      []types.CheckComponent
		expected    bool
	}{
		{
			name:        "process agent with connections check enabled",
			agentFlavor: flavor.ProcessAgent,
			checks: []types.CheckComponent{
				types.NewMockCheckComponent(t, checks.ConnectionsCheckName, true),
				types.NewMockCheckComponent(t, checks.ProcessCheckName, true),
			},
			expected: true,
		},
		{
			name:        "process agent with connections check disabled",
			agentFlavor: flavor.ProcessAgent,
			checks: []types.CheckComponent{
				types.NewMockCheckComponent(t, checks.ProcessCheckName, true),
			},
			expected: false,
		},
		{
			name:        "default agent is always enabled",
			agentFlavor: flavor.DefaultAgent,
			expected:    true,
		},
		{
			name:        "CLC runner should always return false",
			agentFlavor: flavor.DefaultAgent,
			isCLCRunner: true,
			checks: []types.CheckComponent{
				types.NewMockCheckComponent(t, checks.ConnectionsCheckName, true),
				types.NewMockCheckComponent(t, checks.ProcessCheckName, true),
			},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Set agent flavor for this test
			originalFlavor := flavor.GetFlavor()
			flavor.SetFlavor(tc.agentFlavor)
			defer func() {
				flavor.SetFlavor(originalFlavor)
			}()

			// Create mock config
			mockConfig := configmock.New(t)
			mockConfig.SetInTest("clc_runner_enabled", tc.isCLCRunner)

			if tc.isCLCRunner {
				// Add the clusterchecks config provider to the config
				mockConfig.SetInTest("config_providers", []map[string]interface{}{{"name": "clusterchecks"}})

			}

			// Call the function under test and assert the result
			result := enabledHelper(mockConfig, tc.checks, logmock.New(t))
			assert.Equal(t, tc.expected, result)
		})
	}
}
