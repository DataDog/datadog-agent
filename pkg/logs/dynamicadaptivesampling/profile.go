// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package dynamicadaptivesampling is used to make the bridge between anomaly detection
// pipeline and adaptive sampling, it provides severity change events.
package dynamicadaptivesampling

import (
	"fmt"
	"sync/atomic"

	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
)

type readerState struct {
	reader severityeventsdef.Reader
}

var activeReader atomic.Pointer[readerState]

// SetReader registers the active reader. Called once at startup by the
// wiring layer; nil until then (or in builds where the feature is disabled).
func SetReader(r severityeventsdef.Reader) {
	if r == nil {
		fmt.Printf("dynamicadaptivesampling.SetReader: clearing active severity reader\n")
		activeReader.Store(nil)
		return
	}

	level := r.GetSeverity()
	fmt.Printf("dynamicadaptivesampling.SetReader: registered severity reader level=%d\n", level)
	activeReader.Store(&readerState{reader: r})
}

// Current returns the active reader's current severity level, or false when no
// reader is registered yet.
func Current() (severityeventsdef.SeverityLevel, bool) {
	if state := activeReader.Load(); state != nil {
		return state.reader.GetSeverity(), true
	}
	return severityeventsdef.SeverityLow, false
}
