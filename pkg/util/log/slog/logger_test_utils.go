// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package slog

import (
	"io"

	"github.com/DataDog/datadog-agent/pkg/util/log/slog/formatters"
	"github.com/DataDog/datadog-agent/pkg/util/log/slog/handlers"
	"github.com/DataDog/datadog-agent/pkg/util/log/types"
)

// defaultMsgFormat is the default message format for the logger.
const defaultMsgFormat = "{{Ns}} [{{Level}}] {{.msg}}\n"

// LoggerFromWriterWithMinLevelAndFormat creates a new logger from a writer, a minimum log level, and a template format.
func LoggerFromWriterWithMinLevelAndFormat(output io.Writer, minLevel types.LogLevel, tmplFormat string) (types.LoggerInterface, error) {
	formatter := formatters.Template(tmplFormat)
	fmtHandler := handlers.NewFormat(formatter, output)
	// tests often write to a buffer, which might not be thread-safe, so we wrap the writer in a locking handler
	lockingHandler := handlers.NewLocking(fmtHandler)
	handler := handlers.NewLevel(types.ToSlogLevel(minLevel), lockingHandler)
	return NewWrapper(handler), nil
}

// LoggerFromWriterWithMinLevel creates a new logger from a writer and a minimum log level.
func LoggerFromWriterWithMinLevel(output io.Writer, minLevel types.LogLevel) (types.LoggerInterface, error) {
	return LoggerFromWriterWithMinLevelAndFormat(output, minLevel, defaultMsgFormat)
}
