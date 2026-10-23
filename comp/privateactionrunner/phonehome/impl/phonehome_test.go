// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package phonehomeimpl

import (
	"context"
	"runtime"
	"testing"
	"time"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/stretchr/testify/require"
)

type lifecycle struct{ hooks []compdef.Hook }

func (l *lifecycle) Append(h compdef.Hook) { l.hooks = append(l.hooks, h) }

func TestLifecycleDoesNotWaitForPrerequisites(t *testing.T) {
	t.Setenv("DD_PAR_PHONE_HOME_POC", "true")
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{"private_action_runner.enabled": true})
	lc := &lifecycle{}
	tel := telemetrymock.New(t)
	require.NotNil(t, NewComponent(Requires{Config: cfg, Lifecycle: lc, Telemetry: tel}).Comp)
	if runtime.GOOS == "windows" {
		require.Empty(t, lc.hooks)
		return
	}
	require.Len(t, lc.hooks, 1)
	require.NoError(t, lc.hooks[0].OnStart(context.Background()))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, lc.hooks[0].OnStop(ctx))
	// This configuration is unsupported on every platform: verify the real Run path reports it.
	gauges, err := tel.GetGaugeMetric("par_phone_home", "state")
	require.NoError(t, err)
	require.Len(t, gauges, 1)
	require.Equal(t, float64(1), gauges[0].Value())
	require.Equal(t, map[string]string{"state": "blocked", "reason": "unsupported_poc_configuration"}, gauges[0].Tags())
	counts, err := tel.GetCountMetric("par_phone_home", "transitions")
	require.NoError(t, err)
	require.Len(t, counts, 1)
	require.Equal(t, float64(1), counts[0].Value())
}

func TestOptOutDoesNotRegisterBackgroundWork(t *testing.T) {
	t.Setenv("DD_PAR_PHONE_HOME_POC", "false")
	lc := &lifecycle{}
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{"private_action_runner.enabled": true})
	tel := telemetrymock.New(t)
	component := NewComponent(Requires{Config: cfg, Lifecycle: lc, Telemetry: tel}).Comp
	require.NotNil(t, component)
	require.Nil(t, component.Status())
	require.Empty(t, lc.hooks)
	metrics, err := tel.Gather(telemetry.NoFilter)
	require.NoError(t, err)
	for _, metric := range metrics {
		require.NotContains(t, metric.GetName(), "par_phone_home")
	}
}
