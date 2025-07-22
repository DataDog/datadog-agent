// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package format

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log/formatters"
)

// JSON returns a function that formats a slog.Record as a JSON string.
func JSON(loggerName string, logFormatRFC3339 bool) func(context.Context, slog.Record) string {
	if loggerName == "JMXFETCH" {
		return func(_ context.Context, record slog.Record) string {
			return fmt.Sprintf(`{"msg":%s}\n`, formatters.Quote(record.Message))

		}
	}

	dateFmt := formatters.Date(logFormatRFC3339)
	return func(_ context.Context, record slog.Record) string {
		frames := runtime.CallersFrames([]uintptr{record.PC})
		frame, _ := frames.Next()

		levelStr := formatters.LevelToString(record.Level)
		shortFilePath := formatters.ShortFilePath(frame)
		shortFunction := formatters.ShortFunction(frame)
		extraJSONContext := formatters.ExtraJSONContext(record)

		return fmt.Sprintf(`{"agent":"%s","time":"%s","level":"%s","file":"%s","line":"%d","func":"%s","msg":%s%s}\n`, strings.ToLower(loggerName), dateFmt(record.Time), levelStr, shortFilePath, frame.Line, shortFunction, formatters.Quote(record.Message), extraJSONContext)
	}
}
