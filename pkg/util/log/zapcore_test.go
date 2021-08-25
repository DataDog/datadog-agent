// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

// skip go vet since we need functions from log_test.go
// +build !dovet

package log

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestZapBasicLogging(t *testing.T) {
	logger := NewZapLogger()
	tests := []struct {
		desc    string
		f       func(*zap.Logger)
		level   string
		pattern string
	}{
		{
			desc:    "Debug (no fields, debug level)",
			f:       func(l *zap.Logger) { l.Debug("Simple message") },
			level:   "debug",
			pattern: "\\[DEBUG\\] Write: \\(log/zapcore_test.go:\\d+\\) \\| Simple message",
		},
		{
			desc:    "Info (no fields, debug level)",
			f:       func(l *zap.Logger) { l.Info("Simple message") },
			level:   "debug",
			pattern: "\\[INFO\\] Write: \\(log/zapcore_test.go:\\d+\\) \\| Simple message",
		},
		{
			desc:    "Warn (no fields, debug level)",
			f:       func(l *zap.Logger) { l.Warn("Simple message") },
			level:   "debug",
			pattern: "\\[WARN\\] Write: \\(log/zapcore_test.go:\\d+\\) \\| Simple message",
		},
		{
			desc:    "Error (no fields, debug level)",
			f:       func(l *zap.Logger) { l.Error("Simple message") },
			level:   "debug",
			pattern: "\\[ERROR\\] Write: \\(log/zapcore_test.go:\\d+\\) \\| Simple message",
		},
		{
			desc:    "DPanic (no fields, debug level)",
			f:       func(l *zap.Logger) { l.DPanic("Development panic") },
			level:   "debug",
			pattern: "\\[CRITICAL\\] Write: \\(log/zapcore_test.go:\\d+\\) \\| Development panic",
		},
		{
			desc: "Error level",
			f: func(l *zap.Logger) {
				l.Debug("Simple message")
				l.Info("Simple message")
				l.Warn("Simple message")
			},
			level:   "error",
			pattern: "",
		},
		{
			desc:    "Info (fields)",
			f:       func(l *zap.Logger) { l.Info("Fields", zap.Int("int", 1), zap.String("key", "val")) },
			level:   "debug",
			pattern: "\\[INFO\\] Write: int:1, key:val \\| \\(log/zapcore_test.go:\\d+\\) \\| Fields",
		},
		{
			desc:    "Error (fields)",
			f:       func(l *zap.Logger) { l.Error("Fields", zap.Error(fmt.Errorf("an error"))) },
			level:   "debug",
			pattern: "\\[ERROR\\] Write: error:an error \\| \\(log/zapcore_test.go:\\d+\\) \\| Fields",
		},
		{
			desc: "With (using original)",
			f: func(l *zap.Logger) {
				_ = l.With(zap.Int16("int", 1))
				l.Info("Fields", zap.Bool("bool", true))
			},
			level:   "debug",
			pattern: "\\[INFO\\] Write: bool:true \\| \\(log/zapcore_test.go:\\d+\\) \\| Fields",
		},
		{
			desc: "With (using new)",
			f: func(l *zap.Logger) {
				extra := l.With(zap.Int16("int", 1))
				extra.Info("Fields", zap.Bool("bool", true))
			},
			level:   "debug",
			pattern: "\\[INFO\\] Write: int:1, bool:true \\| \\(log/zapcore_test.go:\\d+\\) \\| Fields",
		},
		{
			desc:    "Namespace",
			f:       func(l *zap.Logger) { l.Info("Fields", zap.Namespace("ns"), zap.Int("int", 1)) },
			level:   "debug",
			pattern: "\\[INFO\\] Write: ns/int:1 \\| \\(log/zapcore_test.go:\\d+\\) \\| Fields",
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			var b bytes.Buffer
			w := bufio.NewWriter(&b)
			seelog.RegisterCustomFormatter("ExtraTextContext", createExtraTextContext)
			l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %ExtraTextContext%Msg\n")
			require.Nil(t, err)
			SetupLogger(l, tt.level)
			require.NotNil(t, logger)

			tt.f(logger)
			w.Flush()
			pattern := fmt.Sprintf("^%s$", tt.pattern)
			assert.Regexp(t, pattern, strings.TrimSuffix(b.String(), "\n"))
		})
	}
}
