// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package usm

import (
	"strings"
	"testing"
	"time"

	agentmodel "github.com/DataDog/agent-payload/v5/process"

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
			ec2windows.WithAgentOptions(agentparams.WithSystemProbeConfig(systemProbeConfigIIS)),
		}
		params := ec2windows.GetRunParams(opts...)
		return ec2windows.RunWithEnv(ctx, awsEnv, env, params)
	}
}

func (s *iisRemoteTagsSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	host := s.Env().RemoteHost

	err := windows.InstallIIS(host)
	require.NoError(s.T(), err, "failed to install IIS")

	sites := []windows.IISSiteDefinition{
		{
			Name:        "DatadogTestSiteA",
			BindingPort: "*:8081",
		},
		{
			Name:        "DatadogTestSiteB",
			BindingPort: "*:8082",
		},
	}
	err = windows.CreateIISSite(host, sites)
	require.NoError(s.T(), err, "failed to create IIS sites")
}

func (s *iisRemoteTagsSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)

	if !s.BaseSuite.IsDevMode() {
		s.Env().FakeIntake.Client().FlushServerAndResetAggregators()
	}
}

// TestIISRemoteServiceTags verifies that connections to IIS sites have
// RemoteServiceTagsIdx >= 0 with tags containing http.iis.sitename: prefix.
func (s *iisRemoteTagsSuite) TestIISRemoteServiceTags() {
	t := s.T()
	host := s.Env().RemoteHost

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		// Generate traffic each poll to ensure connections exist in the current collection window
		for i := 0; i < 5; i++ {
			host.MustExecute("Invoke-WebRequest -UseBasicParsing http://localhost:8081/")
			host.MustExecute("Invoke-WebRequest -UseBasicParsing http://localhost:8082/")
		}

		cnx, err := s.Env().FakeIntake.Client().GetConnections()
		if !assert.NoError(c, err, "GetConnections() error") {
			return
		}
		if !assert.NotNil(c, cnx, "GetConnections() returned nil") {
			return
		}
		if !assert.NotEmpty(c, cnx.GetNames(), "no connections yet") {
			return
		}

		totalIISConns := 0
		untaggedConns := 0
		sitesByPort := make(map[int32]map[string]bool) // port -> set of sitenames seen
		cnx.ForeachConnection(func(conn *agentmodel.Connection, cc *agentmodel.CollectorConnections, hostname string) {
			if conn.Raddr.Port != 8081 && conn.Raddr.Port != 8082 {
				return
			}
			totalIISConns++
			if conn.RemoteServiceTagsIdx < 0 {
				untaggedConns++
				t.Logf("untagged IIS connection: port=%d pid=%d", conn.Raddr.Port, conn.Pid)
				return
			}
			remoteTags := cc.GetTags(int(conn.RemoteServiceTagsIdx))
			for _, tag := range remoteTags {
				if strings.HasPrefix(tag, "http.iis.sitename:") {
					siteName := strings.TrimPrefix(tag, "http.iis.sitename:")
					if sitesByPort[conn.Raddr.Port] == nil {
						sitesByPort[conn.Raddr.Port] = make(map[string]bool)
					}
					sitesByPort[conn.Raddr.Port][siteName] = true
				}
			}
		})

		t.Logf("IIS connections: total=%d untagged=%d sitesByPort=%v", totalIISConns, untaggedConns, sitesByPort)

		if !assert.Greater(c, totalIISConns, 0, "no connections to IIS ports found") {
			return
		}
		assert.Equal(c, 0, untaggedConns,
			"all connections to IIS ports should have remote service tags")
		assert.True(c, sitesByPort[8081]["DatadogTestSiteA"],
			"port 8081 should be tagged with DatadogTestSiteA")
		assert.True(c, sitesByPort[8082]["DatadogTestSiteB"],
			"port 8082 should be tagged with DatadogTestSiteB")
	}, 3*time.Minute, 10*time.Second, "IIS remote service tags not found in connections")
}
