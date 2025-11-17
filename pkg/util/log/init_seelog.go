// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"log/slog"

	"github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/util/log/slog/formatters"
)

func parseShortFilePath(_ string) seelog.FormatterFunc {
	return func(_ string, _ seelog.LogLevel, context seelog.LogContextInterface) interface{} {
		return formatters.ExtractShortPathFromFullPath(context.FullPath())
	}
}

func createExtraJSONContext(_ string) seelog.FormatterFunc {
	return func(_ string, _ seelog.LogLevel, context seelog.LogContextInterface) interface{} {
		contextList, ok := context.CustomContext().([]interface{})
		if len(contextList) == 0 || !ok || len(contextList)%2 != 0 {
			return ""
		}

		return formatters.ExtraJSONContext(toSlogAttrs(contextList))
	}
}

func createExtraTextContext(_ string) seelog.FormatterFunc {
	return func(_ string, _ seelog.LogLevel, context seelog.LogContextInterface) interface{} {
		contextList, ok := context.CustomContext().([]interface{})
		if len(contextList) == 0 || !ok || len(contextList)%2 != 0 {
			return ""
		}
		return formatters.ExtraTextContext(toSlogAttrs(contextList))
	}
}

func toSlogAttrs(contextList []interface{}) attrHolderImpl {
	attrs := make([]slog.Attr, 0, len(contextList)/2)
	for i := 0; i < len(contextList); i += 2 {
		key, val := contextList[i], contextList[i+1]
		// Only add if key is string
		if keyStr, ok := key.(string); ok {
			attrs = append(attrs, slog.Attr{Key: keyStr, Value: slog.AnyValue(val)})
		}
	}
	return attrHolderImpl(attrs)
}

type attrHolderImpl []slog.Attr

func (h attrHolderImpl) Attrs(fn func(a slog.Attr) bool) {
	for _, attr := range h {
		if !fn(attr) {
			break
		}
	}
}

func (h attrHolderImpl) NumAttrs() int {
	return len(h)
}

func init() {
	_ = seelog.RegisterCustomFormatter("ShortFilePath", parseShortFilePath)
	_ = seelog.RegisterCustomFormatter("ExtraJSONContext", createExtraJSONContext)
	_ = seelog.RegisterCustomFormatter("ExtraTextContext", createExtraTextContext)
}
