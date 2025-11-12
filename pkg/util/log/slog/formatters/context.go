// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package formatters

import (
	"log/slog"
	"strconv"
	"strings"
)

type contextFormat uint8

const (
	jsonFormat = contextFormat(iota)
	textFormat
)

// ExtraJSONContext creates a JSON string of the record's attributes.
func ExtraJSONContext(record slog.Record) string {
	return extractContextString(jsonFormat, record)
}

// ExtraTextContext creates a text string of the record's attributes.
func ExtraTextContext(record slog.Record) string {
	return extractContextString(textFormat, record)
}

func extractContextString(format contextFormat, record slog.Record) string {
	if record.NumAttrs() == 0 {
		return ""
	}

	builder := strings.Builder{}
	if format == jsonFormat {
		builder.WriteString(",")
	}

	idx := 0
	record.Attrs(func(a slog.Attr) bool {
		idx++
		addToBuilder(&builder, a.Key, a.Value, format, idx == record.NumAttrs())
		return true
	})

	if format != jsonFormat {
		builder.WriteString(" | ")
	}

	return builder.String()
}

func addToBuilder(builder *strings.Builder, key string, value slog.Value, format contextFormat, isLast bool) {
	var buf []byte
	appendFmt(builder, format, key, buf)
	builder.WriteString(":")
	appendFmt(builder, format, value.String(), buf)
	if !isLast {
		builder.WriteString(",")
	}
}

func appendFmt(builder *strings.Builder, format contextFormat, s string, buf []byte) {
	if format == jsonFormat {
		buf = buf[:0]
		buf = strconv.AppendQuote(buf, s)
		builder.Write(buf)
	} else {
		builder.WriteString(s)
	}
}
