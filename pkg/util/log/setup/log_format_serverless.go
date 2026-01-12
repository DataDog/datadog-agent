// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverless

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
	return fmt.Sprintf("%%Date(%s) | %s | %%LEVEL | %%Msg%%n", getLogDateFormat(cfg), loggerName)
}

// buildJSONFormat returns the log JSON format seelog string
func buildJSONFormat(loggerName LoggerName, cfg pkgconfigmodel.Reader) string {
	_ = seelog.RegisterCustomFormatter("QuoteMsg", createQuoteMsgFormatter)
	return fmt.Sprintf(`{"agent":"%s","time":"%%Date(%s)","level":"%%LEVEL","file":"","line":"","func":"%%FuncShort","msg":%%QuoteMsg}%%n`, strings.ToLower(string(loggerName)), getLogDateFormat(cfg))
}

func commonFormatter(loggerName LoggerName, cfg pkgconfigmodel.Reader) func(ctx context.Context, r slog.Record) string {
	dateFmt := formatters.Date(cfg.GetBool("log_format_rfc3339"))
	return func(_ context.Context, r slog.Record) string {
		date := dateFmt(r.Time)
		level := formatters.UppercaseLevel(r.Level)
		return fmt.Sprintf("%s | %s | %s | %s\n", date, loggerName, level, r.Message)
	}
}

func jsonFormatter(loggerName LoggerName, cfg pkgconfigmodel.Reader) func(ctx context.Context, r slog.Record) string {
	dateFmt := formatters.Date(cfg.GetBool("log_format_rfc3339"))
	return func(_ context.Context, r slog.Record) string {
		date := dateFmt(r.Time)
		level := formatters.UppercaseLevel(r.Level)

		frame := formatters.Frame(r)
		funcShort := formatters.ShortFunction(frame)

		return fmt.Sprintf(`{"agent":"%s","time":"%s","level":"%s","file":"","line":"","func":"%s","msg":%s}`+"\n", strings.ToLower(string(loggerName)), date, level, funcShort, formatters.Quote(r.Message))
	}
}
