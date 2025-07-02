// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"

	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/host"
)

const (
	agentUnit      = "datadog-agent.service"
	agentUnitXP    = "datadog-agent-exp.service"
	traceUnit      = "datadog-agent-trace.service"
	traceUnitXP    = "datadog-agent-trace-exp.service"
	processUnit    = "datadog-agent-process.service"
	processUnitXP  = "datadog-agent-process-exp.service"
	probeUnit      = "datadog-agent-sysprobe.service"
	probeUnitXP    = "datadog-agent-sysprobe-exp.service"
	securityUnit   = "datadog-agent-security.service"
	securityUnitXP = "datadog-agent-security-exp.service"
)

type packageAgentSuite struct {
	packageBaseSuite
}

func testAgent(os e2eos.Descriptor, arch e2eos.Architecture, method InstallMethodOption) packageSuite {
	return &packageAgentSuite{
		packageBaseSuite: newPackageSuite("agent", os, arch, method, awshost.WithoutFakeIntake()),
	}
}

func (s *packageAgentSuite) TestInstall() {
	s.RunInstallScript("DD_REMOTE_UPDATES=true", envForceInstall("datadog-agent"))
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(s.T(), agentUnit, traceUnit)

	state := s.host.State()
	s.assertUnits(state, false)

	state.AssertFileExistsAnyUser("/etc/datadog-agent/install_info", 0644)
	state.AssertFileExists("/etc/datadog-agent/datadog.yaml", 0640, "dd-agent", "dd-agent")

	agentVersion := s.host.AgentStableVersion()
	agentDir := fmt.Sprintf("/opt/datadog-packages/datadog-agent/%s", agentVersion)

	state.AssertDirExists(agentDir, 0755, "dd-agent", "dd-agent")

	state.AssertFileExists(path.Join(agentDir, "embedded/bin/system-probe"), 0755, "root", "root")
	state.AssertFileExists(path.Join(agentDir, "embedded/bin/security-agent"), 0755, "root", "root")
	state.AssertDirExists(path.Join(agentDir, "embedded/share/system-probe/ebpf"), 0755, "root", "root")
	state.AssertFileExists(path.Join(agentDir, "embedded/share/system-probe/ebpf/dns.o"), 0644, "root", "root")

	state.AssertSymlinkExists("/opt/datadog-packages/datadog-agent/stable", agentDir, "root", "root")
	state.AssertSymlinkExists("/usr/bin/datadog-agent", "/opt/datadog-packages/datadog-agent/stable/bin/agent/agent", "root", "root")
	state.AssertSymlinkExists("/usr/bin/datadog-installer", "/opt/datadog-packages/datadog-agent/stable/embedded/bin/installer", "root", "root")
	state.AssertFileExistsAnyUser("/etc/datadog-agent/install.json", 0644)
}

func (s *packageAgentSuite) assertUnits(state host.State, oldUnits bool) {
	state.AssertUnitsLoaded(agentUnit, traceUnit, processUnit, probeUnit, securityUnit)
	state.AssertUnitsEnabled(agentUnit)
	state.AssertUnitsRunning(agentUnit, traceUnit) //cannot assert process-agent because it may be running or dead based on timing
	state.AssertUnitsDead(probeUnit, securityUnit)

	systemdPath := "/etc/systemd/system"
	if oldUnits {
		pkgManager := s.host.GetPkgManager()
		switch pkgManager {
		case "apt":
			if s.os.Flavor == e2eos.Ubuntu {
				// Ubuntu 24.04 moved to a new systemd path
				systemdPath = "/usr/lib/systemd/system"
			} else {
				systemdPath = "/lib/systemd/system"
			}
		case "yum", "zypper":
			systemdPath = "/usr/lib/systemd/system"
		default:
			s.T().Fatalf("unsupported package manager: %s", pkgManager)
		}
	}

	for _, unit := range []string{agentUnit, traceUnit, processUnit, probeUnit, securityUnit} {
		s.host.AssertUnitProperty(unit, "FragmentPath", filepath.Join(systemdPath, unit))
	}
}

// TestUpgrade_AgentDebRPM_to_OCI tests the upgrade from DEB/RPM agent to the OCI one.
func (s *packageAgentSuite) TestUpgrade_AgentDebRPM_to_OCI() {
	// install deb/rpm agent
	s.RunInstallScript(envForceNoInstall("datadog-agent"))
	s.host.AssertPackageInstalledByPackageManager("datadog-agent")

	defer s.Purge()
	defer s.purgeAgentDebInstall()

	state := s.host.State()
	s.assertUnits(state, true)
	state.AssertDirExists("/opt/datadog-agent", 0755, "dd-agent", "dd-agent")

	// install OCI agent
	s.RunInstallScript("DD_REMOTE_UPDATES=true", envForceInstall("datadog-agent"))

	state = s.host.State()
	s.assertUnits(state, false)
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.AssertPackageNotInstalledByPackageManager("datadog-agent")
}

// TestUpgrade_Agent_OCI_then_DebRpm agent deb/rpm install while OCI one is installed
func (s *packageAgentSuite) TestUpgrade_Agent_OCI_then_DebRpm() {
	// install OCI agent
	s.RunInstallScript("DD_REMOTE_UPDATES=true", envForceInstall("datadog-agent"))
	defer s.Purge()

	state := s.host.State()
	s.assertUnits(state, false)
	state.AssertPathDoesNotExist("/opt/datadog-agent")

	// is_installed avoids a re-install of datadog-agent with the install script
	s.RunInstallScript(envForceNoInstall("datadog-agent"))
	state.AssertPathDoesNotExist("/opt/datadog-agent")

	// install deb/rpm manually
	s.installDebRPMAgent()
	defer s.purgeAgentDebInstall()
	s.host.AssertPackageInstalledByPackageManager("datadog-agent")

	state = s.host.State()
	s.assertUnits(state, true)
	state.AssertDirExists("/opt/datadog-agent", 0755, "dd-agent", "dd-agent")
	s.host.AssertPackageNotInstalledByInstaller("datadog-agent")
}

func (s *packageAgentSuite) TestExperimentTimeout() {
	s.RunInstallScript("DD_REMOTE_UPDATES=true", envForceInstall("datadog-agent"))
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(s.T(), "datadog-agent.service", "datadog-agent-trace.service")

	s.host.SetupFakeAgentExp().
		SetStopWithSigtermExit0("core-agent").
		SetStopWithSigtermExit0("trace-agent")

	// shorten timeout for tests
	s.host.Run(`sudo sed -i 's/3000s/15s/' /etc/systemd/system/datadog-agent-exp.service`)
	s.host.Run(`sudo systemctl daemon-reload`)

	timestamp := s.host.LastJournaldTimestamp()
	s.host.Run(`sudo systemctl start datadog-agent-exp --no-block`)

	s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().
		// first stop agent dependency
		Unordered(host.SystemdEvents().
			Stopped(traceUnit),
		).
		// then agent
		Stopping(agentUnit).
		Stopped(agentUnit).

		// start experiment dependency
		Unordered(host.SystemdEvents().
			Started(agentUnitXP).
			Started(traceUnitXP).
			SkippedIf(probeUnitXP, s.installMethod != InstallMethodAnsible).
			Skipped(securityUnitXP),
		).

		// timeout
		Unordered(host.SystemdEvents().
			Failed(agentUnitXP).
			Stopped(traceUnitXP),
		).

		// start stable agents
		Started(agentUnit).
		Unordered(host.SystemdEvents().
			Started(traceUnit).
			SkippedIf(probeUnit, s.installMethod != InstallMethodAnsible).
			Skipped(securityUnit),
		),
	)
}

func (s *packageAgentSuite) TestExperimentIgnoringSigterm() {
	s.RunInstallScript("DD_REMOTE_UPDATES=true", envForceInstall("datadog-agent"))
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(s.T(), "datadog-agent.service", "datadog-agent-trace.service")

	s.host.SetupFakeAgentExp().
		SetStopWithSigkill("core-agent").
		SetStopWithSigkill("trace-agent")

	defer func() { s.host.Run("sudo rm -rf /etc/systemd/system/datadog*.d/override.conf") }()

	for _, unit := range []string{traceUnitXP, agentUnitXP} {
		s.T().Logf("Testing timeoutStop of unit %s", unit)
		s.host.Run(fmt.Sprintf("sudo rm -rf /etc/systemd/system/%s.d/override.conf", unit))
		s.host.AssertUnitProperty(unit, "TimeoutStopUSec", "1min 30s")
		s.host.Run(fmt.Sprintf("sudo mkdir -p /etc/systemd/system/%s.d/", unit))
		s.host.Run(fmt.Sprintf(`echo -e "[Service]\nTimeoutStopSec=1" | sudo tee /etc/systemd/system/%s.d/override.conf > /dev/null`, unit))
		if unit == agentUnitXP {
			// Override the timeout for the agent unit
			s.host.Run(`sudo sed -i 's/3000s/5s/' /etc/systemd/system/datadog-agent-exp.service`)
		}
		s.host.Run(`sudo systemctl daemon-reload`)
		s.host.AssertUnitProperty(unit, "TimeoutStopUSec", "1s")
	}

	timestamp := s.host.LastJournaldTimestamp()
	s.host.Run(`sudo systemctl start datadog-agent-exp --no-block`)

	s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().
		// first stop agent dependency
		Unordered(host.SystemdEvents().
			Stopped(traceUnit),
		).
		// then agent
		Stopping(agentUnit).
		Stopped(agentUnit).

		// start experiment dependency
		Unordered(host.SystemdEvents().
			Started(agentUnitXP).
			Started(traceUnitXP).
			SkippedIf(probeUnitXP, s.installMethod != InstallMethodAnsible).
			Skipped(securityUnitXP),
		).

		// timeout
		Unordered(host.SystemdEvents().
			// No agent XP sigterm because the timeout is done with /usr/bin/timeout
			SigtermTimed(traceUnitXP).
			Sigkill(agentUnitXP).
			Sigkill(traceUnitXP).
			Failed(agentUnitXP).
			Stopped(traceUnitXP),
		).

		// start stable agents
		Started(agentUnit).
		Unordered(host.SystemdEvents().
			Started(traceUnit).
			SkippedIf(probeUnit, s.installMethod != InstallMethodAnsible).
			Skipped(securityUnit),
		),
	)
}

func (s *packageAgentSuite) TestExperimentExits() {
	s.RunInstallScript("DD_REMOTE_UPDATES=true", envForceInstall("datadog-agent"))
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(s.T(), "datadog-agent.service", "datadog-agent-trace.service")

	xpAgent := s.host.SetupFakeAgentExp()

	for _, exitProgram := range []string{"exit0", "exit1"} {
		s.T().Logf("Testing exit code of %s", exitProgram)
		if exitProgram == "exit0" {
			xpAgent.SetExit0("core-agent")
		} else {
			xpAgent.SetExit1("core-agent")
		}

		timestamp := s.host.LastJournaldTimestamp()
		s.host.Run(`sudo systemctl start datadog-agent-exp --no-block`)

		s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().
			// first stop agent dependency
			Unordered(host.SystemdEvents().
				Stopped(traceUnit),
			).
			// then agent
			Stopping(agentUnit).
			Stopped(agentUnit).

			// start experiment dependency
			Unordered(host.SystemdEvents().
				Started(agentUnitXP).
				Started(traceUnitXP).
				SkippedIf(probeUnitXP, s.installMethod != InstallMethodAnsible).
				Skipped(securityUnitXP),
			).

			// failed agent XP unit
			Unordered(host.SystemdEvents().
				Failed(agentUnitXP).
				Stopped(traceUnitXP),
			).

			// start stable agents
			Started(agentUnit).
			Unordered(host.SystemdEvents().
				Started(traceUnit).
				SkippedIf(probeUnit, s.installMethod != InstallMethodAnsible).
				Skipped(securityUnit),
			),
		)
	}
}

func (s *packageAgentSuite) TestExperimentStopped() {
	s.RunInstallScript("DD_REMOTE_UPDATES=true", envForceInstall("datadog-agent"))
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(s.T(), "datadog-agent.service", "datadog-agent-trace.service")

	s.host.SetupFakeAgentExp()

	for _, stopCommand := range []string{"start datadog-agent", "stop datadog-agent-exp"} {
		s.T().Logf("Testing stop experiment command %s", stopCommand)
		timestamp := s.host.LastJournaldTimestamp()
		s.host.Run(`sudo systemctl start datadog-agent-exp --no-block`)

		// ensure experiment is running
		s.host.WaitForUnitActive(s.T(),
			"datadog-agent-trace-exp.service",
		)
		s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().Started(traceUnitXP))
		s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().Skipped(securityUnitXP))
		s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().SkippedIf(probeUnitXP, s.installMethod != InstallMethodAnsible))

		// stop experiment
		timestamp = s.host.LastJournaldTimestamp()
		s.host.Run(fmt.Sprintf(`sudo systemctl %s`, stopCommand))

		s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().
			// stop order
			Unordered(host.SystemdEvents().
				Stopped(agentUnitXP).
				Stopped(traceUnitXP),
			).

			// start stable agents
			Started(agentUnit).
			Unordered(host.SystemdEvents().
				Started(traceUnit).
				SkippedIf(probeUnit, s.installMethod != InstallMethodAnsible).
				Skipped(securityUnit),
			),
		)
	}
}

func (s *packageAgentSuite) TestRunPath() {
	s.RunInstallScript("DD_REMOTE_UPDATES=true", envForceInstall("datadog-agent"))
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(s.T(), "datadog-agent.service", "datadog-agent-trace.service")

	var rawConfig string
	var err error
	assert.Eventually(s.T(), func() bool {
		rawConfig, err = s.host.AgentRuntimeConfig()
		return err == nil
	}, 30*time.Second, 5*time.Second, "failed to get agent runtime config: %v", err)
	var config map[string]interface{}
	err = yaml.Unmarshal([]byte(rawConfig), &config)
	assert.NoError(s.T(), err)
	runPath, ok := config["run_path"].(string)
	assert.True(s.T(), ok, "run_path not found in runtime config")
	assert.True(s.T(), strings.HasPrefix(runPath, "/opt/datadog-packages/datadog-agent/"), "run_path is not in the expected location: %s", runPath)
}

func (s *packageAgentSuite) TestUpgrade_DisabledAgentDebRPM_to_OCI() {
	// install deb/rpm agent
	s.RunInstallScript(envForceNoInstall("datadog-agent"))
	s.host.AssertPackageInstalledByPackageManager("datadog-agent")

	defer s.Purge()
	defer s.purgeAgentDebInstall()

	state := s.host.State()
	s.assertUnits(state, true)
	state.AssertDirExists("/opt/datadog-agent", 0755, "dd-agent", "dd-agent")

	// disable the unit
	s.host.Run("sudo systemctl disable datadog-agent")

	// install OCI agent
	s.RunInstallScript("DD_REMOTE_UPDATES=true", envForceInstall("datadog-agent"))

	state = s.host.State()
	s.assertUnits(state, false)
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.AssertPackageNotInstalledByPackageManager("datadog-agent")

	s.host.Run("sudo systemctl show datadog-agent -p ExecStart | grep /opt/datadog-packages")
}

func (s *packageAgentSuite) TestInstallWithLeftoverDebDir() {
	// create /opt/datadog-agent to simulate a disabled agent
	s.host.Run("sudo mkdir -p /opt/datadog-agent")
	defer func() { s.host.Run("sudo rm -rf /opt/datadog-agent") }()

	// install OCI agent
	s.RunInstallScript("DD_REMOTE_UPDATES=true", envForceInstall("datadog-agent"))

	state := s.host.State()
	s.assertUnits(state, false)
	s.host.Run("sudo systemctl show datadog-agent -p ExecStart | grep /opt/datadog-packages")
}

func (s *packageAgentSuite) TestInstallWithGroupPreviouslyCreated() {
	s.host.Run("sudo userdel dd-agent || true")
	s.host.Run("sudo groupdel dd-agent || true")
	s.host.Run("sudo groupadd --system datadog")

	s.RunInstallScript("DD_REMOTE_UPDATES=true", envForceInstall("datadog-agent"))
	defer s.Purge()

	assert.True(s.T(), s.host.UserExists("dd-agent"), "dd-agent user should exist")
	assert.True(s.T(), s.host.GroupExists("dd-agent"), "dd-agent group should exist")
}

func (s *packageAgentSuite) purgeAgentDebInstall() {
	pkgManager := s.host.GetPkgManager()
	switch pkgManager {
	case "apt":
		s.Env().RemoteHost.Execute("sudo apt-get remove -y --purge datadog-agent")
	case "yum":
		s.Env().RemoteHost.Execute("sudo yum remove -y datadog-agent")
	case "zypper":
		s.Env().RemoteHost.Execute("sudo zypper remove -y datadog-agent")
	default:
		s.T().Fatalf("unsupported package manager: %s", pkgManager)
	}
	// Make sure everything is cleaned up -- there are tests where the package is
	// removed but not purged so the directory remains
	s.Env().RemoteHost.Execute("sudo rm -rf /opt/datadog-agent")
}

func (s *packageAgentSuite) installDebRPMAgent() {
	pkgManager := s.host.GetPkgManager()
	switch pkgManager {
	case "apt":
		s.Env().RemoteHost.Execute("sudo apt-get install -y --force-yes datadog-agent")
	case "yum":
		s.Env().RemoteHost.Execute("sudo yum -y install --disablerepo=* --enablerepo=datadog datadog-agent")
	case "zypper":
		s.Env().RemoteHost.Execute("sudo zypper install -y datadog-agent")
	default:
		s.T().Fatalf("unsupported package manager: %s", pkgManager)
	}

}
