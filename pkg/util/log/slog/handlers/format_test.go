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

// attrFormatter returns a formatter that outputs message and all attributes
func attrFormatter() func(ctx context.Context, r slog.Record) string {
	return func(_ context.Context, r slog.Record) string {
		var buf bytes.Buffer
		buf.WriteString(r.Message)
		r.Attrs(func(a slog.Attr) bool {
			buf.WriteString(" ")
			buf.WriteString(attrToString(a))
			return true
		})
		buf.WriteString("\n")
		return buf.String()
	}
}

// attrToString converts an attribute to a string representation
func attrToString(a slog.Attr) string {
	if a.Value.Kind() == slog.KindGroup {
		var buf bytes.Buffer
		buf.WriteString(a.Key)
		buf.WriteString("={")
		first := true
		for _, ga := range a.Value.Group() {
			if !first {
				buf.WriteString(" ")
			}
			first = false
			buf.WriteString(attrToString(ga))
		}
		buf.WriteString("}")
		return buf.String()
	}
	return fmt.Sprintf("%s=%v", a.Key, a.Value.Any())
}

func TestFormatHandlerWithAttrsEmpty(t *testing.T) {
	var buf bytes.Buffer
	handler := NewFormat(attrFormatter(), &buf)

	// WithAttrs with empty slice should return the same handler
	h2 := handler.WithAttrs(nil)
	assert.Same(t, handler, h2, "WithAttrs(nil) should return same handler")

	h3 := handler.WithAttrs([]slog.Attr{})
	assert.Same(t, handler, h3, "WithAttrs([]) should return same handler")
}

func TestFormatHandlerWithGroupEmpty(t *testing.T) {
	var buf bytes.Buffer
	handler := NewFormat(attrFormatter(), &buf)

	// WithGroup with empty string should return the same handler
	h2 := handler.WithGroup("")
	assert.Same(t, handler, h2, "WithGroup(\"\") should return same handler")
}

func TestFormatHandlerWithAttrs(t *testing.T) {
	var buf bytes.Buffer
	handler := NewFormat(attrFormatter(), &buf)

	// Add attributes
	h2 := handler.WithAttrs([]slog.Attr{slog.String("a", "1")})
	require.NotSame(t, handler, h2, "WithAttrs should return new handler")

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	err := h2.Handle(context.Background(), record)

	require.NoError(t, err)
	assert.Equal(t, "msg a=1\n", buf.String())
}

func TestFormatHandlerWithAttrsMultiple(t *testing.T) {
	var buf bytes.Buffer
	handler := NewFormat(attrFormatter(), &buf)

	// Add multiple attributes
	h2 := handler.WithAttrs([]slog.Attr{
		slog.String("a", "1"),
		slog.Int("b", 2),
	})

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	err := h2.Handle(context.Background(), record)

	require.NoError(t, err)
	assert.Equal(t, "msg a=1 b=2\n", buf.String())
}

func TestFormatHandlerWithAttrsChained(t *testing.T) {
	var buf bytes.Buffer
	handler := NewFormat(attrFormatter(), &buf)

	// Chain multiple WithAttrs calls
	h2 := handler.WithAttrs([]slog.Attr{slog.String("a", "1")})
	h3 := h2.WithAttrs([]slog.Attr{slog.String("b", "2")})

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	err := h3.Handle(context.Background(), record)

	require.NoError(t, err)
	assert.Equal(t, "msg a=1 b=2\n", buf.String())
}

func TestFormatHandlerWithAttrsAndRecordAttrs(t *testing.T) {
	var buf bytes.Buffer
	handler := NewFormat(attrFormatter(), &buf)

	// Add handler attributes
	h2 := handler.WithAttrs([]slog.Attr{slog.String("handler_attr", "h1")})

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	record.AddAttrs(slog.String("record_attr", "r1"))

	err := h2.Handle(context.Background(), record)

	require.NoError(t, err)
	// Record's attrs come first (already on the record), then handler attrs are added
	assert.Equal(t, "msg record_attr=r1 handler_attr=h1\n", buf.String())
}

func TestFormatHandlerWithGroup(t *testing.T) {
	var buf bytes.Buffer
	handler := NewFormat(attrFormatter(), &buf)

	// Add a group, then attrs
	h2 := handler.WithGroup("g").WithAttrs([]slog.Attr{slog.String("a", "1")})

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	err := h2.Handle(context.Background(), record)

	require.NoError(t, err)
	// Attribute 'a' should be wrapped in group 'g'
	assert.Equal(t, "msg g={a=1}\n", buf.String())
}

func TestFormatHandlerWithGroupNested(t *testing.T) {
	var buf bytes.Buffer
	handler := NewFormat(attrFormatter(), &buf)

	// Nested groups: g1 -> g2 -> attr
	h2 := handler.WithGroup("g1").WithGroup("g2").WithAttrs([]slog.Attr{slog.String("a", "1")})

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	err := h2.Handle(context.Background(), record)

	require.NoError(t, err)
	// Attribute 'a' should be wrapped in nested groups
	assert.Equal(t, "msg g1={g2={a=1}}\n", buf.String())
}

func TestFormatHandlerWithGroupAndAttrsInterleaved(t *testing.T) {
	var buf bytes.Buffer
	handler := NewFormat(attrFormatter(), &buf)

	// Pattern: attr -> group -> attr
	// This is like: logger.With("a", 1).WithGroup("g").With("b", 2)
	h2 := handler.
		WithAttrs([]slog.Attr{slog.String("a", "1")}).
		WithGroup("g").
		WithAttrs([]slog.Attr{slog.String("b", "2")})

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	err := h2.Handle(context.Background(), record)

	require.NoError(t, err)
	// 'a' is not in a group, 'b' is in group 'g'
	assert.Equal(t, "msg a=1 g={b=2}\n", buf.String())
}

func TestFormatHandlerOriginalUnchanged(t *testing.T) {
	var buf bytes.Buffer
	handler := NewFormat(attrFormatter(), &buf)

	// Create derived handlers
	_ = handler.WithAttrs([]slog.Attr{slog.String("a", "1")})
	_ = handler.WithGroup("g")

	// Original handler should still work without attrs or groups
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "original", 0)
	err := handler.Handle(context.Background(), record)

	require.NoError(t, err)
	assert.Equal(t, "original\n", buf.String())
}

func TestFormatHandlerDerivedHandlersIndependent(t *testing.T) {
	var buf bytes.Buffer
	handler := NewFormat(attrFormatter(), &buf)

	// Create two derived handlers from the same base
	h1 := handler.WithAttrs([]slog.Attr{slog.String("a", "1")})
	h2 := handler.WithAttrs([]slog.Attr{slog.String("b", "2")})

	// Test h1
	record1 := slog.NewRecord(time.Now(), slog.LevelInfo, "msg1", 0)
	err := h1.Handle(context.Background(), record1)
	require.NoError(t, err)
	assert.Equal(t, "msg1 a=1\n", buf.String())

	buf.Reset()

	// Test h2
	record2 := slog.NewRecord(time.Now(), slog.LevelInfo, "msg2", 0)
	err = h2.Handle(context.Background(), record2)
	require.NoError(t, err)
	assert.Equal(t, "msg2 b=2\n", buf.String())
}

func TestFormatHandlerWithGroupMultipleAttrs(t *testing.T) {
	var buf bytes.Buffer
	handler := NewFormat(attrFormatter(), &buf)

	// Group with multiple attributes
	h2 := handler.WithGroup("g").WithAttrs([]slog.Attr{
		slog.String("a", "1"),
		slog.Int("b", 2),
	})

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	err := h2.Handle(context.Background(), record)

	require.NoError(t, err)
	assert.Equal(t, "msg g={a=1 b=2}\n", buf.String())
}

// TestFormatHandlerMatchesJSONHandlerBehavior verifies that the format handler's
// WithAttrs and WithGroup behavior matches the standard slog.JSONHandler.
// This ensures our implementation follows the slog.Handler contract correctly.
func TestFormatHandlerMatchesJSONHandlerBehavior(t *testing.T) {
	// We use slog.JSONHandler as the reference implementation to verify
	// that our format handler produces the same JSON structure for the same
	// WithAttrs/WithGroup sequence.

	tests := []struct {
		name    string
		setupFn func(h slog.Handler) slog.Handler
		logArgs []any // additional args to pass to logger.Info
	}{
		{
			name: "WithAttrs single",
			setupFn: func(h slog.Handler) slog.Handler {
				return h.WithAttrs([]slog.Attr{slog.String("key", "value")})
			},
		},
		{
			name: "WithAttrs multiple",
			setupFn: func(h slog.Handler) slog.Handler {
				return h.WithAttrs([]slog.Attr{
					slog.String("a", "1"),
					slog.Int("b", 2),
					slog.Bool("c", true),
				})
			},
		},
		{
			name: "WithAttrs chained",
			setupFn: func(h slog.Handler) slog.Handler {
				return h.WithAttrs([]slog.Attr{slog.String("a", "1")}).
					WithAttrs([]slog.Attr{slog.String("b", "2")})
			},
		},
		{
			name: "WithGroup then WithAttrs",
			setupFn: func(h slog.Handler) slog.Handler {
				return h.WithGroup("g").WithAttrs([]slog.Attr{slog.String("a", "1")})
			},
		},
		{
			name: "nested groups",
			setupFn: func(h slog.Handler) slog.Handler {
				return h.WithGroup("g1").WithGroup("g2").WithAttrs([]slog.Attr{slog.String("a", "1")})
			},
		},
		{
			name: "interleaved attrs and groups",
			setupFn: func(h slog.Handler) slog.Handler {
				return h.WithAttrs([]slog.Attr{slog.String("a", "1")}).
					WithGroup("g").
					WithAttrs([]slog.Attr{slog.String("b", "2")})
			},
		},
		{
			name: "complex interleaving",
			setupFn: func(h slog.Handler) slog.Handler {
				return h.WithAttrs([]slog.Attr{slog.String("root", "r")}).
					WithGroup("g1").
					WithAttrs([]slog.Attr{slog.String("in_g1", "1")}).
					WithGroup("g2").
					WithAttrs([]slog.Attr{slog.String("in_g2", "2")})
			},
		},
		{
			name: "WithGroup qualifies record attrs",
			setupFn: func(h slog.Handler) slog.Handler {
				return h.WithGroup("g")
			},
			logArgs: []any{"record_key", "record_value"},
		},
		{
			name: "nested groups qualify record attrs",
			setupFn: func(h slog.Handler) slog.Handler {
				return h.WithGroup("g1").WithGroup("g2")
			},
			logArgs: []any{"deep_key", "deep_value"},
		},
		{
			name: "interleaved with record attrs",
			setupFn: func(h slog.Handler) slog.Handler {
				return h.WithAttrs([]slog.Attr{slog.String("a", "1")}).
					WithGroup("g").
					WithAttrs([]slog.Attr{slog.String("b", "2")})
			},
			logArgs: []any{"c", "3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &slog.HandlerOptions{
				// Remove time to make output deterministic
				ReplaceAttr: func(_groups []string, a slog.Attr) slog.Attr {
					if a.Key == slog.TimeKey {
						return slog.Attr{}
					}
					return a
				},
			}
			logMessage := "test"

			// Create a JSON handler and log through it
			var jsonBuf bytes.Buffer
			jsonHandler := slog.NewJSONHandler(&jsonBuf, opts)
			configuredJSON := tt.setupFn(jsonHandler)
			logger := slog.New(configuredJSON)
			logger.Info(logMessage, tt.logArgs...)

			// Create a format handler that outputs JSON in the same format
			var formatBuf bytes.Buffer
			formatHandler := NewFormat(func(_ context.Context, r slog.Record) string {
				// Use a temporary JSON handler to format this record
				var tmpBuf bytes.Buffer
				tmpHandler := slog.NewJSONHandler(&tmpBuf, opts)
				_ = tmpHandler.Handle(context.Background(), r)
				return tmpBuf.String()
			}, &formatBuf)
			configuredFormat := tt.setupFn(formatHandler)
			formatLogger := slog.New(configuredFormat)
			formatLogger.Info(logMessage, tt.logArgs...)

			// Compare the JSON outputs
			assert.JSONEq(t, jsonBuf.String(), formatBuf.String(),
				"format handler should produce same JSON structure as JSONHandler")
		})
	}
}
