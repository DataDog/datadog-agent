// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package slog

import (
	"bytes"
	"context"
	"log/slog"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	template "github.com/DataDog/datadog-agent/pkg/template/text"
	"github.com/DataDog/datadog-agent/pkg/util/log/slog/formatters"
	"github.com/DataDog/datadog-agent/pkg/util/log/slog/types"
)

// TemplateFormatter returns a function that formats a slog.Record as a string using a template.
func TemplateFormatter(t *testing.T, tmpl string) func(context.Context, slog.Record) string {
	return func(_ context.Context, r slog.Record) string {
		frames := runtime.CallersFrames([]uintptr{r.PC})
		frame, _ := frames.Next()
		context := map[string]interface{}{
			"record": r,
			"time":   r.Time,
			"level":  types.LogLevel(r.Level),
			"l":      types.LogLevel(r.Level).String()[0],
			"msg":    r.Message,
			"frame":  frame,
			"line":   frame.Line,
			"func":   frame.Function,
			"file":   frame.File,
		}
		funcs := template.FuncMap{
			"Date":     func(format string) string { return r.Time.Format(format) },
			"DateTime": func() string { return r.Time.Format("2006-01-02 15:04:05.000 MST") },
			"Ns":       func() int64 { return r.Time.UnixNano() },
			"Level":    func() string { return types.LogLevel(r.Level).Capitalized() },
			"LEVEL":    func() string { return types.LogLevel(r.Level).Uppercase() },

			"ToUpper": strings.ToUpper,
			"Quote":   formatters.Quote,

			"FuncShort":        func() string { return formatters.ShortFunction(frame) },
			"ShortFile":        func() string { return formatters.ShortFilePath(frame) },
			"RelFile":          func() string { return formatters.ShortFilePath(frame) },
			"ExtraTextContext": func() string { return formatters.ExtraTextContext(r) },
			"ExtraJSONContext": func() string { return formatters.ExtraJSONContext(r) },
		}

		// Create template with functions registered before parsing
		tmplObj, err := template.New("").Funcs(funcs).Parse(tmpl)
		require.NoError(t, err)

		var buff bytes.Buffer
		err = tmplObj.Execute(&buff, context)
		require.NoError(t, err)

		return buff.String()
	}
}
