// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/host"
	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/stretchr/testify/require"
)

type upgradeScenarioSuite struct {
	packageBaseSuite
}

type packageEntry struct {
	Package string `json:"package"`
	Version string `json:"version"`
	URL     string `json:"url"`
}

type catalog struct {
	Packages []packageEntry `json:"packages"`
}

type packageStatus struct {
	State             string `json:"state"`
	StableVersion     string `json:"stable_version"`
	ExperimentVersion string `json:"experiment_version"`
}

var testCatalog = catalog{
	Packages: []packageEntry{
		{
			Package: "datadog-agent",
			Version: latestAgentImageVersion,
			URL:     fmt.Sprintf("oci://gcr.io/datadoghq/agent-package:%s", latestAgentImageVersion),
		},
	},
}

// datadog-agent
//
//	State: OK
//	Installed versions:
//	  ● stable: v7.52.0-rc.1.git.15.6c19b17.pipeline.28815219-1
//	  ● experiment: none
var installerStatusRegex = regexp.MustCompile(`([a-zA-Z-]+)\n[ \t]+State:.([a-zA-Z-]+)\n[ \t]+Installed versions:\n[ \t]+..stable:.([a-zA-Z0-9-\.]+)\n[ \t]+..experiment:.([a-zA-Z0-9-\.]+)`)

const (
	latestAgentVersion      = "7.54.1"
	latestAgentImageVersion = "7.54.1-1"
)

func testUpgradeScenario(os e2eos.Descriptor, arch e2eos.Architecture) packageSuite {
	return &upgradeScenarioSuite{
		packageBaseSuite: newPackageSuite("upgrade_scenario", os, arch),
	}
}

func (s *upgradeScenarioSuite) TestUpgradeSuccessful() {
	s.RunInstallScript("DD_REMOTE_UPDATES=true")
	defer s.Purge()
	s.host.WaitForUnitActive(
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-process.service",
		"datadog-installer.service",
	)

	_, err := s.setCatalog(testCatalog)
	require.NoError(s.T(), err)

	timestamp := s.host.LastJournaldTimestamp()
	_, err = s.startExperimentCommand(latestAgentImageVersion)
	require.NoError(s.T(), err)
	s.assertSuccessfulStartExperiment(timestamp, latestAgentImageVersion)

	timestamp = s.host.LastJournaldTimestamp()
	_, err = s.promoteExperimentCommand()
	require.NoError(s.T(), err)
	s.assertSuccessfulPromoteExperiment(timestamp, latestAgentImageVersion)
}

func (s *upgradeScenarioSuite) startExperimentCommand(version string) (string, error) {
	cmd := fmt.Sprintf("sudo datadog-installer daemon start-experiment datadog-agent %s", version)
	s.T().Logf("Running start command: %s", cmd)
	return s.Env().RemoteHost.Execute(cmd)
}

func (s *upgradeScenarioSuite) promoteExperimentCommand() (string, error) {
	cmd := "sudo datadog-installer daemon promote-experiment datadog-agent"
	s.T().Logf("Running promote command: %s", cmd)
	return s.Env().RemoteHost.Execute(cmd)
}

//lint:ignore U1000 Ignore unused function for now
func (s *upgradeScenarioSuite) stopExperimentCommand() (string, error) {
	cmd := "sudo datadog-installer daemon stop-experiment datadog-agent"
	s.T().Logf("Running stop command: %s", cmd)
	return s.Env().RemoteHost.Execute(cmd)
}

func (s *upgradeScenarioSuite) setCatalog(newCatalog catalog) (string, error) {
	serializedCatalog, err := json.Marshal(newCatalog)
	if err != nil {
		s.T().Fatal(err)
	}
	s.T().Logf("Running: daemon set-catalog '%s'", string(serializedCatalog))

	return s.Env().RemoteHost.Execute(fmt.Sprintf(
		"sudo datadog-installer daemon set-catalog '%s'", serializedCatalog),
	)
}

func (s *upgradeScenarioSuite) assertSuccessfulStartExperiment(timestamp host.JournaldTimestamp, version string) {
	s.host.WaitForUnitActivating(agentUnitXP)

	// Assert experiment is running
	s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().
		Unordered(host.SystemdEvents().
			Stopped(agentUnit).
			Stopped(traceUnit).
			Stopped(processUnit),
		).
		Unordered(host.SystemdEvents().
			Starting(agentUnitXP).
			Started(traceUnitXP).
			Started(processUnitXP),
		),
	)

	installerStatus := s.getInstallerStatus()
	require.Equal(s.T(), version, installerStatus["datadog-agent"].ExperimentVersion)
}

func (s *upgradeScenarioSuite) assertSuccessfulPromoteExperiment(timestamp host.JournaldTimestamp, version string) {
	s.host.WaitForUnitActive(agentUnit)

	// Assert experiment is promoted
	s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().
		Unordered(host.SystemdEvents().
			Stopped(agentUnitXP).
			Stopped(processUnitXP).
			Stopped(traceUnitXP),
		).
		Unordered(host.SystemdEvents().
			Started(agentUnit).
			Stopped(processUnit).
			Stopped(traceUnit),
		),
	)

	installerStatus := s.getInstallerStatus()
	require.Equal(s.T(), version, installerStatus["datadog-agent"].StableVersion)
	require.Equal(s.T(), "none", installerStatus["datadog-agent"].ExperimentVersion)
}

//lint:ignore U1000 Ignore unused function for now
func (s *upgradeScenarioSuite) assertSuccessfulStopExperiment(timestamp host.JournaldTimestamp) {
	// Assert experiment is stopped
	s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().
		Unordered(host.SystemdEvents().
			Stopped(agentUnitXP).
			Stopped(processUnitXP).
			Stopped(traceUnitXP),
		).
		Started(agentUnit).
		Unordered(host.SystemdEvents().
			Started(traceUnit).
			Started(processUnit),
		),
	)

	installerStatus := s.getInstallerStatus()
	require.Equal(s.T(), "none", installerStatus["datadog-agent"].ExperimentVersion)
}

func (s *upgradeScenarioSuite) getInstallerStatus() map[string]packageStatus {
	// 	Datadog Installer v7.55.0-devel+git.1079.69749ed
	// datadog-agent
	//   State: OK
	//   Installed versions:
	//     ● stable: v7.52.0-rc.1.git.15.6c19b17.pipeline.28815219-1
	//     ● experiment: none
	resp := s.Env().RemoteHost.MustExecute("sudo datadog-installer status")
	status := make(map[string]packageStatus)

	statusResponse := installerStatusRegex.FindAllStringSubmatch(resp, -1)
	for _, st := range statusResponse {
		if len(st) != 5 {
			s.T().Fatal("unexpected status response")
		}
		status[st[1]] = packageStatus{
			State:             st[2],
			StableVersion:     strings.TrimPrefix(st[3], "v"),
			ExperimentVersion: strings.TrimPrefix(st[4], "v"),
		}
	}

	return status
}
