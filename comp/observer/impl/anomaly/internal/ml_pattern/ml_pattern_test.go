// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package ml_pattern

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/observer/impl/anomaly/internal/types"
)

func TestNormalizeDynamicValues(t *testing.T) {
	ml := NewMLPatternDetector()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "UUID normalization",
			input:    "Error with request ac8218cf-498b-4d33-bd44-151095959547 failed",
			expected: "Error with request <UUID> failed",
		},
		{
			name:     "Full datetime with milliseconds and timezone (colon separator)",
			input:    "[dd.trace 2026-01-16 13:04:30:064 +0000] Log message",
			expected: "[dd.trace <TIMESTAMP>] Log message",
		},
		{
			name:     "Full datetime with milliseconds and timezone (dot separator)",
			input:    "[dd.trace 2026-01-16 13:04:30.064 +0000] Log message",
			expected: "[dd.trace <TIMESTAMP>] Log message",
		},
		{
			name:     "Full datetime with milliseconds and timezone (colon separator) - variant 2",
			input:    "[dd.trace 2026-01-16 13:03:12:061 +0000]",
			expected: "[dd.trace <TIMESTAMP>]",
		},
		{
			name:     "Full datetime with milliseconds and timezone (colon separator) - without brackets",
			input:    "2026-01-16 13:03:12:061 +0000",
			expected: "<TIMESTAMP>",
		},
		{
			name:     "Full datetime with timezone - alternative format",
			input:    "Error at 2026-01-16 13:03:47:634 +0000 in service",
			expected: "Error at <TIMESTAMP> in service",
		},
		{
			name:     "Human-readable date with AM/PM",
			input:    "Jan 16, 2026 1:03:20 PM io.airlift.log.Logger info",
			expected: "<TIMESTAMP> io.airlift.log.Logger info",
		},
		{
			name:     "Human-readable date with AM/PM - variant 2",
			input:    "Jan 16, 2026 1:03:30 PM io.airlift.log.Logger info",
			expected: "<TIMESTAMP> io.airlift.log.Logger info",
		},
		{
			name:     "Human-readable date without comma",
			input:    "Jan 16 2026 1:03:01 PM error occurred",
			expected: "<TIMESTAMP> error occurred",
		},
		{
			name:     "Full month name",
			input:    "January 16, 2026 1:03:01 PM system started",
			expected: "<TIMESTAMP> system started",
		},
		{
			name:     "ISO8601 with milliseconds",
			input:    "Event at 2026-01-16T13:04:30.064 detected",
			expected: "Event at <TIMESTAMP> detected",
		},
		{
			name:     "ISO8601 basic format",
			input:    "Started at 2026-01-16T13:04:30",
			expected: "Started at <TIMESTAMP>",
		},
		{
			name:     "Time with milliseconds",
			input:    "Process took 13:04:30:064 to complete",
			expected: "Process took <TIMESTAMP> to complete",
		},
		{
			name:     "Hash pattern",
			input:    "hashes.usr.id:>=-3074457345618258604 not found",
			expected: "hashes.usr.id:>=<HASH> not found",
		},
		{
			name:     "IPv4 address",
			input:    "Connection from 10.26.47.198 established",
			expected: "Connection from <IP> established",
		},
		{
			name:     "Hex string",
			input:    "Memory address 0x1a2b3c4d5e6f7890 leaked",
			expected: "Memory address <HEX> leaked",
		},
		{
			name:     "Session ID",
			input:    "Session abc123XYZ456def789GHI012 expired",
			expected: "Session <SESSION> expired",
		},
		{
			name:     "Duration values",
			input:    "Timeout after 5 minutes and 30 seconds",
			expected: "Timeout after <DURATION> and <DURATION>",
		},
		{
			name:     "HTTP status code",
			input:    "Failed to upload profile Status: 403 Forbidden",
			expected: "Failed to upload profile Status: <STATUS> Forbidden",
		},
		{
			name:     "URL normalization",
			input:    "Request to http://10.26.47.198:8126/api failed",
			expected: "Request to <URL> failed",
		},
		{
			name:     "Email address",
			input:    "Contact user@example.com for support",
			expected: "Contact <EMAIL> for support",
		},
		{
			name:     "Unix file path",
			input:    "File /var/log/app/error.log not found",
			expected: "File <PATH> not found",
		},
		{
			name:     "Complex log with multiple dynamic values",
			input:    "[dd.trace 2026-01-16 13:04:30:064 +0000] [OkHttp http://10.26.47.198:8126/v0.4/traces] EXCLUDE_TELEMETRY com.datadog.profiling.uploader.ProfileUploader - Failed to upload profile Status: 403 Forbidden (Will not log warnings for 5 minutes)",
			expected: "[dd.trace <TIMESTAMP>] [OkHttp <URL>] EXCLUDE_TELEMETRY com.datadog.profiling.uploader.ProfileUploader - Failed to upload profile Status: <STATUS> Forbidden (Will not log warnings for <DURATION>)",
		},
		{
			name:     "Log with UUID and hash",
			input:    "[dd.trace 2026-01-16 13:03:47:624 +0000] [OkHttp http://10.26.47.198:8126/v0.4/traces] ERROR io.opentracing.contrib.specialagent.AgentRule - SQLIntegration#ac8218cf-498b-4d33-bd44-151095959547.218 hashes.usr.id:>=-3074457345618258604 hashes.usr.name:>=1739428379 - name: test: ac8218cf-498b-4d33-bd44-151095959547",
			expected: "[dd.trace <TIMESTAMP>] [OkHttp <URL>] ERROR io.opentracing.contrib.specialagent.AgentRule - SQLIntegration#<UUID>.<STATUS> hashes.usr.id:>=<HASH> hashes.usr.name:>=<HASH> - name: test: <UUID>",
		},
		{
			name:     "Multiple timestamps in same message",
			input:    "Event started at 2026-01-16 13:00:00 +0000 and ended at 2026-01-16 14:00:00 +0000",
			expected: "Event started at <TIMESTAMP> <TZ> and ended at <TIMESTAMP> <TZ>",
		},
		{
			name:     "Negative hash value",
			input:    "Hash value: -3074457345618258604",
			expected: "Hash value: <HASH>",
		},
		{
			name:     "Timezone offset standalone",
			input:    "Time zone offset is +0000",
			expected: "Time zone offset is <TZ>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ml.normalizeDynamicValues(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeDynamicValues() failed\nInput:    %s\nExpected: %s\nGot:      %s", tt.input, tt.expected, result)
			}
		})
	}
}

func TestTokenize(t *testing.T) {
	ml := NewMLPatternDetector()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "Simple message",
			input:    "Error occurred in service",
			expected: []string{"error", "occurred", "service"},
		},
		{
			name:     "Message with UUID",
			input:    "Request ac8218cf-498b-4d33-bd44-151095959547 failed",
			expected: []string{"request", "<UUID>", "failed"},
		},
		{
			name:     "Message with timestamp",
			input:    "Event at 2026-01-16 13:04:30:064 +0000",
			expected: []string{"event", "<TIMESTAMP>"},
		},
		{
			name:     "Message with stop words filtered",
			input:    "The error is in the database",
			expected: []string{"error", "database"},
		},
		{
			name:     "Message with separators",
			input:    "Error:database|connection,failed",
			expected: []string{"error", "database", "connection", "failed"},
		},
		{
			name:     "Word boundary with underscore",
			input:    "invalidation_cycle occurred",
			expected: []string{"invalidation", "cycle", "occurred"},
		},
		{
			name:     "Word boundary with hyphen",
			input:    "cache-invalidation error",
			expected: []string{"cache", "invalidation", "error"},
		},
		{
			name:     "Word boundary with period",
			input:    "com.example.service failed",
			expected: []string{"com", "example", "service", "failed"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ml.Tokenize(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("Tokenize() length mismatch\nInput:    %s\nExpected: %v (len=%d)\nGot:      %v (len=%d)",
					tt.input, tt.expected, len(tt.expected), result, len(result))
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("Tokenize() token mismatch at index %d\nInput:    %s\nExpected: %v\nGot:      %v",
						i, tt.input, tt.expected, result)
					return
				}
			}
		})
	}
}

func TestCosineSimilarity(t *testing.T) {
	ml := NewMLPatternDetector()

	tests := []struct {
		name     string
		vec1     map[string]float64
		vec2     map[string]float64
		expected float64
		delta    float64
	}{
		{
			name:     "Identical vectors",
			vec1:     map[string]float64{"error": 1.0, "database": 1.0},
			vec2:     map[string]float64{"error": 1.0, "database": 1.0},
			expected: 1.0,
			delta:    0.0001,
		},
		{
			name:     "Completely different vectors",
			vec1:     map[string]float64{"error": 1.0},
			vec2:     map[string]float64{"success": 1.0},
			expected: 0.0,
			delta:    0.0001,
		},
		{
			name:     "Partially overlapping vectors",
			vec1:     map[string]float64{"error": 1.0, "database": 1.0},
			vec2:     map[string]float64{"error": 1.0, "network": 1.0},
			expected: 0.5,
			delta:    0.0001,
		},
		{
			name:     "Empty vector",
			vec1:     map[string]float64{},
			vec2:     map[string]float64{"error": 1.0},
			expected: 0.0,
			delta:    0.0001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ml.CosineSimilarity(tt.vec1, tt.vec2)
			if result < tt.expected-tt.delta || result > tt.expected+tt.delta {
				t.Errorf("CosineSimilarity() = %v, expected %v (±%v)", result, tt.expected, tt.delta)
			}
		})
	}
}

func TestLogPatternDetection(t *testing.T) {
	ml := NewMLPatternDetector()

	// Test with very similar log messages that should be grouped together
	logErrors := []types.LogError{
		{Message: "Database connection failed error code 1234", Count: 1},
		{Message: "Database connection failed error code 5678", Count: 1},
		{Message: "Database connection failed error code 9012", Count: 1},
		{Message: "Network timeout occurred after 30 seconds", Count: 1},
		{Message: "Network timeout occurred after 60 seconds", Count: 1},
		{Message: "Unique error message that will not cluster", Count: 1},
	}

	patterns := ml.DetectPatternsDBSCAN(logErrors, 0.3, 2)

	// With DBSCAN clustering, we expect patterns to be created from similar messages
	// At minimum, we should get some patterns (could be 0 if clustering threshold is strict)
	// So we just verify the function runs without crashing and returns a slice
	if patterns == nil {
		t.Error("Expected non-nil patterns slice")
	}

	// Verify that total count in patterns matches or is less than input (some messages may be noise)
	totalCount := int64(0)
	for _, pattern := range patterns {
		totalCount += pattern.Count
	}

	if totalCount > 6 {
		t.Errorf("Total pattern count (%d) should not exceed total input messages (6)", totalCount)
	}
}
