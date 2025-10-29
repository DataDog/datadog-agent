// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strings"

	template "github.com/DataDog/datadog-agent/pkg/template/text"
	"github.com/DataDog/datadog-agent/pkg/util/log/formatters"
	"github.com/DataDog/datadog-agent/pkg/util/log/handlers"
)

// LoggerInterface provides basic logging methods.
type LoggerInterface interface {
	// Tracef formats message according to format specifier
	// and writes to log with level = Trace.
	Tracef(format string, params ...interface{})

	// Debugf formats message according to format specifier
	// and writes to log with level = Debug.
	Debugf(format string, params ...interface{})

	// Infof formats message according to format specifier
	// and writes to log with level = Info.
	Infof(format string, params ...interface{})

	// Warnf formats message according to format specifier
	// and writes to log with level = Warn.
	Warnf(format string, params ...interface{}) error

	// Errorf formats message according to format specifier
	// and writes to log with level = Error.
	Errorf(format string, params ...interface{}) error

	// Criticalf formats message according to format specifier
	// and writes to log with level = Critical.
	Criticalf(format string, params ...interface{}) error

	// Trace formats message using the default formats for its operands
	// and writes to log with level = Trace
	Trace(v ...interface{})

	// Debug formats message using the default formats for its operands
	// and writes to log with level = Debug
	Debug(v ...interface{})

	// Info formats message using the default formats for its operands
	// and writes to log with level = Info
	Info(v ...interface{})

	// Warn formats message using the default formats for its operands
	// and writes to log with level = Warn
	Warn(v ...interface{}) error

	// Error formats message using the default formats for its operands
	// and writes to log with level = Error
	Error(v ...interface{}) error

	// Critical formats message using the default formats for its operands
	// and writes to log with level = Critical
	Critical(v ...interface{}) error
}

// defaultFormatter is the default format used by seelog
func defaultFormatter(_ context.Context, r slog.Record) string {
	return fmt.Sprintf("%d [%s] %s\n", r.Time.UnixNano(), LogLevel(r.Level), r.Message)
}

// Default returns a default logger
func Default() *SlogWrapper {
	formatHandler := handlers.NewFormatHandler(defaultFormatter, os.Stdout)

	var levelVar slog.LevelVar
	levelHandler := handlers.NewLevelHandler(&levelVar, formatHandler)

	asyncHandler := handlers.NewAsyncHandler(levelHandler)

	return NewSlogWrapper(slog.New(asyncHandler), 0, &levelVar, asyncHandler.Flush, asyncHandler.Close)
}

// Disabled returns a disabled logger
func Disabled() *SlogWrapper {
	return NewSlogWrapperFixedLevel(slog.New(handlers.NewDisabledHandler()), 0, Off, nil, nil)
}

// LoggerFromWriterWithMinLevelAndFormat creates a new logger from a writer, a minimum log level and a format.
func LoggerFromWriterWithMinLevelAndFormat(output io.Writer, minLevel LogLevel, formatter func(context.Context, slog.Record) string) (*SlogWrapper, error) {
	handler := handlers.NewLevelHandler(slog.Level(minLevel), handlers.NewFormatHandler(formatter, output))
	return NewSlogWrapper(slog.New(handler), 0, nil, nil, nil), nil
}

// LoggerFromWriterWithMinLevel creates a new logger from a writer and a minimum log level.
func LoggerFromWriterWithMinLevel(output io.Writer, minLevel LogLevel) (*SlogWrapper, error) {
	return LoggerFromWriterWithMinLevelAndFormat(output, minLevel, defaultFormatter)
}

func TemplateFormatter(tmpl string) func(context.Context, slog.Record) string {
	return func(_ context.Context, r slog.Record) string {
		frames := runtime.CallersFrames([]uintptr{r.PC})
		frame, _ := frames.Next()
		context := map[string]interface{}{
			"record": r,
			"time":   r.Time,
			"level":  LogLevel(r.Level),
			"l":      LogLevel(r.Level).String()[0],
			"msg":    r.Message,
			"frame":  frame,
			"line":   frame.Line,
			"func":   frame.Function,
			"file":   frame.File,
		}
		funcs := template.FuncMap{
			"Date":     func(format string) string { return r.Time.Format(format) },
			"DateTime": func() string { return r.Time.Format("2006-01-02 15:04:05.000 MST") },
			"Ns":       func() int64 { return r.Time.UnixNano() },
			"Level":    func() string { return LogLevel(r.Level).Capitalized() },
			"LEVEL":    func() string { return LogLevel(r.Level).Uppercase() },

			"ToUpper": strings.ToUpper,
			"Quote":   formatters.Quote,

			"FuncShort":        func() string { return formatters.ShortFunction(frame) },
			"ShortFile":        func() string { return formatters.ShortFilePath(frame) },
			"RelFile":          func() string { return formatters.ShortFilePath(frame) },
			"ExtraTextContext": func() string { return formatters.ExtraTextContext(r) },
			"ExtraJSONContext": func() string { return formatters.ExtraJSONContext(r) },
		}

		var buff bytes.Buffer
		_ = template.New("").Funcs(funcs).Execute(&buff, context)
		return buff.String()
	}
}
