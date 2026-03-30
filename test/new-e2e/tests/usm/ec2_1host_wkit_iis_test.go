// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package usm

import (
	"strings"
	"testing"
	"time"

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

type iisRemoteTagsSuite struct {
	e2e.BaseSuite[environments.WindowsHost]
}

func TestIISRemoteTagsSuite(t *testing.T) {
	t.Parallel()

	s := &iisRemoteTagsSuite{}

	e2eParams := []e2e.SuiteOption{
		e2e.WithProvisioner(provisioners.NewTypedPulumiProvisioner("iisHost", iisHostProvisionerWindows(), nil)),
	}

	e2e.Run(t, s, e2eParams...)
}

func iisHostProvisionerWindows() provisioners.PulumiEnvRunFunc[environments.WindowsHost] {
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

func (s *iisRemoteTagsSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	host := s.Env().RemoteHost

	// Install IIS BEFORE deploying binaries so system-probe's IIS ETW provider
	// initializes correctly when the agent services restart.
	err := windows.InstallIIS(host)
	require.NoError(s.T(), err, "failed to install IIS")

	sites := []windows.IISSiteDefinition{
		{
			Name:        "DatadogTestSiteA",
			BindingPort: "*:8081:",
		},
		{
			Name:        "DatadogTestSiteB",
			BindingPort: "*:8082:",
		},
	}
	err = windows.CreateIISSite(host, sites)
	require.NoError(s.T(), err, "failed to create IIS sites")

	// Write a default document so IIS returns 200 instead of 403.14
	for _, site := range sites {
		sitePath := "c:/tmp/inetpub/" + site.Name + "/index.html"
		_, err = host.WriteFile(sitePath, []byte("<html><body>ok</body></html>"))
		require.NoError(s.T(), err, "failed to write default document for %s", site.Name)
	}

	// Restart the agent so system-probe's IIS ETW provider initializes
	// now that IIS is installed. In CI the agent was started before IIS.
	host.MustExecute("Restart-Service datadogagent -Force")
	require.Eventually(s.T(), func() bool {
		out, err := host.Execute(`& "C:\Program Files\Datadog\Datadog Agent\bin\agent.exe" "status"`)
		return err == nil && out != ""
	}, 60*time.Second, 5*time.Second, "agent did not become ready after restart")

	// Wait for system-probe's ETW provider to initialize by sending a probe
	// request to IIS and polling the IIS tags cache until it's populated.
	// Without this, traffic sent before ETW is ready won't get IIS tags.
	require.Eventually(s.T(), func() bool {
		// Send a single request to IIS to trigger ETW capture.
		host.Execute(`Invoke-WebRequest -UseBasicParsing -Uri "http://localhost:8081/" -ErrorAction SilentlyContinue`)
		// Check if the IIS tags cache has entries.
		out, err := host.Execute(`Invoke-WebRequest -UseBasicParsing -Uri "http://localhost:3333/network_tracer/iis_tags" | Select-Object -ExpandProperty Content`)
		if err != nil || out == "" {
			return false
		}
		trimmed := strings.TrimSpace(out)
		return trimmed != "" && trimmed != "{}" && trimmed != "null"
	}, 90*time.Second, 5*time.Second, "IIS tags cache not populated — ETW provider may not have initialized")

	// In CI, the provisioner installs the agent built from the current branch.
	// For local dev, uncomment to deploy locally-built binaries:
	// deployWindowsBinaries(s.T(), host)
}

func (s *iisRemoteTagsSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)

	if !s.BaseSuite.IsDevMode() {
		s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	}
}

// TestIISRemoteServiceTags verifies that connections to IIS sites have
// RemoteServiceTagsIdx >= 0 with tags containing http.iis.sitename: prefix.
// It sends connections to each site sequentially, verifying tags after each,
// then sends to the second site to exercise cache key replacement in the IIS
// ETW tag cache when ephemeral ports are reused (cache entries have 2-minute TTL).
func (s *iisRemoteTagsSuite) TestIISRemoteServiceTags() {
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
		statsA = getConnectionStats(t, cnx, "http.iis.sitename:")
		return statsA.connsByPort[8081] >= count
	}, 90*time.Second, 5*time.Second, "timed out waiting for siteA connections on port 8081")

	assertTaggedConnectionsOnPort(t, statsA, "siteA", 8081, count)
	assert.True(t, statsA.tagsByPort[8081]["http.iis.sitename:DatadogTestSiteA"],
		"siteA: port 8081 should be tagged with DatadogTestSiteA")

	// Step 2: quickly send 1000 connections to site B (port 8082).
	// Ephemeral ports from step 1 are recycled by the OS, exercising IIS ETW
	// cache key replacement (entries have a 2-minute TTL).
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	sendWindowsKeepAliveRequestsToPort(host, 8082, count, 20)

	var statsB connectionStats
	require.Eventually(t, func() bool {
		cnx, err := s.Env().FakeIntake.Client().GetConnections()
		if err != nil || cnx == nil {
			return false
		}
		statsB = getConnectionStats(t, cnx, "http.iis.sitename:")
		return statsB.connsByPort[8082] >= count
	}, 90*time.Second, 5*time.Second, "timed out waiting for siteB connections on port 8082")

	assertTaggedConnectionsOnPort(t, statsB, "siteB", 8082, count)
	assert.True(t, statsB.tagsByPort[8082]["http.iis.sitename:DatadogTestSiteB"],
		"siteB: port 8082 should be tagged with DatadogTestSiteB")
}
