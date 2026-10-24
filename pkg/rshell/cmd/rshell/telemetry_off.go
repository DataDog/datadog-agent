// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !with_telemetry

package main

// startTelemetry is a no-op in the default build: spans created inside interp
// register on the global tracer but are never flushed. The interpreter stops
// adding spans after 10,000 process-wide starts. Build with
// `-tags with_telemetry` to include the real sender (telemetry_on.go).
func startTelemetry() func() { return func() {} }
