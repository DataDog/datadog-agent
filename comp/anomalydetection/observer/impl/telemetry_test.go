// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import (
	"testing"

	noopsimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl/noops"
	"github.com/stretchr/testify/require"
)

func TestObserverTelemetry_NoopsDoNotPanic(_ *testing.T) {
	tel := newObserverTelemetry(noopsimpl.GetCompatComponent())
	tel.recordChannelDropped("logs")
	tel.recordRRCFScore("rrcf", 0.7)
	tel.recordRRCFThreshold("rrcf", 0.9)
	tel.recordLogPatternCountDelta("log_pattern_extractor", 1)
	tel.recordLogIngested("internal", 256)
	tel.recordDroppedLog("logs", []string{"source:kubelet"})
	tel.recordFilteredMetric("dogstatsd")
	tel.incrementLogsInFlight("internal")
	tel.decrementLogsInFlight("internal")
	tel.initLogsInFlight()
	tel.setSeriesCount(42)
	tel.recordStorageSeriesEvicted("capacity", 3)
	tel.recordStorageCapacityHit()
	tel.recordAdvanceSkipped("input")
}

func TestClassifyLogSource(t *testing.T) {
	require.Equal(t, "internal", classifyLogSource("agent_logs", nil))
	require.Equal(t, "kubelet", classifyLogSource("logs", []string{"source:kubelet", "service:kubelet"}))
	require.Equal(t, "containers", classifyLogSource("logs", []string{"source:docker"}))
}
