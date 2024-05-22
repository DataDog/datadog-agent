// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/host"
	e2eos "github.com/DataDog/test-infra-definitions/components/os"
)

type packageAgentSuite struct {
	packageBaseSuite
}

func testAgent(os e2eos.Descriptor, arch e2eos.Architecture) packageSuite {
	return &packageAgentSuite{
		packageBaseSuite: newPackageSuite("agent", os, arch, awshost.WithoutFakeIntake()),
	}
}

func (s *packageAgentSuite) TestInstall() {
	s.RunInstallScript(envForceInstall("datadog-agent"))
	defer s.Purge()
	s.host.WaitForUnitActive("datadog-agent.service", "datadog-agent-trace.service", "datadog-agent-process.service")

	state := s.host.State()

	state.AssertUnitsLoaded("datadog-agent.service", "datadog-agent-trace.service", "datadog-agent-process.service", "datadog-agent-sysprobe.service", "datadog-agent-security.service")
	state.AssertUnitsEnabled("datadog-agent.service", "datadog-agent-trace.service", "datadog-agent-process.service", "datadog-agent-sysprobe.service", "datadog-agent-security.service")
	state.AssertUnitsRunning("datadog-agent.service", "datadog-agent-trace.service", "datadog-agent-process.service")
	state.AssertUnitsDead("datadog-agent-sysprobe.service", "datadog-agent-security.service")

	state.AssertFileExists("/etc/datadog-agent/install_info", 0644, "root", "root")
	// FIXME: this file is either dd-agent or root depending on the OS for some reason
	// state.AssertFileExists("/etc/datadog-agent/install.json", 0644, "dd-agent", "dd-agent")
}

func (s *packageAgentSuite) TestExperimentStartedButNotInstalled() {
	s.RunInstallScript(envForceInstall("datadog-agent"))
	defer s.Purge()
	s.host.WaitForUnitActive("datadog-agent.service", "datadog-agent-trace.service", "datadog-agent-process.service")

	timestamp := s.host.LastJournaldTimestamp()
	// Start the experiment while it's not installed. This should immediately revert to the stable version.
	s.host.Run(`sudo systemctl start datadog-agent-exp --no-block`)
	s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().
		Stopping("datadog-agent.service").
		Stopped("datadog-agent.service").
		Starting("datadog-agent-exp.service").
		Failed("datadog-agent-exp.service").
		Started("datadog-agent.service"),
	)
}
