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

// ResetDroppedLogsForTest clears the dropped-logs tracker and the logs-running flag,
// in place for the same reason as ResetMissedBytesForTest.
func ResetDroppedLogsForTest() {
	droppedLogs.reset()
	logsAgentRunning.Store(false)
}

// ResetUntailedFilesForTest clears the untailed-files state and the logs-running flag.
func ResetUntailedFilesForTest() {
	untailedFiles.reset()
	logsAgentRunning.Store(false)
}
