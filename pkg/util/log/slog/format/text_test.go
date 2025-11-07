// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package format

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestText(t *testing.T) {
	formatter := Text("CORE", false)
	require.NotNil(t, formatter)

	record := slog.NewRecord(
		time.Date(2023, 11, 4, 15, 30, 45, 0, time.UTC),
		slog.LevelInfo,
		"test message",
		getCurrentPC(),
	)

	result := formatter(context.Background(), record)

	// Should end with newline
	assert.True(t, strings.HasSuffix(result, "\n"))

	// Should contain expected components
	assert.Contains(t, result, "CORE")
	assert.Contains(t, result, "test message")
	assert.Contains(t, result, "|")
}

func TestTextRFC3339(t *testing.T) {
	formatter := Text("CORE", true)

	testTime := time.Date(2023, 11, 4, 15, 30, 45, 0, time.UTC)
	record := slog.NewRecord(testTime, slog.LevelInfo, "test", getCurrentPC())

	result := formatter(context.Background(), record)

	// Should contain RFC3339 formatted time
	assert.Contains(t, result, "2023-11-04T15:30:45Z")
}

func TestTextJMXFETCH(t *testing.T) {
	formatter := Text("JMXFETCH", false)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
	result := formatter(context.Background(), record)

	// JMXFETCH should have simplified format
	assert.Equal(t, "test message\n", result)
}

func TestTextWithAttributes(t *testing.T) {
	formatter := Text("CORE", false)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", getCurrentPC())
	record.AddAttrs(
		slog.String("key1", "value1"),
		slog.Int("key2", 42),
	)

	result := formatter(context.Background(), record)

	// Should contain attributes
	assert.Contains(t, result, "key1:value1")
	assert.Contains(t, result, "key2:42")
}

func TestTextEmptyMessage(t *testing.T) {
	formatter := Text("CORE", false)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "", getCurrentPC())
	result := formatter(context.Background(), record)

	// Should still have structure even with empty message
	assert.Contains(t, result, "CORE")
	assert.True(t, strings.HasSuffix(result, "\n"))
}

func TestTextDifferentLevels(t *testing.T) {
	formatter := Text("CORE", false)

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

			// Should contain the logger name and message
			assert.Contains(t, result, "CORE")
			assert.Contains(t, result, "test")
		})
	}
}

func TestTextLoggerName(t *testing.T) {
	tests := []string{"CORE", "JMX", "AGENT", "PROCESS"}

	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			if name == "JMXFETCH" {
				// Skip JMXFETCH as it has different format
				return
			}

			formatter := Text(name, false)

			record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", getCurrentPC())
			result := formatter(context.Background(), record)

			assert.Contains(t, result, name)
		})
	}
}

func TestTextFormat(t *testing.T) {
	formatter := Text("CORE", false)

	record := slog.NewRecord(
		time.Date(2023, 11, 4, 15, 30, 45, 0, time.UTC),
		slog.LevelInfo,
		"test message",
		getCurrentPC(),
	)

	result := formatter(context.Background(), record)

	// Verify format structure: date | name | level | (file:line in func) | message
	parts := strings.Split(strings.TrimSpace(result), "|")
	assert.GreaterOrEqual(t, len(parts), 4)

	// Check that parts contain expected content
	assert.Contains(t, parts[0], "2023-11-04") // date
	assert.Contains(t, parts[1], "CORE")       // name
	// parts[2] is level - don't check exact value
}

func TestTextSpecialCharactersInMessage(t *testing.T) {
	formatter := Text("CORE", false)

	// Test message with special characters
	message := "test | message | with | pipes"
	record := slog.NewRecord(time.Now(), slog.LevelInfo, message, getCurrentPC())

	result := formatter(context.Background(), record)

	// Message should be preserved
	assert.Contains(t, result, message)
}

func TestTextMultilineMessage(t *testing.T) {
	formatter := Text("CORE", false)

	message := "line1\nline2\nline3"
	record := slog.NewRecord(time.Now(), slog.LevelInfo, message, getCurrentPC())

	result := formatter(context.Background(), record)

	// Multiline message should be preserved
	assert.Contains(t, result, message)
}
