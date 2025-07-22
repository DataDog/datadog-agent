// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package formatters

import (
	"runtime"
	"strings"
)

// ShortFunction returns the short function name of the function that the log message was emitted from.
func ShortFunction(frame runtime.Frame) string {
	return frame.Function[strings.LastIndexByte(frame.Function, '.')+1:]
}
