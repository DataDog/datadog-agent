// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
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
	Version stableExperimentStatus `json:"Version"`
	Config  stableExperimentStatus `json:"Config"`

	LegacyVersionStable     string `json:"Stable"`
	LegacyVersionExperiment string `json:"Experiment"`
}

type stableExperimentStatus struct {
	Stable     string `json:"Stable"`
	Experiment string `json:"Experiment"`
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
			URL:     fmt.Sprintf("oci://install.datadoghq.com/agent-package:%s", latestAgentImageVersion),
		},
		{
			Package: string(datadogAgent),
			Version: previousAgentImageVersion,
			URL:     fmt.Sprintf("oci://install.datadoghq.com/agent-package:%s", previousAgentImageVersion),
		},
		{
			Package: string(datadogInstaller),
			Version: latestInstallerImageVersion,
			URL:     fmt.Sprintf("oci://install.datadoghq.com/installer-package:%s", latestInstallerImageVersion),
		},
		{
			Package: string(datadogInstaller),
			Version: previousInstallerImageVersion,
			URL:     fmt.Sprintf("oci://install.datadoghq.com/installer-package:%s", previousInstallerImageVersion),
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

func testUpgradeScenario(os e2eos.Descriptor, arch e2eos.Architecture, method InstallMethodOption) packageSuite {
	return &upgradeScenarioSuite{
		packageBaseSuite: newPackageSuite("upgrade_scenario", os, arch, method),
	}
}

func (s *upgradeScenarioSuite) TestUpgradeSuccessful() {
	s.RunInstallScript("DD_REMOTE_UPDATES=true")
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-process.service",
		"datadog-installer.service",
	)
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")

	s.setCatalog(testCatalog)
	s.executeAgentGoldenPath()
}

func (s *upgradeScenarioSuite) TestUpgradeFromExistingExperiment() {
	s.RunInstallScript("DD_REMOTE_UPDATES=true")
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-process.service",
		"datadog-installer.service",
	)
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")

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
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-process.service",
		"datadog-installer.service",
	)
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")

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
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-process.service",
		"datadog-installer.service",
	)
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")

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
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-process.service",
		"datadog-installer.service",
	)
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")

	// Temporary catalog to wait for the installer to be ready
	s.setCatalog(testCatalog)

	currentVersion := s.getInstallerStatus().Packages["datadog-agent"].Version.Stable
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
	s.host.WaitForUnitActive(
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-process.service",
		"datadog-installer.service",
	)
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")

	s.setCatalog(testCatalog)
	beforeStatus := s.getInstallerStatus()
	s.mustStopExperiment(datadogAgent)

	afterStatus := s.getInstallerStatus()
	require.Equal(s.T(), beforeStatus.Packages["datadog-agent"], afterStatus.Packages["datadog-agent"])
}

func (s *upgradeScenarioSuite) TestDoubleExperiments() {
	s.RunInstallScript("DD_REMOTE_UPDATES=true")
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-process.service",
		"datadog-installer.service",
	)
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")

	s.setCatalog(testCatalog)

	timestamp := s.host.LastJournaldTimestamp()
	s.mustStartExperiment(datadogAgent, latestAgentImageVersion)
	s.assertSuccessfulAgentStartExperiment(timestamp, latestAgentImageVersion)

	// Start a second experiment that overrides the first one
	s.mustStartExperiment(datadogAgent, previousAgentImageVersion)
	installerStatus := s.getInstallerStatus()
	require.Equal(s.T(), previousAgentImageVersion, installerStatus.Packages["datadog-agent"].Version.Experiment)

	// Stop the last experiment
	timestamp = s.host.LastJournaldTimestamp()
	s.mustStopExperiment(datadogAgent)
	s.assertSuccessfulAgentStopExperiment(timestamp)
}

func (s *upgradeScenarioSuite) TestPromoteWithoutExperiment() {
	s.RunInstallScript("DD_REMOTE_UPDATES=true")
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-process.service",
		"datadog-installer.service",
	)
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")

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
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-process.service",
		"datadog-installer.service",
	)
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")

	s.setCatalog(testCatalog)
	s.executeInstallerGoldenPath()
}

func (s *upgradeScenarioSuite) TestInstallerBackendFailure() {
	s.RunInstallScript("DD_REMOTE_UPDATES=true")
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-process.service",
		"datadog-installer.service",
	)
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")

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
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-process.service",
		"datadog-installer.service",
	)
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")

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

type installerConfig struct {
	ID      string          `json:"id"`
	Configs json.RawMessage `json:"configs"`
}

func (s *upgradeScenarioSuite) TestConfigUpgradeSuccessful() {
	s.RunInstallScript(
		"DD_REMOTE_UPDATES=true",
		"DD_REMOTE_POLICIES=true",
	)
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-process.service",
		"datadog-installer.service",
	)
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")

	state := s.host.State()
	// Assert setup successful
	state.AssertDirExists("/etc/datadog-agent/managed", 0755, "root", "root")
	state.AssertDirExists("/etc/datadog-agent/managed/datadog-apm-libraries", 0755, "root", "root")
	state.AssertDirExists("/etc/datadog-agent/managed/datadog-agent", 0755, "root", "root")
	state.AssertSymlinkExists("/etc/datadog-agent/managed/datadog-agent/stable", "/etc/datadog-agent/managed/datadog-agent/empty", "root", "root")

	config := installerConfig{
		ID:      "config-1",
		Configs: json.RawMessage(`{"datadog.yaml": {"log_level": "debug"}}`),
	}
	s.executeConfigGoldenPath(config)
}

func (s *upgradeScenarioSuite) TestConfigUpgradeNewAgents() {
	timestamp := s.host.LastJournaldTimestamp()
	s.RunInstallScript(
		"DD_REMOTE_UPDATES=true",
		"DD_REMOTE_POLICIES=true",
	)
	defer s.Purge()

	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-process.service",
		"datadog-installer.service",
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
		Configs: json.RawMessage(`
{
  "datadog.yaml": {
    "sbom": {
      "container_image": {
        "enabled": true
      },
      "host": {
        "enabled": true
      }
    }
  },
  "security-agent.yaml": {
    "runtime_security_config": {
      "enabled": true
    },
    "compliance_config": {
      "enabled": true
    }
  },
  "system-probe.yaml": {
    "runtime_security_config": {
      "enabled": true
    }
  }
}`),
	}
	hash := "4681728e9932105c5eea80056151172764d99e348d09a78158874389d25f3c00"
	timestamp = s.host.LastJournaldTimestamp()
	s.mustStartConfigExperiment(datadogAgent, config)
	// Assert the successful start of the experiment
	s.host.WaitForUnitActivating(s.T(), agentUnitXP)
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
			Started(processUnitXP).
			Started(securityUnitXP).
			Started(probeUnitXP),
		),
	)

	timestamp = s.host.LastJournaldTimestamp()
	s.mustPromoteConfigExperiment(datadogAgent)
	s.host.WaitForUnitActive(agentUnit)

	// Assert experiment is promoted
	s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().
		Unordered(host.SystemdEvents().
			Stopped(agentUnitXP).
			Stopped(processUnitXP).
			Stopped(traceUnitXP).
			Stopped(securityUnitXP).
			Stopped(probeUnitXP),
		).
		Unordered(host.SystemdEvents().
			Started(agentUnit).
			Stopped(processUnit).
			Stopped(traceUnit).
			Started(securityUnit).
			Started(probeUnit),
		),
	)

	state = s.host.State()
	state.AssertSymlinkExists("/etc/datadog-agent/managed/datadog-agent/stable", fmt.Sprintf("/etc/datadog-agent/managed/datadog-agent/%s", hash), "root", "root")
	state.AssertSymlinkExists("/etc/datadog-agent/managed/datadog-agent/experiment", fmt.Sprintf("/etc/datadog-agent/managed/datadog-agent/%s", hash), "root", "root")
}

func (s *upgradeScenarioSuite) TestUpgradeConfigFromExistingExperiment() {
	s.RunInstallScript(
		"DD_REMOTE_UPDATES=true",
		"DD_REMOTE_POLICIES=true",
	)
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-process.service",
		"datadog-installer.service",
	)
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")

	config1 := installerConfig{
		ID:      "config-1",
		Configs: json.RawMessage(`{"datadog.yaml": {"log_level": "error"}}`),
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
		ID:      "config-2",
		Configs: json.RawMessage(`{"datadog.yaml": {"log_level": "debug"}}`),
	}
	s.executeConfigGoldenPath(config2)
}

func (s *upgradeScenarioSuite) TestUpgradeConfigFailure() {
	s.RunInstallScript(
		"DD_REMOTE_UPDATES=true",
		"DD_REMOTE_POLICIES=true",
	)
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-process.service",
		"datadog-installer.service",
	)
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")

	// Non alphanumerical characters are not allowed, the agent should crash
	config := installerConfig{
		ID:      "config",
		Configs: json.RawMessage(`{"datadog.yaml": {"secret_backend_command": "echo", "log_level": "ENC[hi]"}}`),
	}
	timestamp := s.host.LastJournaldTimestamp()
	_, err := s.startConfigExperiment(datadogAgent, config)
	s.T().Logf("Error: %s", s.Env().RemoteHost.MustExecute("cat /tmp/start_config_experiment.log"))
	require.NoError(s.T(), err)

	// Assert experiment is stopped as the agent should've crashed
	s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().
		Unordered(host.SystemdEvents(). // Stable stops
						Stopped(agentUnit).
						Stopped(processUnit).
						Stopped(traceUnit),
		).
		Unordered(host.SystemdEvents(). // Experiment starts
						Starting(agentUnitXP).
						Started(processUnitXP).
						Started(traceUnitXP),
		).
		Unordered(host.SystemdEvents(). // Experiment fails
						Failed(agentUnitXP).
						Stopped(processUnitXP).
						Stopped(traceUnitXP),
		).
		Started(agentUnit). // Stable restarts
		Unordered(host.SystemdEvents().
			Started(traceUnit).
			Started(processUnit),
		),
	)

	s.mustStopExperiment(datadogAgent)
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
	s.Env().RemoteHost.MustExecute(`sudo systemctl restart datadog-agent.service datadog-installer.service`)

	// Set catalog
	s.host.WaitForUnitActive(
		"datadog-agent.service",
		"datadog-installer.service",
	)
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")
	s.setCatalog(testCatalog)

	// Set host proxy setup
	defer s.host.RemoveProxy()
	s.host.SetupProxy()

	s.executeAgentGoldenPath()
}

func (s *upgradeScenarioSuite) startExperiment(pkg packageName, version string) (string, error) {
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")
	cmd := fmt.Sprintf("sudo datadog-installer daemon start-experiment %s %s > /tmp/start_experiment.log 2>&1", pkg, version)
	s.T().Logf("Running start command: %s", cmd)
	return s.Env().RemoteHost.Execute(cmd)
}

func (s *upgradeScenarioSuite) mustStartExperiment(pkg packageName, version string) string {
	output, err := s.startExperiment(pkg, version)
	require.NoError(s.T(), err, "Failed to start experiment: v%s\ndatadog-installer journalctl:\n%s\ndatadog-installer-exp journalctl:\n%s",
		s.Env().RemoteHost.MustExecute("cat /tmp/start_experiment.log"),
		s.Env().RemoteHost.MustExecute("sudo journalctl -xeu datadog-installer --no-pager"),
		s.Env().RemoteHost.MustExecute("sudo journalctl -xeu datadog-installer-exp --no-pager"),
	)
	return output
}

func (s *upgradeScenarioSuite) promoteExperiment(pkg packageName) (string, error) {
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")
	cmd := fmt.Sprintf("sudo datadog-installer daemon promote-experiment %s > /tmp/promote_experiment.log 2>&1", pkg)
	s.T().Logf("Running promote command: %s", cmd)
	return s.Env().RemoteHost.Execute(cmd)
}

func (s *upgradeScenarioSuite) mustPromoteExperiment(pkg packageName) string {
	output, err := s.promoteExperiment(pkg)
	require.NoError(s.T(), err, "Failed to promote experiment: %s\ndatadog-installer journalctl:\n%s\ndatadog-installer-exp journalctl:\n%s",
		s.Env().RemoteHost.MustExecute("cat /tmp/promote_experiment.log"),
		s.Env().RemoteHost.MustExecute("sudo journalctl -xeu datadog-installer --no-pager"),
		s.Env().RemoteHost.MustExecute("sudo journalctl -xeu datadog-installer-exp --no-pager"),
	)
	return output
}

func (s *upgradeScenarioSuite) stopExperiment(pkg packageName) (string, error) {
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")
	cmd := fmt.Sprintf("sudo datadog-installer daemon stop-experiment %s > /tmp/stop_experiment.log 2>&1", pkg)
	s.T().Logf("Running stop command: %s", cmd)
	return s.Env().RemoteHost.Execute(cmd)
}

func (s *upgradeScenarioSuite) mustStopExperiment(pkg packageName) string {
	output, err := s.stopExperiment(pkg)
	require.NoError(s.T(), err, "Failed to stop experiment: %s\ndatadog-installer journalctl:\n%s\ndatadog-installer-exp journalctl:\n%s",
		s.Env().RemoteHost.MustExecute("cat /tmp/stop_experiment.log"),
		s.Env().RemoteHost.MustExecute("sudo journalctl -xeu datadog-installer --no-pager"),
		s.Env().RemoteHost.MustExecute("sudo journalctl -xeu datadog-installer-exp --no-pager"),
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
	s.host.WaitForUnitActivating(s.T(), agentUnitXP)
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
	require.Equal(s.T(), version, installerStatus.Packages["datadog-agent"].Version.Experiment)
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
	require.Equal(s.T(), version, installerStatus.Packages["datadog-agent"].Version.Stable)
	require.Equal(s.T(), "", installerStatus.Packages["datadog-agent"].Version.Experiment)
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
	require.Equal(s.T(), "", installerStatus.Packages["datadog-agent"].Version.Experiment)
}

func (s *upgradeScenarioSuite) startConfigExperiment(pkg packageName, config installerConfig) (string, error) {
	rawConfig, err := json.Marshal(config.Configs)
	require.NoError(s.T(), err)
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")
	cmd := fmt.Sprintf("sudo -E datadog-installer install-config-experiment %s %s '%s' > /tmp/start_config_experiment.log 2>&1", pkg, config.ID, rawConfig)
	s.T().Logf("Running start command: %s", cmd)
	return s.Env().RemoteHost.Execute(cmd, client.WithEnvVariables(map[string]string{"DD_REMOTE_POLICIES": "true"}))
}

func (s *upgradeScenarioSuite) mustStartConfigExperiment(pkg packageName, config installerConfig) string {
	output, err := s.startConfigExperiment(pkg, config)
	require.NoError(s.T(), err, "Failed to start config experiment: %s\ndatadog-installer journalctl:\n%s\ndatadog-installer-exp journalctl:\n%s",
		s.Env().RemoteHost.MustExecute("cat /tmp/start_config_experiment.log"),
		s.Env().RemoteHost.MustExecute("sudo journalctl -xeu datadog-installer --no-pager"),
		s.Env().RemoteHost.MustExecute("sudo journalctl -xeu datadog-installer-exp --no-pager"),
	)
	return output
}

func (s *upgradeScenarioSuite) promoteConfigExperiment(pkg packageName) (string, error) {
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")
	cmd := fmt.Sprintf("sudo -E datadog-installer promote-config-experiment %s > /tmp/promote_config_experiment.log 2>&1", pkg)
	s.T().Logf("Running promote command: %s", cmd)
	return s.Env().RemoteHost.Execute(cmd, client.WithEnvVariables(map[string]string{"DD_REMOTE_POLICIES": "true"}))
}

func (s *upgradeScenarioSuite) mustPromoteConfigExperiment(pkg packageName) string {
	output, err := s.promoteConfigExperiment(pkg)
	require.NoError(s.T(), err, "Failed to promote config experiment: %s\ndatadog-installer journalctl:\n%s\ndatadog-installer-exp journalctl:\n%s",
		s.Env().RemoteHost.MustExecute("cat /tmp/promote_config_experiment.log"),
		s.Env().RemoteHost.MustExecute("sudo journalctl -xeu datadog-installer --no-pager"),
		s.Env().RemoteHost.MustExecute("sudo journalctl -xeu datadog-installer-exp --no-pager"),
	)
	return output
}

func (s *upgradeScenarioSuite) stopConfigExperiment(pkg packageName) (string, error) {
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")
	cmd := fmt.Sprintf("sudo -E datadog-installer remove-config-experiment %s > /tmp/stop_config_experiment.log 2>&1", pkg)
	s.T().Logf("Running stop command: %s", cmd)
	return s.Env().RemoteHost.Execute(cmd, client.WithEnvVariables(map[string]string{"DD_REMOTE_POLICIES": "true"}))
}

func (s *upgradeScenarioSuite) mustStopConfigExperiment(pkg packageName) string {
	output, err := s.stopConfigExperiment(pkg)
	require.NoError(s.T(), err, "Failed to stop experiment: %s\ndatadog-installer journalctl:\n%s\ndatadog-installer-exp journalctl:\n%s",
		s.Env().RemoteHost.MustExecute("cat /tmp/stop_config_experiment.log"),
		s.Env().RemoteHost.MustExecute("sudo journalctl -xeu datadog-installer --no-pager"),
		s.Env().RemoteHost.MustExecute("sudo journalctl -xeu datadog-installer-exp --no-pager"),
	)
	return output
}

func (s *upgradeScenarioSuite) assertSuccessfulConfigStartExperiment(timestamp host.JournaldTimestamp, version string) {
	s.host.WaitForUnitActivating(s.T(), agentUnitXP)
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

	state := s.host.State()
	state.AssertSymlinkExists("/etc/datadog-agent/managed/datadog-agent/experiment", fmt.Sprintf("/etc/datadog-agent/managed/datadog-agent/%s", version), "root", "root")
}

func (s *upgradeScenarioSuite) assertSuccessfulConfigPromoteExperiment(timestamp host.JournaldTimestamp, version string) {
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

	state := s.host.State()
	state.AssertSymlinkExists("/etc/datadog-agent/managed/datadog-agent/stable", fmt.Sprintf("/etc/datadog-agent/managed/datadog-agent/%s", version), "root", "root")
	state.AssertSymlinkExists("/etc/datadog-agent/managed/datadog-agent/experiment", fmt.Sprintf("/etc/datadog-agent/managed/datadog-agent/%s", version), "root", "root")
}

func (s *upgradeScenarioSuite) assertSuccessfulConfigStopExperiment(timestamp host.JournaldTimestamp) {
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

	state := s.host.State()
	state.AssertSymlinkExists("/etc/datadog-agent/managed/datadog-agent/experiment", "/etc/datadog-agent/managed/datadog-agent/stable", "root", "root")
}

func (s *upgradeScenarioSuite) getInstallerStatus() installerStatus {
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
		s.Env().RemoteHost.MustExecute("sudo journalctl -xeu datadog-installer"),
		s.Env().RemoteHost.MustExecute("sudo journalctl -xeu datadog-installer-exp"),
	)

	var status installerStatus
	err := json.Unmarshal([]byte(response), &status)
	if err != nil {
		s.T().Fatal(err)
	}
	// Legacy status handling
	for k, pkg := range status.Packages {
		if pkg.LegacyVersionStable != "" || pkg.LegacyVersionExperiment != "" {
			pkg.Version.Stable = pkg.LegacyVersionStable
			pkg.Version.Experiment = pkg.LegacyVersionExperiment
			status.Packages[k] = pkg
		}
	}
	return status
}

func (s *upgradeScenarioSuite) assertSuccessfulInstallerStartExperiment(timestamp host.JournaldTimestamp, version string) {
	s.host.WaitForUnitActivating(s.T(), installerUnitXP)
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
	require.Equal(s.T(), version, installerStatus.Packages["datadog-installer"].Version.Experiment)
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
	require.Equal(s.T(), "", installerStatus.Packages["datadog-installer"].Version.Experiment)
}

func (s *upgradeScenarioSuite) assertSuccessfulInstallerPromoteExperiment(timestamp host.JournaldTimestamp, version string) {
	s.host.WaitForUnitActive(installerUnit)

	// Assert experiment is running
	s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().
		Stopped(installerUnitXP).
		Started(installerUnit),
	)

	installerStatus := s.getInstallerStatus()
	require.Equal(s.T(), version, installerStatus.Packages["datadog-installer"].Version.Stable)
	require.Equal(s.T(), "", installerStatus.Packages["datadog-installer"].Version.Experiment)
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

func (s *upgradeScenarioSuite) executeConfigGoldenPath(config installerConfig) {
	timestamp := s.host.LastJournaldTimestamp()

	s.mustStartConfigExperiment(datadogAgent, config)
	s.assertSuccessfulConfigStartExperiment(timestamp, config.ID)

	timestamp = s.host.LastJournaldTimestamp()
	s.mustPromoteConfigExperiment(datadogAgent)
	s.assertSuccessfulConfigPromoteExperiment(timestamp, config.ID)
}
