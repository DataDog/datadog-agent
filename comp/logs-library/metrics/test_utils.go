// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package metrics

import "time"

// ResetMissedBytesForTest clears process-wide state. It mutates the tracker rather
// than replacing it, so it stays safe while tailer goroutines hold a reference.
func ResetMissedBytesForTest() {
	missedBytes.reset()
	logsAgentRunning.Store(false)
}

// ResetPipelineMonitorForTest clears the registered pipeline monitor and the memoized
// bottleneck, so a test does not inherit the previous test's pipeline.
func ResetPipelineMonitorForTest() {
	RegisterPipelineMonitor(nil)
}

// RecordMissedBytesWithBottleneckForTest records a loss attributed to a named stage, without
// needing a live pipeline monitor to produce that stage.
func RecordMissedBytesWithBottleneckForTest(source, service string, bytes int64, bottleneck string) {
	missedBytes.record(source, service, bytes, bottleneck)
}

// fakePipelineMonitor answers Snapshots with a fixed set, so a test outside this package can
// drive the backpressure summary without standing up a real pipeline.
type fakePipelineMonitor struct {
	NoopPipelineMonitor
	snaps []ComponentSnapshot
}

func (f *fakePipelineMonitor) Snapshots() []ComponentSnapshot { return f.snaps }

// RegisterFakePipelineMonitorForTest makes BackpressureSnapshot return a summary derived from
// snaps. Pair with ResetPipelineMonitorForTest in a cleanup.
func RegisterFakePipelineMonitorForTest(snaps []ComponentSnapshot) {
	RegisterPipelineMonitor(&fakePipelineMonitor{snaps: snaps})
}

// SaturatedSnapshotForTest builds one component snapshot saturated for sat30m of the last 30
// minutes, and optionally right now.
func SaturatedSnapshotForTest(name, instance string, ratio float64, sat30m time.Duration, currently bool) ComponentSnapshot {
	return ComponentSnapshot{
		Name:     name,
		Instance: instance,
		AvgRatio: ratio,
		Windows: WindowStats{
			Max5m:              ratio,
			Max30m:             ratio,
			Saturated30m:       sat30m,
			CurrentlySaturated: currently,
		},
	}
}
