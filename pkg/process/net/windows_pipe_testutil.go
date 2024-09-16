// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test && windows

package net

// OverrideSystemProbeNamedPipePath sets the active named pipe path for System Probe connections.
// This is used by tests only to avoid conflicts with an existing locally installed Datadog agent.
func OverrideSystemProbeNamedPipePath(path string) {
	activeSystemProbePipeName = path
}
