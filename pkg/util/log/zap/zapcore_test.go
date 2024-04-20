// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

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

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// createExtraTextContext defines custom formatter for context logging on tests.
func createExtraTextContext(string) seelog.FormatterFunc {
	return func(_ string, _ seelog.LogLevel, context seelog.LogContextInterface) interface{} {
		contextList, _ := context.CustomContext().([]interface{})
		builder := strings.Builder{}
		for i := 0; i < len(contextList); i += 2 {
			builder.WriteString(fmt.Sprintf("%s:%v", contextList[i], contextList[i+1]))
			if i != len(contextList)-2 {
				builder.WriteString(", ")
			} else {
				builder.WriteString(" | ")
			}
		}
		return builder.String()
	}
}

func parseShortFilePath(_ string) seelog.FormatterFunc {
	return func(message string, level seelog.LogLevel, context seelog.LogContextInterface) interface{} {
		return extractShortPathFromFullPath(context.FullPath())
	}
}

func extractShortPathFromFullPath(fullPath string) string {
	// We want to trim the part containing the path of the project
	// ie DataDog/datadog-agent/ or DataDog/datadog-process-agent/
	slices := strings.Split(fullPath, "-agent/")
	return slices[len(slices)-1]
}

func TestZapBasicLogging(t *testing.T) {
	logger := zap.New(NewZapCore())
	tests := []struct {
		desc    string
		log     func(*zap.Logger)
		level   string
		message string
	}{
		{
			desc:    "Debug (no fields, debug level)",
			log:     func(l *zap.Logger) { l.Debug("Simple message") },
			level:   "debug",
			message: "[DEBUG] | pkg/util/log/zap/zapcore_test.go | Simple message",
		},
		{
			desc:    "Info (no fields, debug level)",
			log:     func(l *zap.Logger) { l.Info("Simple message") },
			level:   "debug",
			message: "[INFO] | pkg/util/log/zap/zapcore_test.go | Simple message",
		},
		{
			desc:    "Warn (no fields, debug level)",
			log:     func(l *zap.Logger) { l.Warn("Simple message") },
			level:   "debug",
			message: "[WARN] | pkg/util/log/zap/zapcore_test.go | Simple message",
		},
		{
			desc:    "Error (no fields, debug level)",
			log:     func(l *zap.Logger) { l.Error("Simple message") },
			level:   "debug",
			message: "[ERROR] | pkg/util/log/zap/zapcore_test.go | Simple message",
		},
		{
			desc:    "DPanic (no fields, debug level)",
			log:     func(l *zap.Logger) { l.DPanic("Development panic") },
			level:   "debug",
			message: "[CRITICAL] | pkg/util/log/zap/zapcore_test.go | Development panic",
		},
		{
			desc: "Error level",
			log: func(l *zap.Logger) {
				l.Debug("Simple message")
				l.Info("Simple message")
				l.Warn("Simple message")
			},
			level:   "error",
			message: "",
		},
		{
			desc:    "Info (fields)",
			log:     func(l *zap.Logger) { l.Info("Fields", zap.Int("int", 1), zap.String("key", "val")) },
			level:   "debug",
			message: "[INFO] | pkg/util/log/zap/zapcore_test.go | int:1, key:val | Fields",
		},
		{
			desc:    "Error (fields)",
			log:     func(l *zap.Logger) { l.Error("Fields", zap.Error(fmt.Errorf("an error"))) },
			level:   "debug",
			message: "[ERROR] | pkg/util/log/zap/zapcore_test.go | error:an error | Fields",
		},
		{
			desc: "With (using original)",
			log: func(l *zap.Logger) {
				_ = l.With(zap.Int16("int", 1))
				l.Info("Fields", zap.Bool("bool", true))
			},
			level:   "debug",
			message: "[INFO] | pkg/util/log/zap/zapcore_test.go | bool:true | Fields",
		},
		{
			desc: "With (using new)",
			log: func(l *zap.Logger) {
				extra := l.With(zap.Int16("int", 1))
				extra.Info("Fields", zap.Bool("bool", true))
			},
			level:   "debug",
			message: "[INFO] | pkg/util/log/zap/zapcore_test.go | int:1, bool:true | Fields",
		},
		{
			desc:    "Namespace",
			log:     func(l *zap.Logger) { l.Info("Fields", zap.Namespace("ns"), zap.Int("int", 1)) },
			level:   "debug",
			message: "[INFO] | pkg/util/log/zap/zapcore_test.go | ns/int:1 | Fields",
		},
	}

	for _, testInstance := range tests {
		t.Run(testInstance.desc, func(t *testing.T) {
			var b bytes.Buffer
			w := bufio.NewWriter(&b)
			seelog.RegisterCustomFormatter("ExtraTextContext", createExtraTextContext)
			seelog.RegisterCustomFormatter("ShortFilePath", parseShortFilePath)
			l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] | %ShortFilePath | %ExtraTextContext%Msg")
			require.NoError(t, err)
			log.SetupLogger(l, testInstance.level)
			require.NotNil(t, logger)

			testInstance.log(logger)
			w.Flush()
			assert.Equal(t, testInstance.message, b.String())
		})
	}
}
