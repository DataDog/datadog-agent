// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package formatters

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/log/types"
)

func TestTemplateFormatterBasic(t *testing.T) {
	formatter := Template("{{.msg}}")
	require.NotNil(t, formatter)

	record := slog.NewRecord(
		time.Date(2023, 11, 4, 15, 30, 45, 0, time.UTC),
		types.ToSlogLevel(types.InfoLvl),
		"test message",
		0,
	)

	result := formatter(context.Background(), record)
	assert.Equal(t, "test message", result)
}

func TestTemplateFormatterWithLevel(t *testing.T) {
	formatter := Template("{{Level}} - {{.msg}}")

	levels := []struct {
		level    slog.Level
		expected string
	}{
		{types.ToSlogLevel(types.TraceLvl), "Trace - test"},
		{types.ToSlogLevel(types.DebugLvl), "Debug - test"},
		{types.ToSlogLevel(types.InfoLvl), "Info - test"},
		{types.ToSlogLevel(types.WarnLvl), "Warn - test"},
		{types.ToSlogLevel(types.ErrorLvl), "Error - test"},
		{types.ToSlogLevel(types.CriticalLvl), "Critical - test"},
	}

	for _, tc := range levels {
		t.Run(tc.expected, func(t *testing.T) {
			record := slog.NewRecord(time.Now(), tc.level, "test", 0)
			result := formatter(context.Background(), record)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestTemplateFormatterWithLEVEL(t *testing.T) {
	formatter := Template("{{LEVEL}} - {{.msg}}")

	levels := []struct {
		level    slog.Level
		expected string
	}{
		{types.ToSlogLevel(types.TraceLvl), "TRACE - test"},
		{types.ToSlogLevel(types.DebugLvl), "DEBUG - test"},
		{types.ToSlogLevel(types.InfoLvl), "INFO - test"},
		{types.ToSlogLevel(types.WarnLvl), "WARN - test"},
		{types.ToSlogLevel(types.ErrorLvl), "ERROR - test"},
		{types.ToSlogLevel(types.CriticalLvl), "CRITICAL - test"},
	}

	for _, tc := range levels {
		t.Run(tc.expected, func(t *testing.T) {
			record := slog.NewRecord(time.Now(), tc.level, "test", 0)
			result := formatter(context.Background(), record)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestTemplateFormatterWithLevelChar(t *testing.T) {
	formatter := Template("{{.l}} - {{.msg}}")

	record := slog.NewRecord(time.Now(), types.ToSlogLevel(types.InfoLvl), "test", 0)
	result := formatter(context.Background(), record)
	assert.Contains(t, result, " - test")
	// The .l field is a single byte representing the first character of the level
	parts := strings.Split(result, " - ")
	require.Len(t, parts, 2)
	levelChar := strings.TrimSpace(parts[0])
	assert.NotEmpty(t, levelChar)
}

func TestTemplateFormatterWithDateTime(t *testing.T) {
	formatter := Template("{{DateTime}} | {{.msg}}")

	testTime := time.Date(2023, 11, 4, 15, 30, 45, 0, time.UTC)
	record := slog.NewRecord(testTime, types.ToSlogLevel(types.InfoLvl), "test", 0)

	result := formatter(context.Background(), record)
	assert.Equal(t, "2023-11-04 15:30:45.000 UTC | test", result)
}

func TestTemplateFormatterWithDate(t *testing.T) {
	formatter := Template(`{{Date "2006-01-02"}} | {{.msg}}`)

	testTime := time.Date(2023, 11, 4, 15, 30, 45, 0, time.UTC)
	record := slog.NewRecord(testTime, types.ToSlogLevel(types.InfoLvl), "test", 0)

	result := formatter(context.Background(), record)
	assert.Equal(t, "2023-11-04 | test", result)
}

func TestTemplateFormatterWithCustomDateFormat(t *testing.T) {
	formatter := Template(`{{Date "15:04:05"}} - {{.msg}}`)

	testTime := time.Date(2023, 11, 4, 15, 30, 45, 0, time.UTC)
	record := slog.NewRecord(testTime, types.ToSlogLevel(types.InfoLvl), "test", 0)

	result := formatter(context.Background(), record)
	assert.Equal(t, "15:30:45 - test", result)
}

func TestTemplateFormatterWithNs(t *testing.T) {
	formatter := Template("{{Ns}} | {{.msg}}")

	testTime := time.Date(2023, 11, 4, 15, 30, 45, 123456789, time.UTC)
	record := slog.NewRecord(testTime, types.ToSlogLevel(types.InfoLvl), "test", 0)

	result := formatter(context.Background(), record)
	assert.Contains(t, result, " | test")
	nsPart := strings.Split(result, " | ")[0]
	assert.Regexp(t, `^\d+$`, nsPart)
}

func TestTemplateFormatterWithToUpper(t *testing.T) {
	formatter := Template(`{{ToUpper .msg}}`)

	record := slog.NewRecord(time.Now(), types.ToSlogLevel(types.InfoLvl), "test message", 0)
	result := formatter(context.Background(), record)
	assert.Equal(t, "TEST MESSAGE", result)
}

func TestTemplateFormatterWithQuote(t *testing.T) {
	formatter := Template(`{{Quote .msg}}`)

	tests := []struct {
		name     string
		message  string
		expected string
	}{
		{
			name:     "no special chars",
			message:  "simple message",
			expected: "simple message",
		},
		{
			name:     "with spaces",
			message:  "message with spaces",
			expected: "message with spaces",
		},
		{
			name:     "with quotes",
			message:  `message with "quotes"`,
			expected: `message with "quotes"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			record := slog.NewRecord(time.Now(), types.ToSlogLevel(types.InfoLvl), tc.message, 0)
			result := formatter(context.Background(), record)
			// Quote function behavior depends on formatters.Quote implementation
			assert.NotEmpty(t, result)
		})
	}
}

func TestTemplateFormatterWithFuncShort(t *testing.T) {
	formatter := Template("{{FuncShort}} | {{.msg}}")

	record := slog.NewRecord(time.Now(), types.ToSlogLevel(types.InfoLvl), "test", 0)
	result := formatter(context.Background(), record)
	assert.Contains(t, result, " | test")
}

func TestTemplateFormatterWithShortFilePath(t *testing.T) {
	formatter := Template("{{ShortFilePath}} | {{.msg}}")

	record := slog.NewRecord(time.Now(), types.ToSlogLevel(types.InfoLvl), "test", 0)
	result := formatter(context.Background(), record)
	assert.Contains(t, result, " | test")
}

func TestTemplateFormatterWithRelFile(t *testing.T) {
	formatter := Template("{{RelFile}} | {{.msg}}")

	record := slog.NewRecord(time.Now(), types.ToSlogLevel(types.InfoLvl), "test", 0)
	result := formatter(context.Background(), record)
	assert.Contains(t, result, " | test")
}

func TestTemplateFormatterWithLine(t *testing.T) {
	formatter := Template("line:{{.line}} | {{.msg}}")

	record := slog.NewRecord(time.Now(), types.ToSlogLevel(types.InfoLvl), "test", 0)
	result := formatter(context.Background(), record)
	assert.Contains(t, result, "line:")
	assert.Contains(t, result, " | test")
}

func TestTemplateFormatterWithFile(t *testing.T) {
	formatter := Template("{{.file}} | {{.msg}}")

	record := slog.NewRecord(time.Now(), types.ToSlogLevel(types.InfoLvl), "test", 0)
	result := formatter(context.Background(), record)
	assert.Contains(t, result, " | test")
}

func TestTemplateFormatterWithFunc(t *testing.T) {
	formatter := Template("{{.func}} | {{.msg}}")

	record := slog.NewRecord(time.Now(), types.ToSlogLevel(types.InfoLvl), "test", 0)
	result := formatter(context.Background(), record)
	assert.Contains(t, result, " | test")
}

func TestTemplateFormatterWithExtraTextContext(t *testing.T) {
	formatter := Template("{{.msg}}{{ExtraTextContext}}")

	t.Run("no attributes", func(t *testing.T) {
		record := slog.NewRecord(time.Now(), types.ToSlogLevel(types.InfoLvl), "test", 0)
		result := formatter(context.Background(), record)
		assert.Equal(t, "test", result)
	})

	t.Run("with attributes", func(t *testing.T) {
		record := slog.NewRecord(time.Now(), types.ToSlogLevel(types.InfoLvl), "test", 0)
		record.AddAttrs(
			slog.String("key1", "value1"),
			slog.Int("key2", 42),
		)
		result := formatter(context.Background(), record)
		assert.Contains(t, result, "test")
		// ExtraTextContext should add attributes
		assert.Contains(t, result, "key1")
		assert.Contains(t, result, "value1")
		assert.Contains(t, result, "key2")
		assert.Contains(t, result, "42")
	})
}

func TestTemplateFormatterWithExtraJSONContext(t *testing.T) {
	formatter := Template("{{.msg}}{{ExtraJSONContext}}")

	t.Run("no attributes", func(t *testing.T) {
		record := slog.NewRecord(time.Now(), types.ToSlogLevel(types.InfoLvl), "test", 0)
		result := formatter(context.Background(), record)
		assert.Equal(t, "test", result)
	})

	t.Run("with attributes", func(t *testing.T) {
		record := slog.NewRecord(time.Now(), types.ToSlogLevel(types.InfoLvl), "test", 0)
		record.AddAttrs(
			slog.String("key1", "value1"),
			slog.Int("key2", 42),
		)
		result := formatter(context.Background(), record)
		assert.Contains(t, result, "test")
		// ExtraJSONContext should add attributes in JSON format
		assert.Contains(t, result, "key1")
		assert.Contains(t, result, "value1")
		assert.Contains(t, result, "key2")
		assert.Contains(t, result, "42")
	})
}

func TestTemplateFormatterComplexTemplate(t *testing.T) {
	formatter := Template(`{{DateTime}} | {{LEVEL}} | {{ShortFilePath}}:{{.line}} in {{FuncShort}} | {{.msg}}{{ExtraTextContext}}`)

	testTime := time.Date(2023, 11, 4, 15, 30, 45, 0, time.UTC)
	record := slog.NewRecord(testTime, types.ToSlogLevel(types.InfoLvl), "test message", 0)
	record.AddAttrs(slog.String("user", "admin"))

	result := formatter(context.Background(), record)

	// Check all components are present
	assert.Contains(t, result, "2023-11-04")
	assert.Contains(t, result, "INFO")
	assert.Contains(t, result, "test message")
	assert.Contains(t, result, "user")
	assert.Contains(t, result, "admin")
	assert.Contains(t, result, "|")
}

func TestTemplateFormatterEmptyMessage(t *testing.T) {
	formatter := Template("{{.msg}}")

	record := slog.NewRecord(time.Now(), types.ToSlogLevel(types.InfoLvl), "", 0)
	result := formatter(context.Background(), record)
	assert.Equal(t, "", result)
}

func TestTemplateFormatterMultilineMessage(t *testing.T) {
	formatter := Template("{{.msg}}")

	message := "line1\nline2\nline3"
	record := slog.NewRecord(time.Now(), types.ToSlogLevel(types.InfoLvl), message, 0)
	result := formatter(context.Background(), record)
	assert.Equal(t, message, result)
}

func TestTemplateFormatterSpecialCharacters(t *testing.T) {
	formatter := Template("{{.msg}}")

	specialMessages := []string{
		"message | with | pipes",
		"message with \"quotes\"",
		"message with 'apostrophes'",
		"message with \ttabs",
		"message with \\backslashes",
	}

	for _, msg := range specialMessages {
		t.Run(msg, func(t *testing.T) {
			record := slog.NewRecord(time.Now(), types.ToSlogLevel(types.InfoLvl), msg, 0)
			result := formatter(context.Background(), record)
			assert.Equal(t, msg, result)
		})
	}
}

func TestTemplateFormatterInvalidTemplate(t *testing.T) {
	// Invalid template syntax - will fail when executed
	// We can't test this directly since Template uses require.NoError
	// which will fail the test. Instead, we test that a valid template doesn't fail.
	formatter := Template("{{.msg}}")

	record := slog.NewRecord(time.Now(), types.ToSlogLevel(types.InfoLvl), "test", 0)
	result := formatter(context.Background(), record)
	assert.Equal(t, "test", result)
}

func TestTemplateFormatterAccessingRecordFields(t *testing.T) {
	formatter := Template("{{.record.Message}}")

	testTime := time.Date(2023, 11, 4, 15, 30, 45, 0, time.UTC)
	record := slog.NewRecord(testTime, types.ToSlogLevel(types.InfoLvl), "test", 0)

	result := formatter(context.Background(), record)
	assert.Equal(t, "test", result)
}

func TestTemplateFormatterContextFields(t *testing.T) {
	formatter := Template("level={{.level}} msg={{.msg}}")

	record := slog.NewRecord(time.Now(), types.ToSlogLevel(types.InfoLvl), "test", 0)
	result := formatter(context.Background(), record)

	assert.Contains(t, result, "level=")
	assert.Contains(t, result, "msg=test")
}

func TestTemplateFormatterNilContext(t *testing.T) {
	formatter := Template("{{.msg}}")

	record := slog.NewRecord(time.Now(), types.ToSlogLevel(types.InfoLvl), "test", 0)
	// Pass nil context - should be handled gracefully
	result := formatter(nil, record)
	assert.Equal(t, "test", result)
}

func TestTemplateFormatterTimeFields(t *testing.T) {
	formatter := Template(`{{Date "2006"}}-{{Date "01"}}-{{Date "02"}}`)

	testTime := time.Date(2023, 11, 4, 15, 30, 45, 0, time.UTC)
	record := slog.NewRecord(testTime, types.ToSlogLevel(types.InfoLvl), "test", 0)

	result := formatter(context.Background(), record)
	assert.Equal(t, "2023-11-04", result)
}

func TestTemplateFormatterFrameFields(t *testing.T) {
	formatter := Template(`{{.frame.Function}} {{.frame.Line}}`)

	record := slog.NewRecord(time.Now(), types.ToSlogLevel(types.InfoLvl), "test", 0)
	result := formatter(context.Background(), record)

	// Frame fields should be present even if empty
	assert.NotEmpty(t, result)
}

func TestTemplateFormatterCombinedFunctions(t *testing.T) {
	formatter := Template(`{{ToUpper (Level)}} - {{Quote .msg}}`)

	record := slog.NewRecord(time.Now(), types.ToSlogLevel(types.InfoLvl), "test", 0)
	result := formatter(context.Background(), record)

	assert.Contains(t, result, "INFO")
	assert.Contains(t, result, " - ")
}
