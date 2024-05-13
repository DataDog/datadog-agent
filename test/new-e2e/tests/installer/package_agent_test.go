// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	e2eos "github.com/DataDog/test-infra-definitions/components/os"
)

type packageAgentSuite struct {
	packageBaseSuite
}

func testAgent(os e2eos.Descriptor, arch e2eos.Architecture) packageSuite {
	return &packageAgentSuite{
		packageBaseSuite: newPackageSuite("agent", os, arch),
	}
}

func (s *packageAgentSuite) TestInstall() {
	s.RunInstallScript()
	defer s.Purge()
	s.InstallAgentPackage()

	state := s.host.State()

	state.AssertUnitsLoaded("datadog-agent.service", "datadog-agent-trace.service", "datadog-agent-process.service", "datadog-agent-sysprobe.service", "datadog-agent-security.service")
	state.AssertUnitsEnabled("datadog-agent.service", "datadog-agent-trace.service", "datadog-agent-process.service", "datadog-agent-sysprobe.service", "datadog-agent-security.service")
	// FIXME: the agent currently fails to start because /etc/datadog-agent/datadog.yaml is missing
}
