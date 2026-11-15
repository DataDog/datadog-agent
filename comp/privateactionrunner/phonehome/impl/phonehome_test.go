// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package phonehomeimpl

import (
	"bytes"
	"context"
	"runtime"
	"testing"
	"time"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/stretchr/testify/require"
)

type lifecycle struct{ hooks []compdef.Hook }

func (l *lifecycle) Append(h compdef.Hook) { l.hooks = append(l.hooks, h) }

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var output bytes.Buffer
	logger, err := log.LoggerFromWriterWithMinLevelAndLvlMsgFormat(&output, log.DebugLvl)
	require.NoError(t, err)
	log.SetupLogger(logger, "debug")
	output.Reset()
	t.Cleanup(func() { log.SetupLogger(log.Default(), "info") })
	return &output
}

func TestLifecycleDoesNotWaitForPrerequisites(t *testing.T) {
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{"private_action_runner.enabled": true, "private_action_runner.split_enabled": false})
	output := captureLogs(t)
	lc := &lifecycle{}
	tel := telemetrymock.New(t)
	require.NotNil(t, NewComponent(Requires{Config: cfg, Lifecycle: lc, Telemetry: tel}).Comp)
	if runtime.GOOS == "windows" {
		require.Empty(t, lc.hooks)
		require.Empty(t, output.String())
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
	require.Equal(t, map[string]string{"state": "blocked", "reason": "unsupported_configuration"}, gauges[0].Tags())
	counts, err := tel.GetCountMetric("par_phone_home", "transitions")
	require.NoError(t, err)
	require.Len(t, counts, 1)
	require.Equal(t, float64(1), counts[0].Value())
	require.Contains(t, output.String(), "[DEBUG] PAR startup gate: bypassed in an intentionally ungated mode: reason=unsupported_configuration")
	require.NotContains(t, output.String(), "[WARN]")
	require.NotContains(t, output.String(), "[INFO] PAR startup gate")
}

func TestDisabledPARDoesNotRegisterBackgroundWork(t *testing.T) {
	lc := &lifecycle{}
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{"private_action_runner.enabled": false})
	output := captureLogs(t)
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
	require.NotContains(t, output.String(), "PAR startup gate")
}
