// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"github.com/DataDog/datadog-agent/pkg/util/log/slog"
	"github.com/DataDog/datadog-agent/pkg/util/log/types"
)

// LoggerInterface provides basic logging methods that can be used from outside the log package.
type LoggerInterface = types.LoggerInterface

// Disabled returns a disabled logger
func Disabled() LoggerInterface {
	return slog.Disabled()
}
