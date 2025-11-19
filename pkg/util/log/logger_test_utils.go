// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package log

import (
	"io"

	"github.com/DataDog/datadog-agent/pkg/util/log/slog"
)

// Default returns a default logger
func Default() LoggerInterface {
	return slog.Default()
}

// loggerFromWriterWithMinLevelAndFormat creates a new logger from a writer, a minimum log level and a format.
func loggerFromWriterWithMinLevelAndFormat(output io.Writer, minLevel LogLevel, tmplFormat string) (LoggerInterface, error) {
	return slog.LoggerFromWriterWithMinLevelAndFormat(output, minLevel, tmplFormat)
}

// LoggerFromWriterWithMinLevel creates a new logger from a writer and a minimum log level.
func LoggerFromWriterWithMinLevel(output io.Writer, minLevel LogLevel) (LoggerInterface, error) {
	return slog.LoggerFromWriterWithMinLevel(output, minLevel)
}

// LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat creates a new logger from a writer, a minimum log level and the short format "[{{LEVEL}}] {{FuncShort}}: {{.msg}}"
func LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(output io.Writer, minLevel LogLevel) (LoggerInterface, error) {
	return loggerFromWriterWithMinLevelAndFormat(output, minLevel, "[{{LEVEL}}] {{FuncShort}}: {{.msg}}\n")
}

// LoggerFromWriterWithMinLevelAndLvlMsgFormat creates a new logger from a writer, a minimum log level and the message format "[{{LEVEL}}] {{.msg}}"
func LoggerFromWriterWithMinLevelAndLvlMsgFormat(output io.Writer, minLevel LogLevel) (LoggerInterface, error) {
	return loggerFromWriterWithMinLevelAndFormat(output, minLevel, "[{{LEVEL}}] {{.msg}}\n")
}

// LoggerFromWriterWithMinLevelAndLvlFuncCtxMsgFormat creates a new logger from a writer, a minimum log level and the message format "[{{LEVEL}}] {{FuncShort}}: {{ExtraTextContext}}{{.msg}}"
func LoggerFromWriterWithMinLevelAndLvlFuncCtxMsgFormat(output io.Writer, minLevel LogLevel) (LoggerInterface, error) {
	return loggerFromWriterWithMinLevelAndFormat(output, minLevel, "[{{LEVEL}}] {{FuncShort}}: {{ExtraTextContext}}{{.msg}}\n")
}

// LoggerFromWriterWithMinLevelAndFullFormat creates a new logger from a writer, a minimum log level and the full format
func LoggerFromWriterWithMinLevelAndFullFormat(output io.Writer, minLevel LogLevel) (LoggerInterface, error) {
	return loggerFromWriterWithMinLevelAndFormat(output, minLevel, "{{Date \"2006-01-02 15:04:05 MST\"}} | {{LEVEL}} | ({{ShortFilePath}}:{{.line}} in {{FuncShort}}) | {{ExtraTextContext}}{{.msg}}\n")
}

// LoggerFromWriterWithMinLevelAndMsgFormat creates a new logger from a writer, a minimum log level and the message
func LoggerFromWriterWithMinLevelAndMsgFormat(output io.Writer, minLevel LogLevel) (LoggerInterface, error) {
	return loggerFromWriterWithMinLevelAndFormat(output, minLevel, "{{.msg}}\n")
}

// LoggerFromWriterWithMinLevelAndDateFuncLineMsgFormat creates a new logger from a writer, a minimum log level and the date, function, line and message format "[{{Date \"2006-01-02 15:04:05.000\"}}] [{{LEVEL}}] {{.func}}:{{.line}} {{.msg}}"
func LoggerFromWriterWithMinLevelAndDateFuncLineMsgFormat(output io.Writer, minLevel LogLevel) (LoggerInterface, error) {
	return loggerFromWriterWithMinLevelAndFormat(output, minLevel, "[{{Date \"2006-01-02 15:04:05.000\"}}] [{{LEVEL}}] {{.func}}:{{.line}} {{.msg}}\n")
}

// LoggerFromWriterWithMinLevelAndDynTestFormat creates a new logger from a writer, a minimum log level and a different format depending on formatFromEnv
func LoggerFromWriterWithMinLevelAndDynTestFormat(output io.Writer, minLevel LogLevel, formatFromEnv string) (LoggerInterface, error) {
	const defaultFormat = "{{.l}} {{Date \"15:04:05.000000000\"}} @{{.file}}:{{.line}}| {{.msg}}\n"
	var format string
	switch formatFromEnv {
	case "":
		format = defaultFormat
	case "json":
		format = "{\"time\":{{Ns}},\"level\":\"{{Level}}\",\"msg\":\"{{.msg}}\",\"path\":\"{{RelFile}}\",\"func\":\"{{.func}}\",\"line\":{{.line}}}\n"
	case "json-short":
		format = "{\"t\":{{Ns}},\"l\":\"{{Lev}}\",\"m\":\"{{.msg}}\"}\n"
	default:
		format = formatFromEnv
	}

	return loggerFromWriterWithMinLevelAndFormat(output, minLevel, format)
}
