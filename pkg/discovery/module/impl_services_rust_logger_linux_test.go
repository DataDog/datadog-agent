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

// installCapturingLogger replaces the process-wide pkg/util/log logger with one
// that writes "[LEVEL] {{.msg}}\n" records to an in-memory buffer at the
// requested minimum level. On cleanup it installs a Disabled() logger, the
// same pattern comp/core/log/mock uses, since pkg/util/log does not expose a
// way to clear the singleton.
//
// Tests in this file must NOT call t.Parallel(): pkg/util/log.SetupLogger
// mutates a process-wide singleton.
func installCapturingLogger(t *testing.T, level log.LogLevel) (*bytes.Buffer, func()) {
	t.Helper()
	buf := &bytes.Buffer{}
	w := bufio.NewWriter(buf)
	l, err := log.LoggerFromWriterWithMinLevelAndLvlMsgFormat(w, level)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = w.Flush()
		log.SetupLogger(log.Disabled(), log.DebugStr)
	})

	log.SetupLogger(l, level.String())
	return buf, func() { _ = w.Flush() }
}

func TestDispatchDiscoveryLog_LevelMapping(t *testing.T) {
	tests := []struct {
		name          string
		rustLevel     uint32
		expectedLevel string
	}{
		{"zero routes to trace (out of range below)", 0, "[TRACE]"},
		{"error", 1, "[ERROR]"},
		{"warn", 2, "[WARN]"},
		{"info", 3, "[INFO]"},
		{"debug", 4, "[DEBUG]"},
		{"trace", 5, "[TRACE]"},
		{"above trace routes to trace", 99, "[TRACE]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf, flush := installCapturingLogger(t, log.TraceLvl)
			dispatchDiscoveryLog(tt.rustLevel, "hello from rust")
			flush()
			out := buf.String()
			assert.Contains(t, out, tt.expectedLevel)
			assert.Contains(t, out, "[dd_discovery] hello from rust")
		})
	}
}

func TestDispatchDiscoveryLog_DropsRecordBelowConfiguredLevel(t *testing.T) {
	buf, flush := installCapturingLogger(t, log.InfoLvl)
	dispatchDiscoveryLog(4, "should not appear") // debug
	dispatchDiscoveryLog(5, "should not appear") // trace
	dispatchDiscoveryLog(0, "should not appear") // 0 maps to trace via default
	flush()
	assert.Empty(t, buf.String())
}

func TestDispatchDiscoveryLog_PassesRecordAtOrAboveConfiguredLevel(t *testing.T) {
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
			buf, flush := installCapturingLogger(t, log.InfoLvl)
			dispatchDiscoveryLog(tt.rustLevel, "should appear")
			flush()
			assert.Contains(t, buf.String(), "[dd_discovery] should appear")
		})
	}
}

// TestDispatchDiscoveryLog_FormatVerbsAreLiteral ensures that a Rust-originated
// message that happens to contain Go fmt verbs (%s, %d, %[1]v, %%, etc.) is
// treated as data, not as a format string. The implementation passes the
// message as the *argument* to log.Errorf("[dd_discovery] %s", message), so
// the verbs in `message` must not be re-expanded.
func TestDispatchDiscoveryLog_FormatVerbsAreLiteral(t *testing.T) {
	tests := []struct {
		name    string
		level   uint32
		message string
	}{
		{"percent-s", 1, "user input contains %s here"},
		{"percent-d", 2, "value=%d is unsafe"},
		{"indexed-verb", 3, "first=%[1]v second=%[2]v"},
		{"double-percent", 4, "literal %% should survive"},
		{"format error placeholder", 5, "broken=%!s(MISSING)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf, flush := installCapturingLogger(t, log.TraceLvl)
			require.NotPanics(t, func() {
				dispatchDiscoveryLog(tt.level, tt.message)
			})
			flush()
			out := buf.String()
			assert.Contains(t, out, "[dd_discovery] "+tt.message,
				"format verbs must pass through verbatim")
			assert.NotContains(t, out, "%!(EXTRA",
				"verbs must not be re-interpreted as a format string")
		})
	}
}

func TestDispatchDiscoveryLog_UTF8AndNewlinesPassThroughUnchanged(t *testing.T) {
	tests := []struct {
		name    string
		message string
	}{
		{"utf8 multibyte", "héllo wörld - 日本語 - emoji"},
		{"emoji and accents", "héllo wörld 🐶 — naïve façade"},
		{"japanese", "こんにちは世界"},
		{"single newline", "line1\nline2"},
		{"multi-line", "alpha\nbeta\ngamma\n"},
		{"tab", "col1\tcol2\tcol3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf, flush := installCapturingLogger(t, log.InfoLvl)
			dispatchDiscoveryLog(3, tt.message)
			flush()
			assert.Contains(t, buf.String(), "[dd_discovery] "+tt.message)
		})
	}
}
