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
					agentparams.WithSystemProbeConfig(systemProbeConfig),
					agentparams.WithPipeline("102564312"),
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
	out, _ := host.Execute(`Test-Path "` + pythonExe + `"`)
	require.Contains(s.T(), out, "True", "embedded Python not found at %s", pythonExe)

	// Write the shared socket-based HTTP server script.
	host.MustExecute(`New-Item -ItemType Directory -Force -Path C:\temp | Out-Null`)
	_, err := host.WriteFile(`C:\temp\httpserver.py`, []byte(httpServerScript))
	require.NoError(s.T(), err, "failed to write HTTP server script")

	// Start servers using WMI to create truly detached processes that survive SSH session cleanup.
	// Start-Process spawned children get killed when the SSH session ends; WMI-created processes
	// are fully detached from the session.
	for _, port := range []string{"8081", "8082"} {
		out, err = host.Execute(`$r = Invoke-CimMethod -ClassName Win32_Process -MethodName Create -Arguments @{` +
			`CommandLine='"` + pythonExe + `" C:\temp\httpserver.py ` + port + `'}; ` +
			`Write-Output "port` + port + `: pid=$($r.ProcessId) rc=$($r.ReturnValue)"`)
		require.NoError(s.T(), err, "WMI process creation failed for port %s", port)
		require.Contains(s.T(), out, "rc=0", "WMI process creation returned non-zero for port %s", port)
	}

	time.Sleep(5 * time.Second)

	_, err = host.Execute("Invoke-WebRequest -UseBasicParsing http://localhost:8081/")
	require.NoError(s.T(), err, "Python HTTP server on port 8081 not responding")
	_, err = host.Execute("Invoke-WebRequest -UseBasicParsing http://localhost:8082/")
	require.NoError(s.T(), err, "Python HTTP server on port 8082 not responding")
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

	const requestsPerPort = 4000
	sendWindowsKeepAliveRequestsToPort(host, 8081, requestsPerPort, 20)
	sendWindowsKeepAliveRequestsToPort(host, 8082, requestsPerPort, 20)
	fetchAndAssertTaggedConnections(t, s.Env().FakeIntake.Client(), "http", requestsPerPort)
}
