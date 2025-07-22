// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import "github.com/DataDog/datadog-agent/pkg/util/log/types"

// LogLevel is the type of log levels
//
//nolint:revive // keeping the original type name from seelog
type LogLevel = types.LogLevel

// Log levels
const (
	TraceLvl    LogLevel = types.TraceLvl
	DebugLvl    LogLevel = types.DebugLvl
	InfoLvl     LogLevel = types.InfoLvl
	WarnLvl     LogLevel = types.WarnLvl
	ErrorLvl    LogLevel = types.ErrorLvl
	CriticalLvl LogLevel = types.CriticalLvl
	Off         LogLevel = types.Off
)

// Log level string representations
const (
	TraceStr    = types.TraceStr
	DebugStr    = types.DebugStr
	InfoStr     = types.InfoStr
	WarnStr     = types.WarnStr
	ErrorStr    = types.ErrorStr
	CriticalStr = types.CriticalStr
	OffStr      = types.OffStr
)

// LogLevelFromString returns a LogLevel from a string
//
//nolint:revive // keeping the original function name from seelog
func LogLevelFromString(levelStr string) (LogLevel, bool) {
	switch levelStr {
	case TraceStr:
		return TraceLvl, true
	case DebugStr:
		return DebugLvl, true
	case InfoStr:
		return InfoLvl, true
	case WarnStr:
		return WarnLvl, true
	case ErrorStr:
		return ErrorLvl, true
	case CriticalStr:
		return CriticalLvl, true
	case OffStr:
		return Off, true
	default:
		return 0, false
	}
}
