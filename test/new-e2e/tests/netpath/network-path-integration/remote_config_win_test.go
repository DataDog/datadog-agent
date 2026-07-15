// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package networkpathintegration

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

type windowsRemoteConfigTestSuite struct {
	remoteConfigTestSuite
}

func TestWindowsRemoteConfigSuite(t *testing.T) {
	t.Parallel()

	e2e.Run(t, &windowsRemoteConfigTestSuite{
		remoteConfigTestSuite: remoteConfigTestSuite{
			platform:        remoteConfigPlatformWindows,
			scheduledConfig: crossPlatformScheduledNetworkPathRCConfig,
			expectedPaths: []remoteConfigPathExpectation{
				{
					hostname:         "api.datadoghq.eu",
					protocol:         "TCP",
					port:             443,
					configSubstrings: []string{"hostname: api.datadoghq.eu", "protocol: TCP", "port: 443", "test_config_id: aaa-bbb-ccc"},
				},
			},
		},
	}, e2e.WithProvisioner(awshost.Provisioner(
		awshost.WithRunOptions(
			scenec2.WithAgentOptions(remoteConfigAgentOptions()...),
			scenec2.WithEC2InstanceOptions(scenec2.WithOS(os.WindowsServerDefault)),
		),
	)))
}
