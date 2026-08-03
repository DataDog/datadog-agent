// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logs

import (
	"context"
	"fmt"
	stdslog "log/slog"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log/slog/formatters"
	"github.com/DataDog/datadog-agent/pkg/util/log/syslog"
	"github.com/DataDog/datadog-agent/pkg/util/log/types"
)

// commonSyslogFormatter returns a function that formats a syslog message in the common format.
//
// It is equivalent to the seelog format string:
// %CustomSyslogHeader(20,<syslog-rfc>) <logger-name> | %LEVEL | (%ShortFilePath:%Line in %FuncShort) | %ExtraTextContext%Msg%n
func commonSyslogFormatter(loggerName string, syslogRFC bool) func(context.Context, stdslog.Record) string {
	syslogHeaderFmt := syslog.HeaderFormatter(20, syslogRFC)
	return func(_ context.Context, r stdslog.Record) string {
		syslogHeader := syslogHeaderFmt(types.FromSlogLevel(r.Level))
		frame := formatters.Frame(r)
		level := formatters.UppercaseLevel(r.Level)
		shortFilePath := formatters.ShortFilePath(frame)
		funcShort := formatters.ShortFunction(frame)
		extraContext := formatters.ExtraTextContext(r)
		return fmt.Sprintf("%s %s | %s | (%s:%d in %s) | %s%s\n", syslogHeader, loggerName, level, shortFilePath, frame.Line, funcShort, extraContext, r.Message)
	}
}

// jsonSyslogFormatter returns a function that formats a syslog message in the JSON format.
//
// It is equivalent to the seelog format string:
// %CustomSyslogHeader(20,<syslog-rfc>) {"agent":"<lowercase-logger-name>","level":"%LEVEL","relfile":"%ShortFilePath","line":"%Line","msg":"%Msg"%ExtraJSONContext}%n
func jsonSyslogFormatter(loggerName string, syslogRFC bool) func(context.Context, stdslog.Record) string {
	syslogHeaderFmt := syslog.HeaderFormatter(20, syslogRFC)
	return func(_ context.Context, r stdslog.Record) string {
		syslogHeader := syslogHeaderFmt(types.FromSlogLevel(r.Level))
		frame := formatters.Frame(r)
		level := formatters.UppercaseLevel(r.Level)
		relfile := formatters.ShortFilePath(frame)
		extraContext := formatters.ExtraJSONContext(r)
		return fmt.Sprintf(`%s {"agent":"%s","level":"%s","relfile":"%s","line":"%d","msg":%s%s}`+"\n", syslogHeader, strings.ToLower(loggerName), level, relfile, frame.Line, formatters.Quote(r.Message), extraContext)
	}
}
