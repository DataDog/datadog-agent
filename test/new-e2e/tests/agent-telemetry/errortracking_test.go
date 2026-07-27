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
	"encoding/json"
	"io"
	"net/http"
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
	"github.com/DataDog/datadog-agent/test/fakeintake/api"
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

// processAgentSubmissionErrorMessage is logged by submitProcessLikePayload
// when the connections check's submission to the connection-refused testdata
// endpoint times out rather than failing fast (security-group-blocked port).
const processAgentSubmissionErrorMessage = "timed out waiting for responses"

// securityAgentCWSConnectionErrorMessage is logged by startEventStreamListener
// (pkg/security/agent/agent.go) whenever the CWS event-stream client fails to
// reach the runtime security module, which the testdata configs never enable.
const securityAgentCWSConnectionErrorMessage = "error while connecting to the runtime security module"

// systemProbeFilterErrorMessage is logged by npcollector's newConfig when
// network_path.collector.filters fails to unmarshal. newConfig runs
// unconditionally at startup, so a malformed value fires deterministically.
const systemProbeFilterErrorMessage = "Error unmarshalling network_path.collector.filters"

type errorTrackingSuite struct {
	e2e.BaseSuite[environments.Host]
}

// errorTrackingAgentOptions builds the shared agent options that misconfigure
// every binary sharing the errortracking pipeline to emit a deterministic
// error, layered on agentConfig (which toggles errortracking.enabled).
func errorTrackingAgentOptions(agentConfig string) []agentparams.Option {
	return []agentparams.Option{
		agentparams.WithAgentConfig(agentConfig),
		agentparams.WithSystemProbeConfig(errorTrackingSystemProbeConfig),
		agentparams.WithSecurityAgentConfig(errorTrackingSecurityAgentConfig),
		agentparams.WithIntegration("error_check.d", errorCheckConfig),
		agentparams.WithFile("/etc/datadog-agent/checks.d/error_check.py", errorCheckPy, true),
	}
}

// TestAgentTelemetryErrorTrackingSuite provisions ONE host misconfigured so
// every binary sharing the errortracking pipeline emits a deterministic
// error, covering all of them with a single VM instead of one per binary.
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

// dumpDiagnosticsOnFailure registers a Cleanup that, on failure, greps each
// binary's log for its trigger (a plain tail could miss a one-shot startup
// trigger) and decodes every stored apmtelemetry payload. TEMPORARY: remove once stable.
func dumpDiagnosticsOnFailure(t *testing.T, env *environments.Host) {
	t.Helper()
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		logFilePatterns := map[string]string{
			"agent.log":          "errortracking|ERROR.*Error running check",
			"process-agent.log":  "errortracking|" + processAgentSubmissionErrorMessage,
			"security-agent.log": "errortracking|" + securityAgentCWSConnectionErrorMessage,
			"system-probe.log":   "errortracking|network_path|Unknown key|npcollector|" + systemProbeFilterErrorMessage,
		}
		for logFile, pattern := range logFilePatterns {
			out, _ := env.RemoteHost.Execute(
				"sudo grep -n -i -E '" + pattern + "' /var/log/datadog/" + logFile + " | tail -n 80 || true")
			t.Logf("%s matches (diagnostic):\n%s", logFile, out)
		}

		// Each binary resolves its own endpoint from agent_telemetry.* config;
		// if one ever resolves to a different route, FakeIntake would store the
		// payload where this suite never looks even though the POST succeeded.
		if routes, err := env.FakeIntake.Client().RouteStats(); err == nil {
			t.Logf("fakeintake route stats (diagnostic): %v", routes)
		} else {
			t.Logf("fakeintake route stats fetch failed: %v", err)
		}

		dumpAPMTelemetryPayloadsOnFailure(t, env)
	})
}

// dumpAPMTelemetryPayloadsOnFailure fetches every raw payload stored for
// /api/v2/apmtelemetry and logs a one-line summary per record (or the
// inflate/json error for anything that fails to decode).
func dumpAPMTelemetryPayloadsOnFailure(t *testing.T, env *environments.Host) {
	t.Helper()
	resp, httpErr := http.Get(env.FakeIntake.Client().URL() + "/fakeintake/payloads?endpoint=/api/v2/apmtelemetry")
	if httpErr != nil {
		t.Logf("apmtelemetry payload dump failed: %v", httpErr)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var envelope api.APIFakeIntakePayloadsRawGETResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Logf("apmtelemetry envelope unmarshal failed: %v; raw body:\n%s", err, body)
		return
	}

	t.Logf("apmtelemetry payloads (diagnostic): %d payload(s) stored", len(envelope.Payloads))
	for i, p := range envelope.Payloads {
		inflated, err := aggregator.Inflate(p.Data, p.Encoding)
		if err != nil {
			t.Logf("payload[%d]: inflate FAILED (encoding=%q, len=%d): %v", i, p.Encoding, len(p.Data), err)
			continue
		}
		var env struct {
			RequestType string `json:"request_type"`
			Payload     struct {
				Logs []struct {
					Tags string `json:"tags"`
				} `json:"logs"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(inflated, &env); err != nil {
			preview := inflated
			if len(preview) > 200 {
				preview = preview[:200]
			}
			t.Logf("payload[%d]: json unmarshal FAILED (encoding=%q, len=%d): %v; first bytes: %q",
				i, p.Encoding, len(p.Data), err, preview)
			continue
		}
		if env.RequestType != "agent-logs" {
			t.Logf("payload[%d]: request_type=%q (not agent-logs, skipped)", i, env.RequestType)
			continue
		}
		for _, l := range env.Payload.Logs {
			t.Logf("payload[%d]: agent-logs record, tags=%q", i, l.Tags)
		}
	}
}

// TestPayloadShape verifies the happy path for every origin this branch
// wires into errortracking (core agent Python/Go paths, process-agent,
// security-agent); system-probe assertions land once its branch wires in production code.
func (s *errorTrackingSuite) TestPayloadShape() {
	dumpDiagnosticsOnFailure(s.T(), s.Env())
	// BeforeTest already reset the environment to the suite's original
	// (enabled) provisioner regardless of run order, and both remaining
	// triggers here recur on every check run, so no re-provisioning is needed.
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	var pythonLogs, coreLogs, processLogs, securityLogs []*aggregator.AgentTelemetryLog
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		logs, err := s.Env().FakeIntake.Client().GetAgentTelemetryLogs()
		require.NoError(c, err)

		pythonLogs, coreLogs, processLogs, securityLogs = nil, nil, nil, nil
		for _, l := range logs {
			// agent.flavor disambiguates process-agent/security-agent from the core
			// agent more robustly than pinning to an internal call site. The core
			// agent shares flavor.DefaultAgent across Python/Go-core, hence the stack-trace split.
			switch {
			case strings.Contains(l.Tags, "agent.flavor:"+flavor.ProcessAgent):
				processLogs = append(processLogs, l)
			case strings.Contains(l.Tags, "agent.flavor:"+flavor.SecurityAgent):
				securityLogs = append(securityLogs, l)
			case strings.Contains(l.StackTrace, "datadog_agent.go"):
				pythonLogs = append(pythonLogs, l)
			case strings.Contains(l.StackTrace, "check_logger.go"):
				coreLogs = append(coreLogs, l)
			}
		}
		assert.NotEmpty(c, pythonLogs, "no core-agent Python-path error logs received yet")
		assert.NotEmpty(c, coreLogs, "no core-agent Go-core error logs received yet")
		assert.NotEmpty(c, processLogs, "no process-agent error logs received yet")
		assert.NotEmpty(c, securityLogs, "no security-agent error logs received yet")
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

// assertCommonLogShape checks wire-shape properties every agent-logs record
// must hold regardless of origin. expectedFlavor is the pkg/util/flavor value
// of the binary that emitted the log — callers must pass their own, not assume the core agent's.
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

// waitForLocalErrorOccurrence optionally truncates logPath, then waits for
// grepPattern to appear in it. truncate must be false for a one-shot trigger
// already produced by an earlier restart, or truncating here erases it for good.
func waitForLocalErrorOccurrence(t *testing.T, env *environments.Host, logPath, grepPattern, waitTimeoutMsg string, truncate bool) {
	t.Helper()
	if truncate {
		env.RemoteHost.MustExecute("sudo truncate -s 0 " + logPath)
	}
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
	dumpDiagnosticsOnFailure(s.T(), s.Env())

	env := s.Env()
	// system-probe's filter-unmarshal error is a startup-only trigger; truncate
	// its log BEFORE switching config so the restart writes a fresh occurrence,
	// unlike waitForLocalErrorOccurrence's post-restart truncate for recurring triggers.
	env.RemoteHost.MustExecute("sudo truncate -s 0 /var/log/datadog/system-probe.log")

	s.UpdateEnv(awshost.Provisioner(
		awshost.WithRunOptions(
			ec2.WithAgentOptions(errorTrackingAgentOptions(errorTrackingDisabledConfig)...),
		),
	))
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

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
		"timed out waiting for submission error to appear in process-agent log", true)
	waitForLocalErrorOccurrence(s.T(), env, "/var/log/datadog/security-agent.log", securityAgentCWSConnectionErrorMessage,
		"timed out waiting for connection error to appear in security-agent log", true)
	// truncate=false: already truncated above, before the restart that produced
	// this one-shot trigger's sole log line.
	waitForLocalErrorOccurrence(s.T(), env, "/var/log/datadog/system-probe.log", systemProbeFilterErrorMessage,
		"timed out waiting for filter unmarshal error to appear in system-probe log", false)

	// Confirm nothing is forwarded. The config sets flush_interval_seconds: 1, so
	// 5 s covers five flush cycles: if a regression enabled the forwarder, it would
	// flush within this window and the assertion would catch it.
	assert.Never(s.T(), func() bool {
		logs, err := s.Env().FakeIntake.Client().GetAgentTelemetryLogs()
		require.NoError(s.T(), err)
		return len(logs) > 0
	}, 5*time.Second, 500*time.Millisecond, "agent telemetry logs must not arrive when errortracking is disabled")
}
