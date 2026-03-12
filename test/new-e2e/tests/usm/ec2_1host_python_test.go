// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package usm

import (
	"path/filepath"
	"testing"
	"time"

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
	t.Skip("Skip until lower connection capture rate on Linux is resolved")
	t.Parallel()

	e2eParams := []e2e.SuiteOption{
		e2e.WithProvisioner(awshost.Provisioner(
			awshost.WithRunOptions(
				scenec2.WithAgentOptions(
					agentparams.WithAgentConfig("log_level: debug"),
					agentparams.WithSystemProbeConfig(systemProbeConfigPython),
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

	time.Sleep(2 * time.Second)

	_, err = host.Execute(`python3 -c "import urllib.request; urllib.request.urlopen('http://localhost:8081/')"`)
	require.NoError(s.T(), err, "HTTP server on port 8081 not responding")
	_, err = host.Execute(`python3 -c "import urllib.request; urllib.request.urlopen('http://localhost:8082/')"`)
	require.NoError(s.T(), err, "HTTP server on port 8082 not responding")

	// In CI, the provisioner installs the agent built from the current branch.
	// For local dev, uncomment to deploy locally-built binaries:
	deployLinuxBinaries(s.T(), host)
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
	fetchAndAssertTaggedConnections(t, host, s.Env().FakeIntake.Client(), "python", requestsPerPort)

	// Download agent logs for debugging.
	outputDir := s.SessionOutputDir()
	for _, logFile := range []string{"system-probe.log", "process-agent.log"} {
		remotePath := "/var/log/datadog/" + logFile
		tmpPath := "/tmp/" + logFile
		localPath := filepath.Join(outputDir, logFile)
		host.MustExecute("sudo cp " + remotePath + " " + tmpPath + " && sudo chmod 644 " + tmpPath)
		if err := host.GetFile(tmpPath, localPath); err != nil {
			t.Logf("failed to download %s: %v", logFile, err)
		}
	}
}
