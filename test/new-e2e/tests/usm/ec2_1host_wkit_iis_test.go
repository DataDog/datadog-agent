// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package usm

import (
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
	host.MustExecute("Stop-Service datadogagent -Force")
	time.Sleep(5 * time.Second)
	host.MustExecute("Start-Service datadogagent")
	time.Sleep(15 * time.Second)

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
	time.Sleep(30 * time.Second)

	cnx, err := s.Env().FakeIntake.Client().GetConnections()
	require.NoError(t, err, "GetConnections() error")
	require.NotNil(t, cnx, "GetConnections() returned nil")

	stats := getConnectionStats(t, cnx, "http.iis.sitename:")
	assertTaggedConnectionsOnPort(t, stats, "siteA", 8081, count)
	assert.True(t, stats.tagsByPort[8081]["http.iis.sitename:DatadogTestSiteA"],
		"siteA: port 8081 should be tagged with DatadogTestSiteA")

	// Step 2: quickly send 1000 connections to site B (port 8082).
	// Ephemeral ports from step 1 are recycled by the OS, exercising IIS ETW
	// cache key replacement (entries have a 2-minute TTL).
	s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	sendWindowsKeepAliveRequestsToPort(host, 8082, count, 20)
	time.Sleep(30 * time.Second)

	cnx, err = s.Env().FakeIntake.Client().GetConnections()
	require.NoError(t, err, "GetConnections() error")
	require.NotNil(t, cnx, "GetConnections() returned nil")

	stats = getConnectionStats(t, cnx, "http.iis.sitename:")
	assertTaggedConnectionsOnPort(t, stats, "siteB", 8082, count)
	assert.True(t, stats.tagsByPort[8082]["http.iis.sitename:DatadogTestSiteB"],
		"siteB: port 8082 should be tagged with DatadogTestSiteB")
}
