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
//
// See https://github.com/cihub/seelog/blob/f561c5e57575bb1e0a2167028b7339b3a8d16fb4/format.go#L400
func ShortFunction(frame runtime.Frame) string {
	return frame.Function[strings.LastIndexByte(frame.Function, '.')+1:]
}
