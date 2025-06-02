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

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
)

type windowsNetworkPathIntegrationTestSuite struct {
	baseNetworkPathIntegrationTestSuite
}

//go:embed fixtures/network_path_windows.yaml
var networkPathIntegrationWindows []byte

var testAgentRunningMetricTagsTCPSocket = []string{"destination_hostname:8.8.8.8", "protocol:TCP", "destination_port:443"}

// TestNetworkPathIntegrationSuiteLinux runs the Network Path Integration e2e suite for linux
func TestWindowsNetworkPathIntegrationSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &windowsNetworkPathIntegrationTestSuite{}, e2e.WithProvisioner(awshost.Provisioner(
		awshost.WithAgentOptions(
			agentparams.WithSystemProbeConfig(string(sysProbeConfig)),
			agentparams.WithIntegration("network_path.d", string(networkPathIntegrationWindows)),
		),
		awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)),
	)))
}

func (s *windowsNetworkPathIntegrationTestSuite) SetupSuite() {
	s.baseNetworkPathIntegrationTestSuite.SetupSuite()

	// disable defender firewall for windows
	// this is needed to avoid firewall rules blocking the network path
	err := s.disableFirewall()
	s.Require().NoError(err)
}

func (s *windowsNetworkPathIntegrationTestSuite) TestWindowsNetworkPathIntegrationMetrics() {
	// TODO remove after fixing metrics flake
	flake.Mark(s.T())

	fakeIntake := s.Env().FakeIntake
	hostname := s.Env().Agent.Client.Hostname()
	s.EventuallyWithT(func(c *assert.CollectT) {
		assertMetrics(fakeIntake, c, [][]string{
			testAgentRunningMetricTagsTCP,
			testAgentRunningMetricTagsTCPSocket,
			testAgentRunningMetricTagsUDP,
		})

		s.checkDatadogEUTCP(c, hostname)
		s.checkGoogleTCPSocket(c, hostname)
		s.checkGoogleDNSUDP(c, hostname)

	}, 5*time.Minute, 3*time.Second)
}

// disable defender firewall for windows
func (s *windowsNetworkPathIntegrationTestSuite) disableFirewall() error {
	_, err := s.Env().RemoteHost.Host.Execute("Set-NetFirewallProfile -Profile Domain,Public,Private -Enabled False")
	return err
}
