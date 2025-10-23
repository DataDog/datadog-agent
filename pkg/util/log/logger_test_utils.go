// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package log

import (
	"io"

	"github.com/cihub/seelog"
)

// loggerFromWriterWithMinLevelAndFormat creates a new logger from a writer, a minimum log level and a format.
func loggerFromWriterWithMinLevelAndFormat(output io.Writer, minLevel LogLevel, format string) (LoggerInterface, error) {
	return seelog.LoggerFromWriterWithMinLevelAndFormat(output, seelog.LogLevel(minLevel), format)
}

// LoggerFromWriterWithMinLevel creates a new logger from a writer and a minimum log level.
func LoggerFromWriterWithMinLevel(output io.Writer, minLevel LogLevel) (LoggerInterface, error) {
	return seelog.LoggerFromWriterWithMinLevel(output, seelog.LogLevel(minLevel))
}

// LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat creates a new logger from a writer, a minimum log level and the short format "[%LEVEL] %FuncShort: %Msg"
func LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(output io.Writer, minLevel LogLevel) (LoggerInterface, error) {
	return loggerFromWriterWithMinLevelAndFormat(output, minLevel, "[%LEVEL] %FuncShort: %Msg%n")
}

// LoggerFromWriterWithMinLevelAndLvlMsgFormat creates a new logger from a writer, a minimum log level and the message format "[%LEVEL] %Msg"
func LoggerFromWriterWithMinLevelAndLvlMsgFormat(output io.Writer, minLevel LogLevel) (LoggerInterface, error) {
	return loggerFromWriterWithMinLevelAndFormat(output, minLevel, "[%LEVEL] %Msg%n")
}

// LoggerFromWriterWithMinLevelAndLvlFuncCtxMsgFormat creates a new logger from a writer, a minimum log level and the message format "[%LEVEL] %FuncShort: %ExtraTextContext%Msg"
func LoggerFromWriterWithMinLevelAndLvlFuncCtxMsgFormat(output io.Writer, minLevel LogLevel) (LoggerInterface, error) {
	return loggerFromWriterWithMinLevelAndFormat(output, minLevel, "[%LEVEL] %FuncShort: %ExtraTextContext%Msg%n")
}

// LoggerFromWriterWithMinLevelAndFullFormat creates a new logger from a writer, a minimum log level and the full format
func LoggerFromWriterWithMinLevelAndFullFormat(output io.Writer, minLevel LogLevel) (LoggerInterface, error) {
	return loggerFromWriterWithMinLevelAndFormat(output, minLevel, "%Date(2006-01-02 15:04:05 MST) | %LEVEL | (%ShortFilePath:%Line in %FuncShort) | %ExtraTextContext%Msg%n")
}

// LoggerFromWriterWithMinLevelAndMsgFormat creates a new logger from a writer, a minimum log level and the message
func LoggerFromWriterWithMinLevelAndMsgFormat(output io.Writer, minLevel LogLevel) (LoggerInterface, error) {
	return loggerFromWriterWithMinLevelAndFormat(output, minLevel, "%Msg%n")
}

// LoggerFromWriterWithMinLevelAndDateFuncLineMsgFormat creates a new logger from a writer, a minimum log level and the date, function, line and message format "[%Date(2006-01-02 15:04:05.000)] [%LEVEL] %Func:%Line %Msg"
func LoggerFromWriterWithMinLevelAndDateFuncLineMsgFormat(output io.Writer, minLevel LogLevel) (LoggerInterface, error) {
	return loggerFromWriterWithMinLevelAndFormat(output, minLevel, "[%Date(2006-01-02 15:04:05.000)] [%LEVEL] %Func:%Line %Msg%n")
}

// LoggerFromWriterWithMinLevelAndDynTestFormat creates a new logger from a writer, a minimum log level and a different format depending on formatFromEnv
func LoggerFromWriterWithMinLevelAndDynTestFormat(output io.Writer, minLevel LogLevel, formatFromEnv string) (LoggerInterface, error) {
	const defaultFormat = "%l %Date(15:04:05.000000000) @%File:%Line| %Msg%n"
	var format string
	switch formatFromEnv {
	case "":
		format = defaultFormat
	case "json":
		format = `{"time":%Ns,"level":"%Level","msg":"%Msg","path":"%RelFile","func":"%Func","line":%Line}%n`
	case "json-short":
		format = `{"t":%Ns,"l":"%Lev","m":"%Msg"}%n`
	default:
		format = formatFromEnv
	}

	return loggerFromWriterWithMinLevelAndFormat(output, minLevel, format)
}
