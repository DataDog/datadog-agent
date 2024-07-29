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

type packageName string

const (
	datadogAgent     packageName = "datadog-agent"
	datadogInstaller packageName = "datadog-installer"
)

const (
	installerUnit   = "datadog-installer.service"
	installerUnitXP = "datadog-installer-exp.service"
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
			Package: string(datadogAgent),
			Version: latestAgentImageVersion,
			URL:     fmt.Sprintf("oci://gcr.io/datadoghq/agent-package:%s", latestAgentImageVersion),
		},
		{
			Package: string(datadogAgent),
			Version: previousAgentImageVersion,
			URL:     fmt.Sprintf("oci://gcr.io/datadoghq/agent-package:%s", previousAgentImageVersion),
		},
		{
			Package: string(datadogInstaller),
			Version: latestInstallerImageVersion,
			URL:     fmt.Sprintf("oci://gcr.io/datadoghq/installer-package:%s", latestInstallerImageVersion),
		},
		{
			Package: string(datadogInstaller),
			Version: previousInstallerImageVersion,
			URL:     fmt.Sprintf("oci://gcr.io/datadoghq/installer-package:%s", previousInstallerImageVersion),
		},
	},
}

const (
	unknownAgentImageVersion  = "7.52.1-1"
	previousAgentImageVersion = "7.54.0-1"
	latestAgentImageVersion   = "7.54.1-1"

	latestInstallerImageVersion   = "7.56.0-installer-0.4.5-1"
	previousInstallerImageVersion = "7.55.0-installer-0.4.1-1"
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
	s.executeAgentGoldenPath()
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
	s.mustStartExperiment(datadogAgent, previousAgentImageVersion)
	s.assertSuccessfulAgentStartExperiment(timestamp, previousAgentImageVersion)

	// Host was left with a not-latest experiment, we're now testing
	// that we can still upgrade
	timestamp = s.host.LastJournaldTimestamp()
	s.mustStopExperiment(datadogAgent)
	s.assertSuccessfulAgentStopExperiment(timestamp)

	s.executeAgentGoldenPath()
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
	s.mustStartExperiment(datadogAgent, latestAgentImageVersion)
	s.assertSuccessfulAgentStartExperiment(timestamp, latestAgentImageVersion)

	// Receive a failure from the backend, stops the experiment
	timestamp = s.host.LastJournaldTimestamp()
	s.mustStopExperiment(datadogAgent)
	s.assertSuccessfulAgentStopExperiment(timestamp)
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
	_, err := s.startExperiment(datadogAgent, unknownAgentImageVersion)
	require.Error(s.T(), err)

	// Receive a failure from the experiment, stops the experiment
	beforeStatus := s.getInstallerStatus()
	s.mustStopExperiment(datadogAgent)
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
	_, err := s.startExperiment(datadogAgent, currentVersion)
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
	s.mustStopExperiment(datadogAgent)

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
	s.mustStartExperiment(datadogAgent, latestAgentImageVersion)
	s.assertSuccessfulAgentStartExperiment(timestamp, latestAgentImageVersion)

	// Start a second experiment that overrides the first one
	s.mustStartExperiment(datadogAgent, previousAgentImageVersion)
	installerStatus := s.getInstallerStatus()
	require.Equal(s.T(), previousAgentImageVersion, installerStatus.Packages["datadog-agent"].ExperimentVersion)

	// Stop the last experiment
	timestamp = s.host.LastJournaldTimestamp()
	s.mustStopExperiment(datadogAgent)
	s.assertSuccessfulAgentStopExperiment(timestamp)
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
	_, err := s.promoteExperiment(datadogAgent)
	require.Error(s.T(), err)

	afterStatus := s.getInstallerStatus()
	require.Equal(s.T(), beforeStatus.Packages["datadog-agent"], afterStatus.Packages["datadog-agent"])
	require.Equal(s.T(), beforeStatus.Version, afterStatus.Version)

	// Try a golden path to make sure nothing is broken
	s.executeAgentGoldenPath()
}

func (s *upgradeScenarioSuite) TestInstallerSuccessful() {
	s.RunInstallScript("DD_REMOTE_UPDATES=true")
	defer s.Purge()
	s.host.WaitForUnitActive(
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-process.service",
		"datadog-installer.service",
	)

	s.setCatalog(testCatalog)
	s.executeInstallerGoldenPath()
}

func (s *upgradeScenarioSuite) TestInstallerBackendFailure() {
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
	s.startExperiment(datadogInstaller, latestInstallerImageVersion) // Can't check error
	s.assertSuccessfulInstallerStartExperiment(timestamp, latestInstallerImageVersion)

	// Receive a failure from the backend or the experiments fail, stops the experiment
	timestamp = s.host.LastJournaldTimestamp()
	s.stopExperiment(datadogInstaller) // Can't check error
	s.assertSuccessfulInstallerStopExperiment(timestamp)

	// Continue with the agent
	s.setCatalog(testCatalog)
	s.executeAgentGoldenPath()
}

func (s *upgradeScenarioSuite) TestInstallerAgentFailure() {
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
	s.startExperiment(datadogInstaller, latestInstallerImageVersion) // Can't check error
	s.assertSuccessfulInstallerStartExperiment(timestamp, latestInstallerImageVersion)

	// Change the catalog of the experiment installer
	s.setCatalog(testCatalog)

	timestamp = s.host.LastJournaldTimestamp()
	s.mustStartExperiment(datadogAgent, latestAgentImageVersion)
	s.assertSuccessfulAgentStartExperiment(timestamp, latestAgentImageVersion)

	// Receive a failure from the agent, stops the agent
	timestamp = s.host.LastJournaldTimestamp()
	s.mustStopExperiment(datadogAgent)
	s.assertSuccessfulAgentStopExperiment(timestamp)

	timestamp = s.host.LastJournaldTimestamp()
	s.stopExperiment(datadogInstaller) // Can't check error
	s.assertSuccessfulInstallerStopExperiment(timestamp)

	// Retry the golden path to check if everything fine
	s.setCatalog(testCatalog)
	s.executeInstallerGoldenPath()
}

func (s *upgradeScenarioSuite) TestUpgradeSuccessfulWithUmask() {
	oldmask := s.host.SetUmask("0027")
	defer s.host.SetUmask(oldmask)

	s.TestUpgradeSuccessful()
}

func (s *upgradeScenarioSuite) startExperiment(pkg packageName, version string) (string, error) {
	cmd := fmt.Sprintf("sudo datadog-installer daemon start-experiment %s %s > /tmp/start_experiment.log 2>&1", pkg, version)
	s.T().Logf("Running start command: %s", cmd)
	return s.Env().RemoteHost.Execute(cmd)
}

func (s *upgradeScenarioSuite) mustStartExperiment(pkg packageName, version string) string {
	output, err := s.startExperiment(pkg, version)
	require.NoError(s.T(), err, "Failed to start experiment: %s\ndatadog-installer journalctl:\n%s",
		s.Env().RemoteHost.MustExecute("cat /tmp/start_experiment.log"),
		s.Env().RemoteHost.MustExecute("journalctl -xeu datadog-installer --no-pager"),
	)
	return output
}

func (s *upgradeScenarioSuite) promoteExperiment(pkg packageName) (string, error) {
	cmd := fmt.Sprintf("sudo datadog-installer daemon promote-experiment %s > /tmp/promote_experiment.log 2>&1", pkg)
	s.T().Logf("Running promote command: %s", cmd)
	return s.Env().RemoteHost.Execute(cmd)
}

func (s *upgradeScenarioSuite) mustPromoteExperiment(pkg packageName) string {
	output, err := s.promoteExperiment(pkg)
	require.NoError(s.T(), err, "Failed to promote experiment: %s\ndatadog-installer journalctl:\n%s",
		s.Env().RemoteHost.MustExecute("cat /tmp/promote_experiment.log"),
		s.Env().RemoteHost.MustExecute("journalctl -xeu datadog-installer --no-pager"),
	)
	return output
}

func (s *upgradeScenarioSuite) stopExperiment(pkg packageName) (string, error) {
	cmd := fmt.Sprintf("sudo datadog-installer daemon stop-experiment %s > /tmp/stop_experiment.log 2>&1", pkg)
	s.T().Logf("Running stop command: %s", cmd)
	return s.Env().RemoteHost.Execute(cmd)
}

func (s *upgradeScenarioSuite) mustStopExperiment(pkg packageName) string {
	output, err := s.stopExperiment(pkg)
	require.NoError(s.T(), err, "Failed to stop experiment: %s\ndatadog-installer journalctl:\n%s",
		s.Env().RemoteHost.MustExecute("cat /tmp/stop_experiment.log"),
		s.Env().RemoteHost.MustExecute("journalctl -xeu datadog-installer --no-pager"),
	)
	return output
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

func (s *upgradeScenarioSuite) assertSuccessfulAgentStartExperiment(timestamp host.JournaldTimestamp, version string) {
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

func (s *upgradeScenarioSuite) assertSuccessfulAgentPromoteExperiment(timestamp host.JournaldTimestamp, version string) {
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

func (s *upgradeScenarioSuite) assertSuccessfulAgentStopExperiment(timestamp host.JournaldTimestamp) {
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

func (s *upgradeScenarioSuite) assertSuccessfulInstallerStartExperiment(timestamp host.JournaldTimestamp, version string) {
	s.host.WaitForUnitActivating(installerUnitXP)
	// TODO: check the pid file
	// s.host.WaitForFileExists(false, "/opt/datadog-packages/datadog-installer/experiment/installer.pid")

	// Assert experiment is running
	s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().
		Unordered(host.SystemdEvents().
			Stopped(installerUnit).
			Starting(installerUnitXP),
		),
	)

	installerStatus := s.getInstallerStatus()
	require.Equal(s.T(), version, installerStatus.Packages["datadog-installer"].ExperimentVersion)
}

// TODO : remove version param after fixing the stop cleanup
func (s *upgradeScenarioSuite) assertSuccessfulInstallerStopExperiment(timestamp host.JournaldTimestamp) {
	// Assert experiment is stopped
	s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().
		Unordered(host.SystemdEvents().
			Starting(installerUnitXP).
			Stopped(installerUnit),
		),
	)

	installerStatus := s.getInstallerStatus()
	require.Equal(s.T(), "", installerStatus.Packages["datadog-installer"].ExperimentVersion)
}

func (s *upgradeScenarioSuite) assertSuccessfulInstallerPromoteExperiment(timestamp host.JournaldTimestamp, version string) {
	s.host.WaitForUnitActive(installerUnit)

	// Assert experiment is running
	s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().
		Stopped(installerUnitXP).
		Started(installerUnit),
	)

	installerStatus := s.getInstallerStatus()
	require.Equal(s.T(), version, installerStatus.Packages["datadog-installer"].StableVersion)
	require.Equal(s.T(), "", installerStatus.Packages["datadog-installer"].ExperimentVersion)
}

func (s *upgradeScenarioSuite) executeAgentGoldenPath() {
	timestamp := s.host.LastJournaldTimestamp()
	s.mustStartExperiment(datadogAgent, latestAgentImageVersion)
	s.assertSuccessfulAgentStartExperiment(timestamp, latestAgentImageVersion)

	timestamp = s.host.LastJournaldTimestamp()
	s.mustPromoteExperiment(datadogAgent)
	s.assertSuccessfulAgentPromoteExperiment(timestamp, latestAgentImageVersion)
}

func (s *upgradeScenarioSuite) executeInstallerGoldenPath() {
	// Experiments the installer then the agent
	timestamp := s.host.LastJournaldTimestamp()
	// Can't check the error status of the command, because it gets terminated by design
	// We check the unit history instead
	s.startExperiment(datadogInstaller, previousInstallerImageVersion)
	s.assertSuccessfulInstallerStartExperiment(timestamp, previousInstallerImageVersion)

	// Change the catalog of the experiment installer
	s.setCatalog(testCatalog)

	timestamp = s.host.LastJournaldTimestamp()
	s.mustStartExperiment(datadogAgent, latestAgentImageVersion)
	s.assertSuccessfulAgentStartExperiment(timestamp, latestAgentImageVersion)

	// Promote the agent then the installer
	timestamp = s.host.LastJournaldTimestamp()
	s.mustPromoteExperiment(datadogAgent)
	s.assertSuccessfulAgentPromoteExperiment(timestamp, latestAgentImageVersion)

	timestamp = s.host.LastJournaldTimestamp()
	// Can't check the error status of the command, because it gets terminated by design
	// We check the unit history instead
	s.promoteExperiment(datadogInstaller)
	s.assertSuccessfulInstallerPromoteExperiment(timestamp, previousInstallerImageVersion)
}
