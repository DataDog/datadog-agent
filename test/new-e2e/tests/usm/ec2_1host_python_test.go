// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package usm

import (
	"fmt"
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

	// Start two Python HTTP servers on different ports.
	host.MustExecute("nohup python3 -m http.server 8081 --bind 0.0.0.0 > /tmp/http8081.log 2>&1 </dev/null &")
	host.MustExecute("nohup python3 -m http.server 8082 --bind 0.0.0.0 > /tmp/http8082.log 2>&1 </dev/null &")

	time.Sleep(2 * time.Second)

	_, err = host.Execute(`python3 -c "import urllib.request; urllib.request.urlopen('http://localhost:8081/')"`)
	require.NoError(s.T(), err, "Python HTTP server on port 8081 not responding")
	_, err = host.Execute(`python3 -c "import urllib.request; urllib.request.urlopen('http://localhost:8082/')"`)
	require.NoError(s.T(), err, "Python HTTP server on port 8082 not responding")

	// In CI, the provisioner installs the agent built from the current branch.
	// For local dev, uncomment to deploy locally-built binaries:
	// deployLinuxBinaries(s.T(), host)
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

	// Open 100 keep-alive connections per port and hold them for 40 seconds so
	// system-probe has time to resolve process context before connections close.
	const requestsPerPort = 100
	host.MustExecute(fmt.Sprintf(`python3 -c "
import socket, time

conns = []
for port in [8081, 8082]:
    for i in range(%d):
        s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        s.connect(('127.0.0.1', port))
        s.sendall(b'GET / HTTP/1.1\r\nHost: localhost\r\nConnection: keep-alive\r\n\r\n')
        s.recv(4096)
        conns.append(s)

print(f'opened {len(conns)} connections, holding 40s')
time.sleep(40)

for s in conns:
    s.close()
print('done')
"`, requestsPerPort))

	time.Sleep(30 * time.Second)

	cnx, err := s.Env().FakeIntake.Client().GetConnections()
	require.NoError(t, err, "GetConnections() error")
	require.NotNil(t, cnx, "GetConnections() returned nil")

	stats := getConnectionStats(t, cnx, "process_context:")
	assertTaggedConnections(t, stats, "python", requestsPerPort)
}
