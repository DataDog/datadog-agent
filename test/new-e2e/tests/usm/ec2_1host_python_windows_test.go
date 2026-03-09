// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package usm

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	ec2windows "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2/windows"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"

	"github.com/stretchr/testify/require"
)

// httpRemoteTagsWindowsSuite tests remote service tags with an HTTP listener on Windows.
type httpRemoteTagsWindowsSuite struct {
	e2e.BaseSuite[environments.WindowsHost]
}

func TestHTTPRemoteTagsWindowsSuite(t *testing.T) {
	t.Parallel()

	e2eParams := []e2e.SuiteOption{
		e2e.WithProvisioner(winawshost.Provisioner(
			winawshost.WithRunOptions(
				ec2windows.WithAgentOptions(
					agentparams.WithAgentConfig("log_level: debug"),
					agentparams.WithSystemProbeConfig(systemProbeConfigPython),
				),
			),
		)),
	}

	e2e.Run(t, &httpRemoteTagsWindowsSuite{}, e2eParams...)
}

func (s *httpRemoteTagsWindowsSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	host := s.Env().RemoteHost

	// Kill anything listening on test ports from previous runs on a reused stack.
	host.Execute(`(Get-NetTCPConnection -LocalPort 8081,8082 -State Listen -ErrorAction SilentlyContinue).OwningProcess | Sort-Object -Unique | ForEach-Object { Stop-Process -Id $_ -Force -ErrorAction SilentlyContinue }`)
	time.Sleep(2 * time.Second)

	// Find the embedded Python executable.
	pythonExe := `C:\Program Files\Datadog\Datadog Agent\embedded3\python.exe`
	out, err := host.Execute(`Test-Path "` + pythonExe + `"`)
	s.T().Logf("embedded python exists: %s (err=%v)", out, err)
	require.Contains(s.T(), out, "True", "embedded Python not found at %s", pythonExe)

	out, _ = host.Execute(`& "` + pythonExe + `" --version`)
	s.T().Logf("embedded python version: %s", out)

	// Write a minimal HTTP server script using only the built-in socket module.
	host.MustExecute(`New-Item -ItemType Directory -Force -Path C:\temp | Out-Null`)
	serverScript := `import socket, sys
port = int(sys.argv[1])
s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind(("0.0.0.0", port))
s.listen(200)
while True:
    conn, addr = s.accept()
    conn.recv(4096)
    conn.sendall(b"HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: keep-alive\r\n\r\nok")
    conn.close()
`
	_, err = host.WriteFile(`C:\temp\httpserver.py`, []byte(serverScript))
	require.NoError(s.T(), err, "failed to write HTTP server script")

	// Start servers using WMI to create truly detached processes that survive SSH session cleanup.
	// Start-Process spawned children get killed when the SSH session ends; WMI-created processes
	// are fully detached from the session.
	for _, port := range []string{"8081", "8082"} {
		out, err = host.Execute(`$r = Invoke-CimMethod -ClassName Win32_Process -MethodName Create -Arguments @{` +
			`CommandLine='"` + pythonExe + `" C:\temp\httpserver.py ` + port + `'}; ` +
			`Write-Output "port` + port + `: pid=$($r.ProcessId) rc=$($r.ReturnValue)"`)
		s.T().Logf("start server %s: %s (err=%v)", port, out, err)
		require.NoError(s.T(), err, "WMI process creation failed for port %s", port)
		require.Contains(s.T(), out, "rc=0", "WMI process creation returned non-zero for port %s", port)
	}

	time.Sleep(5 * time.Second)

	// Verify servers are running and listening.
	out, _ = host.Execute(`Get-Process python* | Format-Table Id,ProcessName -AutoSize`)
	s.T().Logf("python processes:\n%s", out)
	out, _ = host.Execute(`Get-NetTCPConnection -LocalPort 8081,8082 -State Listen -ErrorAction SilentlyContinue | Format-Table`)
	s.T().Logf("listening on 8081/8082:\n%s", out)

	_, err = host.Execute("Invoke-WebRequest -UseBasicParsing http://localhost:8081/")
	require.NoError(s.T(), err, "Python HTTP server on port 8081 not responding")
	_, err = host.Execute("Invoke-WebRequest -UseBasicParsing http://localhost:8082/")
	require.NoError(s.T(), err, "Python HTTP server on port 8082 not responding")

	// Deploy locally-built binaries from the branch.
	deployWindowsBinaries(s.T(), host)
}

func (s *httpRemoteTagsWindowsSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)

	if !s.BaseSuite.IsDevMode() {
		s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	}
}

// TestHTTPRemoteServiceTags verifies that connections to HTTP listeners
// have RemoteServiceTagsIdx >= 0 with process-based remote service tags.
func (s *httpRemoteTagsWindowsSuite) TestHTTPRemoteServiceTags() {
	t := s.T()
	host := s.Env().RemoteHost

	const requestsPerPort = 100
	sendWindowsKeepAliveRequests(host, requestsPerPort, 40)

	time.Sleep(30 * time.Second)

	cnx, err := s.Env().FakeIntake.Client().GetConnections()
	require.NoError(t, err, "GetConnections() error")
	require.NotNil(t, cnx, "GetConnections() returned nil")

	stats := getConnectionStats(t, cnx, "process_context:")
	assertTaggedConnections(t, stats, "http", requestsPerPort)
}
