// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package render

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormat(t *testing.T) {
	originalTZ := os.Getenv("TZ")
	os.Setenv("TZ", "UTC")
	defer func() {
		os.Setenv("TZ", originalTZ)
	}()

	const statusRenderErrors = "Status render errors"

	tests := []struct {
		name           string
		formatFunction func([]byte) (string, error)
		jsonFile       string
		resultFile     string
	}{
		{
			name:           "Core status",
			formatFunction: FormatStatus,
			jsonFile:       "fixtures/agent_status.json",
			resultFile:     "fixtures/agent_status.text",
		},
		{
			name:           "Cluster Agent Status",
			formatFunction: FormatDCAStatus,
			jsonFile:       "fixtures/cluster_agent_status.json",
			resultFile:     "fixtures/cluster_agent_status.text",
		},
		{
			name:           "Security Agent Status",
			formatFunction: FormatSecurityAgentStatus,
			jsonFile:       "fixtures/security_agent_status.json",
			resultFile:     "fixtures/security_agent_status.text",
		},
		{
			name:           "Process Agent Status",
			formatFunction: FormatProcessAgentStatus,
			jsonFile:       "fixtures/process_agent_status.json",
			resultFile:     "fixtures/process_agent_status.text",
		},
		{
			name:           "Check Stats",
			formatFunction: FormatCheckStats,
			jsonFile:       "fixtures/check_stats.json",
			resultFile:     "fixtures/check_stats.text",
		},
	}

	for _, tt := range tests {
		jsonBytes, err := os.ReadFile(tt.jsonFile)
		require.NoError(t, err)
		expectedOutput, err := os.ReadFile(tt.resultFile)
		require.NoError(t, err)

		t.Run(fmt.Sprintf("%s: render errors", tt.name), func(t *testing.T) {
			output, err := tt.formatFunction([]byte{})
			require.NoError(t, err)
			assert.Contains(t, output, statusRenderErrors)
		})

		t.Run(fmt.Sprintf("%s: no render errors", tt.name), func(t *testing.T) {
			output, err := tt.formatFunction(jsonBytes)
			require.NoError(t, err)

			// We replace windows line break by linux so the tests pass on every OS
			result := strings.Replace(string(expectedOutput), "\r\n", "\n", -1)
			output = strings.Replace(output, "\r\n", "\n", -1)

			assert.Equal(t, output, result)
			assert.NotContains(t, output, statusRenderErrors)
		})
	}
}
