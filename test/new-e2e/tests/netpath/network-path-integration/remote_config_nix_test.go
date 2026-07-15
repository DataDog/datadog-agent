// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package networkpathintegration

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

type linuxRemoteConfigTestSuite struct {
	remoteConfigTestSuite
}

func TestLinuxRemoteConfigSuite(t *testing.T) {
	t.Parallel()

	agentOptions := append(remoteConfigAgentOptions(),
		agentparams.WithIntegration("network_path.d", string(remoteConfigLocalNetworkPathYaml)),
		agentparams.WithFile("/tmp/router_setup.sh", string(fakeRouterSetupScript), false),
		agentparams.WithFile("/tmp/router_teardown.sh", string(fakeRouterTeardownScript), false),
	)

	e2e.Run(t, &linuxRemoteConfigTestSuite{
		remoteConfigTestSuite: remoteConfigTestSuite{
			platform:         remoteConfigPlatformLinux,
			scheduledConfig:  linuxScheduledNetworkPathRCConfig,
			localConfigCount: 2,
			expectedPaths: []remoteConfigPathExpectation{
				{
					hostname:         "198.51.100.2",
					protocol:         "UDP",
					port:             0,
					configSubstrings: []string{"hostname: 198.51.100.2", "protocol: UDP", "test_config_id: aaa-bbb-ccc"},
				},
				{
					hostname:         "198.51.100.2",
					protocol:         "TCP",
					port:             443,
					configSubstrings: []string{"hostname: 198.51.100.2", "protocol: TCP", "port: 443", "test_config_id: aaa-bbb-ccc"},
				},
			},
		},
	}, e2e.WithProvisioner(awshost.Provisioner(
		awshost.WithRunOptions(scenec2.WithAgentOptions(agentOptions...)),
	)))
}
