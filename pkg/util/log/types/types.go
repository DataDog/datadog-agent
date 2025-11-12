// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types contains the types for the log package.
package types

import (
	"fmt"
	"log/slog"

	"github.com/cihub/seelog"
)

// LoggerInterface provides basic logging methods that can be used from outside the log package.
type LoggerInterface interface {
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

	// Close flushes all the messages in the logger and closes it. It cannot be used after this operation.
	Close()

	// Flush flushes all the messages in the logger.
	Flush()

	// SetAdditionalStackDepth sets the additional number of frames to skip by runtime.Caller
	SetAdditionalStackDepth(depth int) error

	// Sets logger context that can be used in formatter funcs and custom receivers
	SetContext(context interface{})

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
}

// LogLevel is the type of log levels
//
//nolint:revive // keeping the original type name from seelog
type LogLevel seelog.LogLevel

// Log levels
const (
	TraceLvl    LogLevel = seelog.TraceLvl
	DebugLvl    LogLevel = seelog.DebugLvl
	InfoLvl     LogLevel = seelog.InfoLvl
	WarnLvl     LogLevel = seelog.WarnLvl
	ErrorLvl    LogLevel = seelog.ErrorLvl
	CriticalLvl LogLevel = seelog.CriticalLvl
	Off         LogLevel = seelog.Off
)

// Log level string representations
const (
	TraceStr    = seelog.TraceStr
	DebugStr    = seelog.DebugStr
	InfoStr     = seelog.InfoStr
	WarnStr     = seelog.WarnStr
	ErrorStr    = seelog.ErrorStr
	CriticalStr = seelog.CriticalStr
	OffStr      = seelog.OffStr
)

func (level LogLevel) String() string {
	return seelog.LogLevel(level).String()
}

const (
	slogTraceLvl    slog.Level = slog.LevelDebug - 4
	slogDebugLvl    slog.Level = slog.LevelDebug
	slogInfoLvl     slog.Level = slog.LevelInfo
	slogWarnLvl     slog.Level = slog.LevelWarn
	slogErrorLvl    slog.Level = slog.LevelError
	slogCriticalLvl slog.Level = slog.LevelError + 4
	slogOff         slog.Level = slog.LevelError + 8
)

// ToSlogLevel converts a LogLevel to a slog.Level
func ToSlogLevel(level LogLevel) slog.Level {
	switch level {
	case TraceLvl:
		return slogTraceLvl
	case DebugLvl:
		return slogDebugLvl
	case InfoLvl:
		return slogInfoLvl
	case WarnLvl:
		return slogWarnLvl
	case ErrorLvl:
		return slogErrorLvl
	case CriticalLvl:
		return slogCriticalLvl
	case Off:
		return slogOff
	default:
		// unreachable
		panic(fmt.Sprintf("unknown log level: %d", level))
	}
}

// FromSlogLevel converts a slog.Level to a LogLevel
func FromSlogLevel(level slog.Level) LogLevel {
	switch level {
	case slogTraceLvl:
		return TraceLvl
	case slogDebugLvl:
		return DebugLvl
	case slogInfoLvl:
		return InfoLvl
	case slogWarnLvl:
		return WarnLvl
	case slogErrorLvl:
		return ErrorLvl
	case slogCriticalLvl:
		return CriticalLvl
	case slogOff:
		return Off
	default:
		// unreachable
		panic(fmt.Sprintf("unknown slog log level: %d", level))
	}
}
