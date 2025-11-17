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
		return formatters.ExtraJSONContext(attrHolderImpl(formatters.ToSlogAttrs(context.CustomContext())))
	}
}

func createExtraTextContext(_ string) seelog.FormatterFunc {
	return func(_ string, _ seelog.LogLevel, context seelog.LogContextInterface) interface{} {
		return formatters.ExtraTextContext(attrHolderImpl(formatters.ToSlogAttrs(context.CustomContext())))
	}
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
