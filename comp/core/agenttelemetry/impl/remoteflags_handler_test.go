// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agenttelemetryimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/remoteflags"
)

// A config with a single diagnostic profile named to match dataLossProfileName,
// so the dataLossFlag handler toggles it. The metric bar.zoo stands in for the
// real drop/saturation metrics.
const dataLossTestConfig = `
agent_telemetry:
  enabled: true
  profiles:
    - name: data_loss
      diagnostic: true
      metric:
        metrics:
          - name: bar.zoo
            preserve_tags: []
`

func TestDataLossFlag_FlagNameAndSubscriber(t *testing.T) {
	a := getTestAtel(t, nil, dataLossTestConfig, &senderMock{}, nil, newRunnerMock())

	sub := atelFlagSubscriber{a: a}
	handlers := sub.Handlers()
	require.Len(t, handlers, 1)
	assert.Equal(t, remoteflags.FlagName("diagnostics_data_loss"), handlers[0].FlagName())
}

func TestDataLossFlag_GateTransitions(t *testing.T) {
	a := getTestAtel(t, nil, dataLossTestConfig, &senderMock{}, nil, newRunnerMock())
	gate := a.diagnosticEnabled[dataLossProfileName]
	require.NotNil(t, gate, "the diagnostic profile must have a runtime gate")

	h := dataLossFlag{a: a}

	// Off by default.
	assert.False(t, gate.Load())

	// Enable / disable via OnChange.
	require.NoError(t, h.OnChange(true))
	assert.True(t, gate.Load())
	require.NoError(t, h.OnChange(false))
	assert.False(t, gate.Load())

	// OnNoConfig reverts to the safe (off) default.
	require.NoError(t, h.OnChange(true))
	h.OnNoConfig()
	assert.False(t, gate.Load())

	// SafeRecover forces off and is idempotent.
	require.NoError(t, h.OnChange(true))
	h.SafeRecover(nil, true)
	assert.False(t, gate.Load())
	h.SafeRecover(nil, true)
	assert.False(t, gate.Load())
}

func TestDataLossFlag_IsHealthyReflectsLastRun(t *testing.T) {
	a := getTestAtel(t, nil, dataLossTestConfig, &senderMock{}, nil, newRunnerMock())
	h := dataLossFlag{a: a}

	// Initialized healthy so the flag is not tripped before the first run.
	assert.True(t, h.IsHealthy())

	a.lastRunOK.Store(false)
	assert.False(t, h.IsHealthy())

	a.lastRunOK.Store(true)
	assert.True(t, h.IsHealthy())
}

func TestDataLossFlag_GateControlsShipping(t *testing.T) {
	tel := makeTelMock(t)
	counter := tel.NewCounter("bar", "zoo", nil, "")
	counter.Add(10)

	s := &senderMock{}
	r := newRunnerMock()
	a := getTestAtel(t, tel, dataLossTestConfig, s, nil, r)
	require.True(t, a.enabled)

	a.start()
	t.Cleanup(a.cancel)

	// Disabled by default: the scheduled run ships nothing for the profile.
	r.(*runnerMock).run()
	assert.Empty(t, s.sentMetrics, "diagnostic profile must not ship while its flag is off")

	// Enable the flag: the next run ships the profile's metric.
	h := dataLossFlag{a: a}
	require.NoError(t, h.OnChange(true))

	r.(*runnerMock).run()
	require.Len(t, s.sentMetrics, 1)
	assert.Equal(t, "bar.zoo", s.sentMetrics[0].name)

	// Disable again: subsequent runs stop shipping.
	s.sentMetrics = nil
	require.NoError(t, h.OnChange(false))
	r.(*runnerMock).run()
	assert.Empty(t, s.sentMetrics, "diagnostic profile must stop shipping once its flag is off")
}
