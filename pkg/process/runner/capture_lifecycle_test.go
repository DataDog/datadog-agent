// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package runner

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

func readinessSubmitter(t *testing.T) (*CheckSubmitter, *telemetrycapture.Manager) {
	t.Helper()
	deps := getSubmitterDeps(t, nil, nil)
	s, err := NewSubmitter(deps.Config, deps.Log, deps.Forwarders, deps.Statsd, "fixture-host", deps.SysProbeConfig)
	require.NoError(t, err)
	m := telemetrycapture.NewManager("process-agent", "fixture", "fixture")
	t.Cleanup(m.Close)
	s.CaptureManager = m
	return s, m
}

func TestCaptureReadinessRequiresSubmitterAndHealthyScheduler(t *testing.T) {
	s, manager := readinessSubmitter(t)
	s.SetCaptureCadence(checks.ProcessCheckName, 17*time.Second)
	s.SetCaptureCheckRunning(checks.ProcessCheckName, true)
	s.SetCaptureCheckHealthy(checks.ProcessCheckName, true)
	require.Empty(t, manager.Status().Capabilities, "constructing or configuring a producer must not advertise readiness")
	require.NoError(t, s.Start())
	defer s.Stop()
	require.Equal(t, []telemetrycapture.Capability{{Stream: telemetrycapture.Processes, Cadence: 17 * time.Second}}, manager.Status().Capabilities)
	s.SetCaptureCheckRunning(checks.ProcessCheckName, false)
	require.Empty(t, manager.Status().Capabilities)
	// A collection finishing during shutdown must not re-advertise the check.
	s.SetCaptureCheckHealthy(checks.ProcessCheckName, true)
	require.Empty(t, manager.Status().Capabilities)
}

func TestCaptureReadinessExcludesUnavailableAndDroppedChecks(t *testing.T) {
	for _, drop := range []bool{false, true} {
		t.Run(map[bool]string{false: "unavailable", true: "dropped"}[drop], func(t *testing.T) {
			s, manager := readinessSubmitter(t)
			if drop {
				s.dropCheckPayloads = []string{checks.ConnectionsCheckName}
			}
			require.NoError(t, s.Start())
			defer s.Stop()
			s.SetCaptureCadence(checks.ConnectionsCheckName, 31*time.Second)
			s.SetCaptureCheckRunning(checks.ConnectionsCheckName, true)
			s.SetCaptureCheckHealthy(checks.ConnectionsCheckName, false)
			require.Empty(t, manager.Status().Capabilities, "unavailable system-probe must not make a configured fallback ready")
			s.SetCaptureCheckHealthy(checks.ConnectionsCheckName, true)
			if drop {
				require.Empty(t, manager.Status().Capabilities)
				return
			}
			require.Equal(t, []telemetrycapture.Capability{{Stream: telemetrycapture.Connections, Cadence: 31 * time.Second, ConnectionOwner: "process"}}, manager.Status().Capabilities)
			control := telemetrycapture.Control{ProtocolVersion: 1, SessionID: "process-readiness-session"}
			_, err := manager.Prepare(telemetrycapture.PrepareRequest{Control: control, Streams: []telemetrycapture.Stream{telemetrycapture.Connections}})
			require.NoError(t, err)
			_, err = manager.Activate(control)
			require.NoError(t, err)
			s.SetCaptureCheckHealthy(checks.ConnectionsCheckName, false)
			require.False(t, manager.Enabled())
			require.Empty(t, manager.Status().Capabilities)
			require.Equal(t, telemetrycapture.Failed, manager.Status().State)
		})
	}
}

func TestCaptureSubmitterStopDisarmsAndIgnoresLateHealth(t *testing.T) {
	s, manager := readinessSubmitter(t)
	require.NoError(t, s.Start())
	s.SetCaptureCadence(checks.ProcessCheckName, time.Second)
	s.SetCaptureCheckRunning(checks.ProcessCheckName, true)
	s.SetCaptureCheckHealthy(checks.ProcessCheckName, true)
	require.Len(t, manager.Status().Capabilities, 1)
	control := telemetrycapture.Control{ProtocolVersion: 1, SessionID: "process-stop-session"}
	_, err := manager.Prepare(telemetrycapture.PrepareRequest{Control: control, Streams: []telemetrycapture.Stream{telemetrycapture.Processes}})
	require.NoError(t, err)
	_, err = manager.Activate(control)
	require.NoError(t, err)
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		for range 100 {
			s.SetCaptureCheckHealthy(checks.ProcessCheckName, true)
		}
	}()
	s.Stop()
	workers.Wait()
	require.False(t, manager.Enabled())
	require.Empty(t, manager.Status().Capabilities)
	require.Equal(t, telemetrycapture.Failed, manager.Status().State)
}

type readinessCheck struct {
	testCheck
	err error
}

func (c *readinessCheck) Run(func() int32, *checks.RunOptions) (checks.RunResult, error) {
	return checks.StandardRunResult{}, c.err
}

func TestNormalCollectionResultsDriveCaptureReadiness(t *testing.T) {
	s, manager := readinessSubmitter(t)
	require.NoError(t, s.Start())
	defer s.Stop()
	runner, err := NewRunnerWithChecks(configmock.New(t), nil, nil, nil, false, nil)
	require.NoError(t, err)
	runner.Submitter = s
	runner.setCaptureCadence(checks.ConnectionsCheckName, 30*time.Second)
	runner.setCaptureCheckRunning(checks.ConnectionsCheckName, true)
	c := &readinessCheck{testCheck: testCheck{name: checks.ConnectionsCheckName}, err: errors.New("fixture unavailable")}
	runner.runCheck(c)
	require.Empty(t, manager.Status().Capabilities)
	c.err = nil
	runner.runCheck(c)
	require.Len(t, manager.Status().Capabilities, 1)
	c.err = errors.New("fixture unavailable")
	runner.runCheck(c)
	require.Empty(t, manager.Status().Capabilities)
}
