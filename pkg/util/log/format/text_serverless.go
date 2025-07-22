// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverless

package format

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/DataDog/datadog-agent/pkg/util/log/formatters"
)

// Text returns a function that formats a slog.Record as a string.
func Text(loggerName string, logFormatRFC3339 bool) func(context.Context, slog.Record) string {
	dateFmt := formatters.Date(logFormatRFC3339)
	return func(_ context.Context, record slog.Record) string {
		levelStr := formatters.LevelToString(record.Level)

		return fmt.Sprintf("%s | %s | %s | %s\n", dateFmt(record.Time), loggerName, levelStr, record.Message)
	}
}
