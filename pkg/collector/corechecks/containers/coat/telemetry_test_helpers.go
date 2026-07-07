// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package coat

import telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"

// SetAgentPodCOATTelemetryForTest replaces the package telemetry instance and
// returns a cleanup function that restores the previous instance.
func SetAgentPodCOATTelemetryForTest(tm telemetry.Component) func() {
	previous := agentPodCOAT
	agentPodCOAT = newAgentPodCOATTelemetry(tm)
	return func() {
		agentPodCOAT = previous
	}
}
