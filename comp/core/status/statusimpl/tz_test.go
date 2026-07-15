// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package statusimpl

import (
	"testing"
	"time"
)

// forceUTC pins the process timezone to UTC for the duration of the test and
// restores the previous value via t.Cleanup.
//
// It assigns time.Local directly rather than calling os.Setenv("TZ", "UTC").
// Go resolves the local timezone only once, lazily, the first time time.Local
// is used (see time.initLocal), and caches it for the rest of the process. If
// anything reads local time before the test runs — the agent logger timestamps
// every line in local time — then a later os.Setenv("TZ", ...) has no effect and
// the status renderer emits "<local> / UTC" instead of "UTC", failing these
// golden-output assertions on any host whose timezone is not already UTC.
// Setting time.Local works regardless of when the local zone was first resolved.
func forceUTC(t *testing.T) {
	t.Helper()
	original := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = original })
}
