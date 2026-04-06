// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package usm

import (
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	ec2windows "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2/windows"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type windowsUSMSuite struct {
	e2e.BaseSuite[environments.WindowsHost]
}

func TestWindowsUSMSuite(t *testing.T) {
	t.Parallel()

	e2eParams := []e2e.SuiteOption{
		e2e.WithProvisioner(provisioners.NewTypedPulumiProvisioner("iisHost", windowsUSMProvisioner(), nil)),
	}

	e2e.Run(t, &windowsUSMSuite{}, e2eParams...)
}

func windowsUSMProvisioner() provisioners.PulumiEnvRunFunc[environments.WindowsHost] {
	return func(ctx *pulumi.Context, env *environments.WindowsHost) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}
		opts := []ec2windows.RunOption{
			ec2windows.WithAgentOptions(
				agentparams.WithAgentConfig("log_level: debug"),
				agentparams.WithSystemProbeConfig(systemProbeConfig),
			),
		}
		params := ec2windows.GetRunParams(opts...)
		return ec2windows.RunWithEnv(ctx, awsEnv, env, params)
	}
}

func (s *windowsUSMSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	host := s.Env().RemoteHost

	s.setupIISSites(host)
	s.setupPythonServers(host)
	s.restartAgent(host)
	waitForConnectionsPipeline(s.T(), s.Env().FakeIntake.Client())
}

func (s *windowsUSMSuite) setupIISSites(host *components.RemoteHost) {
	err := windows.InstallIIS(host)
	require.NoError(s.T(), err, "failed to install IIS")

	sites := []windows.IISSiteDefinition{
		{Name: "DatadogTestSiteA", BindingPort: "*:8081:"},
		{Name: "DatadogTestSiteB", BindingPort: "*:8082:"},
	}
	err = windows.CreateIISSite(host, sites)
	require.NoError(s.T(), err, "failed to create IIS sites")

	for _, site := range sites {
		sitePath := "c:/tmp/inetpub/" + site.Name + "/index.html"
		_, err = host.WriteFile(sitePath, []byte("<html><body>ok</body></html>"))
		require.NoError(s.T(), err, "failed to write default document for %s", site.Name)
	}
}

func (s *windowsUSMSuite) setupPythonServers(host *components.RemoteHost) {
	pythonExe := `C:\Program Files\Datadog\Datadog Agent\embedded3\python.exe`
	out, _ := host.Execute(`Test-Path "` + pythonExe + `"`)
	require.Contains(s.T(), out, "True", "embedded Python not found at %s", pythonExe)

	host.MustExecute(`New-Item -ItemType Directory -Force -Path C:\temp | Out-Null`)
	_, err := host.WriteFile(`C:\temp\httpserver.py`, []byte(httpServerScript))
	require.NoError(s.T(), err, "failed to write HTTP server script")

	for _, port := range []string{"8083", "8084"} {
		out, err = host.Execute(`$r = Invoke-CimMethod -ClassName Win32_Process -MethodName Create -Arguments @{` +
			`CommandLine='"` + pythonExe + `" C:\temp\httpserver.py ` + port + `'}; ` +
			`Write-Output "port` + port + `: pid=$($r.ProcessId) rc=$($r.ReturnValue)"`)
		require.NoError(s.T(), err, "WMI process creation failed for port %s", port)
		require.Contains(s.T(), out, "rc=0", "WMI process creation returned non-zero for port %s", port)
	}

	waitForHTTPServer(s.T(), host, "Invoke-WebRequest -UseBasicParsing http://localhost:%d/", 8083)
	waitForHTTPServer(s.T(), host, "Invoke-WebRequest -UseBasicParsing http://localhost:%d/", 8084)
}

// restartAgent stops and starts the agent so system-probe's IIS ETW provider
// initializes with IIS installed. In CI the agent was started before IIS.
func (s *windowsUSMSuite) restartAgent(host *components.RemoteHost) {
	host.MustExecute("Stop-Service datadogagent -Force")
	require.Eventually(s.T(), func() bool {
		out, _ := host.Execute(`(Get-Service datadogagent).Status`)
		return strings.Contains(out, "Stopped")
	}, 30*time.Second, 2*time.Second, "datadogagent did not stop")

	host.MustExecute("Start-Service datadogagent")
	require.Eventually(s.T(), func() bool {
		out, err := host.Execute(`& "C:\Program Files\Datadog\Datadog Agent\bin\agent.exe" "status"`)
		return err == nil && out != ""
	}, 60*time.Second, 5*time.Second, "agent did not become ready after restart")

	require.Eventually(s.T(), func() bool {
		out, _ := host.Execute(`Test-Path \\.\pipe\dd_system_probe`)
		return strings.Contains(out, "True")
	}, 60*time.Second, 2*time.Second, "system-probe named pipe not ready after restart")
}

func (s *windowsUSMSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)

	if !s.BaseSuite.IsDevMode() {
		s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	}
}

// TestIISRemoteServiceTags verifies that connections to IIS sites have
// RemoteServiceTagsIdx >= 0 with tags containing http.iis.sitename: prefix.
func (s *windowsUSMSuite) TestIISRemoteServiceTags() {
	t := s.T()
	host := s.Env().RemoteHost
	const count = 1000

	// Flush any stale data from previous runs (e.g. when reusing --keep-stack).
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()

	// Step 1: send 1000 connections to site A (port 8081), hold 20s, then verify.
	sendWindowsKeepAliveRequestsToPort(host, 8081, count, 20)

	var statsA connectionStats
	require.Eventually(t, func() bool {
		cnx, err := s.Env().FakeIntake.Client().GetConnections()
		if err != nil || cnx == nil {
			return false
		}
		statsA = getConnectionStats(t, cnx, []int32{8081, 8082}, "http.iis.sitename:")
		return statsA.connsByPort[8081] >= count
	}, 90*time.Second, 5*time.Second, "timed out waiting for siteA connections on port 8081")

	assertTaggedConnectionsOnPort(t, statsA, "siteA", 8081, count)
	assert.True(t, statsA.tagsByPort[8081]["http.iis.sitename:DatadogTestSiteA"],
		"siteA: port 8081 should be tagged with DatadogTestSiteA")

	// Step 2: send 1000 connections to site B (port 8082).
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	sendWindowsKeepAliveRequestsToPort(host, 8082, count, 20)

	var statsB connectionStats
	require.Eventually(t, func() bool {
		cnx, err := s.Env().FakeIntake.Client().GetConnections()
		if err != nil || cnx == nil {
			return false
		}
		statsB = getConnectionStats(t, cnx, []int32{8081, 8082}, "http.iis.sitename:")
		return statsB.connsByPort[8082] >= count
	}, 90*time.Second, 5*time.Second, "timed out waiting for siteB connections on port 8082")

	assertTaggedConnectionsOnPort(t, statsB, "siteB", 8082, count)
	assert.True(t, statsB.tagsByPort[8082]["http.iis.sitename:DatadogTestSiteB"],
		"siteB: port 8082 should be tagged with DatadogTestSiteB")
}

// TestHTTPRemoteServiceTags verifies that connections to Python HTTP listeners
// have RemoteServiceTagsIdx >= 0 with process-based remote service tags.
func (s *windowsUSMSuite) TestHTTPRemoteServiceTags() {
	t := s.T()
	host := s.Env().RemoteHost

	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()

	const requestsPerPort = 4000
	sendWindowsKeepAliveRequestsToPort(host, 8083, requestsPerPort, 20)
	sendWindowsKeepAliveRequestsToPort(host, 8084, requestsPerPort, 20)

	var stats connectionStats
	require.Eventually(t, func() bool {
		cnx, err := s.Env().FakeIntake.Client().GetConnections()
		if err != nil || cnx == nil {
			return false
		}
		stats = getConnectionStats(t, cnx, []int32{8083, 8084}, "process_context:")
		return stats.connsByPort[8083] >= requestsPerPort && stats.connsByPort[8084] >= requestsPerPort &&
			stats.untaggedByPort[8083] == 0 && stats.untaggedByPort[8084] == 0
	}, 120*time.Second, 5*time.Second, "http: timed out waiting for tagged connections on both ports (8083: %d/%d untagged, 8084: %d/%d untagged)",
		stats.untaggedByPort[8083], stats.connsByPort[8083], stats.untaggedByPort[8084], stats.connsByPort[8084])

	assertTaggedConnectionsOnPort(t, stats, "http", 8083, requestsPerPort)
	assertTaggedConnectionsOnPort(t, stats, "http", 8084, requestsPerPort)
}
