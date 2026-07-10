// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package smartadaptivesampling bridges anomaly-detection severity to log sampling.
package smartadaptivesampling

import (
	"sync/atomic"

	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
)

type readerState struct {
	reader severityeventsdef.Reader
}

var activeReader atomic.Pointer[readerState]

// SetReader registers the active severity reader.
func SetReader(r severityeventsdef.Reader) {
	if r == nil {
		activeReader.Store(nil)
		return
	}

	activeReader.Store(&readerState{reader: r})
}

// Current returns the active severity level, if any.
func Current() (severityeventsdef.SeverityLevel, bool) {
	if state := activeReader.Load(); state != nil {
		return state.reader.GetSeverity(), true
	}
	return severityeventsdef.SeverityLow, false
}
