// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agenttelemetry

import (
	_ "embed"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	ec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

//go:embed testdata/errortracking-python.yaml
var errorTrackingPythonConfig string

//go:embed testdata/directory_error.yaml
var directoryErrorConfig string

//go:embed testdata/tcp_check_error.yaml
var tcpCheckErrorConfig string

// checkExceptionKind is the exception raised by tcp_check when the host cannot
// be resolved, as named by the interpreter (module qualified, not builtin).
const checkExceptionKind = "datadog_checks.base.errors.CheckException"

type errorTrackingPythonSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestAgentTelemetryErrorTrackingPythonSuite provisions one host running two
// Datadog-shipped integrations that log errors (directory and tcp_check, the
// latter also raising) and the custom check used by the main suite.
func TestAgentTelemetryErrorTrackingPythonSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &errorTrackingPythonSuite{},
		e2e.WithProvisioner(
			awshost.Provisioner(
				awshost.WithRunOptions(
					ec2.WithAgentOptions(
						agentparams.WithAgentConfig(errorTrackingPythonConfig),
						agentparams.WithIntegration("directory.d", directoryErrorConfig),
						agentparams.WithIntegration("tcp_check.d", tcpCheckErrorConfig),
						agentparams.WithIntegration("error_check.d", errorCheckConfig),
						agentparams.WithFile("/etc/datadog-agent/checks.d/error_check.py", errorCheckPy, true),
					),
				),
			),
		),
	)
}

// assertPythonLogShape checks the properties every Python-origin record must
// hold: the wire shape, an integration tag, Python frames, and no agent
// repository tags (the frames do not live in the agent repository).
func assertPythonLogShape(t *testing.T, l *aggregator.AgentTelemetryLog) {
	t.Helper()
	assert.Equal(t, "ERROR", l.Level)
	assert.False(t, l.IsCrash)
	assert.GreaterOrEqual(t, l.Count, 1)
	assert.Empty(t, l.Message, "message must not be on the wire")
	assert.True(t, stackFrameRe.MatchString(l.StackTrace),
		"stack_trace must follow the frame format (function\\n\\tfile:line +0xaddr); got:\n%s", l.StackTrace)
	assert.Contains(t, l.Tags, "integration:", "tags must carry the integration")
	assert.Contains(t, l.Tags, "agent.flavor:agent")
	assert.NotContains(t, l.Tags, "git.repository_url", "python frames must not resolve in the agent repository")
	assert.NotContains(t, l.Tags, "git.commit.sha")
	assert.NotContains(t, l.StackTrace, "datadog_agent.go", "the cgo bridge must not appear in python stacks")
	assertNoLocalPaths(t, l)
}

// assertNoLocalPaths checks no absolute path, user or install layout detail is
// shipped.
func assertNoLocalPaths(t *testing.T, l *aggregator.AgentTelemetryLog) {
	t.Helper()
	for _, forbidden := range []string{"/opt/", "/etc/", "/home/", "/usr/", "site-packages", "checks.d", "errortracking-e2e"} {
		assert.NotContains(t, l.StackTrace, forbidden, "stack_trace must not reveal %q", forbidden)
	}
}

// firstFrame returns the function line of the first frame of a stack.
func firstFrame(stack string) string {
	first, _, _ := strings.Cut(stack, "\n")
	return first
}

func (s *errorTrackingPythonSuite) TestPythonIntegrationStacks() {
	dumpDiagnosticsOnFailure(s.T(), s.Env())
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	var directoryLogs, tcpLogs, customLogs []*aggregator.AgentTelemetryLog
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		logs, err := s.Env().FakeIntake.Client().GetAgentTelemetryLogs()
		require.NoError(c, err)

		directoryLogs, tcpLogs, customLogs = nil, nil, nil
		for _, l := range logs {
			switch {
			case strings.Contains(l.Tags, "integration:directory"):
				directoryLogs = append(directoryLogs, l)
			case strings.Contains(l.Tags, "integration:tcp_check"):
				tcpLogs = append(tcpLogs, l)
			case strings.Contains(l.Tags, "integration:custom"):
				customLogs = append(customLogs, l)
			}
		}
		assert.NotEmpty(c, directoryLogs, "no directory integration error logs received yet")
		assert.NotEmpty(c, tcpLogs, "no tcp_check integration error logs received yet")
		assert.NotEmpty(c, customLogs, "no custom check error logs received yet")
	}, 2*time.Minute, 5*time.Second, "timed out waiting for Python error logs")

	// Datadog-shipped integrations: the first frame is the integration code
	// that logged, with a package-relative path.
	for _, l := range directoryLogs {
		assertPythonLogShape(s.T(), l)
		assert.True(s.T(), strings.HasPrefix(firstFrame(l.StackTrace), "datadog_checks.directory.directory."),
			"first frame must be the directory integration; got:\n%s", l.StackTrace)
		assert.Contains(s.T(), l.StackTrace, "datadog_checks/directory/directory.py:")
	}
	for _, l := range tcpLogs {
		assertPythonLogShape(s.T(), l)
		assert.True(s.T(), strings.HasPrefix(firstFrame(l.StackTrace), "datadog_checks.tcp_check.tcp_check."),
			"first frame must be the tcp_check integration; got:\n%s", l.StackTrace)
	}

	// Two integrations give two distinct stacks (and so two issues).
	assert.NotEqual(s.T(), directoryLogs[0].StackTrace, tcpLogs[0].StackTrace,
		"different integrations must not share a stack")

	// The raised exception: tcp_check raises after logging, and the exception
	// type is the kind of that record.
	var raised []*aggregator.AgentTelemetryLog
	for _, l := range tcpLogs {
		if l.ErrorKind == checkExceptionKind {
			raised = append(raised, l)
		}
	}
	require.NotEmpty(s.T(), raised, "no record carries the raised exception type %q", checkExceptionKind)
	for _, l := range raised {
		assert.Contains(s.T(), l.StackTrace, "datadog_checks.base.checks.base.run",
			"the raise path stack must include the base check runner")
	}
}

// TestCustomCheckFramesAreRedacted asserts the frames of a custom check, which
// is customer code, never reach the wire, for both the logged error and the
// raised exception.
func (s *errorTrackingPythonSuite) TestCustomCheckFramesAreRedacted() {
	dumpDiagnosticsOnFailure(s.T(), s.Env())
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	var logged, raised []*aggregator.AgentTelemetryLog
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		logs, err := s.Env().FakeIntake.Client().GetAgentTelemetryLogs()
		require.NoError(c, err)

		logged, raised = nil, nil
		for _, l := range logs {
			if !strings.Contains(l.Tags, "integration:custom") {
				continue
			}
			if l.ErrorKind == "ValueError" {
				raised = append(raised, l)
			} else {
				logged = append(logged, l)
			}
		}
		assert.NotEmpty(c, logged, "no custom check log-path record received yet")
		assert.NotEmpty(c, raised, "no custom check raise-path record received yet")
	}, 2*time.Minute, 5*time.Second, "timed out waiting for custom check error logs")

	for _, l := range append(logged, raised...) {
		assertPythonLogShape(s.T(), l)
		assert.True(s.T(), strings.HasPrefix(l.StackTrace, "<custom_check>\n\t<redacted>:0 +0x0"),
			"the custom check frame must be a placeholder first frame; got:\n%s", l.StackTrace)
		assert.NotContains(s.T(), l.StackTrace, "error_check", "custom check names must not be shipped")
		assert.NotContains(s.T(), l.StackTrace, "intentional", "exception messages must not be shipped")
	}
}
