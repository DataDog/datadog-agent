// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agenttelemetryimpl

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPhoneHomeDefaultProfilePayloads(t *testing.T) {
	tel := makeTelMock(t)
	client := &clientMock{}
	a := getTestAtel(t, tel, "api_key: synthetic-key\nagent_telemetry:\n  enabled: true\n  use_compression: false", nil, client, nil)
	require.True(t, a.enabled)
	t.Cleanup(a.lightTracer.Stop)
	a.cancelCtx = context.Background()
	var profile *Profile
	for _, p := range a.atelCfg.Profiles {
		if p.Name == "par-phone-home" {
			profile = p
		}
	}
	require.NotNil(t, profile, "the metrics must be selected by the shipped default profile")
	require.EqualValues(t, 30, profile.Schedule.StartAfter)
	require.EqualValues(t, 900, profile.Schedule.Period)
	require.Zero(t, profile.Schedule.Iterations)

	// An extra producer label must not leak through the COAT allowlist.
	labels := []string{"state", "reason", "hostname"}
	state := tel.NewGauge("par_phone_home", "state", labels, "")
	transitions := tel.NewCounter("par_phone_home", "transitions", labels, "")
	const unwanted = "must-not-export-this-label"
	state.Set(1, "waiting", "rc_key_unauthorized", unwanted)
	transitions.Inc("waiting", "rc_key_unauthorized", unwanted)

	collect := func() map[string]map[string]float64 {
		t.Helper()
		session, err := a.loadPayloads([]*Profile{profile})
		require.NoError(t, err)
		require.NoError(t, a.sender.flushSession(session))
		require.NotContains(t, string(client.body), unwanted)
		var payload Payload
		require.NoError(t, json.Unmarshal(client.body, &payload))
		payloads := []Payload{payload}
		if batch, ok := payload.Payload.([]Payload); ok {
			payloads = batch
		}
		values := map[string]map[string]float64{}
		for _, p := range payloads {
			metrics := p.Payload.(AgentMetricsPayload).Metrics
			require.Contains(t, metrics, "agent_metadata")
			for name, raw := range metrics {
				if name == "agent_metadata" {
					continue
				}
				m := raw.(MetricPayload)
				require.Len(t, m.Tags, 3)
				require.Equal(t, "agent", m.Tags["emitter"])
				key := m.Tags["state"].(string) + "/" + m.Tags["reason"].(string)
				if values[name] == nil {
					values[name] = map[string]float64{}
				}
				values[name][key] = m.Value
				if name == "par_phone_home.state" {
					require.Equal(t, "gauge", m.Type)
				} else {
					require.Equal(t, "par_phone_home.transitions", name)
					require.Equal(t, "counter", m.Type)
				}
			}
		}
		return values
	}
	first := collect()
	require.Equal(t, map[string]float64{"waiting/rc_key_unauthorized": 1}, first["par_phone_home.state"])
	require.Equal(t, map[string]float64{"waiting/rc_key_unauthorized": 1}, first["par_phone_home.transitions"])

	// The intermediate state disappears before collection, but its entry is counted.
	state.Set(0, "waiting", "rc_key_unauthorized", unwanted)
	state.Set(1, "waiting", "missing_enrollment_scope", unwanted)
	transitions.Inc("waiting", "missing_enrollment_scope", unwanted)
	state.Set(0, "waiting", "missing_enrollment_scope", unwanted)
	state.Set(1, "handed_off", "supervisor_start_acknowledged", unwanted)
	transitions.Inc("handed_off", "supervisor_start_acknowledged", unwanted)
	second := collect()
	require.Equal(t, map[string]float64{"handed_off/supervisor_start_acknowledged": 1}, second["par_phone_home.state"])
	require.Equal(t, map[string]float64{
		"waiting/rc_key_unauthorized": 0, "waiting/missing_enrollment_scope": 1,
		"handed_off/supervisor_start_acknowledged": 1,
	}, second["par_phone_home.transitions"])
	third := collect()
	require.Equal(t, second["par_phone_home.state"], third["par_phone_home.state"])
	for _, delta := range third["par_phone_home.transitions"] {
		require.Zero(t, delta, "unchanged counters must not be recounted at the next collection")
	}
}
