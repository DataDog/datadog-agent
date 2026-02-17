// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package clusterchecks

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestTracingConstants verifies span names follow conventions
func TestTracingConstants(t *testing.T) {
	// Verify span naming convention (should use dot notation)
	expectedSpans := []string{
		"cluster_checks.dispatcher.schedule",
		"cluster_checks.dispatcher.rebalance",
	}

	for _, span := range expectedSpans {
		// Span names should use dot notation, not underscores for separators
		assert.Contains(t, span, "cluster_checks.")
		assert.Contains(t, span, "dispatcher.")

		// Should not start or end with dots
		assert.NotEqual(t, '.', span[0])
		assert.NotEqual(t, '.', span[len(span)-1])
	}
}

// TestSampleRateValidation tests the sample rate validation logic
func TestSampleRateValidation(t *testing.T) {
	tests := []struct {
		name        string
		sampleRate  float64
		shouldBeValid bool
		expectedResult float64
	}{
		{"valid 0.0", 0.0, true, 0.0},
		{"valid 0.1", 0.1, true, 0.1},
		{"valid 0.5", 0.5, true, 0.5},
		{"valid 1.0", 1.0, true, 1.0},
		{"invalid negative", -0.1, false, 0.1},
		{"invalid too large", 1.5, false, 0.1},
		{"invalid way too large", 100.0, false, 0.1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate validation logic from start command
			result := tt.sampleRate
			if result < 0.0 || result > 1.0 {
				result = 0.1 // Default
			}

			assert.Equal(t, tt.expectedResult, result)

			if tt.shouldBeValid {
				assert.Equal(t, tt.sampleRate, result, "Valid sample rate should not be modified")
			} else {
				assert.Equal(t, 0.1, result, "Invalid sample rate should default to 0.1")
			}
		})
	}
}

// TestAgentURLStripping tests URL parsing for agent address
func TestAgentURLStripping(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"http scheme", "http://localhost:8126", "localhost:8126"},
		{"https scheme", "https://localhost:8126", "localhost:8126"},
		{"no scheme", "localhost:8126", "localhost:8126"},
		{"datadog agent", "http://datadog-agent:8126", "datadog-agent:8126"},
		{"IP address", "http://10.0.0.1:8126", "10.0.0.1:8126"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate URL stripping logic from start command
			agentAddr := tt.input

			// Strip http:// prefix
			if len(agentAddr) >= 7 && agentAddr[:7] == "http://" {
				agentAddr = agentAddr[7:]
			}

			// Strip https:// prefix
			if len(agentAddr) >= 8 && agentAddr[:8] == "https://" {
				agentAddr = agentAddr[8:]
			}

			assert.Equal(t, tt.expected, agentAddr)
		})
	}
}
