// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package metrics

// ResetMissedBytesForTest clears process-wide state. It mutates the tracker rather
// than replacing it, so it stays safe while tailer goroutines hold a reference.
func ResetMissedBytesForTest() {
	missedBytes.reset()
	logsAgentRunning.Store(false)
}
