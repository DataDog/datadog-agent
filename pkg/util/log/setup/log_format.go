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

	"github.com/cihub/seelog"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log/slog/formatters"
)

// buildCommonFormat returns the log common format seelog string
func buildCommonFormat(loggerName LoggerName, cfg pkgconfigmodel.Reader) string {
	if loggerName == "JMXFETCH" {
		return `%Msg%n`
	}
	return fmt.Sprintf("%%Date(%s) | %s | %%LEVEL | (%%ShortFilePath:%%Line in %%FuncShort) | %%ExtraTextContext%%Msg%%n", getLogDateFormat(cfg), loggerName)
}

// buildJSONFormat returns the log JSON format seelog string
func buildJSONFormat(loggerName LoggerName, cfg pkgconfigmodel.Reader) string {
	_ = seelog.RegisterCustomFormatter("QuoteMsg", createQuoteMsgFormatter)
	if loggerName == "JMXFETCH" {
		return `{"msg":%QuoteMsg}%n`
	}
	return fmt.Sprintf(`{"agent":"%s","time":"%%Date(%s)","level":"%%LEVEL","file":"%%ShortFilePath","line":"%%Line","func":"%%FuncShort","msg":%%QuoteMsg%%ExtraJSONContext}%%n`, strings.ToLower(string(loggerName)), getLogDateFormat(cfg))
}

// commonFormatter formats the same way as buildCommonFormat does
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

// jsonFormatter formats the same way as buildJSONFormat does
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
		shortFilePath := formatters.ShortFilePath(formatters.Frame(r))
		line := formatters.Frame(r).Line
		funcShort := formatters.ShortFunction(formatters.Frame(r))
		extraContext := formatters.ExtraJSONContext(r)

		return fmt.Sprintf(`{"agent":"%s","time":"%s","level":"%s","file":"%s","line":"%d","func":"%s","msg":%s%s}`+"\n", strings.ToLower(string(loggerName)), date, level, shortFilePath, line, funcShort, formatters.Quote(r.Message), extraContext)
	}
}
