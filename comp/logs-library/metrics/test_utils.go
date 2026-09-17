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

// ResetPipelineMonitorForTest clears the registered pipeline monitor and the memoized bottleneck.
func ResetPipelineMonitorForTest() {
	RegisterPipelineMonitor(nil)
}

// PipelineMonitorRegisteredForTest reports whether process-wide readers have a live monitor.
func PipelineMonitorRegisteredForTest() bool {
	return registeredPipelineMonitor() != nil
}

type fakePipelineMonitor struct {
	NoopPipelineMonitor
	snaps []ComponentSnapshot
}

func (f *fakePipelineMonitor) Snapshots() []ComponentSnapshot { return f.snaps }

// RegisterFakePipelineMonitorForTest makes BackpressureSnapshot derive its summary from snaps.
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
