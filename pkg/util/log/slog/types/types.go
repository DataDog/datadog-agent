// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types contains the types for the slog based implementation of the log package.
package types

import (
	"log/slog"
)

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

// String returns the string representation of the log level
func (l LogLevel) String() string {
	switch l {
	case TraceLvl:
		return "trace"
	case DebugLvl:
		return "debug"
	case InfoLvl:
		return "info"
	case WarnLvl:
		return "warn"
	case ErrorLvl:
		return "error"
	case CriticalLvl:
		return "critical"
	case Off:
		return "off"
	default:
		return "unknown"
	}
}

// Capitalized returns the capitalized string representation of the log level
func (l LogLevel) Capitalized() string {
	switch l {
	case TraceLvl:
		return "Trace"
	case DebugLvl:
		return "Debug"
	case InfoLvl:
		return "Info"
	case WarnLvl:
		return "Warn"
	case ErrorLvl:
		return "Error"
	case CriticalLvl:
		return "Critical"
	case Off:
		return "Off"
	default:
		return "Unknown"
	}
}

// Uppercase returns the uppercase string representation of the log level
func (l LogLevel) Uppercase() string {
	switch l {
	case TraceLvl:
		return "TRACE"
	case DebugLvl:
		return "DEBUG"
	case InfoLvl:
		return "INFO"
	case WarnLvl:
		return "WARN"
	case ErrorLvl:
		return "ERROR"
	case CriticalLvl:
		return "CRITICAL"
	case Off:
		return "OFF"
	default:
		return "UNKNOWN"
	}
}
