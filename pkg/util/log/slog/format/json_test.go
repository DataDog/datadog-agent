// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package format

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSON(t *testing.T) {
	formatter := JSON("CORE", false)
	require.NotNil(t, formatter)

	record := slog.NewRecord(
		time.Date(2023, 11, 4, 15, 30, 45, 0, time.UTC),
		slog.LevelInfo,
		"test message",
		getCurrentPC(),
	)

	result := formatter(context.Background(), record)

	// Result should end with newline and contain expected fields
	assert.NotEmpty(t, result)

	// Should contain expected components (not parsing JSON due to multiple lines)
	assert.Contains(t, result, `"agent":"core"`)
	assert.Contains(t, result, `"msg":"test message"`)
	assert.Contains(t, result, `"level"`)
	assert.Contains(t, result, `"file"`)
	assert.Contains(t, result, `"line"`)
	assert.Contains(t, result, `"func"`)
	assert.Contains(t, result, `"time"`)
}

func TestJSONRFC3339(t *testing.T) {
	formatter := JSON("CORE", true)

	testTime := time.Date(2023, 11, 4, 15, 30, 45, 0, time.UTC)
	record := slog.NewRecord(testTime, slog.LevelInfo, "test", getCurrentPC())

	result := formatter(context.Background(), record)

	// Check that time is in RFC3339 format (2023-11-04T15:30:45Z)
	assert.Contains(t, result, "2023-11-04T15:30:45Z")
}

func TestJSONJMXFETCH(t *testing.T) {
	formatter := JSON("JMXFETCH", false)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
	result := formatter(context.Background(), record)

	// JMXFETCH should have simplified format with only msg field
	assert.Contains(t, result, `"msg":"test message"`)
	assert.NotContains(t, result, `"agent"`)
	assert.NotContains(t, result, `"time"`)
}

func TestJSONWithAttributes(t *testing.T) {
	formatter := JSON("CORE", false)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", getCurrentPC())
	record.AddAttrs(
		slog.String("key1", "value1"),
		slog.Int("key2", 42),
	)

	result := formatter(context.Background(), record)

	// Check that attributes are included in JSON output
	assert.Contains(t, result, `"key1":"value1"`)
	// key2 might be quoted as "42" in the output
	assert.Contains(t, result, `"key2":`)
}

func TestJSONSpecialCharacters(t *testing.T) {
	formatter := JSON("CORE", false)

	// Test message with special characters that need escaping
	message := `test "quoted"`

	record := slog.NewRecord(time.Now(), slog.LevelInfo, message, getCurrentPC())
	result := formatter(context.Background(), record)

	// Should contain escaped quotes in JSON
	assert.Contains(t, result, `\"`)
}

func TestJSONEmptyMessage(t *testing.T) {
	formatter := JSON("CORE", false)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "", getCurrentPC())
	result := formatter(context.Background(), record)

	// Should have empty msg field
	assert.Contains(t, result, `"msg":""`)
}

func TestJSONLoggerNameCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"CORE", "core"},
		{"JMX", "jmx"},
		{"core", "core"},
		{"Agent", "agent"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			formatter := JSON(tt.input, false)

			record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", getCurrentPC())
			result := formatter(context.Background(), record)

			if tt.input == "JMXFETCH" {
				// Skip for JMXFETCH as it has different format
				return
			}

			// Check agent name in JSON
			assert.Contains(t, result, fmt.Sprintf(`"agent":"%s"`, tt.expected))
		})
	}
}

func TestJSONDifferentLevels(t *testing.T) {
	formatter := JSON("CORE", false)

	levels := []slog.Level{
		slog.LevelDebug,
		slog.LevelInfo,
		slog.LevelWarn,
		slog.LevelError,
	}

	for _, l := range levels {
		t.Run(l.String(), func(t *testing.T) {
			record := slog.NewRecord(time.Now(), l, "test", getCurrentPC())
			result := formatter(context.Background(), record)

			// Just check that level is present in output
			assert.Contains(t, result, `"level"`)
		})
	}
}
