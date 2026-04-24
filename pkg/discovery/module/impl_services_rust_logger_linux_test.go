// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package module

import (
	"bufio"
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// setupLogCapture initialises the global logger at the given level and returns
// a buffer that captures all output. Call w.Flush() before reading b.
// The writer is automatically flushed when the test ends.
func setupLogCapture(t *testing.T, level log.LogLevel) (b *bytes.Buffer, w *bufio.Writer) {
	t.Helper()
	b = &bytes.Buffer{}
	w = bufio.NewWriter(b)
	l, err := log.LoggerFromWriterWithMinLevelAndLvlMsgFormat(w, level)
	require.NoError(t, err)
	log.SetupLogger(l, level.String())
	t.Cleanup(func() { _ = w.Flush() })
	return b, w
}

func TestRustLevelToGoLevel(t *testing.T) {
	tests := []struct {
		rustLevel uint32
		expected  log.LogLevel
	}{
		{1, log.ErrorLvl},
		{2, log.WarnLvl},
		{3, log.InfoLvl},
		{4, log.DebugLvl},
		{5, log.TraceLvl},
		{99, log.TraceLvl}, // values above 5 default to Trace
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, rustLevelToGoLevel(tt.rustLevel))
	}
}

func TestHandleDiscoveryLog_LevelMapping(t *testing.T) {
	tests := []struct {
		name          string
		rustLevel     uint32
		expectedLevel string
	}{
		{"error", 1, "[ERROR]"},
		{"warn", 2, "[WARN]"},
		{"info", 3, "[INFO]"},
		{"debug", 4, "[DEBUG]"},
		{"trace", 5, "[TRACE]"},
		{"above trace defaults to trace", 99, "[TRACE]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, w := setupLogCapture(t, log.TraceLvl)
			handleDiscoveryLog(tt.rustLevel, "hello from rust")
			w.Flush()
			out := b.String()
			assert.Contains(t, out, tt.expectedLevel)
			assert.Contains(t, out, "[dd_discovery] hello from rust")
		})
	}
}

func TestHandleDiscoveryLog_DropsRecordBelowConfiguredLevel(t *testing.T) {
	b, w := setupLogCapture(t, log.InfoLvl)
	handleDiscoveryLog(4, "should not appear") // debug
	handleDiscoveryLog(5, "should not appear") // trace
	w.Flush()
	assert.Empty(t, b.String())
}

func TestHandleDiscoveryLog_PassesRecordAtOrAboveConfiguredLevel(t *testing.T) {
	tests := []struct {
		name      string
		rustLevel uint32
	}{
		{"error passes at info", 1},
		{"warn passes at info", 2},
		{"info passes at info", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, w := setupLogCapture(t, log.InfoLvl)
			handleDiscoveryLog(tt.rustLevel, "should appear")
			w.Flush()
			assert.Contains(t, b.String(), "[dd_discovery] should appear")
		})
	}
}

func TestHandleDiscoveryLog_EmptyMessage(t *testing.T) {
	// The C boundary guards against msgLen == 0, so an empty string is unreachable
	// from the real FFI path. This test documents that the Go bridge passes it through
	// unchanged rather than silently dropping it — an info-level record with an empty
	// body is still emitted, not replaced or dropped.
	b, w := setupLogCapture(t, log.InfoLvl)
	handleDiscoveryLog(3, "") // info level, empty body
	w.Flush()
	out := b.String()
	assert.Contains(t, out, "[INFO]")
	assert.Regexp(t, `\[dd_discovery\]\s*\n?$`, out)
}

func TestGoLevelToRust(t *testing.T) {
	tests := []struct {
		name     string
		level    log.LogLevel
		expected uint32
	}{
		{"error", log.ErrorLvl, 1},
		{"critical", log.CriticalLvl, 1},
		{"warn", log.WarnLvl, 2},
		{"info", log.InfoLvl, 3},
		{"debug", log.DebugLvl, 4},
		{"trace", log.TraceLvl, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupLogCapture(t, tt.level)
			assert.Equal(t, tt.expected, goLevelToRust())
		})
	}
}
