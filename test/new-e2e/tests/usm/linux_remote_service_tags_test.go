// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package usm

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"

	"github.com/stretchr/testify/require"
)

// pythonRemoteTagsLinuxSuite tests remote service tags with a Python HTTP server on Linux.
type pythonRemoteTagsLinuxSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestPythonRemoteTagsLinuxSuite(t *testing.T) {
	t.Parallel()

	e2eParams := []e2e.SuiteOption{
		e2e.WithProvisioner(awshost.Provisioner(
			awshost.WithRunOptions(
				scenec2.WithAgentOptions(
					agentparams.WithAgentConfig("log_level: debug"),
					agentparams.WithSystemProbeConfig(systemProbeConfig),
				),
			),
		)),
	}

	e2e.Run(t, &pythonRemoteTagsLinuxSuite{}, e2eParams...)
}

func (s *pythonRemoteTagsLinuxSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	host := s.Env().RemoteHost

	// Verify python3 is available on the host.
	_, err := host.Execute("python3 --version")
	require.NoError(s.T(), err, "python3 not found on remote host")

	// Write and start socket-based HTTP servers on ports 8081 and 8082.
	_, err = host.WriteFile("/tmp/httpserver.py", []byte(httpServerScript))
	require.NoError(s.T(), err, "failed to write HTTP server script")
	host.MustExecute("nohup python3 /tmp/httpserver.py 8081 > /tmp/http8081.log 2>&1 </dev/null &")
	host.MustExecute("nohup python3 /tmp/httpserver.py 8082 > /tmp/http8082.log 2>&1 </dev/null &")

	waitForHTTPServer(s.T(), host, `python3 -c "import urllib.request; urllib.request.urlopen('http://localhost:%d/')"`, 8081)
	waitForHTTPServer(s.T(), host, `python3 -c "import urllib.request; urllib.request.urlopen('http://localhost:%d/')"`, 8082)

	// Wait for the connections pipeline to be active before running tests.
	waitForConnectionsPipeline(s.T(), s.Env().FakeIntake.Client())
}

func (s *pythonRemoteTagsLinuxSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)

	if !s.BaseSuite.IsDevMode() {
		s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	}
}

// TestPythonRemoteServiceTags verifies that connections to Python HTTP servers
// have RemoteServiceTagsIdx >= 0 with process-based remote service tags.
func (s *pythonRemoteTagsLinuxSuite) TestPythonRemoteServiceTags() {
	t := s.T()
	host := s.Env().RemoteHost

	const requestsPerPort = 4000
	sendPythonHTTPRequests(host, "python3", requestsPerPort)
	fetchAndAssertTaggedConnections(t, s.Env().FakeIntake.Client(), "python", 8081, 8082, requestsPerPort)
}

// pythonRemoteTagsDirectLinuxSuite is the direct send variant of pythonRemoteTagsLinuxSuite.
type pythonRemoteTagsDirectLinuxSuite struct {
	pythonRemoteTagsLinuxSuite
}

func TestPythonRemoteTagsDirectLinuxSuite(t *testing.T) {
	t.Parallel()

	e2eParams := []e2e.SuiteOption{
		e2e.WithProvisioner(awshost.Provisioner(
			awshost.WithRunOptions(
				scenec2.WithAgentOptions(
					agentparams.WithAgentConfig("log_level: debug"),
					agentparams.WithSystemProbeConfig(systemProbeConfigDirect),
				),
			),
		)),
	}

	e2e.Run(t, &pythonRemoteTagsDirectLinuxSuite{}, e2eParams...)
}
