// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"bufio"
	"bytes"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newBufferedLogger returns a trace level logger writing to b, and its writer to flush.
func newBufferedLogger(t *testing.T, b *bytes.Buffer) (LoggerInterface, *bufio.Writer) {
	t.Helper()

	w := bufio.NewWriter(b)
	l, err := LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, TraceLvl)
	require.NoError(t, err)

	return l, w
}

// replayBuffered emits a log entry while no logger is set up, then initializes one and returns
// what the buffered entry produced once replayed.
func replayBuffered(t *testing.T, emit func()) string {
	t.Helper()

	logsBuffer = []func(){}
	logger.Store(nil)

	emit()
	require.Len(t, logsBuffer, 1, "the log entry should have been buffered")

	var b bytes.Buffer
	l, w := newBufferedLogger(t, &b)

	SetupLogger(l, TraceStr)
	w.Flush()

	return b.String()
}

// TestBufferedLogsKeepCallerPosition checks that a log entry emitted before the logger is set
// up is replayed with the position of its original call site, and not of the flush loop.
//
// It covers every combination of level and helper family, because the position is derived from
// a frame count that depends on both: 'depth' does not mean the same thing for the log and the
// logContext families, see bufferLog.
func TestBufferedLogsKeepCallerPosition(t *testing.T) {
	tests := []struct {
		name string
		// wantMsg is the message expected next to the position, "hello" when empty
		wantMsg string
		// emit logs and returns the line its call site is expected to point at
		emit func() int
	}{
		// Plain functions: the direct caller is the call site.
		{name: "Trace", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			Trace("hello")
			return line + 1
		}},
		{name: "Debug", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			Debug("hello")
			return line + 1
		}},
		{name: "Info", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			Info("hello")
			return line + 1
		}},
		{name: "Warn", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			_ = Warn("hello")
			return line + 1
		}},
		{name: "Error", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			_ = Error("hello")
			return line + 1
		}},
		{name: "Critical", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			_ = Critical("hello")
			return line + 1
		}},

		// Format functions.
		{name: "Tracef", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			Tracef("%s", "hello")
			return line + 1
		}},
		{name: "Debugf", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			Debugf("%s", "hello")
			return line + 1
		}},
		{name: "Infof", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			Infof("%s", "hello")
			return line + 1
		}},
		{name: "Warnf", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			_ = Warnf("%s", "hello")
			return line + 1
		}},
		{name: "Errorf", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			_ = Errorf("%s", "hello")
			return line + 1
		}},
		{name: "Criticalf", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			_ = Criticalf("%s", "hello")
			return line + 1
		}},
		// The message is formatted once, when the entry is buffered: a verb coming from the
		// parameters must not be expanded again at replay time.
		{name: "Warnf with a verb in a parameter", wantMsg: "hello %d", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			_ = Warnf("hello %s", "%d")
			return line + 1
		}},

		// Context functions, they go through the logContext family.
		{name: "Tracec", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			Tracec("hello", "key", "value")
			return line + 1
		}},
		{name: "Debugc", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			Debugc("hello", "key", "value")
			return line + 1
		}},
		{name: "Infoc", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			Infoc("hello", "key", "value")
			return line + 1
		}},
		{name: "Warnc", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			_ = Warnc("hello", "key", "value")
			return line + 1
		}},
		{name: "Errorc", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			_ = Errorc("hello", "key", "value")
			return line + 1
		}},
		{name: "Criticalc", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			_ = Criticalc("hello", "key", "value")
			return line + 1
		}},

		// *StackDepth functions called directly: depth 1 designates the direct caller.
		{name: "TraceStackDepth", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			TraceStackDepth(1, "hello")
			return line + 1
		}},
		{name: "DebugStackDepth", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			DebugStackDepth(1, "hello")
			return line + 1
		}},
		{name: "InfoStackDepth", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			InfoStackDepth(1, "hello")
			return line + 1
		}},
		{name: "WarnStackDepth", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			_ = WarnStackDepth(1, "hello")
			return line + 1
		}},
		{name: "ErrorStackDepth", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			_ = ErrorStackDepth(1, "hello")
			return line + 1
		}},
		{name: "CriticalStackDepth", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			_ = CriticalStackDepth(1, "hello")
			return line + 1
		}},

		// *fStackDepth functions called directly. Trace and Debug are missing on purpose: they
		// are dropped before being buffered, as GetLogLevel defaults to info when the logger is
		// not set up yet.
		{name: "InfofStackDepth", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			InfofStackDepth(1, "%s", "hello")
			return line + 1
		}},
		{name: "WarnfStackDepth", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			_ = WarnfStackDepth(1, "%s", "hello")
			return line + 1
		}},
		{name: "ErrorfStackDepth", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			_ = ErrorfStackDepth(1, "%s", "hello")
			return line + 1
		}},
		{name: "CriticalfStackDepth", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			_ = CriticalfStackDepth(1, "%s", "hello")
			return line + 1
		}},

		// *cStackDepth functions called directly: unlike the log family, depth 0 designates the
		// direct caller there.
		{name: "InfocStackDepth", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			InfocStackDepth("hello", 0, "key", "value")
			return line + 1
		}},
		{name: "WarncStackDepth", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			_ = WarncStackDepth("hello", 0, "key", "value")
			return line + 1
		}},

		// The *StackDepth functions exist to be called from a wrapper: the position must then
		// point at the caller of the wrapper, like the logger does once initialized.
		{name: "TraceStackDepth through a wrapper", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			traceWrapper("hello")
			return line + 1
		}},
		{name: "DebugStackDepth through a wrapper", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			debugWrapper("hello")
			return line + 1
		}},
		{name: "WarnfStackDepth through a wrapper", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			warnfWrapper("%s", "hello")
			return line + 1
		}},
		{name: "ErrorcStackDepth through a wrapper", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			errorcWrapper("hello")
			return line + 1
		}},
		{name: "CriticalcStackDepth through a wrapper", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			criticalcWrapper("hello")
			return line + 1
		}},

		// The *Func variants wrap *StackDepth with a depth of 2. Trace and Debug are missing for
		// the same reason as the *fStackDepth ones.
		{name: "InfoFunc", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			InfoFunc(func() string { return "hello" })
			return line + 1
		}},
		{name: "WarnFunc", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			WarnFunc(func() string { return "hello" })
			return line + 1
		}},
		{name: "ErrorFunc", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			ErrorFunc(func() string { return "hello" })
			return line + 1
		}},
		{name: "CriticalFunc", emit: func() int {
			_, _, line, _ := runtime.Caller(0)
			CriticalFunc(func() string { return "hello" })
			return line + 1
		}},
	}

	_, file, _, _ := runtime.Caller(0)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wantMsg := tc.wantMsg
			if wantMsg == "" {
				wantMsg = "hello"
			}

			var line int
			out := replayBuffered(t, func() { line = tc.emit() })

			assert.Contains(t, out, fmt.Sprintf("%s:%d %s", file, line, wantMsg))
		})
	}
}

// TestLogsAfterSetupHaveNoCallerPosition checks that the position is only added to buffered
// entries: once the logger is initialized it reports the call site on its own, and prepending it
// to the message would duplicate it.
func TestLogsAfterSetupHaveNoCallerPosition(t *testing.T) {
	logsBuffer = []func(){}

	var b bytes.Buffer
	l, w := newBufferedLogger(t, &b)
	SetupLogger(l, TraceStr)

	Trace("hello")
	Debug("hello")
	Info("hello")
	_ = Warnf("%s", "hello")
	_ = Errorc("hello", "key", "value")
	InfoStackDepth(1, "hello")
	_ = CriticalfStackDepth(1, "%s", "hello")
	w.Flush()

	out := b.String()
	assert.Equal(t, 7, strings.Count(out, "hello"))
	assert.NotContains(t, out, ".go:")
}

func traceWrapper(v ...interface{}) {
	TraceStackDepth(2, v...)
}

func debugWrapper(v ...interface{}) {
	DebugStackDepth(2, v...)
}

func warnfWrapper(format string, params ...interface{}) {
	_ = WarnfStackDepth(2, format, params...)
}

func errorcWrapper(message string) {
	_ = ErrorcStackDepth(message, 1, "key", "value")
}

func criticalcWrapper(message string) {
	_ = CriticalcStackDepth(message, 1, "key", "value")
}
