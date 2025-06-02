// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import "github.com/cihub/seelog"

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

// LogLevelFromString returns a LogLevel from a string
//
//nolint:revive // keeping the original function name from seelog
func LogLevelFromString(levelStr string) (LogLevel, bool) {
	level, ok := seelog.LogLevelFromString(levelStr)
	return LogLevel(level), ok
}
