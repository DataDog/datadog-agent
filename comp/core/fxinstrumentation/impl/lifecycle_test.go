// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package fxinstrumentationimpl

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil/lifecycle"
)

func TestRecordFilePath(t *testing.T) {
	assert.Equal(t, filepath.Join("run", "lifecycle-system-probe.json"), RecordFilePath("run", "system_probe"))
}

func TestSetupLifecycleTelemetry(t *testing.T) {
	runPath := t.TempDir()
	tm := telemetryimpl.NewMock(t)
	logger := logmock.New(t)

	// A previous run killed while stopping a system-probe module.
	previous := lifecycle.NewRecorder(time.Now(), time.Now)
	previous.Persist(RecordFilePath(runPath, "system_probe"), nil)
	previous.MarkReady()
	previous.BeginStop(lifecycle.TriggerSignal)
	_ = previous.StartShutdownPhase(lifecycle.PhaseAppStop)
	previous.ComponentStopping("event_monitor")

	// The current run.
	current := lifecycle.NewRecorder(time.Now().Add(-3*time.Second), time.Now)
	SetupLifecycleTelemetry(current, tm, logger, runPath, "system_probe")

	exits := metrics(t, "previous_exit", tm.GetCountMetric)
	require.Len(t, exits, 1)
	assert.Equal(t, map[string]string{
		"outcome":   string(lifecycle.OutcomeKilledDuringShutdown),
		"trigger":   lifecycle.TriggerSignal,
		"phase":     lifecycle.PhaseAppStop,
		"component": "event_monitor",
	}, exits[0].Tags())

	shutdown := metrics(t, "previous_shutdown_seconds", tm.GetHistogramMetric)
	require.Len(t, shutdown, 1)
	assert.Equal(t, map[string]string{"phase": lifecycle.PhaseTotal, "outcome": string(lifecycle.OutcomeKilledDuringShutdown)}, shutdown[0].Tags())

	// The startup is only reported once the process is ready.
	_, err := tm.GetHistogramMetric(subsystem, "startup_duration_seconds")
	assert.Error(t, err)
	current.MarkReady()
	startup := metrics(t, "startup_duration_seconds", tm.GetHistogramMetric)
	require.Len(t, startup, 1)
	assert.Equal(t, map[string]string{"phase": lifecycle.PhaseTotal}, startup[0].Tags())
	assert.InDelta(t, 3, startup[0].Value(), 1)

	// The current run is persisted for the next one.
	rec, found, err := lifecycle.LoadPrevious(RecordFilePath(runPath, "system_probe"))
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, lifecycle.OutcomeDiedWhileRunning, rec.Outcome())
}

func TestSetupLifecycleTelemetryWithoutRunPath(t *testing.T) {
	tm := telemetryimpl.NewMock(t)
	current := lifecycle.NewRecorder(time.Now(), time.Now)
	SetupLifecycleTelemetry(current, tm, logmock.New(t), "", "agent")
	current.MarkReady()

	startup := metrics(t, "startup_duration_seconds", tm.GetHistogramMetric)
	assert.Len(t, startup, 1)
	_, err := tm.GetCountMetric(subsystem, "previous_exit")
	assert.Error(t, err, "no previous run is reported without a run path")
}

func metrics(t *testing.T, name string, get func(subsystem, name string) ([]telemetry.Metric, error)) []telemetry.Metric {
	t.Helper()
	m, err := get(subsystem, name)
	require.NoError(t, err)
	return m
}
