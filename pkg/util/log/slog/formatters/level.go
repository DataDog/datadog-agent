// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package formatters

import (
	"log/slog"

	"github.com/DataDog/datadog-agent/pkg/util/log/types"
)

// LevelToString converts a slog.Level to a string
func LevelToString(level slog.Level) string {
	return types.FromSlogLevel(level).String()
}

// https://github.com/cihub/seelog/blob/f561c5e57575bb1e0a2167028b7339b3a8d16fb4/format.go#L314
const wrongLogLevel = "WRONG_LOGLEVEL"

// ShortLevel converts a slog.Level to a short string
//
// See https://github.com/cihub/seelog/blob/f561c5e57575bb1e0a2167028b7339b3a8d16fb4/format.go#L328
func ShortLevel(level slog.Level) string {
	switch types.FromSlogLevel(level) {
	case types.TraceLvl:
		return "Trc"
	case types.DebugLvl:
		return "Dbg"
	case types.InfoLvl:
		return "Inf"
	case types.WarnLvl:
		return "Wrn"
	case types.ErrorLvl:
		return "Err"
	case types.CriticalLvl:
		return "Crt"
	case types.Off:
		return "Off"
	default:
		return wrongLogLevel
	}
}

// CapitalizedLevel returns a capitalized string representation of the log level
//
// See https://github.com/cihub/seelog/blob/f561c5e57575bb1e0a2167028b7339b3a8d16fb4/format.go#L318
func CapitalizedLevel(level slog.Level) string {
	switch types.FromSlogLevel(level) {
	case types.TraceLvl:
		return "Trace"
	case types.DebugLvl:
		return "Debug"
	case types.InfoLvl:
		return "Info"
	case types.WarnLvl:
		return "Warn"
	case types.ErrorLvl:
		return "Error"
	case types.CriticalLvl:
		return "Critical"
	case types.Off:
		return "Off"
	default:
		return wrongLogLevel
	}
}

// UppercaseLevel returns an uppercase string representation of the log level
//
// See https://github.com/cihub/seelog/blob/f561c5e57575bb1e0a2167028b7339b3a8d16fb4/format.go#L365
func UppercaseLevel(level slog.Level) string {
	switch types.FromSlogLevel(level) {
	case types.TraceLvl:
		return "TRACE"
	case types.DebugLvl:
		return "DEBUG"
	case types.InfoLvl:
		return "INFO"
	case types.WarnLvl:
		return "WARN"
	case types.ErrorLvl:
		return "ERROR"
	case types.CriticalLvl:
		return "CRITICAL"
	case types.Off:
		return "OFF"
	default:
		return wrongLogLevel
	}
}

// ShortestLevel returns a single character representation of the log level
//
// https://github.com/cihub/seelog/blob/f561c5e57575bb1e0a2167028b7339b3a8d16fb4/format.go#L338
func ShortestLevel(level slog.Level) string {
	switch types.FromSlogLevel(level) {
	case types.TraceLvl:
		return "t"
	case types.DebugLvl:
		return "d"
	case types.InfoLvl:
		return "i"
	case types.WarnLvl:
		return "w"
	case types.ErrorLvl:
		return "e"
	case types.CriticalLvl:
		return "c"
	case types.Off:
		return "o"
	default:
		return wrongLogLevel
	}
}
