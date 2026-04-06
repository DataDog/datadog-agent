// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package observerimpl

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	observerdef "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/require"
)

func TestTelemetryHandler_CounterAdd(_t *testing.T) {
	tel := noopsimpl.GetCompatComponent()
	h := newTelemetryHandler(tel)

	const counterName = "observer.telemetry.test_counter"
	h.telemetryCounters[counterName] = tel.NewCounter(
		"observer",
		counterName,
		[]string{"detector"},
		"test counter",
	)

	h.handleTelemetry([]observerdef.ObserverTelemetry{
		newTelemetryCounter([]string{"detector:det-a"}, counterName, 2, 100),
		newTelemetryCounter([]string{"detector:det-a"}, counterName, 3, 101),
	})
	// No-op telemetry: asserts routing does not warn or panic.
}

func TestTelemetryHandler_GaugeSet(_t *testing.T) {
	tel := noopsimpl.GetCompatComponent()
	h := newTelemetryHandler(tel)

	h.handleTelemetry([]observerdef.ObserverTelemetry{
		newTelemetryGauge([]string{"detector:det"}, telemetryRRCFScore, 0.5, 100),
	})
}

func TestTelemetryHandler_IsMetricRegistered(t *testing.T) {
	tel := noopsimpl.GetCompatComponent()
	h := newTelemetryHandler(tel)

	const counterName = "observer.telemetry.test_counter2"
	h.telemetryCounters[counterName] = tel.NewCounter(
		"observer",
		counterName,
		[]string{"detector"},
		"test",
	)

	require.True(t, h.isMetricRegistered(telemetryRRCFScore))
	require.True(t, h.isMetricRegistered(counterName))
	require.False(t, h.isMetricRegistered("observer.unknown.metric"))
}
