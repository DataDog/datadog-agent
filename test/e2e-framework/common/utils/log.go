// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import "time"

type logger interface {
	Logf(format string, args ...any)
}

// Logf logs a message prepended with the current timestamp, along with the given format and args
func Logf(l logger, format string, args ...any) {
	args = append([]any{time.Now().Format("02-01-2006 15:04:05")}, args...)
	l.Logf("%s - "+format, args...)
}
