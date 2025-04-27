// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"encoding/json"
	"fmt"
	"time"

	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/host"
)

type packageName string

const (
	datadogAgent     packageName = "datadog-agent"
	datadogInstaller packageName = "datadog-installer"
	datadogApmInject packageName = "datadog-apm-inject"
)

const (
	installerUnit    = "datadog-agent-installer.service"
	installerUnitXP  = "datadog-agent-installer-exp.service"
	apmInjectVersion = "0.1.2"
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
	States       map[string]stableExperimentStatus `json:"states"`
	ConfigStates map[string]stableExperimentStatus `json:"config_states"`
}

type stableExperimentStatus struct {
	Stable     string `json:"Stable"`
	Experiment string `json:"Experiment"`
}

type installerStatus struct {
	Version  string        `json:"version"`
	Packages packageStatus `json:"packages"`
}

// For older installer versions
type packageStatusLegacy struct {
	Version stableExperimentStatus `json:"Version"`
	Config  stableExperimentStatus `json:"Config"`

	LegacyVersionStable     string `json:"Stable"`
	LegacyVersionExperiment string `json:"Experiment"`
}

// For older installer versions
type installerStatusLegacy struct {
	Version  string                         `json:"version"`
	Packages map[string]packageStatusLegacy `json:"packages"`
}

type installerConfigFile struct {
	Path     string          `json:"path"`
	Contents json.RawMessage `json:"contents"`
}

type installerConfig struct {
	ID    string                `json:"id"`
	Files []installerConfigFile `json:"files"`
}

var testCatalog = catalog{
	Packages: []packageEntry{
		{
			Package: string(datadogAgent),
			Version: latestAgentImageVersion,
			URL:     fmt.Sprintf("oci://install.datad0g.com/agent-package:%s", latestAgentImageVersion),
		},
		{
			Package: string(datadogApmInject),
			Version: apmInjectVersion,
			URL:     "oci://install.datadoghq.com/apm-inject-package:latest",
		},
	},
}

const (
	unknownAgentImageVersion = "7.52.1-1"

	// TODO: use the latest prod images when they are out
	latestAgentImageVersion = "7.66.0-devel.git.534.4e40dec.pipeline.62473533-1"
)

func testUpgradeScenario(os e2eos.Descriptor, arch e2eos.Architecture, method InstallMethodOption) packageSuite {
	return &upgradeScenarioSuite{
		packageBaseSuite: newPackageSuite("upgrade_scenario", os, arch, method),
	}
}

func (s *upgradeScenarioSuite) TestUpgradeSuccessful() {
	s.RunInstallScript("DD_REMOTE_UPDATES=true")
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(s.T(),
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-installer.service",
	)
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")

	s.setCatalog(testCatalog)
	s.executeAgentGoldenPath()
}

func (s *upgradeScenarioSuite) TestBackendFailure() {
	s.RunInstallScript("DD_REMOTE_UPDATES=true")
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(s.T(),
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-installer.service",
	)
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")

	s.setCatalog(testCatalog)

	timestamp := s.host.LastJournaldTimestamp()
	s.startExperiment(datadogAgent, latestAgentImageVersion)
	s.assertSuccessfulAgentStartExperiment(timestamp, latestAgentImageVersion)

	// Receive a failure from the backend, stops the experiment
	timestamp = s.host.LastJournaldTimestamp()
	s.stopExperiment(datadogAgent)
	s.assertSuccessfulAgentStopExperiment(timestamp)
}

func (s *upgradeScenarioSuite) TestExperimentFailure() {
	s.RunInstallScript("DD_REMOTE_UPDATES=true")
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(s.T(),
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-installer.service",
	)
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")

	s.setCatalog(testCatalog)

	// Also tests if the version is not available in the catalog
	_, err := s.startExperiment(datadogAgent, unknownAgentImageVersion)
	require.Error(s.T(), err)

	// Receive a failure from the experiment, stops the experiment
	beforeStatus := s.getInstallerStatus()
	s.stopExperiment(datadogAgent)
	afterStatus := s.getInstallerStatus()

	require.Equal(s.T(), beforeStatus.Packages.States["datadog-agent"], afterStatus.Packages.States["datadog-agent"])
}

func (s *upgradeScenarioSuite) TestExperimentCurrentVersion() {
	s.RunInstallScript("DD_REMOTE_UPDATES=true")
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(s.T(),
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-installer.service",
	)
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")

	// Temporary catalog to wait for the installer to be ready
	s.setCatalog(testCatalog)

	currentVersion := s.getInstallerStatus().Packages.States["datadog-agent"].Stable
	newCatalog := catalog{
		Packages: []packageEntry{
			{
				Package: "datadog-agent",
				Version: currentVersion,
				URL:     fmt.Sprintf("oci://install.datadoghq.com/agent-package:%s", currentVersion),
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
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(s.T(),
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-installer.service",
	)
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")

	s.setCatalog(testCatalog)
	beforeStatus := s.getInstallerStatus()
	s.stopExperiment(datadogAgent)

	afterStatus := s.getInstallerStatus()
	require.Equal(s.T(), beforeStatus.Packages.States["datadog-agent"], afterStatus.Packages.States["datadog-agent"])
}

func (s *upgradeScenarioSuite) TestPromoteWithoutExperiment() {
	s.RunInstallScript("DD_REMOTE_UPDATES=true")
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(s.T(),
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-installer.service",
	)
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")

	s.setCatalog(testCatalog)

	beforeStatus := s.getInstallerStatus()
	_, err := s.promoteExperiment(datadogAgent)
	require.Error(s.T(), err)

	afterStatus := s.getInstallerStatus()
	require.Equal(s.T(), beforeStatus.Packages.States["datadog-agent"], afterStatus.Packages.States["datadog-agent"])
	require.Equal(s.T(), beforeStatus.Version, afterStatus.Version)

	// Try a golden path to make sure nothing is broken
	s.executeAgentGoldenPath()
}

func (s *upgradeScenarioSuite) TestUpgradeSuccessfulWithUmask() {
	oldmask := s.host.SetUmask("0027")
	defer s.host.SetUmask(oldmask)

	s.TestUpgradeSuccessful()
}

func (s *upgradeScenarioSuite) TestConfigUpgradeSuccessful() {
	s.RunInstallScript(
		"DD_REMOTE_UPDATES=true",
	)
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(s.T(),
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-installer.service",
	)
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")

	state := s.host.State()
	// Assert setup successful
	state.AssertDirExists("/etc/datadog-agent/managed", 0755, "root", "root")
	state.AssertDirExists("/etc/datadog-agent/managed/datadog-agent", 0755, "root", "root")
	state.AssertSymlinkExists("/etc/datadog-agent/managed/datadog-agent/stable", "/etc/datadog-agent/managed/datadog-agent/empty", "root", "root")

	config := installerConfig{
		ID:    "config-1",
		Files: []installerConfigFile{{Path: "/datadog.yaml", Contents: json.RawMessage(`{"log_level": "debug"}`)}},
	}
	s.executeConfigGoldenPath(config)
}

func (s *upgradeScenarioSuite) TestConfigUpgradeNewAgents() {
	timestamp := s.host.LastJournaldTimestamp()
	s.RunInstallScript(
		"DD_REMOTE_UPDATES=true",
	)
	defer s.Purge()

	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(s.T(),
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-installer.service",
	)
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")
	// Make sure security agent and system probe are disabled
	s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().
		Unordered(host.SystemdEvents().
			Skipped(probeUnit).
			Skipped(securityUnit),
		),
	)

	state := s.host.State()
	state.AssertSymlinkExists("/etc/datadog-agent/managed/datadog-agent/stable", "/etc/datadog-agent/managed/datadog-agent/empty", "root", "root")

	// Enables security agent & sysprobe
	config := installerConfig{
		ID: "config-1",
		Files: []installerConfigFile{
			{
				Path:     "/datadog.yaml",
				Contents: json.RawMessage(`{"sbom": {"container_image": {"enabled": true}, "host": {"enabled": true}}}`),
			},
			{
				Path:     "/security-agent.yaml",
				Contents: json.RawMessage(`{"runtime_security_config": {"enabled": true}, "compliance_config": {"enabled": true}}`),
			},
			{
				Path:     "/system-probe.yaml",
				Contents: json.RawMessage(`{"runtime_security_config": {"enabled": true}}`),
			},
		},
	}
	timestamp = s.host.LastJournaldTimestamp()
	s.mustStartConfigExperiment(datadogAgent, config)
	// Assert the successful start of the experiment
	s.host.WaitForUnitActive(s.T(), agentUnitXP)
	s.host.WaitForFileExists(false, "/opt/datadog-packages/datadog-agent/experiment/run/agent.pid")

	// Assert experiment is running
	s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().
		Unordered(host.SystemdEvents().
			Stopped(agentUnit).
			Stopped(traceUnit),
		).
		Unordered(host.SystemdEvents().
			Started(agentUnitXP).
			Started(traceUnitXP).
			Started(securityUnitXP).
			Started(probeUnitXP),
		),
	)

	timestamp = s.host.LastJournaldTimestamp()
	s.mustPromoteConfigExperiment(datadogAgent)
	s.host.WaitForUnitActive(s.T(), agentUnit)

	// Assert experiment is promoted
	s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().
		Unordered(host.SystemdEvents().
			Stopped(agentUnitXP).
			Stopped(traceUnitXP).
			Stopped(securityUnitXP).
			Stopped(probeUnitXP),
		).
		Unordered(host.SystemdEvents().
			Started(agentUnit).
			Stopped(traceUnit).
			Started(securityUnit).
			Started(probeUnit),
		),
	)

	state = s.host.State()
	state.AssertSymlinkExists("/etc/datadog-agent/managed/datadog-agent/stable", fmt.Sprintf("/etc/datadog-agent/managed/datadog-agent/%s", config.ID), "root", "root")
	state.AssertSymlinkExists("/etc/datadog-agent/managed/datadog-agent/experiment", fmt.Sprintf("/etc/datadog-agent/managed/datadog-agent/%s", config.ID), "root", "root")
}

func (s *upgradeScenarioSuite) TestUpgradeConfigFromExistingExperiment() {
	s.RunInstallScript(
		"DD_REMOTE_UPDATES=true",
	)
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(s.T(),
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-installer.service",
	)
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")

	config1 := installerConfig{
		ID:    "config-1",
		Files: []installerConfigFile{{Path: "/datadog.yaml", Contents: json.RawMessage(`{"log_level": "error"}`)}},
	}

	timestamp := s.host.LastJournaldTimestamp()
	s.mustStartConfigExperiment(datadogAgent, config1)
	s.assertSuccessfulConfigStartExperiment(timestamp, "config-1")

	// Host was left with a config experiment, we're now testing
	// that we can still upgrade
	timestamp = s.host.LastJournaldTimestamp()
	s.mustStopConfigExperiment(datadogAgent)
	s.assertSuccessfulConfigStopExperiment(timestamp)

	config2 := installerConfig{
		ID:    "config-2",
		Files: []installerConfigFile{{Path: "/datadog.yaml", Contents: json.RawMessage(`{"log_level": "debug"}`)}},
	}
	s.executeConfigGoldenPath(config2)
}

func (s *upgradeScenarioSuite) TestUpgradeConfigFailure() {
	s.RunInstallScript(
		"DD_REMOTE_UPDATES=true",
	)
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(s.T(),
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-installer.service",
	)
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")

	// Non alphanumerical characters are not allowed, the agent should crash
	config := installerConfig{
		ID:    "config",
		Files: []installerConfigFile{{Path: "/datadog.yaml", Contents: json.RawMessage(`{"log_level": "ENC[hi]"}`)}},
	}
	timestamp := s.host.LastJournaldTimestamp()
	_, err := s.startConfigExperiment(datadogAgent, config)
	s.T().Logf("Error: %s", s.Env().RemoteHost.MustExecute("cat /tmp/start_config_experiment.log"))
	require.NoError(s.T(), err)

	// Assert experiment is stopped as the agent should've crashed
	s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().
		Unordered(host.SystemdEvents(). // Stable stops
						Stopped(agentUnit).
						Stopped(traceUnit),
		).
		Unordered(host.SystemdEvents(). // Experiment starts
						Started(agentUnitXP).
						Started(traceUnitXP),
		).
		Unordered(host.SystemdEvents(). // Experiment fails
						Failed(agentUnitXP).
						Stopped(traceUnitXP),
		).
		Started(agentUnit). // Stable restarts
		Unordered(host.SystemdEvents().
			Started(traceUnit),
		),
	)

	s.mustStopConfigExperiment(datadogAgent)
}

func (s *upgradeScenarioSuite) TestUpgradeWithProxy() {
	if s.Env().RemoteHost.OSFlavor == e2eos.Fedora || s.Env().RemoteHost.OSFlavor == e2eos.RedHat {
		s.T().Skip("Fedora & RedHat can't start the Squid proxy")
	}

	s.RunInstallScript("DD_REMOTE_UPDATES=true") // No proxy during install, to avoid setting up the APT proxy
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")

	// Set proxy config
	s.Env().RemoteHost.MustExecute(`printf "proxy:\n  http: http://localhost:3128\n  https: http://localhost:3128" | sudo tee -a /etc/datadog-agent/datadog.yaml`)
	defer func() {
		s.Env().RemoteHost.MustExecute(`sudo sed -i '/proxy:/,/https:/d' /etc/datadog-agent/datadog.yaml`)
	}()
	s.Env().RemoteHost.MustExecute(`sudo systemctl restart datadog-agent.service`)

	// Set catalog
	s.host.WaitForUnitActive(s.T(),
		"datadog-agent.service",
		"datadog-agent-installer.service",
	)
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")
	s.setCatalog(testCatalog)

	// Set host proxy setup
	defer s.host.RemoveProxy()
	s.host.SetupProxy()

	s.executeAgentGoldenPath()
}

func (s *upgradeScenarioSuite) TestRemoteInstallUninstall() {
	s.RunInstallScript("DD_REMOTE_UPDATES=true")
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(s.T(),
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-installer.service",
	)
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")

	s.setCatalog(testCatalog)
	s.mustInstallPackage(datadogApmInject, apmInjectVersion)
	s.host.AssertPackageInstalledByInstaller("datadog-apm-inject")

	s.mustRemovePackage(datadogApmInject)
	state := s.host.State()
	state.AssertPathDoesNotExist("/opt/datadog-packages/datadog-apm-inject")

}

func (s *upgradeScenarioSuite) installPackage(pkg packageName, version string) (string, error) {
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")
	cmd := fmt.Sprintf("sudo datadog-installer daemon install %s %s > /tmp/install_package.log 2>&1", pkg, version)
	s.T().Logf("Running install command: %s", cmd)
	return s.Env().RemoteHost.Execute(cmd)
}

func (s *upgradeScenarioSuite) mustInstallPackage(pkg packageName, version string) string {
	output, err := s.installPackage(pkg, version)
	require.NoError(s.T(), err, "Failed to install package: %s\ndatadog-agent-installer journalctl:\n%s\ndatadog-agent-installer-exp journalctl:\n%s",
		s.Env().RemoteHost.MustExecute("cat /tmp/install_package.log"),
		s.Env().RemoteHost.MustExecute("sudo journalctl -xeu datadog-agent-installer.service --no-pager"),
		s.Env().RemoteHost.MustExecute("sudo journalctl -xeu datadog-agent-installer-exp.service --no-pager"),
	)
	return output
}

func (s *upgradeScenarioSuite) removePackage(pkg packageName) (string, error) {
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")
	cmd := fmt.Sprintf("sudo datadog-installer daemon remove %s > /tmp/install_package.log 2>&1", pkg)

	s.T().Logf("Running remove command: %s", cmd)
	return s.Env().RemoteHost.Execute(cmd)
}

func (s *upgradeScenarioSuite) mustRemovePackage(pkg packageName) string {
	output, err := s.removePackage(pkg)
	require.NoError(s.T(), err, "Failed to remove package: %s\ndatadog-agent-installer journalctl:\n%s\ndatadog-agent-installer-exp journalctl:\n%s",
		s.Env().RemoteHost.MustExecute("cat /tmp/install_package.log"),
		s.Env().RemoteHost.MustExecute("sudo journalctl -xeu datadog-agent-installer --no-pager"),
		s.Env().RemoteHost.MustExecute("sudo journalctl -xeu datadog-agent-installer-exp --no-pager"),
	)
	return output
}

func (s *upgradeScenarioSuite) startExperiment(pkg packageName, version string) (string, error) {
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")
	cmd := fmt.Sprintf("sudo datadog-installer daemon start-experiment %s %s > /tmp/start_experiment.log 2>&1", pkg, version)
	s.T().Logf("Running start command: %s", cmd)
	return s.Env().RemoteHost.Execute(cmd)
}

func (s *upgradeScenarioSuite) promoteExperiment(pkg packageName) (string, error) {
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")
	cmd := fmt.Sprintf("sudo datadog-installer daemon promote-experiment %s > /tmp/promote_experiment.log 2>&1", pkg)
	s.T().Logf("Running promote command: %s", cmd)
	return s.Env().RemoteHost.Execute(cmd)
}

func (s *upgradeScenarioSuite) stopExperiment(pkg packageName) (string, error) {
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")
	cmd := fmt.Sprintf("sudo datadog-installer daemon stop-experiment %s > /tmp/stop_experiment.log 2>&1", pkg)
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

func (s *upgradeScenarioSuite) assertSuccessfulAgentStartExperiment(timestamp host.JournaldTimestamp, version string) {
	s.host.WaitForUnitActive(s.T(), agentUnitXP)
	s.host.WaitForFileExists(false, "/opt/datadog-packages/datadog-agent/experiment/run/agent.pid")

	// Assert experiment is running
	s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().
		Unordered(host.SystemdEvents().
			Stopped(agentUnit).
			Stopped(traceUnit),
		).
		Unordered(host.SystemdEvents().
			Started(agentUnitXP).
			Started(traceUnitXP),
		),
	)

	installerStatus := s.getInstallerStatus()
	require.Equal(s.T(), version, installerStatus.Packages.States["datadog-agent"].Experiment)
}

func (s *upgradeScenarioSuite) assertSuccessfulAgentPromoteExperiment(timestamp host.JournaldTimestamp, version string) {
	s.host.WaitForUnitActive(s.T(), agentUnit)

	// Assert experiment is promoted
	s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().
		Unordered(host.SystemdEvents().
			Stopped(agentUnitXP).
			Stopped(traceUnitXP),
		).
		Unordered(host.SystemdEvents().
			Started(agentUnit).
			Stopped(traceUnit),
		),
	)

	installerStatus := s.getInstallerStatus()
	require.Equal(s.T(), version, installerStatus.Packages.States["datadog-agent"].Stable)
	require.Equal(s.T(), "", installerStatus.Packages.States["datadog-agent"].Experiment)
}

func (s *upgradeScenarioSuite) assertSuccessfulAgentStopExperiment(timestamp host.JournaldTimestamp) {
	// Assert experiment is stopped
	s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().
		Unordered(host.SystemdEvents().
			Stopped(agentUnitXP).
			Stopped(traceUnitXP),
		).
		Started(agentUnit).
		Unordered(host.SystemdEvents().
			Started(traceUnit),
		),
	)

	installerStatus := s.getInstallerStatus()
	require.Equal(s.T(), "", installerStatus.Packages.States["datadog-agent"].Experiment)
}

func (s *upgradeScenarioSuite) startConfigExperiment(pkg packageName, config installerConfig) (string, error) {
	rawConfig, err := json.Marshal(config.Files)
	require.NoError(s.T(), err)
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")
	cmd := fmt.Sprintf("sudo -E datadog-installer install-config-experiment %s %s '%s' > /tmp/start_config_experiment.log 2>&1", pkg, config.ID, rawConfig)
	s.T().Logf("Running start command: %s", cmd)
	return s.Env().RemoteHost.Execute(cmd)
}

func (s *upgradeScenarioSuite) mustStartConfigExperiment(pkg packageName, config installerConfig) string {
	output, err := s.startConfigExperiment(pkg, config)
	require.NoError(s.T(), err, "Failed to start config experiment: %s\ndatadog-agent-installer journalctl:\n%s\ndatadog-agent-installer-exp journalctl:\n%s",
		s.Env().RemoteHost.MustExecute("cat /tmp/start_config_experiment.log"),
		s.Env().RemoteHost.MustExecute("sudo journalctl -xeu datadog-agent-installer --no-pager"),
		s.Env().RemoteHost.MustExecute("sudo journalctl -xeu datadog-agent-installer-exp --no-pager"),
	)
	return output
}

func (s *upgradeScenarioSuite) promoteConfigExperiment(pkg packageName) (string, error) {
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")
	cmd := fmt.Sprintf("sudo -E datadog-installer promote-config-experiment %s > /tmp/promote_config_experiment.log 2>&1", pkg)
	s.T().Logf("Running promote command: %s", cmd)
	return s.Env().RemoteHost.Execute(cmd)
}

func (s *upgradeScenarioSuite) mustPromoteConfigExperiment(pkg packageName) string {
	output, err := s.promoteConfigExperiment(pkg)
	require.NoError(s.T(), err, "Failed to promote config experiment: %s\ndatadog-agent-installer journalctl:\n%s\ndatadog-agent-installer-exp journalctl:\n%s",
		s.Env().RemoteHost.MustExecute("cat /tmp/promote_config_experiment.log"),
		s.Env().RemoteHost.MustExecute("sudo journalctl -xeu datadog-agent-installer --no-pager"),
		s.Env().RemoteHost.MustExecute("sudo journalctl -xeu datadog-agent-installer-exp --no-pager"),
	)
	return output
}

func (s *upgradeScenarioSuite) stopConfigExperiment(pkg packageName) (string, error) {
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")
	cmd := fmt.Sprintf("sudo -E datadog-installer remove-config-experiment %s > /tmp/stop_config_experiment.log 2>&1", pkg)
	s.T().Logf("Running stop command: %s", cmd)
	return s.Env().RemoteHost.Execute(cmd)
}

func (s *upgradeScenarioSuite) mustStopConfigExperiment(pkg packageName) string {
	output, err := s.stopConfigExperiment(pkg)
	require.NoError(s.T(), err, "Failed to stop experiment: %s\ndatadog-agent-installer journalctl:\n%s\ndatadog-agent-installer-exp journalctl:\n%s",
		s.Env().RemoteHost.MustExecute("cat /tmp/stop_config_experiment.log"),
		s.Env().RemoteHost.MustExecute("sudo journalctl -xeu datadog-agent-installer --no-pager"),
		s.Env().RemoteHost.MustExecute("sudo journalctl -xeu datadog-agent-installer-exp --no-pager"),
	)
	return output
}

func (s *upgradeScenarioSuite) assertSuccessfulConfigStartExperiment(timestamp host.JournaldTimestamp, version string) {
	s.host.WaitForUnitActive(s.T(), agentUnitXP)
	s.host.WaitForFileExists(false, "/opt/datadog-packages/datadog-agent/experiment/run/agent.pid")

	// Assert experiment is running
	s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().
		Unordered(host.SystemdEvents().
			Stopped(agentUnit).
			Stopped(traceUnit),
		).
		Unordered(host.SystemdEvents().
			Started(agentUnitXP).
			Started(traceUnitXP),
		),
	)

	state := s.host.State()
	state.AssertSymlinkExists("/etc/datadog-agent/managed/datadog-agent/experiment", fmt.Sprintf("/etc/datadog-agent/managed/datadog-agent/%s", version), "root", "root")
}

func (s *upgradeScenarioSuite) assertSuccessfulConfigPromoteExperiment(timestamp host.JournaldTimestamp, version string) {
	s.host.WaitForUnitActive(s.T(), agentUnit)

	// Assert experiment is promoted
	s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().
		Unordered(host.SystemdEvents().
			Stopped(agentUnitXP).
			Stopped(traceUnitXP),
		).
		Unordered(host.SystemdEvents().
			Started(agentUnit).
			Stopped(traceUnit),
		),
	)

	state := s.host.State()
	state.AssertSymlinkExists("/etc/datadog-agent/managed/datadog-agent/stable", fmt.Sprintf("/etc/datadog-agent/managed/datadog-agent/%s", version), "root", "root")
	state.AssertSymlinkExists("/etc/datadog-agent/managed/datadog-agent/experiment", fmt.Sprintf("/etc/datadog-agent/managed/datadog-agent/%s", version), "root", "root")
}

func (s *upgradeScenarioSuite) assertSuccessfulConfigStopExperiment(timestamp host.JournaldTimestamp) {
	// Assert experiment is stopped
	s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().
		Unordered(host.SystemdEvents().
			Stopped(agentUnitXP).
			Stopped(traceUnitXP),
		).
		Started(agentUnit).
		Unordered(host.SystemdEvents().
			Started(traceUnit),
		),
	)

	state := s.host.State()
	state.AssertSymlinkExists("/etc/datadog-agent/managed/datadog-agent/experiment", "/etc/datadog-agent/managed/datadog-agent/stable", "root", "root")
}

// getInstallerStatusLegacy retrieves the status of older installers
func (s *upgradeScenarioSuite) getInstallerStatusLegacy() installerStatus {
	socketPath := "/opt/datadog-packages/run/installer.sock"

	var response string
	assert.Eventually(s.T(), func() bool {
		var err error
		requestHeader := " -H 'Content-Type: application/json' -H 'Accept: application/json' "
		response, err = s.Env().RemoteHost.Execute(fmt.Sprintf(
			"sudo curl -s --unix-socket %s %s http://daemon/status",
			socketPath,
			requestHeader,
		))
		return err == nil
	}, time.Second*30, time.Second*1, "Failed to get installer status: %s\n\n%s",
		s.Env().RemoteHost.MustExecute("sudo journalctl -xeu datadog-agent-installer"),
		s.Env().RemoteHost.MustExecute("sudo journalctl -xeu datadog-agent-installer-exp"),
	)

	var statusLegacy installerStatusLegacy
	err := json.Unmarshal([]byte(response), &statusLegacy)
	if err != nil {
		s.T().Fatal(err)
	}
	// Legacy status handling
	for k, pkg := range statusLegacy.Packages {
		if pkg.LegacyVersionStable != "" || pkg.LegacyVersionExperiment != "" {
			pkg.Version.Stable = pkg.LegacyVersionStable
			pkg.Version.Experiment = pkg.LegacyVersionExperiment
			statusLegacy.Packages[k] = pkg
		}
	}

	// Convert to the new format
	status := installerStatus{
		Version: statusLegacy.Version,
		Packages: packageStatus{
			States: make(map[string]stableExperimentStatus),
		},
	}

	for k, pkg := range statusLegacy.Packages {
		status.Packages.States[k] = stableExperimentStatus{
			Stable:     pkg.Version.Stable,
			Experiment: pkg.Version.Experiment,
		}
	}

	return status
}

// getInstallerStatus retrieves the status of the installer as a JSON string
func (s *upgradeScenarioSuite) getInstallerStatus() (status installerStatus) {
	var err error

	defer func() {
		// Handle legacy installers
		if err != nil {
			status = s.getInstallerStatusLegacy()
		}
	}()

	var response string
	response, err = s.Env().RemoteHost.Execute("sudo datadog-installer status --json")
	if err != nil {
		s.T().Logf("Failed to get installer status, trying legacy format (err is %v)", err)
		return
	}
	err = json.Unmarshal([]byte(response), &status)
	if err != nil {
		s.T().Logf("Failed to unmarshal installer status, trying legacy format (err is %v)", err)
		return
	}
	return status
}

func (s *upgradeScenarioSuite) executeAgentGoldenPath() {
	timestamp := s.host.LastJournaldTimestamp()
	s.startExperiment(datadogAgent, latestAgentImageVersion)
	s.assertSuccessfulAgentStartExperiment(timestamp, latestAgentImageVersion)

	timestamp = s.host.LastJournaldTimestamp()
	s.promoteExperiment(datadogAgent)
	s.assertSuccessfulAgentPromoteExperiment(timestamp, latestAgentImageVersion)
}

func (s *upgradeScenarioSuite) executeConfigGoldenPath(config installerConfig) {
	timestamp := s.host.LastJournaldTimestamp()

	s.mustStartConfigExperiment(datadogAgent, config)
	s.assertSuccessfulConfigStartExperiment(timestamp, config.ID)

	timestamp = s.host.LastJournaldTimestamp()
	s.mustPromoteConfigExperiment(datadogAgent)
	s.assertSuccessfulConfigPromoteExperiment(timestamp, config.ID)
}
