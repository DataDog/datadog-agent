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

	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/stretchr/testify/assert"

	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/host"
)

const (
	agentUnit       = "datadog-agent.service"
	agentUnitXP     = "datadog-agent-exp.service"
	ddotUnit        = "datadog-agent-ddot.service"
	ddotUnitXP      = "datadog-agent-ddot-exp.service"
	traceUnit       = "datadog-agent-trace.service"
	traceUnitXP     = "datadog-agent-trace-exp.service"
	processUnit     = "datadog-agent-process.service"
	processUnitXP   = "datadog-agent-process-exp.service"
	probeUnit       = "datadog-agent-sysprobe.service"
	probeUnitXP     = "datadog-agent-sysprobe-exp.service"
	securityUnit    = "datadog-agent-security.service"
	securityUnitXP  = "datadog-agent-security-exp.service"
	dataPlaneUnit   = "datadog-agent-data-plane.service"
	dataPlaneUnitXP = "datadog-agent-data-plane-exp.service"
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
	s.RunInstallScript("DD_REMOTE_UPDATES=true")
	defer s.Purge()
	s.host.AssertPackageInstalledByPackageManager("datadog-agent")
	s.host.WaitForUnitActive(s.T(), agentUnit, traceUnit)
	s.host.WaitForUnitExited(s.T(), 0, processUnit, dataPlaneUnit)

	state := s.host.State()
	s.assertUnits(state, true)

	state.AssertFileExistsAnyUser("/etc/datadog-agent/install_info", 0644)
	state.AssertFileExists("/etc/datadog-agent/datadog.yaml", 0640, "dd-agent", "dd-agent")

	agentVersion := s.host.AgentStableVersion()
	agentDir := "/opt/datadog-agent"
	agentRunSymlink := fmt.Sprintf("/opt/datadog-packages/run/datadog-agent/%s", agentVersion)
	installerSymlink := path.Join(agentDir, "embedded/bin/installer")
	agentSymlink := path.Join(agentDir, "bin/agent/agent")

	state.AssertDirExists(agentDir, 0755, "dd-agent", "dd-agent")

	state.AssertFileExists(path.Join(agentDir, "embedded/bin/system-probe"), 0755, "root", "root")
	state.AssertFileExists(path.Join(agentDir, "embedded/bin/security-agent"), 0755, "root", "root")
	state.AssertDirExists(path.Join(agentDir, "embedded/share/system-probe/ebpf"), 0755, "root", "root")
	state.AssertFileExists(path.Join(agentDir, "embedded/share/system-probe/ebpf/dns.o"), 0644, "root", "root")

	state.AssertSymlinkExists("/opt/datadog-packages/datadog-agent/stable", agentRunSymlink, "root", "root")
	state.AssertSymlinkExists("/usr/bin/datadog-agent", agentSymlink, "root", "root")
	state.AssertSymlinkExists("/usr/bin/datadog-installer", installerSymlink, "root", "root")
	state.AssertFileExistsAnyUser("/etc/datadog-agent/install.json", 0644)
}

func (s *packageAgentSuite) assertUnits(state host.State, oldUnits bool) {
	state.AssertUnitsLoaded(agentUnit, traceUnit, processUnit, probeUnit, securityUnit, dataPlaneUnit)
	state.AssertUnitsEnabled(agentUnit, traceUnit, processUnit, securityUnit, dataPlaneUnit)

	// we cannot assert here on process-agent/agent-data-plane being either running or dead due to timing issues,
	// so it has to be checked prior (i.e., using WaitForUnitExited)
	state.AssertUnitsRunning(agentUnit, traceUnit)
	state.AssertUnitsDead(probeUnit, securityUnit)

	systemdPath := "/etc/systemd/system"
	if oldUnits || s.installMethod == InstallMethodAnsible {
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

	for _, unit := range []string{agentUnit, traceUnit, processUnit, probeUnit, securityUnit, dataPlaneUnit} {
		s.host.AssertUnitProperty(unit, "FragmentPath", filepath.Join(systemdPath, unit))
	}
}

func (s *packageAgentSuite) TestExperimentTimeout() {
	s.RunInstallScript("DD_REMOTE_UPDATES=true")
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
	s.RunInstallScript("DD_REMOTE_UPDATES=true")
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
	s.RunInstallScript("DD_REMOTE_UPDATES=true")
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
	s.RunInstallScript("DD_REMOTE_UPDATES=true")
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

func (s *packageAgentSuite) TestInstallWithGroupPreviouslyCreated() {
	s.host.Run("sudo userdel dd-agent || true")
	s.host.Run("sudo groupdel dd-agent || true")
	s.host.Run("sudo groupadd --system datadog")

	s.RunInstallScript("DD_REMOTE_UPDATES=true")
	defer s.Purge()

	assert.True(s.T(), s.host.UserExists("dd-agent"), "dd-agent user should exist")
	assert.True(s.T(), s.host.GroupExists("dd-agent"), "dd-agent group should exist")
}

func (s *packageAgentSuite) TestInstallWithFapolicyd() {
	if s.os != e2eos.RedHat9 {
		s.T().Skip("fapolicyd is only supported on RedHat 9")
	}
	defer func() {
		s.host.Run("sudo yum remove -y fapolicyd")
	}()
	s.host.Run("sudo yum install -y fapolicyd")

	s.TestInstall()
}

func (s *packageAgentSuite) TestNoWorldWritableFiles() {
	s.RunInstallScript()
	defer s.Purge()

	state := s.host.State()
	for path, file := range state.FS {
		if !strings.HasPrefix(path, "/opt/datadog") || file.IsSymlink {
			continue
		}
		if file.Perms&002 != 0 {
			s.T().Fatalf("file %v is world writable", path)
		}
	}
}
