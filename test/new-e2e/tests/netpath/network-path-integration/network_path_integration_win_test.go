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

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"
)

type windowsNetworkPathIntegrationTestSuite05 struct {
	baseNetworkPathIntegrationTestSuite
}

// TestNetworkPathIntegrationSuiteLinux runs the Network Path Integration e2e suite for linux
func TestWindowsNetworkPathIntegrationSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &windowsNetworkPathIntegrationTestSuite05{}, e2e.WithProvisioner(awshost.Provisioner(
		awshost.WithAgentOptions(
			agentparams.WithSystemProbeConfig(string(sysProbeConfig)),
			agentparams.WithIntegration("network_path.d", string(networkPathIntegration)),
		),
		awshost.WithEC2InstanceOptions(ec2.WithOS(os.WindowsDefault)),
	)))
}

func (s *windowsNetworkPathIntegrationTestSuite05) TestWindowsNetworkPathIntegrationMetrics() {
	fakeIntake := s.Env().FakeIntake
	s.EventuallyWithT(func(c *assert.CollectT) {
		s.assertMetrics(fakeIntake, nil, [][]string{
			{"destination_hostname:api.datadoghq.eu", "protocol:TCP", "destination_port:443"},

			// TODO: Test UDP once implemented for windows
			//{"destination_hostname:8.8.8.8", "protocol:UDP"},
		})
	}, 5*time.Minute, 3*time.Second)
}
