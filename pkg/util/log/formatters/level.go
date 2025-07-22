// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package formatters

import (
	"log/slog"

	"github.com/DataDog/datadog-agent/pkg/util/log/types"
)

// LevelToString converts a slog.Level to a string
func LevelToString(level slog.Level) string {
	return types.LogLevel(level).String()
}
