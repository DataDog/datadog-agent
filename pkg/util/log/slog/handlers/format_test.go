// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package handlers

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFormat(t *testing.T) {
	var buf bytes.Buffer
	formatter := func(_ context.Context, r slog.Record) string {
		return r.Message + "\n"
	}

	handler := NewFormat(formatter, &buf)
	require.NotNil(t, handler)
}

func TestFormatHandlerHandle(t *testing.T) {
	var buf bytes.Buffer
	formatter := func(_ context.Context, r slog.Record) string {
		return r.Message + "\n"
	}

	handler := NewFormat(formatter, &buf)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
	err := handler.Handle(context.Background(), record)

	assert.NoError(t, err)
	assert.Equal(t, "test message\n", buf.String())
}

func TestFormatHandlerEnabled(t *testing.T) {
	var buf bytes.Buffer
	formatter := func(_ context.Context, r slog.Record) string {
		return r.Message
	}

	handler := NewFormat(formatter, &buf)

	// Should always return true
	assert.True(t, handler.Enabled(context.Background(), slog.LevelDebug))
	assert.True(t, handler.Enabled(context.Background(), slog.LevelInfo))
	assert.True(t, handler.Enabled(context.Background(), slog.LevelWarn))
	assert.True(t, handler.Enabled(context.Background(), slog.LevelError))
}

func TestFormatHandlerMultipleMessages(t *testing.T) {
	var buf bytes.Buffer
	formatter := func(_ context.Context, r slog.Record) string {
		return fmt.Sprintf("[%s] %s\n", r.Level, r.Message)
	}

	handler := NewFormat(formatter, &buf)

	messages := []struct {
		level slog.Level
		msg   string
	}{
		{slog.LevelInfo, "info message"},
		{slog.LevelWarn, "warn message"},
		{slog.LevelError, "error message"},
	}

	for _, m := range messages {
		record := slog.NewRecord(time.Now(), m.level, m.msg, 0)
		err := handler.Handle(context.Background(), record)
		assert.NoError(t, err)
	}

	expected := "[INFO] info message\n[WARN] warn message\n[ERROR] error message\n"
	assert.Equal(t, expected, buf.String())
}

func TestFormatHandlerCustomFormatter(t *testing.T) {
	var buf bytes.Buffer

	// Custom formatter that includes timestamp and level
	formatter := func(_ context.Context, r slog.Record) string {
		return fmt.Sprintf("%s [%s] %s\n",
			r.Time.Format("15:04:05"),
			r.Level,
			r.Message)
	}

	handler := NewFormat(formatter, &buf)

	now := time.Now()
	record := slog.NewRecord(now, slog.LevelWarn, "warning message", 0)
	err := handler.Handle(context.Background(), record)

	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "[WARN] warning message")
	assert.Contains(t, buf.String(), now.Format("15:04:05"))
}

func TestFormatHandlerEmptyMessage(t *testing.T) {
	var buf bytes.Buffer
	formatter := func(_ context.Context, r slog.Record) string {
		return r.Message + "\n"
	}

	handler := NewFormat(formatter, &buf)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "", 0)
	err := handler.Handle(context.Background(), record)

	assert.NoError(t, err)
	assert.Equal(t, "\n", buf.String())
}

func TestFormatHandlerWithRecordAttributes(t *testing.T) {
	var buf bytes.Buffer
	formatter := func(_ context.Context, r slog.Record) string {
		var attrs []string
		r.Attrs(func(a slog.Attr) bool {
			attrs = append(attrs, fmt.Sprintf("%s=%v", a.Key, a.Value))
			return true
		})

		if len(attrs) > 0 {
			return fmt.Sprintf("%s [%s]\n", r.Message, attrs[0])
		}
		return r.Message + "\n"
	}

	handler := NewFormat(formatter, &buf)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	record.AddAttrs(slog.String("key", "value"))

	err := handler.Handle(context.Background(), record)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "key=")
}

func TestFormatHandlerFormatterUsesContext(t *testing.T) {
	var buf bytes.Buffer

	type contextKey string // to please the linter
	var requestIDKey contextKey = "request_id"

	// Formatter that uses context
	formatter := func(ctx context.Context, r slog.Record) string {
		if ctx != nil {
			if value := ctx.Value(requestIDKey); value != nil {
				return fmt.Sprintf("[%v] %s\n", value, r.Message)
			}
		}
		return r.Message + "\n"
	}

	handler := NewFormat(formatter, &buf)

	ctx := context.WithValue(context.Background(), requestIDKey, "12345")
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)

	err := handler.Handle(ctx, record)
	assert.NoError(t, err)
	assert.Equal(t, "[12345] test\n", buf.String())
}
