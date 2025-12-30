// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package log

import (
	"bufio"
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/log/slog"
	"github.com/DataDog/datadog-agent/pkg/util/log/slog/formatters"
	"github.com/DataDog/datadog-agent/pkg/util/log/slog/handlers"
	"github.com/DataDog/datadog-agent/pkg/util/log/types"
)

func TestZapBasicLogging(t *testing.T) {
	logger := zap.New(NewZapCore())
	tests := []struct {
		desc    string
		log     func(*zap.Logger)
		level   types.LogLevel
		message string
	}{
		{
			desc:    "Debug (no fields, debug level)",
			log:     func(l *zap.Logger) { l.Debug("Simple message") },
			level:   types.DebugLvl,
			message: "[DEBUG] | pkg/util/log/zap/zapcore_test.go | Simple message",
		},
		{
			desc:    "Info (no fields, debug level)",
			log:     func(l *zap.Logger) { l.Info("Simple message") },
			level:   types.DebugLvl,
			message: "[INFO] | pkg/util/log/zap/zapcore_test.go | Simple message",
		},
		{
			desc:    "Warn (no fields, debug level)",
			log:     func(l *zap.Logger) { l.Warn("Simple message") },
			level:   types.DebugLvl,
			message: "[WARN] | pkg/util/log/zap/zapcore_test.go | Simple message",
		},
		{
			desc:    "Error (no fields, debug level)",
			log:     func(l *zap.Logger) { l.Error("Simple message") },
			level:   types.DebugLvl,
			message: "[ERROR] | pkg/util/log/zap/zapcore_test.go | Simple message",
		},
		{
			desc:    "DPanic (no fields, debug level)",
			log:     func(l *zap.Logger) { l.DPanic("Development panic") },
			level:   types.DebugLvl,
			message: "[CRITICAL] | pkg/util/log/zap/zapcore_test.go | Development panic",
		},
		{
			desc: "Error level",
			log: func(l *zap.Logger) {
				l.Debug("Simple message")
				l.Info("Simple message")
				l.Warn("Simple message")
			},
			level:   types.ErrorLvl,
			message: "",
		},
		{
			desc:    "Info (fields)",
			log:     func(l *zap.Logger) { l.Info("Fields", zap.Int("int", 1), zap.String("key", "val")) },
			level:   types.DebugLvl,
			message: "[INFO] | pkg/util/log/zap/zapcore_test.go | int:1,key:val | Fields",
		},
		{
			desc:    "Error (fields)",
			log:     func(l *zap.Logger) { l.Error("Fields", zap.Error(errors.New("an error"))) },
			level:   types.DebugLvl,
			message: "[ERROR] | pkg/util/log/zap/zapcore_test.go | error:an error | Fields",
		},
		{
			desc: "With (using original)",
			log: func(l *zap.Logger) {
				_ = l.With(zap.Int16("int", 1))
				l.Info("Fields", zap.Bool("bool", true))
			},
			level:   types.DebugLvl,
			message: "[INFO] | pkg/util/log/zap/zapcore_test.go | bool:true | Fields",
		},
		{
			desc: "With (using new)",
			log: func(l *zap.Logger) {
				extra := l.With(zap.Int16("int", 1))
				extra.Info("Fields", zap.Bool("bool", true))
			},
			level:   types.DebugLvl,
			message: "[INFO] | pkg/util/log/zap/zapcore_test.go | int:1,bool:true | Fields",
		},
		{
			desc:    "Namespace",
			log:     func(l *zap.Logger) { l.Info("Fields", zap.Namespace("ns"), zap.Int("int", 1)) },
			level:   types.DebugLvl,
			message: "[INFO] | pkg/util/log/zap/zapcore_test.go | ns/int:1 | Fields",
		},
	}

	for _, testInstance := range tests {
		t.Run(testInstance.desc, func(t *testing.T) {
			var b bytes.Buffer
			w := bufio.NewWriter(&b)
			fmtFunc := formatters.Template("[{{LEVEL}}] | {{ShortFilePath}} | {{ExtraTextContext}}{{.msg}}")
			handler := handlers.NewFormat(fmtFunc, w)
			levelHandler := handlers.NewLevel(types.ToSlogLevel(testInstance.level), handler)
			l := slog.NewWrapper(levelHandler)
			log.SetupLogger(l, testInstance.level.String())
			require.NotNil(t, logger)

			testInstance.log(logger)
			w.Flush()
			assert.Equal(t, testInstance.message, b.String())
		})
	}
}
