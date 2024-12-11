// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clientimpl

import "github.com/DataDog/datadog-agent/comp/core/telemetry"

const subsystem = "language_detection_dca_client"

type componentTelemetry struct {
	ProcessedEvents   telemetry.Counter
	ProcessWithoutPod telemetry.Counter
	Latency           telemetry.Histogram
	Requests          telemetry.Counter
}

var (
	// statusSuccess is the value for the "status" tag that represents a successful operation
	statusSuccess = "success"
	// statusError is the value for the "status" tag that represents an error
	statusError = "error"

	commonOpts = telemetry.Options{NoDoubleUnderscoreSep: true}
)

func newComponentTelemetry(telemetry telemetry.Component) *componentTelemetry {
	return &componentTelemetry{
		// ProcessedEvents tracks the number of events processed for the given pod, container and language.
		ProcessedEvents: telemetry.NewCounterWithOpts(
			subsystem,
			"processed_events",
			[]string{"scanned_pod_namespace", "scanned_pod", "scanned_container", "detected_language"},
			"Number of events processed for the given pod, container and language",
			commonOpts,
		),

		// Latency measures the time that it takes to post metadata to the cluster-agent.
		Latency: telemetry.NewHistogramWithOpts(
			subsystem,
			"latency",
			[]string{},
			"The time it takes to pull from the collectors (in seconds)",
			[]float64{0.25, 0.5, 0.75, 1, 2, 5, 10, 15, 30, 45, 60},
			commonOpts,
		),

		// ProcessWithoutPod counts the number of process events for which the associated pod was not found.
		ProcessWithoutPod: telemetry.NewCounterWithOpts(
			subsystem,
			"process_without_pod",
			[]string{},
			"Number of process events that have been retried because the associated pod was missing",
			commonOpts,
		),

		// Requests reports the number of requests sent from the language detection client to the Cluster-Agents
		// and is tagged by status, which can be either "success" or "error"
		Requests: telemetry.NewCounterWithOpts(
			subsystem,
			"requests",
			[]string{"status"},
			"Number of post requests sent from the language detection client to the Cluster-Agent",
			commonOpts,
		),
	}
}
