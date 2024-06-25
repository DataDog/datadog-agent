// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"encoding/json"
	"fmt"

	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/stretchr/testify/require"
)

type upgradeScenarioSuite struct {
	packageBaseSuite
}

type packageEntry struct {
	Package string `json:"package"`
	Version string `json:"version"`
	Url     string `json:"url"`
}

type catalog struct {
	Packages map[string]packageEntry `json:"packages"`
}

var testCatalog = catalog{
	Packages: map[string]packageEntry{
		"datadog-agent": {
			Package: "datadog-agent",
			Version: "latest",
			Url:     "oci://gcr.io/datadoghq/agent-package@latest",
		},
	},
}

const (
	agentExperimentSymlink = "/opt/datadog-packages/datadog-agent/experiment"
	agentStableSymlink     = "/opt/datadog-packages/datadog-agent/stable"

	versionPathTemplate = "/opt/datadog-packages/datadog-agent/%s"
)

func testUpgradeScenario(os e2eos.Descriptor, arch e2eos.Architecture) packageSuite {
	return &upgradeScenarioSuite{
		packageBaseSuite: newPackageSuite("upgrade_scenario", os, arch),
	}
}

func (s *upgradeScenarioSuite) TestUpgradeSuccessful() {
	s.RunInstallScript(envForceInstall("datadog-agent"))
	defer s.Purge()
	s.host.WaitForUnitActive("datadog-agent.service", "datadog-agent-trace.service", "datadog-agent-process.service")

	resp, err := s.setCatalog(testCatalog)
	require.NoError(s.T(), err)
	s.T().Log(resp)

	resp, err = s.startExperimentCommand("latest")
	require.NoError(s.T(), err)
	s.T().Log(resp)
	require.Equal(s.T(), "latest", s.host.AgentVersion())
	s.assertAgentExperiment("latest")

	resp, err = s.promoteExperimentCommand()
	require.NoError(s.T(), err)
	s.T().Log(resp)
	require.Equal(s.T(), "latest", s.host.AgentVersion())
}

func (s *upgradeScenarioSuite) startExperimentCommand(version string) (string, error) {
	return s.Env().RemoteHost.Execute(fmt.Sprintf(
		"sudo datadog-installer daemon start-experiment datadog-agent %s", version),
	)
}

func (s *upgradeScenarioSuite) promoteExperimentCommand() (string, error) {
	return s.Env().RemoteHost.Execute(fmt.Sprintf(
		"sudo datadog-installer daemon promote-experiment datadog-agent"),
	)
}

func (s *upgradeScenarioSuite) stopExperimentCommand() (string, error) {
	return s.Env().RemoteHost.Execute(fmt.Sprintf(
		"sudo datadog-installer daemon stop-experiment datadog-agent"),
	)
}

func (s *upgradeScenarioSuite) setCatalog(newCatalog catalog) (string, error) {
	serializedCatalog, err := json.Marshal(newCatalog)
	if err != nil {
		s.T().Fatal(err)
	}

	return s.Env().RemoteHost.Execute(fmt.Sprintf(
		"sudo datadog-installer daemon set-catalog '%s'", serializedCatalog),
	)
}

func (s *upgradeScenarioSuite) assertAgentExperiment(version string) {
	state := s.host.State()
	versionPath := fmt.Sprintf(versionPathTemplate, version)
	state.AssertSymlinkExists(agentExperimentSymlink, versionPath, "root", "root")
}
