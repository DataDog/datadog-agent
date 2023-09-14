// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import "github.com/DataDog/datadog-agent/pkg/telemetry"

const subsystem = "language_detection_dca_client"

var (
	// StatusSuccess is the value for the "status" tag that represents a successful operation
	StatusSuccess = "success"
	// StatusError is the value for the "status" tag that represents an error
	StatusError = "error"

	commonOpts = telemetry.Options{NoDoubleUnderscoreSep: true}
)

var (
	// Running tracks it the language detection client is running.
	Running = telemetry.NewSimpleGaugeWithOpts(
		subsystem,
		"running",
		"Tracks if the language detection client is running",
		commonOpts,
	)

	// ProcessedEvents tracks the number of events processed for the given pod, container and language.
	ProcessedEvents = telemetry.NewCounterWithOpts(
		subsystem,
		"processed_events",
		[]string{"pod_language", "container_language", "detected_language"},
		"Number of events processed for the given pod, container and language",
		commonOpts,
	)

	// Latency measures the time that it takes to post metadata to the cluster-agent.
	Latency = telemetry.NewHistogramWithOpts(
		subsystem,
		"latency",
		[]string{},
		"The time it takes to pull from the collectors (in seconds)",
		[]float64{0.25, 0.5, 0.75, 1, 2, 5, 10, 15, 30, 45, 60},
		commonOpts,
	)

	// Requests reports the number of requests sent from the language detection client to the Cluster-Agents
	// and is tagged by status, which can be either "success" or "error"
	Requests = telemetry.NewCounterWithOpts(
		subsystem,
		"requests",
		[]string{"status"},
		"Number of post requests sent from the language detection client to the Cluster-Agent",
		commonOpts,
	)
)
