// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !serverless

package wire

import (
	"testing"

	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/stretchr/testify/require"
)

func TestInternerTelemetry(t *testing.T) {
	tel := telemetryimpl.NewMock(t)
	interner := newStringInterner(2, 7, NewInternerTelemetry(true, tel))
	for _, value := range []string{"aa", "bb", "aa", "c"} {
		require.Equal(t, value, interner.LoadOrStore([]byte(value)))
	}
	for name, want := range map[string]float64{"string_interner_hits": 1, "string_interner_miss": 3, "string_interner_resets": 1} {
		values, err := tel.GetCountMetric("dogstatsd", name)
		require.NoError(t, err)
		require.Len(t, values, 1)
		require.Equal(t, "interner_7", values[0].Tags()["interner_id"])
		require.Equal(t, want, values[0].Value())
	}
	for name, want := range map[string]float64{"string_interner_entries": 1, "string_interner_bytes": 1} {
		values, err := tel.GetGaugeMetric("dogstatsd", name)
		require.NoError(t, err)
		require.Len(t, values, 1)
		require.Equal(t, "interner_7", values[0].Tags()["interner_id"])
		require.Equal(t, want, values[0].Value())
	}
}

func TestInternerTelemetryDisabled(t *testing.T) {
	tel := telemetryimpl.NewMock(t)
	interner := newStringInterner(1, 7, NewInternerTelemetry(false, tel))
	for _, value := range []string{"aa", "aa", "bb"} {
		require.Equal(t, value, interner.LoadOrStore([]byte(value)))
	}
	values, err := tel.GetRegistry().Gather()
	require.NoError(t, err)
	require.Empty(t, values)
}

func TestPoolTelemetryDisabled(t *testing.T) {
	tel := telemetryimpl.NewMock(t)
	pool := NewFloat64ListPool(false, tel)
	before, err := tel.GetRegistry().Gather()
	require.NoError(t, err)
	values := pool.Get()
	require.Empty(t, values)
	pool.Put(append(values, 1, 2))
	after, err := tel.GetRegistry().Gather()
	require.NoError(t, err)
	require.Equal(t, before, after)
}
