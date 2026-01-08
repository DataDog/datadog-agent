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
func ExtraJSONContext(record AttrHolder) string {
	return extractContextString(jsonFormat, record)
}

// ExtraTextContext creates a text string of the record's attributes.
func ExtraTextContext(record AttrHolder) string {
	return extractContextString(textFormat, record)
}

// AttrHolder provides attributes
//
// This is an abstraction to allow using it with both seelog and slog
type AttrHolder interface {
	Attrs(func(a slog.Attr) bool)
	NumAttrs() int
}

// ToSlogAttrs converts an opaque context to a list of slog.Attr
//
// The context is expected to be a slice of interface{}, containing an even number of elements,
// with keys being strings.
//
// We can lift the restrictions and/or change the API later, but for now we want
// the exact same behavior as previously.
//
// This is exported to allow using it with seelog and slog, once we stop using seelog
// this can be moved to the slog package.
func ToSlogAttrs(context interface{}) []slog.Attr {
	if context == nil {
		return nil
	}

	contextList, ok := context.([]interface{})
	if !ok || len(contextList) == 0 || len(contextList)%2 != 0 {
		return nil
	}

	attrs := make([]slog.Attr, 0, len(contextList)/2)
	for i := 0; i < len(contextList); i += 2 {
		key, val := contextList[i], contextList[i+1]
		// Only add if key is string
		if keyStr, ok := key.(string); ok {
			attrs = append(attrs, slog.Attr{Key: keyStr, Value: slog.AnyValue(val)})
		}
	}
	return attrs
}

func extractContextString(format contextFormat, record AttrHolder) string {
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
