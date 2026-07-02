// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package observer provides a component for observing data flowing through the agent.
//
// The observer component allows other components to report metrics, logs, and other
// signals for sampling and analysis. It provides lightweight handles that can be
// passed to data pipelines without adding significant overhead.
package observer

import severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"

// team: q-branch

// Component is the central observer that receives data via handles.
type Component interface {
	// GetHandle returns a lightweight handle for a named source.
	// The source name is used to identify where observations originate.
	GetHandle(name string) Handle

	// RecordSamplerDropped increments the rate-limiter dropped counter for the
	// given source ("internal", "kubelet", "containers") and priority ("high",
	// "medium", "low"). Only rate-limit drops are counted; min_severity drops
	// are intentional and not tracked here.
	RecordSamplerDropped(source, priority string)

	// DumpMetrics writes all stored metrics to the specified file (for debugging).
	DumpMetrics(path string) error

	// SubscribeScorer registers a scorer event listener described by cfg.
	// cfg.Listener is called synchronously on the data clock for every severity
	// transition that matches cfg.Filter. The zero-value filter delivers all
	// transitions. If the current severity level is already known, an initial
	// synthetic event reflecting it is delivered immediately (see
	// severityeventsdef.Subscriber for details). Returns an unsubscribe
	// function; call it to stop delivery.
	SubscribeScorer(cfg severityeventsdef.SeverityEventsConfiguration) func()
}
