// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/host"
)

type packageName string

const (
	datadogAgent     packageName = "datadog-agent"
	datadogApmInject packageName = "datadog-apm-inject"
)

const (
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

const (
	unknownAgentImageVersion = "7.52.1-1"
)

func testUpgradeScenario(os e2eos.Descriptor, arch e2eos.Architecture, method InstallMethodOption) packageSuite {
	return &upgradeScenarioSuite{
		packageBaseSuite: newPackageSuite("upgrade_scenario", os, arch, method),
	}
}

func (s *upgradeScenarioSuite) testCatalog() catalog {
	return catalog{
		Packages: []packageEntry{
			{
				Package: string(datadogAgent),
				Version: s.pipelineAgentVersion,
				URL:     "oci://installtesting.datad0g.com.internal.dda-testing.com/agent-package:pipeline-" + os.Getenv("E2E_PIPELINE_ID"),
			},
			{
				Package: string(datadogApmInject),
				Version: apmInjectVersion,
				URL:     "oci://dd-agent.s3.amazonaws.com/apm-inject-package:latest",
			},
		},
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

	s.setCatalog(s.testCatalog())
	s.executeAgentGoldenPath()
}

func (s *upgradeScenarioSuite) TestUpgradeSuccessfulFromDebRPM() {
	s.RunInstallScript()
	defer s.Purge()
	s.host.AssertPackageInstalledByPackageManager("datadog-agent")
	currentVersion := s.getInstallerStatus().Packages.States["datadog-agent"].Stable
	// Assert stable symlink exists properly
	state := s.host.State()
	state.AssertSymlinkExists("/opt/datadog-packages/datadog-agent/stable", "/opt/datadog-packages/run/datadog-agent/"+currentVersion, "root", "root")
	state.AssertSymlinkExists("/opt/datadog-packages/run/datadog-agent/"+currentVersion, "/opt/datadog-agent", "root", "root")

	// Set remote_updates to true in datadog.yaml
	s.Env().RemoteHost.MustExecute(`printf "\nremote_updates: true\n" | sudo tee -a /etc/datadog-agent/datadog.yaml`)
	s.Env().RemoteHost.MustExecute(`sudo systemctl restart datadog-agent`)

	s.host.WaitForUnitActive(s.T(),
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-installer.service",
	)
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")

	s.setCatalog(s.testCatalog())

	timestamp := s.host.LastJournaldTimestamp()
	s.startExperiment(datadogAgent, s.pipelineAgentVersion)
	s.assertSuccessfulAgentStartExperiment(timestamp, s.pipelineAgentVersion)

	// Assert stable symlink still exists properly
	state = s.host.State()
	state.AssertSymlinkExists("/opt/datadog-packages/datadog-agent/stable", "/opt/datadog-packages/run/datadog-agent/"+currentVersion, "root", "root")
	state.AssertSymlinkExists("/opt/datadog-packages/run/datadog-agent/"+currentVersion, "/opt/datadog-agent", "root", "root")

	timestamp = s.host.LastJournaldTimestamp()
	s.promoteExperiment(datadogAgent)
	s.assertSuccessfulAgentPromoteExperiment(timestamp, s.pipelineAgentVersion)
	state = s.host.State()
	state.AssertPathDoesNotExist("/opt/datadog-agent")
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

	s.setCatalog(s.testCatalog())

	timestamp := s.host.LastJournaldTimestamp()
	s.startExperiment(datadogAgent, s.pipelineAgentVersion)
	s.assertSuccessfulAgentStartExperiment(timestamp, s.pipelineAgentVersion)

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

	s.setCatalog(s.testCatalog())

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
	s.setCatalog(s.testCatalog())

	currentVersion := s.getInstallerStatus().Packages.States["datadog-agent"].Stable
	newCatalog := catalog{
		Packages: []packageEntry{
			{
				Package: "datadog-agent",
				Version: currentVersion,
				URL:     "oci://dd-agent.s3.amazonaws.com/agent-package:" + currentVersion,
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

	s.setCatalog(s.testCatalog())
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

	s.setCatalog(s.testCatalog())

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
	s.setCatalog(s.testCatalog())

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

	s.setCatalog(s.testCatalog())
	s.mustInstallPackage(datadogApmInject, apmInjectVersion)
	s.host.AssertPackageInstalledByInstaller("datadog-apm-inject")

	s.mustRemovePackage(datadogApmInject)
	state := s.host.State()
	state.AssertPathDoesNotExist("/opt/datadog-packages/datadog-apm-inject")

}

func (s *upgradeScenarioSuite) TestUpgradeAndUninstall() {
	// 1. Install with remote updates enabled
	s.RunInstallScript("DD_REMOTE_UPDATES=true")
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(s.T(),
		"datadog-agent.service",
		"datadog-agent-trace.service",
		"datadog-agent-installer.service",
	)
	s.host.WaitForFileExists(true, "/opt/datadog-packages/run/installer.sock")

	// 2. Perform remote upgrade (experiment + promote)
	s.setCatalog(s.testCatalog())
	s.executeAgentGoldenPath()

	// 3. Uninstall/purge the agent and installer
	pkgManager := s.host.GetPkgManager()
	if pkgManager == "apt" {
		// Use purge for Debian-based systems
		s.Env().RemoteHost.MustExecute("sudo apt-get purge -y --purge datadog-agent")
	} else if pkgManager == "yum" {
		// Use remove for RedHat-based systems
		s.Env().RemoteHost.MustExecute("sudo yum remove -y datadog-agent")
	} else if pkgManager == "zypper" {
		// Use remove for SUSE-based systems
		s.Env().RemoteHost.MustExecute("sudo zypper remove -y datadog-agent")
	}

	// 4. Verify no leftover files exist
	state := s.host.State()
	state.AssertPathDoesNotExist("/opt/datadog-agent")

	// Check that /opt/datadog-packages is either fully removed or only contains packages.db
	packagesDbPath := "/opt/datadog-packages/packages.db"
	datadogPackagesPath := "/opt/datadog-packages"

	// Get the contents of /opt/datadog-packages if it exists
	output, err := s.Env().RemoteHost.Execute("sudo ls -la " + datadogPackagesPath + " 2>/dev/null || echo 'DIR_DOES_NOT_EXIST'")
	if err == nil && !strings.Contains(output, "DIR_DOES_NOT_EXIST") {
		// Directory exists, check its contents
		files, err := s.Env().RemoteHost.Execute("sudo find " + datadogPackagesPath + " -mindepth 1 -not -path " + packagesDbPath)
		require.NoError(s.T(), err, "Failed to list files in /opt/datadog-packages")

		// Only packages.db should remain (if anything)
		filesOutput := strings.TrimSpace(files)
		if filesOutput != "" {
			s.T().Errorf("Unexpected leftover files in /opt/datadog-packages:\n%s", filesOutput)
		}
	}
	// If directory doesn't exist at all, that's fine too
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
	s.startExperiment(datadogAgent, s.pipelineAgentVersion)
	s.assertSuccessfulAgentStartExperiment(timestamp, s.pipelineAgentVersion)

	timestamp = s.host.LastJournaldTimestamp()
	s.promoteExperiment(datadogAgent)
	s.assertSuccessfulAgentPromoteExperiment(timestamp, s.pipelineAgentVersion)
}
