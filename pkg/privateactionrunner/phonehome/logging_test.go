// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && !windows

package phonehome

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/procmgr"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

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

func TestStateLoggingLevelsAndDeduplication(t *testing.T) {
	for _, tc := range []struct{ reason, level string }{
		{"unsupported_configuration", "WARN"},
		{"invalid_supervisor_socket", "WARN"},
		{"supervisor_connection_failed", "WARN"},
		{"unsupported_supervisor_policy", "WARN"},
		{"previous_process_exit_requires_review", "WARN"},
		{"legacy_attempt_requires_reconciliation", "WARN"},
		{"launch_outcome_unknown_requires_review", "WARN"},
		{"rc_credential_or_endpoint_mismatch", "WARN"},
		{"invalid_credentials", "WARN"},
		{"invalid_endpoint", "WARN"},
		{"rc_locally_disabled", "INFO"},
		{"rc_unchecked", "INFO"},
		{"rc_org_disabled", "INFO"},
		{"rc_key_unauthorized", "INFO"},
		{"par_scope_unchecked", "INFO"},
		{"par_scope_waiting_for_rc_or_demand", "INFO"},
		{"par_scope_expired", "INFO"},
		{"missing_enrollment_scope", "INFO"},
		{"startup_jitter", "DEBUG"},
		{"supervisor_unavailable", "DEBUG"},
		{"supervisor_registration_unavailable", "DEBUG"},
		{"supervisor_registration_change_pending", "DEBUG"},
		{"rc_service_unavailable", "DEBUG"},
		{"discovery_unavailable", "DEBUG"},
		{"discovery_rate_limited", "DEBUG"},
		{"invalid_discovery_response", "DEBUG"},
		{"discovery_http_403", "DEBUG"},
	} {
		t.Run(tc.reason, func(t *testing.T) {
			output := captureLogs(t)
			report := newStateLogger(false)
			s := State{State: "waiting", Reason: tc.reason}
			report(s)
			first := output.String()
			require.Contains(t, first, "["+tc.level+"] PAR startup gate:")
			require.Equal(t, 1, strings.Count(first, "\n"))
			report(s)
			s.NextCheck = "timestamp-must-not-be-logged"
			report(s)
			require.Equal(t, first, output.String(), "polls and NextCheck changes are silent")
			s.State = "blocked"
			report(s)
			require.Equal(t, first+first, output.String(), "state is part of the deduplication key")
			report(State{State: "waiting", Reason: "rc_key_unauthorized"})
			before := output.String()
			report(s)
			require.Equal(t, before+first, output.String(), "re-entry is logged")
		})
	}
}

func TestStateLoggingExcludesUnexpectedData(t *testing.T) {
	output := captureLogs(t)
	report := newStateLogger(false)
	for _, reason := range []string{"api-key-private-key-urn", "discovery_http_https://user:password@private.example"} {
		report(State{State: "secret-state", Reason: reason, NextCheck: "secret-next-check"})
	}
	require.Equal(t, "[DEBUG] PAR startup gate: waiting to retry: reason=unrecognized_observation\n"+
		"[DEBUG] PAR startup gate: waiting to retry: reason=discovery_http_error\n", output.String())
}

func TestStateLoggingIntentionalBypass(t *testing.T) {
	output := captureLogs(t)
	report := newStateLogger(true)
	report(State{State: "blocked", Reason: "unsupported_configuration"})
	require.Equal(t, "[DEBUG] PAR startup gate: bypassed in an intentionally ungated mode: reason=unsupported_configuration\n", output.String())
}

func TestControllerLoggingStartupAndRecovery(t *testing.T) {
	f := newFixture(t)
	output := captureLogs(t)
	ctx := context.Background()
	f.rc.rc.Authorized = false
	f.c.step(ctx)
	f.c.step(ctx)
	require.Equal(t, "[INFO] PAR startup gate: waiting for permission: reason=rc_key_unauthorized\n", output.String())
	f.rc.rc.Authorized = true
	f.c.step(ctx)
	f.c.step(ctx)
	f.pm.control.State = pb.ProcessState_FAILED
	f.c.step(ctx)
	f.pm.describeErr = errors.New("https://secret:password@supervisor.invalid/private-key")
	f.c.step(ctx)
	f.c.step(ctx)
	f.pm.describeErr = nil
	f.c.step(ctx)
	require.Equal(t, 1, strings.Count(output.String(), "requesting supervisor Start"))
	require.Equal(t, 1, strings.Count(output.String(), "Start acknowledged"), "reconnecting is not a new handoff")
	require.Equal(t, 1, strings.Count(output.String(), "reason=supervisor_unavailable"))
	f.pm.control.Uuid = "private-registration-control"
	f.pm.control.State = pb.ProcessState_SKIPPED
	f.c.step(ctx)
	require.NotContains(t, output.String(), "fresh supervisor lifecycle")
	f.pm.executor.Uuid = "private-registration-executor"
	f.rc.rc.Authorized = false
	f.c.step(ctx)
	f.c.step(ctx)
	require.Equal(t, 1, strings.Count(output.String(), "[INFO] PAR startup gate: fresh supervisor lifecycle detected; new startup requires RC and fresh AAA authorization"))
	require.Equal(t, 1, f.pm.starts)
	f.rc.rc.Authorized = true
	f.c.step(ctx)
	require.Equal(t, 2, f.pm.starts)
	require.Equal(t, 2, strings.Count(output.String(), "[INFO] PAR startup gate: requesting supervisor Start: process=datadog-agent-par-control"))
	require.Equal(t, 2, strings.Count(output.String(), "[INFO] PAR startup gate: supervisor Start acknowledged; handoff is not enrollment or readiness"))
	require.NotContains(t, output.String(), "private-registration")
	require.NotContains(t, output.String(), "password")
	require.NotContains(t, output.String(), "private-key")
	require.NotContains(t, output.String(), "supervisor.invalid")
	require.NotContains(t, output.String(), "[WARN]")
}

func TestControllerLoggingExistingChildren(t *testing.T) {
	f := newFixture(t)
	f.pm.executor.State = pb.ProcessState_RUNNING
	output := captureLogs(t)
	f.c.step(context.Background())
	f.c.step(context.Background())
	require.Zero(t, f.pm.starts)
	require.Equal(t, "[INFO] PAR startup gate: existing children left untouched; supervisor retains ownership\n", output.String())
}

func TestControllerLoggingStartErrors(t *testing.T) {
	for _, code := range []codes.Code{codes.NotFound, codes.Unavailable, codes.DeadlineExceeded, codes.Canceled} {
		t.Run(code.String(), func(t *testing.T) {
			f := newFixture(t)
			f.pm.startErr = grpcstatus.Error(code, "private-key api-key key-hash urn:private https://secret.example")
			output := captureLogs(t)
			delay := f.c.step(context.Background())
			require.Equal(t, 1, f.pm.starts)
			if code == codes.NotFound {
				require.Equal(t, pollInterval, delay)
				require.Contains(t, output.String(), "[DEBUG] PAR startup gate: waiting to retry: reason=supervisor_registration_change_pending")
				require.NotContains(t, output.String(), "[WARN]")
			} else {
				require.Zero(t, delay)
				require.Contains(t, output.String(), "[WARN] PAR startup gate: startup requires operator review: reason=launch_outcome_unknown_requires_review")
				f.pm.control.Uuid, f.pm.executor.Uuid = "fresh-control", "fresh-executor"
				f.c.step(context.Background())
				require.Equal(t, 1, f.pm.starts)
			}
			require.Equal(t, 2, strings.Count(output.String(), "\n"))
			require.NotContains(t, output.String(), "Start acknowledged")
			for _, secret := range []string{"private-key", "api-key", "key-hash", "urn:private", "secret.example", "fresh-control", "fresh-executor"} {
				require.NotContains(t, output.String(), secret)
			}
		})
	}
}

func TestControllerLoggingCancellationAndDisabledPAR(t *testing.T) {
	f := newFixture(t)
	output := captureLogs(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f.c.run(ctx)
	require.Contains(t, output.String(), "[DEBUG] PAR startup gate: canceled")
	require.NotContains(t, output.String(), "[WARN]")
	require.NotContains(t, output.String(), "[INFO]")
	output.Reset()
	f.cfg.SetInTest(par.Enabled, false)
	Run(context.Background(), f.cfg, nil, func(State) { t.Fatal("disabled PAR must not report") })
	require.Empty(t, output.String())
}
