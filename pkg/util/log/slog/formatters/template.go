// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package formatters

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"strings"

	template "github.com/DataDog/datadog-agent/pkg/template/text"
	"github.com/DataDog/datadog-agent/pkg/util/log/types"
)

// Template returns a function that formats a slog.Record as a string using a template.
//
// The formatter panics if it fails to parse the template or execute it. It is only meant to be used in tests,
// so it makes the implementation simpler by not returning an error.
//
// Available context variables:
// - record: the slog.Record
// - time: the time.time of the record
// - level: the string representation of the log level
// - l: the first character of the log level string
// - msg: the message string
// - frame: the runtime.Frame of the caller
// - line: the line number of the caller
// - func: the function name of the caller
// - file: the full path of the caller
//
// Available functions:
// - Date(format string): formats the time of the record using the given format
// - DateTime(): returns the time of the record in the format "2006-01-02 15:04:05.000 MST"
// - Ns(): returns the nanosecond time of the record
// - Lev(): returns the short log level
// - Level(): returns the capitalized log level
// - LEVEL(): returns the uppercase log level
// - ToUpper(string): converts the given string to uppercase
// - Quote(string): quotes the given string
// - FuncShort(): returns the short function name
// - ShortFilePath(): returns the short file path
// - RelFile(): returns the relative file path
// - ExtraTextContext(): returns the extra context in text format
// - ExtraJSONContext(): returns the extra context in JSON format
func Template(tmpl string) func(context.Context, slog.Record) string {
	return func(_ context.Context, r slog.Record) string {
		frames := runtime.CallersFrames([]uintptr{r.PC})
		frame, _ := frames.Next()
		context := map[string]interface{}{
			"record": r,
			"time":   r.Time,
			"level":  types.FromSlogLevel(r.Level).String(),
			"l":      ShortestLevel(r.Level),
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

			"Lev":   func() string { return ShortLevel(r.Level) },
			"Level": func() string { return CapitalizedLevel(r.Level) },
			"LEVEL": func() string { return UppercaseLevel(r.Level) },

			"ToUpper": strings.ToUpper,
			"Quote":   Quote,

			"FuncShort":     func() string { return ShortFunction(frame) },
			"ShortFilePath": func() string { return ShortFilePath(frame) },
			"RelFile":       func() string { return RelFile(frame) },

			"ExtraTextContext": func() string { return ExtraTextContext(r) },
			"ExtraJSONContext": func() string { return ExtraJSONContext(r) },
		}

		// Create template with functions registered before parsing
		tmplObj, err := template.New("").Funcs(funcs).Parse(tmpl)
		if err != nil {
			panic(fmt.Sprintf("failed to parse log format template: %v", err))
		}

		var buff bytes.Buffer
		err = tmplObj.Execute(&buff, context)
		if err != nil {
			panic(fmt.Sprintf("failed to render log format template: %v", err))
		}

		return buff.String()
	}
}
