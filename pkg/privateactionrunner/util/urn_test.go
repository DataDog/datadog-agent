// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRunnerURN(t *testing.T) {
	tests := []struct {
		name        string
		urn         string
		expected    RunnerURNParts
		shouldError bool
	}{
		{
			name: "valid URN",
			urn:  "urn:dd:apps:on-prem-runner:us1:12345:runner-abc-123",
			expected: RunnerURNParts{
				Region:   "us1",
				OrgID:    12345,
				RunnerID: "runner-abc-123",
			},
			shouldError: false,
		},
		{
			name: "valid URN - different region",
			urn:  "urn:dd:apps:on-prem-runner:eu1:67890:my-runner",
			expected: RunnerURNParts{
				Region:   "eu1",
				OrgID:    67890,
				RunnerID: "my-runner",
			},
			shouldError: false,
		},
		{
			name: "valid URN - large org ID",
			urn:  "urn:dd:apps:on-prem-runner:ap1:9999999999:runner-id",
			expected: RunnerURNParts{
				Region:   "ap1",
				OrgID:    9999999999,
				RunnerID: "runner-id",
			},
			shouldError: false,
		},
		{
			name:        "invalid URN - too few parts",
			urn:         "urn:dd:apps:on-prem-runner:us1:12345",
			shouldError: true,
		},
		{
			name:        "invalid URN - too many parts",
			urn:         "urn:dd:apps:on-prem-runner:us1:12345:runner:extra",
			shouldError: true,
		},
		{
			name:        "invalid URN - non-numeric org ID",
			urn:         "urn:dd:apps:on-prem-runner:us1:not-a-number:runner-id",
			shouldError: true,
		},
		{
			name:        "invalid URN - empty string",
			urn:         "",
			shouldError: true,
		},
		{
			name:        "invalid URN - no colons",
			urn:         "invalid-urn-format",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseRunnerURN(tt.urn)
			if tt.shouldError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected.Region, result.Region)
				assert.Equal(t, tt.expected.OrgID, result.OrgID)
				assert.Equal(t, tt.expected.RunnerID, result.RunnerID)
			}
		})
	}
}

func TestMakeRunnerURN(t *testing.T) {
	tests := []struct {
		name     string
		region   string
		orgID    int64
		runnerID string
		expected string
	}{
		{
			name:     "standard URN",
			region:   "us1",
			orgID:    12345,
			runnerID: "runner-abc-123",
			expected: "urn:dd:apps:on-prem-runner:us1:12345:runner-abc-123",
		},
		{
			name:     "EU region",
			region:   "eu1",
			orgID:    67890,
			runnerID: "my-runner",
			expected: "urn:dd:apps:on-prem-runner:eu1:67890:my-runner",
		},
		{
			name:     "large org ID",
			region:   "ap1",
			orgID:    9999999999,
			runnerID: "runner",
			expected: "urn:dd:apps:on-prem-runner:ap1:9999999999:runner",
		},
		{
			name:     "empty values",
			region:   "",
			orgID:    0,
			runnerID: "",
			expected: "urn:dd:apps:on-prem-runner::0:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MakeRunnerURN(tt.region, tt.orgID, tt.runnerID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRoundTrip(t *testing.T) {
	// Test that MakeRunnerURN and ParseRunnerURN are inverses
	region := "us1"
	orgID := int64(12345)
	runnerID := "test-runner"

	urn := MakeRunnerURN(region, orgID, runnerID)
	parsed, err := ParseRunnerURN(urn)

	require.NoError(t, err)
	assert.Equal(t, region, parsed.Region)
	assert.Equal(t, orgID, parsed.OrgID)
	assert.Equal(t, runnerID, parsed.RunnerID)
}
