// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types contains the types for the log package.
package types

import "log/slog"

// LogLevel is the type of log levels
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

var capitalized = map[LogLevel]string{
	TraceLvl:    "Trace",
	DebugLvl:    "Debug",
	InfoLvl:     "Info",
	WarnLvl:     "Warn",
	ErrorLvl:    "Error",
	CriticalLvl: "Critical",
	Off:         "Off",
}

var uppercased = map[LogLevel]string{
	TraceLvl:    "TRACE",
	DebugLvl:    "DEBUG",
	InfoLvl:     "INFO",
	WarnLvl:     "WARN",
	ErrorLvl:    "ERROR",
	CriticalLvl: "CRITICAL",
	Off:         "OFF",
}

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
		return "" // same behavior as previous implementation using seelog
	}
}

// Capitalized returns a capitalized string representation of the log level
// Avoids allocations when logging.
func (level LogLevel) Capitalized() string {
	return capitalized[level]
}

// Uppercase returns an uppercase string representation of the log level
// Avoids allocations when logging.
func (level LogLevel) Uppercase() string {
	return uppercased[level]
}
