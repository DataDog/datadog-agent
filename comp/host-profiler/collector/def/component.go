// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build hostprofiler

// Package collector defines the host profiler collector component.
//
// The collector component is responsible for collecting host profiling data
// and sending it to Datadog. It implements the main functionality of the
// host profiler agent.
package collector

// team: opentelemetry-agent profiling-full-host

// Component is the component type for the host profiler collector.
//
// The collector component provides the main functionality for collecting
// host profiling data and sending it to Datadog.
type Component interface {
	// Run starts the collector and begins collecting host profiling data.
	// It should be called to start the main collection loop.
	Run() error
}
