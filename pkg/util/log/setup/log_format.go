// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package logs

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log/slog/formatters"
)

// commonFormatter formats the log message
func commonFormatter(loggerName LoggerName, cfg pkgconfigmodel.Reader) func(ctx context.Context, r slog.Record) string {
	if loggerName == "JMXFETCH" {
		return func(_ context.Context, r slog.Record) string {
			return r.Message + "\n"
		}
	}
	dateFmt := formatters.Date(cfg.GetBool("log_format_rfc3339"))
	return func(_ context.Context, r slog.Record) string {
		date := dateFmt(r.Time)
		level := formatters.UppercaseLevel(r.Level)
		extraContext := formatters.ExtraTextContext(r)

		frame := formatters.Frame(r)
		shortFilePath := formatters.ShortFilePath(frame)
		funcShort := formatters.ShortFunction(frame)

		return fmt.Sprintf("%s | %s | %s | (%s:%d in %s) | %s%s\n", date, loggerName, level, shortFilePath, frame.Line, funcShort, extraContext, r.Message)
	}
}

// jsonFormatter formats the log message in the JSON format
func jsonFormatter(loggerName LoggerName, cfg pkgconfigmodel.Reader) func(ctx context.Context, r slog.Record) string {
	if loggerName == "JMXFETCH" {
		return func(_ context.Context, r slog.Record) string {
			return `{"msg":` + formatters.Quote(r.Message) + "}\n"
		}
	}

	dateFmt := formatters.Date(cfg.GetBool("log_format_rfc3339"))
	return func(_ context.Context, r slog.Record) string {
		date := dateFmt(r.Time)
		level := formatters.UppercaseLevel(r.Level)
		extraContext := formatters.ExtraJSONContext(r)

		frame := formatters.Frame(r)
		shortFilePath := formatters.ShortFilePath(frame)
		funcShort := formatters.ShortFunction(frame)

		return fmt.Sprintf(`{"agent":"%s","time":"%s","level":"%s","file":"%s","line":"%d","func":"%s","msg":%s%s}`+"\n", strings.ToLower(string(loggerName)), date, level, shortFilePath, frame.Line, funcShort, formatters.Quote(r.Message), extraContext)
	}
}
