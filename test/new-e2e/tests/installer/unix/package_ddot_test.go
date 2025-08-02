// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	e2eos "github.com/DataDog/test-infra-definitions/components/os"

	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
)

type packageDDOTSuite struct {
	packageBaseSuite
}

func testDDOT(os e2eos.Descriptor, arch e2eos.Architecture, method InstallMethodOption) packageSuite {
	return &packageDDOTSuite{
		packageBaseSuite: newPackageSuite("ddot", os, arch, method, awshost.WithoutFakeIntake()),
	}
}

func (s *packageDDOTSuite) TestInstallDDOTWithAgent() {
	// Install agent and DDOT together via environment variable
	s.RunInstallScript("DD_REMOTE_UPDATES=true", "DD_OTEL_COLLECTOR_ENABLED=true", envForceInstall("datadog-agent"))
	defer s.Purge()

	// Verify both packages are installed
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.AssertPackageInstalledByInstaller("datadog-agent-ddot")

	// Wait for services to be active
	s.host.WaitForUnitActive(s.T(), agentUnit, traceUnit, ddotUnit)

	state := s.host.State()
	// agentSuite := &packageAgentSuite{packageBaseSuite: s.packageBaseSuite}
	// agentSuite.assertUnits(state, false)
	// // Verify DDOT units
	// s.assertDDOTUnits(state, false)

	// Verify configuration files exist
	state.AssertFileExists("/etc/datadog-agent/datadog.yaml", 0640, "dd-agent", "dd-agent")
	state.AssertFileExists("/etc/datadog-agent/otel-config.yaml", 0644, "dd-agent", "dd-agent")

	// Verify DDOT binary exists
	state.AssertFileExists("/opt/datadog-packages/datadog-agent-ddot/stable/embedded/bin/otel-agent", 0755, "dd-agent", "dd-agent")

	// Verify otelcollector configuration is present in datadog.yaml
	s.host.Run("sudo grep -q 'otelcollector:' /etc/datadog-agent/datadog.yaml")
}
