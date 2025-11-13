// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cihub/seelog"
)

type contextFormat uint8

const (
	jsonFormat = contextFormat(iota)
	textFormat
)

func parseShortFilePath(_ string) seelog.FormatterFunc {
	return func(_ string, _ seelog.LogLevel, context seelog.LogContextInterface) interface{} {
		return extractShortPathFromFullPath(context.FullPath())
	}
}

func extractShortPathFromFullPath(fullPath string) string {
	shortPath := ""
	if strings.Contains(fullPath, "-agent/") {
		// We want to trim the part containing the path of the project
		// ie DataDog/datadog-agent/ or DataDog/datadog-process-agent/
		slices := strings.Split(fullPath, "-agent/")
		shortPath = slices[len(slices)-1]
	} else {
		// For logging from dependencies, we want to log e.g.
		// "collector@v0.35.0/service/collector.go"
		slices := strings.Split(fullPath, "/")
		atSignIndex := len(slices) - 1
		for ; atSignIndex > 0; atSignIndex-- {
			if strings.Contains(slices[atSignIndex], "@") {
				break
			}
		}
		shortPath = strings.Join(slices[atSignIndex:], "/")
	}
	return shortPath
}

func createExtraJSONContext(_ string) seelog.FormatterFunc {
	return func(_ string, _ seelog.LogLevel, context seelog.LogContextInterface) interface{} {
		contextList, ok := context.CustomContext().([]interface{})
		if len(contextList) == 0 || !ok {
			return ""
		}
		return extractContextString(jsonFormat, contextList)
	}
}

func createExtraTextContext(_ string) seelog.FormatterFunc {
	return func(_ string, _ seelog.LogLevel, context seelog.LogContextInterface) interface{} {
		contextList, ok := context.CustomContext().([]interface{})
		if len(contextList) == 0 || !ok {
			return ""
		}
		return extractContextString(textFormat, contextList)
	}
}

func extractContextString(format contextFormat, contextList []interface{}) string {
	if len(contextList) == 0 || len(contextList)%2 != 0 {
		return ""
	}

	builder := strings.Builder{}
	if format == jsonFormat {
		builder.WriteString(",")
	}

	for i := 0; i < len(contextList); i += 2 {
		key, val := contextList[i], contextList[i+1]
		// Only add if key is string
		if keyStr, ok := key.(string); ok {
			addToBuilder(&builder, keyStr, val, format, i == len(contextList)-2)
		}
	}

	if format != jsonFormat {
		builder.WriteString(" | ")
	}

	return builder.String()
}

func addToBuilder(builder *strings.Builder, key string, value interface{}, format contextFormat, isLast bool) {
	var buf []byte
	appendFmt(builder, format, key, buf)
	builder.WriteString(":")
	switch val := value.(type) {
	case string:
		appendFmt(builder, format, val, buf)
	default:
		appendFmt(builder, format, fmt.Sprintf("%v", val), buf)
	}
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

func init() {
	_ = seelog.RegisterCustomFormatter("ShortFilePath", parseShortFilePath)
	_ = seelog.RegisterCustomFormatter("ExtraJSONContext", createExtraJSONContext)
	_ = seelog.RegisterCustomFormatter("ExtraTextContext", createExtraTextContext)
}
