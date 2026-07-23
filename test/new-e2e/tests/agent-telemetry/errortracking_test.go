// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agenttelemetry contains E2E tests for the agent's internal telemetry
// error-tracking pipeline: pkg/util/log/errortracking → comp/core/agenttelemetry →
// /api/v2/apmtelemetry (request_type: agent-logs).
package agenttelemetry

import (
	_ "embed"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	ec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

//go:embed testdata/errortracking-enabled.yaml
var errorTrackingEnabledConfig string

//go:embed testdata/errortracking-disabled.yaml
var errorTrackingDisabledConfig string

//go:embed testdata/error_check.yaml
var errorCheckConfig string

//go:embed testdata/error_check.py
var errorCheckPy string

type errorTrackingSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestAgentTelemetryErrorTrackingSuite is the entry point for the suite.
func TestAgentTelemetryErrorTrackingSuite(t *testing.T) {
	e2e.Run(t, &errorTrackingSuite{},
		e2e.WithProvisioner(
			awshost.Provisioner(
				awshost.WithRunOptions(
					ec2.WithAgentOptions(
						agentparams.WithAgentConfig(errorTrackingEnabledConfig),
						agentparams.WithIntegration("error_check.d", errorCheckConfig),
						agentparams.WithFile("/etc/datadog-agent/checks.d/error_check.py", errorCheckPy, true),
					),
				),
			),
		),
	)
}

// stackFrameRe matches Go's standard stack frame format:
// "function\n\tfile:line +0xaddr" — the format the Error Tracking parser expects.
var stackFrameRe = regexp.MustCompile(`\S+\n\t\S+:\d+ \+0x[0-9a-f]+`)

// commitSHARe matches a git.commit.sha tag carrying a 40-char hex SHA.
var commitSHARe = regexp.MustCompile(`git\.commit\.sha:[0-9a-f]{40}`)

// TestPayloadShape verifies the happy path end-to-end for both error origins:
//
//   - Python path: error_check.py calls self.log.error(...), which crosses the
//     Python→Go bridge at pkg/collector/python.LogMessage. PCs[0] lands in
//     datadog_agent.go.
//
//   - Go core path: error_check.py raises ValueError, the Go check worker catches
//     it and logs via pkg/collector/worker.(*CheckLogger).Error. PCs[0] lands in
//     check_logger.go.
//
// FakeIntake must receive at least one record of each kind with the expected
// wire shape, stack format, and Source Code Integration tags.
func (s *errorTrackingSuite) TestPayloadShape() {
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	var pythonLogs, coreLogs []*aggregator.AgentTelemetryLog
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		logs, err := s.Env().FakeIntake.Client().GetAgentTelemetryLogs()
		require.NoError(c, err)

		pythonLogs = nil
		coreLogs = nil
		for _, l := range logs {
			switch {
			case strings.Contains(l.StackTrace, "datadog_agent.go"):
				pythonLogs = append(pythonLogs, l)
			case strings.Contains(l.StackTrace, "check_logger.go"):
				coreLogs = append(coreLogs, l)
			}
		}
		assert.NotEmpty(c, pythonLogs, "no Python-path error logs received yet")
		assert.NotEmpty(c, coreLogs, "no Go-core error logs received yet")
	}, 1*time.Minute, 5*time.Second, "timed out waiting for both Python-path and Go-core error logs")

	for _, l := range append(pythonLogs, coreLogs...) {
		assertCommonLogShape(s.T(), l, flavor.DefaultAgent)
	}

	// Python path: log.Error(string) carries no error-typed slog attribute,
	// so ErrorKind is always empty. Call site is in datadog_agent.go.
	for _, l := range pythonLogs {
		assert.Empty(s.T(), l.ErrorKind, "error_kind must be empty for Python-path logs")
		assert.True(s.T(), strings.Contains(l.StackTrace, "datadog_agent.go"),
			"Python-path PCs[0] must be in datadog_agent.go; got stack:\n%s", l.StackTrace)
	}

	// Go core path: log.Errorc(string, ...) also carries no error-typed slog
	// attribute, so ErrorKind is empty here too. Call site is in check_logger.go,
	// not the Python bridge — a distinct stack that deduplicates independently.
	for _, l := range coreLogs {
		assert.Empty(s.T(), l.ErrorKind, "error_kind must be empty for Go-core path (Errorc passes string context)")
		assert.True(s.T(), strings.Contains(l.StackTrace, "check_logger.go"),
			"Go-core PCs[0] must be in check_logger.go; got stack:\n%s", l.StackTrace)
	}
}

// assertCommonLogShape checks wire-shape properties that must hold for every
// agent-logs record regardless of the error's origin. expectedFlavor is the
// pkg/util/flavor value of the binary that emitted the log (e.g.
// flavor.DefaultAgent, flavor.SecurityAgent) — each binary tags its own logs
// via agent.flavor, so callers running a non-core-agent binary must pass
// their own flavor rather than assume the core agent's.
func assertCommonLogShape(t *testing.T, l *aggregator.AgentTelemetryLog, expectedFlavor string) {
	t.Helper()
	assert.Equal(t, "ERROR", l.Level)
	assert.False(t, l.IsCrash, "agent error logs are not crash reports")
	assert.GreaterOrEqual(t, l.Count, 1)
	// Message is intentionally empty — user-controlled data is never shipped.
	assert.Empty(t, l.Message, "message must not be on the wire")
	// Stack format: must use Go's standard "function\n\tfile:line +0xaddr"
	// layout so the Error Tracking backend parser can extract frames.
	assert.NotEmpty(t, l.StackTrace, "stack_trace must be non-empty")
	assert.True(t, stackFrameRe.MatchString(l.StackTrace),
		"stack_trace must follow Go standard format (function\\n\\tfile:line +0xaddr); got:\n%s", l.StackTrace)
	// Source Code Integration tags: git.repository_url is always present;
	// git.commit.sha is injected via ldflags in CI builds.
	assert.Contains(t, l.Tags, "git.repository_url:https://github.com/DataDog/datadog-agent",
		"tags must carry git.repository_url for Source Code Integration")
	assert.True(t, commitSHARe.MatchString(l.Tags),
		"tags must carry a 40-char git.commit.sha; got: %q", l.Tags)
	// Origin tag: COAT uses agent.flavor to attribute errors across the
	// agent, cluster-agent, process-agent, etc.
	assert.Contains(t, l.Tags, "agent.flavor:"+expectedFlavor,
		"tags must carry agent.flavor identifying the emitting binary; got: %q", l.Tags)
}

// assertErrorTrackingLogReceived polls FakeIntake until at least one agent-logs
// record whose stack trace contains stackSubstr arrives, asserts each match
// has the expected common wire shape and agent.flavor tag, and returns the
// matches. Shared by the single-origin, Host-based binary variants of this
// suite (process-agent, security-agent, ...); the core agent's own
// TestPayloadShape above has two origins (Python and Go-core) and doesn't fit
// this single-substring shape.
func assertErrorTrackingLogReceived(t *testing.T, env *environments.Host, stackSubstr, expectedFlavor, notFoundMsg string) []*aggregator.AgentTelemetryLog {
	t.Helper()
	var logs []*aggregator.AgentTelemetryLog
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		all, err := env.FakeIntake.Client().GetAgentTelemetryLogs()
		require.NoError(c, err)

		logs = nil
		for _, l := range all {
			if strings.Contains(l.StackTrace, stackSubstr) {
				logs = append(logs, l)
			}
		}
		assert.NotEmpty(c, logs, notFoundMsg)
	}, 2*time.Minute, 5*time.Second, notFoundMsg)

	for _, l := range logs {
		assertCommonLogShape(t, l, expectedFlavor)
	}
	return logs
}

// assertNoErrorTrackingWhenDisabled truncates logPath, waits for grepPattern
// to appear in it (confirming the error still fires locally even though
// errortracking is disabled), then asserts no agent-logs record reaches
// FakeIntake for a window covering several flush cycles. Shared by the
// Host-based binary variants' TestDisabledByDefault.
func assertNoErrorTrackingWhenDisabled(t *testing.T, env *environments.Host, logPath, grepPattern, waitTimeoutMsg string) {
	t.Helper()
	env.RemoteHost.MustExecute("sudo truncate -s 0 " + logPath)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		out, execErr := env.RemoteHost.Execute("sudo grep -cF -- '" + grepPattern + "' " + logPath + " || true")
		assert.NoError(c, execErr)
		assert.NotEqual(c, "0", strings.TrimSpace(out))
	}, 2*time.Minute, 5*time.Second, waitTimeoutMsg)

	// The config sets flush_interval_seconds: 1, so 5 s covers five flush
	// cycles: if a regression enabled the forwarder, it would flush within
	// this window and the assertion would catch it.
	assert.Never(t, func() bool {
		logs, err := env.FakeIntake.Client().GetAgentTelemetryLogs()
		require.NoError(t, err)
		return len(logs) > 0
	}, 5*time.Second, 500*time.Millisecond, "agent telemetry logs must not arrive when errortracking is disabled")
}

// TestDisabledByDefault verifies that when the errortracking stanza is absent,
// no agent-logs records reach FakeIntake even when errors occur.
func (s *errorTrackingSuite) TestDisabledByDefault() {
	s.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(
			ec2.WithAgentOptions(
				// agent telemetry enabled; errortracking.enabled omitted (defaults to
				// false) with a 1s flush interval so the negative assertion below is fast.
				agentparams.WithAgentConfig(errorTrackingDisabledConfig),
				agentparams.WithIntegration("error_check.d", errorCheckConfig),
				agentparams.WithFile("/etc/datadog-agent/checks.d/error_check.py", errorCheckPy, true),
			),
		),
	))
	// flush fakeintake and clear log file
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())
	_, execErr := s.Env().RemoteHost.Execute("sudo truncate -s 0 /var/log/datadog/agent.log")
	require.NoError(s.T(), execErr)

	// Wait until the check error appears in the agent log — confirming errors are
	// generated locally before asserting they are not forwarded to telemetry.
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		out, execErr := s.Env().RemoteHost.Execute("sudo awk '/ERROR.*Error running check/{count++} END{print count+0}' /var/log/datadog/agent.log")
		assert.NoError(c, execErr)
		assert.NotEqual(c, "0", strings.TrimSpace(out))
	}, 1*time.Minute, 5*time.Second, "timed out waiting for check error to appear in agent log")

	// Confirm nothing is forwarded. The config sets flush_interval_seconds: 1, so
	// 5 s covers five flush cycles: if a regression enabled the forwarder, it would
	// flush within this window and the assertion would catch it.
	assert.Never(s.T(), func() bool {
		logs, err := s.Env().FakeIntake.Client().GetAgentTelemetryLogs()
		require.NoError(s.T(), err)
		return len(logs) > 0
	}, 5*time.Second, 500*time.Millisecond, "agent telemetry logs must not arrive when errortracking is disabled")
}
