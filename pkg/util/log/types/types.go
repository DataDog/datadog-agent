// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types contains the types for the log package.
package types

import (
	"fmt"
	"log/slog"
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
type LogLevel slog.Level

// Log levels
const (
	TraceLvl    LogLevel = LogLevel(slog.LevelDebug - 4)
	DebugLvl    LogLevel = LogLevel(slog.LevelDebug)
	InfoLvl     LogLevel = LogLevel(slog.LevelInfo)
	WarnLvl     LogLevel = LogLevel(slog.LevelWarn)
	ErrorLvl    LogLevel = LogLevel(slog.LevelError)
	CriticalLvl LogLevel = LogLevel(slog.LevelError + 4)
	Off         LogLevel = LogLevel(slog.LevelError + 8)
)

// Log level string representations
const (
	TraceStr    = "trace"
	DebugStr    = "debug"
	InfoStr     = "info"
	WarnStr     = "warn"
	ErrorStr    = "error"
	CriticalStr = "critical"
	OffStr      = "off"
)

func (level LogLevel) String() string {
	switch level {
	case TraceLvl:
		return TraceStr
	case DebugLvl:
		return DebugStr
	case InfoLvl:
		return InfoStr
	case WarnLvl:
		return WarnStr
	case ErrorLvl:
		return ErrorStr
	case CriticalLvl:
		return CriticalStr
	case Off:
		return OffStr
	default:
		return ""
	}
}

// ToSlogLevel converts a LogLevel to a slog.Level
func ToSlogLevel(level LogLevel) slog.Level {
	switch level {
	case TraceLvl:
		return slog.Level(TraceLvl)
	case DebugLvl:
		return slog.Level(DebugLvl)
	case InfoLvl:
		return slog.Level(InfoLvl)
	case WarnLvl:
		return slog.Level(WarnLvl)
	case ErrorLvl:
		return slog.Level(ErrorLvl)
	case CriticalLvl:
		return slog.Level(CriticalLvl)
	case Off:
		return slog.Level(Off)
	default:
		// unreachable
		panic(fmt.Sprintf("unknown log level: %d", level))
	}
}

// FromSlogLevel converts a slog.Level to a LogLevel
func FromSlogLevel(level slog.Level) LogLevel {
	switch level {
	case slog.Level(TraceLvl):
		return TraceLvl
	case slog.Level(DebugLvl):
		return DebugLvl
	case slog.Level(InfoLvl):
		return InfoLvl
	case slog.Level(WarnLvl):
		return WarnLvl
	case slog.Level(ErrorLvl):
		return ErrorLvl
	case slog.Level(CriticalLvl):
		return CriticalLvl
	case slog.Level(Off):
		return Off
	default:
		// unreachable
		panic(fmt.Sprintf("unknown slog log level: %d", level))
	}
}
