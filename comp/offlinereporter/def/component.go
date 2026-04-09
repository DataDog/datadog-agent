// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package offlinereporter tracks the time gap between agent runs by maintaining a
// heartbeat file updated every 5 seconds. On startup it captures the
// last-written timestamp so callers can measure how long the agent was offline.
package offlinereporter

// team: agent-runtimes

// Component is the component type.
type Component interface {
	// SendOfflineDuration sends a single gauge metric (value = seconds offline)
	// to the Datadog intake. If no previous heartbeat file exists (first run),
	// the call is a no-op.
	SendOfflineDuration(metricName string, tags []string)
}
