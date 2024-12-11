// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package netpath contains e2e tests for Network Path Integration feature
package netpathintegration

import (
	_ "embed"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
)

type linuxNetworkPathIntegrationTestSuite struct {
	baseNetworkPathIntegrationTestSuite
}

//func TestLinuxFlareSuite(t *testing.T) {
//	t.Parallel()
//	e2e.Run(t, &linuxNetworkPathIntegrationTestSuite{}, e2e.WithProvisioner(awshost.Provisioner()))
//}

// TestNetworkPathIntegrationSuiteLinux runs the Network Path Integration e2e suite for linux
func TestLinuxNetworkPathIntegrationSuite(t *testing.T) {
	e2e.Run(t, &baseNetworkPathIntegrationTestSuite{}, e2e.WithProvisioner(awshost.Provisioner(
		awshost.WithAgentOptions(
			agentparams.WithSystemProbeConfig(string(sysProbeConfig)),
			agentparams.WithIntegration("network_path.d", string(networkPathIntegration)),
		)),
	))
}
