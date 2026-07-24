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

//go:embed testdata/errortracking-system-probe.yaml
var errorTrackingSystemProbeConfig string

//go:embed testdata/errortracking-security-agent.yaml
var errorTrackingSecurityAgentConfig string

//go:embed testdata/error_check.yaml
var errorCheckConfig string

//go:embed testdata/error_check.py
var errorCheckPy string

// processAgentSubmissionErrorMessage is logged by readResponseStatuses
// (pkg/process/runner/runner.go) whenever the connections check — guaranteed
// to run standalone in process-agent — fails to submit to the
// connection-refused address configured in the testdata above.
const processAgentSubmissionErrorMessage = "[connections] Error from"

// securityAgentCWSConnectionErrorMessage is logged by startEventStreamListener
// (pkg/security/agent/agent.go) whenever the CWS event-stream client fails to
// reach the runtime security module, which the testdata configs never enable.
const securityAgentCWSConnectionErrorMessage = "error while connecting to the runtime security module"

// systemProbeFilterErrorMessage is logged by npcollector's newConfig
// (comp/networkpath/npcollector/impl/config.go) whenever
// network_path.collector.filters fails to unmarshal into []connfilter.Config.
// newConfig runs unconditionally on every system-probe startup, before any
// enabled-check, so a malformed filters value fires this deterministically.
const systemProbeFilterErrorMessage = "Error unmarshalling network_path.collector.filters"

type errorTrackingSuite struct {
	e2e.BaseSuite[environments.Host]
}

// errorTrackingAgentOptions builds the shared set of agent options that
// misconfigure every binary sharing the errortracking pipeline (core agent,
// process-agent, security-agent, system-probe) to emit a deterministic error,
// layered on top of agentConfig (which toggles agent_telemetry.errortracking.enabled).
func errorTrackingAgentOptions(agentConfig string) []agentparams.Option {
	return []agentparams.Option{
		agentparams.WithAgentConfig(agentConfig),
		agentparams.WithSystemProbeConfig(errorTrackingSystemProbeConfig),
		agentparams.WithSecurityAgentConfig(errorTrackingSecurityAgentConfig),
		agentparams.WithIntegration("error_check.d", errorCheckConfig),
		agentparams.WithFile("/etc/datadog-agent/checks.d/error_check.py", errorCheckPy, true),
	}
}

// TestAgentTelemetryErrorTrackingSuite is the entry point for the suite. It
// provisions ONE host misconfigured so every binary sharing the errortracking
// pipeline — core agent, process-agent, security-agent, system-probe — emits
// a deterministic error, and asserts each reaches FakeIntake with the correct
// agent.flavor tag. This covers all four binaries with a single VM instead of
// one per binary; cluster-agent and otel-agent are Kubernetes-based and
// covered by their own suites.
func TestAgentTelemetryErrorTrackingSuite(t *testing.T) {
	e2e.Run(t, &errorTrackingSuite{},
		e2e.WithProvisioner(
			awshost.Provisioner(
				awshost.WithRunOptions(
					ec2.WithAgentOptions(errorTrackingAgentOptions(errorTrackingEnabledConfig)...),
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

// TestPayloadShape verifies the happy path end-to-end for every origin:
//
//   - Core agent, Python path: error_check.py calls self.log.error(...),
//     crossing the Python→Go bridge at pkg/collector/python.LogMessage.
//     PCs[0] lands in datadog_agent.go.
//   - Core agent, Go path: error_check.py raises ValueError, caught and
//     logged via pkg/collector/worker.(*CheckLogger).Error. PCs[0] lands in
//     check_logger.go.
//   - process-agent: the connections check's submission error.
//   - security-agent: the CWS event-stream connection error.
//   - system-probe: the network_path.collector.filters unmarshal error.
//
// FakeIntake must receive at least one record of each kind with the expected
// wire shape, stack format, Source Code Integration tags, and agent.flavor.
func (s *errorTrackingSuite) TestPayloadShape() {
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	// system-probe's network_path.collector.filters error is a startup-only
	// trigger fired once by npcollector's newConfig — unlike the other three
	// binaries' errors, which recur (checks and submission retries keep
	// firing, so even if an earlier delivery is flushed away, a fresh one
	// arrives within the wait below). BeforeTest already reset to the
	// suite's original (enabled) provisioner before this method ran, which
	// may have restarted system-probe and delivered its one-shot error
	// BEFORE the flush above wiped it, leaving nothing left to fire again.
	// Explicitly re-provisioning here, AFTER the flush, guarantees system-probe
	// restarts and its one-shot error survives to be queried below.
	s.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(
			ec2.WithAgentOptions(errorTrackingAgentOptions(errorTrackingEnabledConfig)...),
		),
	))

	var pythonLogs, coreLogs, processLogs, securityLogs, systemProbeLogs []*aggregator.AgentTelemetryLog
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		logs, err := s.Env().FakeIntake.Client().GetAgentTelemetryLogs()
		require.NoError(c, err)

		pythonLogs, coreLogs, processLogs, securityLogs, systemProbeLogs = nil, nil, nil, nil, nil
		for _, l := range logs {
			switch {
			case strings.Contains(l.StackTrace, "datadog_agent.go"):
				pythonLogs = append(pythonLogs, l)
			case strings.Contains(l.StackTrace, "check_logger.go"):
				coreLogs = append(coreLogs, l)
			case strings.Contains(l.StackTrace, "pkg/process/runner/runner.go"):
				processLogs = append(processLogs, l)
			case strings.Contains(l.StackTrace, "pkg/security/agent/agent.go"):
				securityLogs = append(securityLogs, l)
			case strings.Contains(l.StackTrace, "comp/networkpath/npcollector/impl/config.go"):
				systemProbeLogs = append(systemProbeLogs, l)
			}
		}
		assert.NotEmpty(c, pythonLogs, "no core-agent Python-path error logs received yet")
		assert.NotEmpty(c, coreLogs, "no core-agent Go-core error logs received yet")
		assert.NotEmpty(c, processLogs, "no process-agent error logs received yet")
		assert.NotEmpty(c, securityLogs, "no security-agent error logs received yet")
		assert.NotEmpty(c, systemProbeLogs, "no system-probe error logs received yet")
	}, 2*time.Minute, 5*time.Second, "timed out waiting for error logs from every agent binary")

	for _, l := range append(pythonLogs, coreLogs...) {
		assertCommonLogShape(s.T(), l, flavor.DefaultAgent)
	}
	for _, l := range processLogs {
		assertCommonLogShape(s.T(), l, flavor.ProcessAgent)
	}
	for _, l := range securityLogs {
		assertCommonLogShape(s.T(), l, flavor.SecurityAgent)
	}
	for _, l := range systemProbeLogs {
		assertCommonLogShape(s.T(), l, flavor.SystemProbe)
	}

	// Python path: log.Error(string) carries no error-typed slog attribute,
	// so ErrorKind is always empty. Call site is in datadog_agent.go.
	for _, l := range pythonLogs {
		assert.Empty(s.T(), l.ErrorKind, "error_kind must be empty for Python-path logs")
	}
	// Go core path: log.Errorc(string, ...) also carries no error-typed slog
	// attribute, so ErrorKind is empty here too. Call site is in
	// check_logger.go, not the Python bridge.
	for _, l := range coreLogs {
		assert.Empty(s.T(), l.ErrorKind, "error_kind must be empty for Go-core path (Errorc passes string context)")
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

// waitForLocalErrorOccurrence truncates logPath, then waits for grepPattern
// (a fixed string, matched via grep -F) to appear in it — confirming the
// error still fires locally even though errortracking is disabled.
func waitForLocalErrorOccurrence(t *testing.T, env *environments.Host, logPath, grepPattern, waitTimeoutMsg string) {
	t.Helper()
	env.RemoteHost.MustExecute("sudo truncate -s 0 " + logPath)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		out, execErr := env.RemoteHost.Execute("sudo grep -cF -- '" + grepPattern + "' " + logPath + " || true")
		assert.NoError(c, execErr)
		assert.NotEqual(c, "0", strings.TrimSpace(out))
	}, 2*time.Minute, 5*time.Second, waitTimeoutMsg)
}

// TestDisabledByDefault verifies that when the errortracking stanza omits
// `enabled` (defaulting to false), no agent-logs records reach FakeIntake from
// any binary even though every misconfigured trigger keeps firing locally.
func (s *errorTrackingSuite) TestDisabledByDefault() {
	s.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(
			ec2.WithAgentOptions(errorTrackingAgentOptions(errorTrackingDisabledConfig)...),
		),
	))
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	env := s.Env()

	// Core agent's check error uses a regex ("ERROR.*Error running check"),
	// unlike the other three binaries' fixed-string messages, so it can't
	// share waitForLocalErrorOccurrence's grep -F.
	_, execErr := env.RemoteHost.Execute("sudo truncate -s 0 /var/log/datadog/agent.log")
	require.NoError(s.T(), execErr)
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		out, execErr := env.RemoteHost.Execute("sudo awk '/ERROR.*Error running check/{count++} END{print count+0}' /var/log/datadog/agent.log")
		assert.NoError(c, execErr)
		assert.NotEqual(c, "0", strings.TrimSpace(out))
	}, 2*time.Minute, 5*time.Second, "timed out waiting for check error to appear in agent log")

	waitForLocalErrorOccurrence(s.T(), env, "/var/log/datadog/process-agent.log", processAgentSubmissionErrorMessage,
		"timed out waiting for submission error to appear in process-agent log")
	waitForLocalErrorOccurrence(s.T(), env, "/var/log/datadog/security-agent.log", securityAgentCWSConnectionErrorMessage,
		"timed out waiting for connection error to appear in security-agent log")
	waitForLocalErrorOccurrence(s.T(), env, "/var/log/datadog/system-probe.log", systemProbeFilterErrorMessage,
		"timed out waiting for filter unmarshal error to appear in system-probe log")

	// Confirm nothing is forwarded. The config sets flush_interval_seconds: 1, so
	// 5 s covers five flush cycles: if a regression enabled the forwarder, it would
	// flush within this window and the assertion would catch it.
	assert.Never(s.T(), func() bool {
		logs, err := s.Env().FakeIntake.Client().GetAgentTelemetryLogs()
		require.NoError(s.T(), err)
		return len(logs) > 0
	}, 5*time.Second, 500*time.Millisecond, "agent telemetry logs must not arrive when errortracking is disabled")
}
