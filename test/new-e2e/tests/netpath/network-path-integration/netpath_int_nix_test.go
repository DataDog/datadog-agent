// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package netpath contains e2e tests for Network Path Integration feature
package networkpathintegration

import (
	_ "embed"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

type linuxNetworkPathIntegrationTestSuite struct {
	baseNetworkPathIntegrationTestSuite
}

// TestNetworkPathIntegrationSuiteLinux runs the Network Path Integration e2e suite for linux
func TestLinuxNetworkPathIntegrationSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &linuxNetworkPathIntegrationTestSuite{}, e2e.WithProvisioner(awshost.Provisioner(
		awshost.WithRunOptions(
			scenec2.WithAgentOptions(
				agentparams.WithSystemProbeConfig(string(sysProbeConfig)),
				agentparams.WithIntegration("network_path.d", string(networkPathIntegration)),
			)),
	),
	))

}

func (s *linuxNetworkPathIntegrationTestSuite) TestLinuxNetworkPathIntegrationMetrics() {
	fakeIntake := s.Env().FakeIntake
	hostname := s.Env().Agent.Client.Hostname()
	s.EventuallyWithT(func(c *assert.CollectT) {
		assertMetrics(fakeIntake, c, [][]string{
			testAgentRunningMetricTagsTCP,
			testAgentRunningMetricTagsUDP,
		})

		s.checkDatadogEUTCP(c, hostname)
		s.checkGoogleDNSUDP(c, hostname)
	}, 5*time.Minute, 3*time.Second)
}
