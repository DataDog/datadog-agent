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

	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/host"
	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
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
	s.RunInstallScript(envForceInstall("datadog-agent"))
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(agentUnit, traceUnit, processUnit)

	state := s.host.State()
	s.assertUnits(state, false)

	state.AssertFileExistsAnyUser("/etc/datadog-agent/install_info", 0644)
	state.AssertFileExists("/etc/datadog-agent/datadog.yaml", 0640, "dd-agent", "dd-agent")

	agentVersion := s.host.AgentStableVersion()
	agentDir := fmt.Sprintf("/opt/datadog-packages/datadog-agent/%s", agentVersion)

	state.AssertDirExists(agentDir, 0755, "dd-agent", "dd-agent")

	state.AssertFileExists(path.Join(agentDir, "embedded/bin/system-probe"), 0755, "root", "root")
	state.AssertFileExists(path.Join(agentDir, "embedded/bin/security-agent"), 0755, "root", "root")
	state.AssertDirExists(path.Join(agentDir, "embedded/share/system-probe/java"), 0755, "root", "root")
	state.AssertDirExists(path.Join(agentDir, "embedded/share/system-probe/ebpf"), 0755, "root", "root")
	state.AssertFileExists(path.Join(agentDir, "embedded/share/system-probe/ebpf/dns.o"), 0644, "root", "root")

	state.AssertSymlinkExists("/opt/datadog-packages/datadog-agent/stable", agentDir, "root", "root")
	state.AssertFileExistsAnyUser("/etc/datadog-agent/install.json", 0644)
}

func (s *packageAgentSuite) assertUnits(state host.State, oldUnits bool) {
	state.AssertUnitsLoaded(agentUnit, traceUnit, processUnit, probeUnit, securityUnit)
	state.AssertUnitsEnabled(agentUnit)
	state.AssertUnitsRunning(agentUnit, traceUnit, processUnit)
	state.AssertUnitsDead(probeUnit, securityUnit)

	systemdPath := "/etc/systemd/system"
	if oldUnits {
		pkgManager := s.host.GetPkgManager()
		switch pkgManager {
		case "apt":
			systemdPath = "/lib/systemd/system"
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
	s.RunInstallScript(envForceInstall("datadog-agent"))

	state = s.host.State()
	s.assertUnits(state, false)
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.AssertPackageInstalledByPackageManager("datadog-agent")
}

// TestUpgrade_Agent_OCI_then_DebRpm agent deb/rpm install while OCI one is installed
func (s *packageAgentSuite) TestUpgrade_Agent_OCI_then_DebRpm() {
	// install OCI agent
	s.RunInstallScript(envForceInstall("datadog-agent"))
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
	s.assertUnits(state, false)
	state.AssertDirExists("/opt/datadog-agent", 0755, "dd-agent", "dd-agent")
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
}

func (s *packageAgentSuite) TestExperimentTimeout() {
	s.RunInstallScript(envForceInstall("datadog-agent"))
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive("datadog-agent.service", "datadog-agent-trace.service", "datadog-agent-process.service")

	s.host.SetupFakeAgentExp().
		SetStopWithSigtermExit0("core-agent").
		SetStopWithSigtermExit0("process-agent").
		SetStopWithSigtermExit0("trace-agent")

	// assert timeout is already set
	s.host.AssertUnitProperty("datadog-agent-exp.service", "JobTimeoutUSec", "50min")

	// shorten timeout for tests
	s.host.Run("sudo mkdir -p /etc/systemd/system/datadog-agent-exp.service.d/")
	defer s.host.Run("sudo rm -rf /etc/systemd/system/datadog-agent-exp.service.d/")
	s.host.Run(`echo -e "[Unit]\nJobTimeoutSec=5" | sudo tee /etc/systemd/system/datadog-agent-exp.service.d/override.conf > /dev/null`)
	s.host.Run(`sudo systemctl daemon-reload`)

	s.host.AssertUnitProperty("datadog-agent-exp.service", "JobTimeoutUSec", "5s")

	timestamp := s.host.LastJournaldTimestamp()
	s.host.Run(`sudo systemctl start datadog-agent-exp --no-block`)

	s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().
		// first stop agent dependency
		Unordered(host.SystemdEvents().
			Stopped(traceUnit).
			Stopped(processUnit),
		).
		// then agent
		Stopping(agentUnit).
		Stopped(agentUnit).

		// start experiment dependency
		Unordered(host.SystemdEvents().
			Starting(agentUnitXP).
			Started(processUnitXP).
			Started(traceUnitXP).
			SkippedIf(probeUnitXP, s.installMethod != InstallMethodAnsible).
			Skipped(securityUnitXP),
		).

		// timeout
		Timed(agentUnitXP).
		Unordered(host.SystemdEvents().
			Stopped(agentUnitXP).
			Stopped(processUnitXP).
			Stopped(traceUnitXP),
		).

		// start stable agents
		Started(agentUnit).
		Unordered(host.SystemdEvents().
			Started(traceUnit).
			Started(processUnit).
			SkippedIf(probeUnitXP, s.installMethod != InstallMethodAnsible).
			Skipped(securityUnit),
		),
	)
}

func (s *packageAgentSuite) TestExperimentIgnoringSigterm() {
	s.RunInstallScript(envForceInstall("datadog-agent"))
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive("datadog-agent.service", "datadog-agent-trace.service", "datadog-agent-process.service")

	s.host.SetupFakeAgentExp().
		SetStopWithSigkill("core-agent").
		SetStopWithSigkill("process-agent").
		SetStopWithSigkill("trace-agent")

	for _, unit := range []string{traceUnitXP, processUnitXP, agentUnitXP} {
		s.T().Logf("Testing timeoutStop of unit %s", unit)
		s.host.AssertUnitProperty(unit, "TimeoutStopUSec", "1min 30s")
		s.host.Run(fmt.Sprintf("sudo mkdir -p /etc/systemd/system/%s.d/", unit))
		defer s.host.Run(fmt.Sprintf("sudo rm -rf /etc/systemd/system/%s.d/", unit))
		if unit != agentUnitXP {
			s.host.Run(fmt.Sprintf(`echo -e "[Service]\nTimeoutStopSec=1" | sudo tee /etc/systemd/system/%s.d/override.conf > /dev/null`, unit))
		} else {
			// using timeout on core agent to trigger the kill
			s.host.Run(`echo -e "[Unit]\nJobTimeoutSec=5\n[Service]\nTimeoutStopSec=1" | sudo tee /etc/systemd/system/datadog-agent-exp.service.d/override.conf > /dev/null`)
		}
		s.host.Run(`sudo systemctl daemon-reload`)
		s.host.AssertUnitProperty(unit, "TimeoutStopUSec", "1s")
	}

	timestamp := s.host.LastJournaldTimestamp()
	s.host.Run(`sudo systemctl start datadog-agent-exp --no-block`)

	s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().
		// first stop agent dependency
		Unordered(host.SystemdEvents().
			Stopped(traceUnit).
			Stopped(processUnit),
		).
		// then agent
		Stopping(agentUnit).
		Stopped(agentUnit).

		// start experiment dependency
		Unordered(host.SystemdEvents().
			Starting(agentUnitXP).
			Started(processUnitXP).
			Started(traceUnitXP).
			SkippedIf(probeUnitXP, s.installMethod != InstallMethodAnsible).
			Skipped(securityUnitXP),
		).

		// timeout
		Timed(agentUnitXP).
		Unordered(host.SystemdEvents().
			SigtermTimed(agentUnitXP).
			SigtermTimed(processUnitXP).
			SigtermTimed(traceUnitXP).
			Sigkill(agentUnitXP).
			Sigkill(processUnitXP).
			Sigkill(traceUnitXP).
			Stopped(agentUnitXP).
			Stopped(processUnitXP).
			Stopped(traceUnitXP),
		).

		// start stable agents
		Started(agentUnit).
		Unordered(host.SystemdEvents().
			Started(traceUnit).
			Started(processUnit).
			SkippedIf(probeUnitXP, s.installMethod != InstallMethodAnsible).
			Skipped(securityUnit),
		),
	)
}

func (s *packageAgentSuite) TestExperimentExits() {
	s.RunInstallScript(envForceInstall("datadog-agent"))
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive("datadog-agent.service", "datadog-agent-trace.service", "datadog-agent-process.service")

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
				Stopped(traceUnit).
				Stopped(processUnit),
			).
			// then agent
			Stopping(agentUnit).
			Stopped(agentUnit).

			// start experiment dependency
			Unordered(host.SystemdEvents().
				Starting(agentUnitXP).
				Started(processUnitXP).
				Started(traceUnitXP).
				SkippedIf(probeUnitXP, s.installMethod != InstallMethodAnsible).
				Skipped(securityUnitXP),
			).

			// failed agent XP unit
			Failed(agentUnitXP).
			Unordered(host.SystemdEvents().
				Stopped(processUnitXP).
				Stopped(traceUnitXP),
			).

			// start stable agents
			Started(agentUnit).
			Unordered(host.SystemdEvents().
				Started(traceUnit).
				Started(processUnit).
				SkippedIf(probeUnitXP, s.installMethod != InstallMethodAnsible).
				Skipped(securityUnit),
			),
		)
	}
}

func (s *packageAgentSuite) TestExperimentStopped() {
	s.RunInstallScript(envForceInstall("datadog-agent"))
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive("datadog-agent.service", "datadog-agent-trace.service", "datadog-agent-process.service")

	s.host.SetupFakeAgentExp()

	for _, stopCommand := range []string{"start datadog-agent", "stop datadog-agent-exp"} {
		s.T().Logf("Testing stop experiment command %s", stopCommand)
		timestamp := s.host.LastJournaldTimestamp()
		s.host.Run(`sudo systemctl start datadog-agent-exp --no-block`)

		// ensure experiment is running
		s.host.WaitForUnitActive(
			"datadog-agent-trace-exp.service",
			"datadog-agent-process-exp.service",
		)
		s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().Started(traceUnitXP))
		s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().Started(processUnitXP))
		s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().Skipped(securityUnitXP))
		s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().SkippedIf(probeUnitXP, s.installMethod != InstallMethodAnsible))

		// stop experiment
		timestamp = s.host.LastJournaldTimestamp()
		s.host.Run(fmt.Sprintf(`sudo systemctl %s`, stopCommand))

		s.host.AssertSystemdEvents(timestamp, host.SystemdEvents().
			// stop order
			Unordered(host.SystemdEvents().
				Stopped(agentUnitXP).
				Stopped(processUnitXP).
				Stopped(traceUnitXP),
			).

			// start stable agents
			Started(agentUnit).
			Unordered(host.SystemdEvents().
				Started(traceUnit).
				Started(processUnit).
				SkippedIf(probeUnit, s.installMethod != InstallMethodAnsible).
				Skipped(securityUnit),
			),
		)
	}
}

func (s *packageAgentSuite) TestRunPath() {
	s.RunInstallScript(envForceInstall("datadog-agent"))
	defer s.Purge()
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive("datadog-agent.service", "datadog-agent-trace.service", "datadog-agent-process.service")

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
	s.RunInstallScript(envForceInstall("datadog-agent"))

	state = s.host.State()
	s.assertUnits(state, false)
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.AssertPackageInstalledByPackageManager("datadog-agent")

	s.host.Run("sudo systemctl show datadog-agent -p ExecStart | grep /opt/datadog-packages")
}

func (s *packageAgentSuite) TestInstallWithLeftoverDebDir() {
	// create /opt/datadog-agent to simulate a disabled agent
	s.host.Run("sudo mkdir -p /opt/datadog-agent")

	// install OCI agent
	s.RunInstallScript(envForceInstall("datadog-agent"))

	state := s.host.State()
	s.assertUnits(state, false)
	s.host.Run("sudo systemctl show datadog-agent -p ExecStart | grep /opt/datadog-packages")
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
