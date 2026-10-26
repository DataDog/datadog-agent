// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package phonehomeimpl

import (
	"testing"

	"github.com/stretchr/testify/require"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	phonehome "github.com/DataDog/datadog-agent/comp/privateactionrunner/phonehome/def"
)

func TestTelemetryTracksStateEntriesNotPolls(t *testing.T) {
	tel := telemetrymock.New(t)
	report := newTelemetryReporter(tel)
	counts := map[string]float64{}
	for _, step := range []struct {
		state phonehome.State
		entry bool
	}{
		{phonehome.State{State: phonehome.StateWaiting, Reason: phonehome.ReasonStartupJitter}, true},
		{phonehome.State{State: phonehome.StateWaiting, Reason: phonehome.ReasonRCKeyUnauthorized}, true},
		{phonehome.State{State: phonehome.StateWaiting, Reason: phonehome.ReasonRCKeyUnauthorized, NextCheck: "not-a-metric-label"}, false},
		{phonehome.State{State: phonehome.StateWaiting, Reason: phonehome.ReasonMissingEnrollmentScope}, true},
		{phonehome.State{State: phonehome.StateWaiting, Reason: phonehome.ReasonRCKeyUnauthorized}, true},
		{phonehome.State{State: phonehome.StateBlocked, Reason: phonehome.ReasonLegacyAttemptRequiresReconciliation}, true},
		{phonehome.State{State: phonehome.StateHandedOff, Reason: phonehome.ReasonSupervisorStartAcknowledged}, true},
	} {
		report(step.state)
		current := string(step.state.State) + "/" + string(step.state.Reason)
		if step.entry {
			counts[current]++
		}
		metrics, err := tel.Gather(telemetry.NoFilter)
		require.NoError(t, err)
		gauges, counters := map[string]float64{}, map[string]float64{}
		for _, family := range metrics {
			if family.GetName() != "par_phone_home__state" && family.GetName() != "par_phone_home__transitions" {
				continue
			}
			for _, metric := range family.Metric {
				labels := map[string]string{}
				for _, label := range metric.Label {
					labels[label.GetName()] = label.GetValue()
				}
				require.Len(t, labels, 2)
				key := labels["state"] + "/" + labels["reason"]
				if family.GetName() == "par_phone_home__state" {
					gauges[key] = metric.GetGauge().GetValue()
				} else {
					counters[key] = metric.GetCounter().GetValue()
				}
			}
		}
		require.Equal(t, counts, counters)
		require.Len(t, gauges, len(counts))
		for key, value := range gauges {
			if key == current {
				require.Equal(t, float64(1), value)
			} else {
				require.Zero(t, value, "previous state must not remain active: %s", key)
			}
		}
	}
}
