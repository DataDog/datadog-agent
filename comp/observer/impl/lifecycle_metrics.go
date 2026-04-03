// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"time"
)

// lifecycleMetricsExtractor converts container lifecycle events into counter
// metrics that the existing detector pipeline can analyze.
//
// Each lifecycle event becomes a single metric point:
//
//	observer.container.lifecycle{type:create|start|delete,image:alpine,runtime:docker} = 1
//
// For delete events, an exit_code tag is added:
//
//	observer.container.lifecycle{type:delete,exit_code:137,image:alpine,runtime:docker} = 1
//
// The count aggregation in the observer's time series storage gives detectors a
// "container deaths per second" signal that BOCPD can detect changepoints on —
// a spike in exit_code:137 events correlates with OOM kills or forced restarts.
type lifecycleMetricsExtractor struct{}

const lifecycleMetricName = "observer.container.lifecycle"

// processLifecycleEvent converts a lifecycle observation into metric samples
// and stores them in the engine's time series storage.
func (e *lifecycleMetricsExtractor) processLifecycleEvent(eng *engine, source string, lc *lifecycleObs) {
	tags := []string{
		"type:" + lc.eventType,
		"image:" + lc.image,
		"runtime:" + lc.runtime,
	}
	if lc.exitCode != nil {
		tags = append(tags, fmt.Sprintf("exit_code:%d", *lc.exitCode))
	}

	ts := lc.timestamp
	if ts == 0 {
		ts = time.Now().Unix()
	}

	eng.storage.Add(source, lifecycleMetricName, 1.0, ts, tags)
}

// lifecycleObs getter methods and LifecycleView assertion are in observer.go.
