// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package metrics

// ResetMissedBytesForTest clears the process-wide tracker state. Test-only.
// It mutates the existing tracker rather than replacing it, so it stays safe to
// call while tailer goroutines hold a reference.
func ResetMissedBytesForTest() {
	missedBytes.reset()
	fileTailingActive.Store(false)
}
