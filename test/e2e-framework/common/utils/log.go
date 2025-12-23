// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"
	"time"
)

// logf logs a message prepended with the current timestamp + test name, along with the given format and args
func Logf(t *testing.T, format string, args ...any) {
	t.Helper()
	args = append([]any{time.Now().Format("02-01-2006 15:04:05"), t.Name()}, args...)
	t.Logf("%s - %s - "+format, args...)
}

// errorf logs an error message prepended with the current timestamp + test name, along with the given format and args
func Errorf(t *testing.T, format string, args ...any) {
	t.Helper()
	args = append([]any{time.Now().Format("02-01-2006 15:04:05"), t.Name()}, args...)
	t.Errorf("%s - %s - "+format, args...)
}
