// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"io"

	"github.com/cihub/seelog"
)

// LoggerInterface provides basic logging methods.
type LoggerInterface seelog.LoggerInterface

// Default returns a default logger
func Default() LoggerInterface {
	return seelog.Default
}

// Disabled returns a disabled logger
func Disabled() LoggerInterface {
	return seelog.Disabled
}

// LoggerFromWriterWithMinLevelAndFormat creates a new logger from a writer, a minimum log level and a format.
func LoggerFromWriterWithMinLevelAndFormat(output io.Writer, minLevel LogLevel, format string) (LoggerInterface, error) {
	return seelog.LoggerFromWriterWithMinLevelAndFormat(output, seelog.LogLevel(minLevel), format)
}

// LoggerFromWriterWithMinLevel creates a new logger from a writer and a minimum log level.
func LoggerFromWriterWithMinLevel(output io.Writer, minLevel LogLevel) (LoggerInterface, error) {
	return seelog.LoggerFromWriterWithMinLevel(output, seelog.LogLevel(minLevel))
}
