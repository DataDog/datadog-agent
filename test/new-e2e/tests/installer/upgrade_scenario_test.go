// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/host"
	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/stretchr/testify/assert"
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
	State             string `json:"State"`
	StableVersion     string `json:"Stable"`
	ExperimentVersion string `json:"Experiment"`
}

type installerStatus struct {
	Version  string                   `json:"version"`
	Packages map[string]packageStatus `json:"packages"`
}

var testCatalog = catalog{
	Packages: []packageEntry{
		{
			Package: "datadog-agent",
			Version: latestAgentImageVersion,
			URL:     fmt.Sprintf("oci://gcr.io/datadoghq/agent-package:%s", latestAgentImageVersion),
		},
		{
			Package: "datadog-agent",
			Version: previousAgentImageVersion,
			URL:     fmt.Sprintf("oci://gcr.io/datadoghq/agent-package:%s", previousAgentImageVersion),
		},
	},
}

const (
	unknownAgentImageVersion  = "7.52.1-1"
	previousAgentVersion      = "7.54.0"
	previousAgentImageVersion = "7.54.0-1"
	latestAgentVersion        = "7.54.1"
	latestAgentImageVersion   = "7.54.1-1"
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

	s.setCatalog(testCatalog)

	timestamp := s.host.LastJournaldTimestamp()
	_, err := s.startExperimentCommand(latestAgentImageVersion)
	require.NoError(s.T(), err)
	s.assertSuccessfulStartExperiment(timestamp, latestAgentImageVersion)

	timestamp = s.host.LastJournaldTimestamp()
	_, err = s.promoteExperimentCommand()
	require.NoError(s.T(), err)
	s.assertSuccessfulPromoteExperiment(timestamp, latestAgentImageVersion)
}

func (s *upgradeScenarioSuite) TestUpgradeFromExistingExperiment() {
	s.RunInstallScript("DD_REMOTE_UPDATES=true")
	defer s.Purge()
	s.host.WaitForUnitActive(
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-process.service",
		"datadog-installer.service",
	)

	s.host.WaitForFileExists(true, "/var/run/datadog-installer/installer.sock")

	s.setCatalog(testCatalog)

	// Start with 7.54.0
	timestamp := s.host.LastJournaldTimestamp()
	_, err := s.startExperimentCommand(previousAgentImageVersion)
	require.NoError(s.T(), err)
	s.assertSuccessfulStartExperiment(timestamp, previousAgentImageVersion)

	// Host was left with a not-latest experiment, we're now testing
	// that we can still upgrade
	timestamp = s.host.LastJournaldTimestamp()
	_, err = s.stopExperimentCommand()
	require.NoError(s.T(), err)
	s.assertSuccessfulStopExperiment(timestamp)

	timestamp = s.host.LastJournaldTimestamp()
	_, err = s.startExperimentCommand(latestAgentImageVersion)
	require.NoError(s.T(), err)
	s.assertSuccessfulStartExperiment(timestamp, latestAgentImageVersion)

	timestamp = s.host.LastJournaldTimestamp()
	_, err = s.promoteExperimentCommand()
	require.NoError(s.T(), err)
	s.assertSuccessfulPromoteExperiment(timestamp, latestAgentImageVersion)
}

func (s *upgradeScenarioSuite) TestBackendFailure() {
	s.RunInstallScript("DD_REMOTE_UPDATES=true")
	defer s.Purge()
	s.host.WaitForUnitActive(
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-process.service",
		"datadog-installer.service",
	)

	s.setCatalog(testCatalog)

	timestamp := s.host.LastJournaldTimestamp()
	_, err := s.startExperimentCommand(latestAgentImageVersion)
	require.NoError(s.T(), err)
	s.assertSuccessfulStartExperiment(timestamp, latestAgentImageVersion)

	// Receive a failure from the backend, stops the experiment
	timestamp = s.host.LastJournaldTimestamp()
	_, err = s.stopExperimentCommand()
	require.NoError(s.T(), err)
	s.assertSuccessfulStopExperiment(timestamp)
}

func (s *upgradeScenarioSuite) TestExperimentFailure() {
	s.RunInstallScript("DD_REMOTE_UPDATES=true")
	defer s.Purge()
	s.host.WaitForUnitActive(
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-process.service",
		"datadog-installer.service",
	)

	s.setCatalog(testCatalog)

	// Also tests if the version is not available in the catalog
	_, err := s.startExperimentCommand(unknownAgentImageVersion)
	require.Error(s.T(), err)

	// Receive a failure from the experiment, stops the experiment
	beforeStatus := s.getInstallerStatus()
	_, err = s.stopExperimentCommand()
	require.NoError(s.T(), err)
	afterStatus := s.getInstallerStatus()

	require.Equal(s.T(), beforeStatus.Packages["datadog-agent"], afterStatus.Packages["datadog-agent"])
}

func (s *upgradeScenarioSuite) TestExperimentCurrentVersion() {
	s.RunInstallScript("DD_REMOTE_UPDATES=true")
	defer s.Purge()
	s.host.WaitForUnitActive(
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-process.service",
		"datadog-installer.service",
	)

	// Temporary catalog to wait for the installer to be ready
	s.setCatalog(testCatalog)

	currentVersion := s.getInstallerStatus().Packages["datadog-agent"].StableVersion
	newCatalog := catalog{
		Packages: []packageEntry{
			{
				Package: "datadog-agent",
				Version: currentVersion,
				URL:     fmt.Sprintf("oci://gcr.io/datadoghq/agent-package:%s", currentVersion),
			},
		},
	}

	s.setCatalog(newCatalog)
	_, err := s.startExperimentCommand(currentVersion)
	require.Error(s.T(), err)
}

func (s *upgradeScenarioSuite) TestStopWithoutExperiment() {
	s.RunInstallScript("DD_REMOTE_UPDATES=true")
	defer s.Purge()
	s.host.WaitForUnitActive(
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-process.service",
		"datadog-installer.service",
	)

	s.setCatalog(testCatalog)

	beforeStatus := s.getInstallerStatus()

	_, err := s.stopExperimentCommand()
	require.NoError(s.T(), err)

	afterStatus := s.getInstallerStatus()
	require.Equal(s.T(), beforeStatus.Packages["datadog-agent"], afterStatus.Packages["datadog-agent"])
}

func (s *upgradeScenarioSuite) TestDoubleExperiments() {
	s.RunInstallScript("DD_REMOTE_UPDATES=true")
	defer s.Purge()
	s.host.WaitForUnitActive(
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-process.service",
		"datadog-installer.service",
	)

	s.setCatalog(testCatalog)

	timestamp := s.host.LastJournaldTimestamp()
	_, err := s.startExperimentCommand(latestAgentImageVersion)
	require.NoError(s.T(), err)
	s.assertSuccessfulStartExperiment(timestamp, latestAgentImageVersion)

	// Start a second experiment that overrides the first one
	_, err = s.startExperimentCommand(previousAgentImageVersion)
	require.NoError(s.T(), err)
	installerStatus := s.getInstallerStatus()
	require.Equal(s.T(), previousAgentImageVersion, installerStatus.Packages["datadog-agent"].ExperimentVersion)

	// Stop the last experiment
	timestamp = s.host.LastJournaldTimestamp()
	_, err = s.stopExperimentCommand()
	require.NoError(s.T(), err)
	s.assertSuccessfulStopExperiment(timestamp)
}

func (s *upgradeScenarioSuite) TestPromoteWithoutExperiment() {
	s.RunInstallScript("DD_REMOTE_UPDATES=true")
	defer s.Purge()
	s.host.WaitForUnitActive(
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-process.service",
		"datadog-installer.service",
	)

	s.setCatalog(testCatalog)

	beforeStatus := s.getInstallerStatus()
	_, err := s.promoteExperimentCommand()
	require.Error(s.T(), err)

	afterStatus := s.getInstallerStatus()
	require.Equal(s.T(), beforeStatus.Packages["datadog-agent"], afterStatus.Packages["datadog-agent"])
	require.Equal(s.T(), beforeStatus.Version, afterStatus.Version)

	// Try a golden path to make sure nothing is broken
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

func (s *upgradeScenarioSuite) stopExperimentCommand() (string, error) {
	cmd := "sudo datadog-installer daemon stop-experiment datadog-agent"
	s.T().Logf("Running stop command: %s", cmd)
	return s.Env().RemoteHost.Execute(cmd)
}

func (s *upgradeScenarioSuite) setCatalog(newCatalog catalog) {
	serializedCatalog, err := json.Marshal(newCatalog)
	if err != nil {
		s.T().Fatal(err)
	}
	s.T().Logf("Running: daemon set-catalog '%s'", string(serializedCatalog))

	assert.Eventually(s.T(), func() bool {
		_, err := s.Env().RemoteHost.Execute(fmt.Sprintf(
			"sudo datadog-installer daemon set-catalog '%s'", serializedCatalog),
		)

		return err == nil
	}, time.Second*30, time.Second*1)
}

func (s *upgradeScenarioSuite) assertSuccessfulStartExperiment(timestamp host.JournaldTimestamp, version string) {
	s.host.WaitForUnitActivating(agentUnitXP)
	s.host.WaitForFileExists(false, "/opt/datadog-packages/datadog-agent/experiment/run/agent.pid")

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
	require.Equal(s.T(), version, installerStatus.Packages["datadog-agent"].ExperimentVersion)
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
	require.Equal(s.T(), version, installerStatus.Packages["datadog-agent"].StableVersion)
	require.Equal(s.T(), "", installerStatus.Packages["datadog-agent"].ExperimentVersion)
}

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
	require.Equal(s.T(), "", installerStatus.Packages["datadog-agent"].ExperimentVersion)
}

func (s *upgradeScenarioSuite) getInstallerStatus() installerStatus {
	socketPath := "/var/run/datadog-installer/installer.sock"

	requestHeader := " -H 'Content-Type: application/json' -H 'Accept: application/json' "
	response := s.Env().RemoteHost.MustExecute(fmt.Sprintf(
		"sudo curl -s --unix-socket %s %s http://daemon/status",
		socketPath,
		requestHeader,
	))

	// {"version":"7.56.0-devel+git.446.acf2836","packages":{
	//     "datadog-agent":{"Stable":"7.56.0-devel.git.446.acf2836.pipeline.37567760-1","Experiment":"7.54.1-1"},
	//     "datadog-installer":{"Stable":"7.56.0-devel.git.446.acf2836.pipeline.37567760-1","Experiment":""}}}
	var status installerStatus
	err := json.Unmarshal([]byte(response), &status)
	if err != nil {
		s.T().Fatal(err)
	}

	return status
}
