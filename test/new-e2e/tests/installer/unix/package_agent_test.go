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

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/stretchr/testify/assert"

	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
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
	installerUnit   = "datadog-agent-installer.service"
)

type packageAgentSuite struct {
	packageBaseSuite
}

func testAgent(os e2eos.Descriptor, arch e2eos.Architecture, method InstallMethodOption) packageSuite {
	return &packageAgentSuite{
		packageBaseSuite: newPackageSuite("agent", os, arch, method, awshost.WithRunOptions(scenec2.WithoutFakeIntake())),
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
	agentRunSymlink := "/opt/datadog-packages/run/datadog-agent/" + agentVersion
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
	state.AssertUnitsEnabled(agentUnit)

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
	s.host.WaitForUnitActive(s.T(), agentUnit, traceUnit)

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
	s.host.WaitForUnitActive(s.T(), agentUnit, traceUnit)

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
	s.host.WaitForUnitActive(s.T(), agentUnit, traceUnit)

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
	s.host.WaitForUnitActive(s.T(), agentUnit, traceUnit)

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
		s.host.Run("sudo systemctl " + stopCommand)

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

func (s *packageAgentSuite) TestInstallWithNSSUser() {
	// This test verifies that the agent installer works correctly when dd-agent
	// user/group exist in NSS (Name Service Switch) but not in /etc/passwd or /etc/group.
	// This simulates scenarios where users are managed via LDAP, Active Directory, or other
	// NSS-compatible systems.

	// Step 1: Clean up any existing dd-agent user/group
	s.host.Run("sudo userdel dd-agent 2>/dev/null || true")
	s.host.Run("sudo groupdel dd-agent 2>/dev/null || true")

	// Capture initial state of /etc/passwd and /etc/group to verify no changes
	initialPasswd := s.host.Run("cat /etc/passwd")
	initialGroup := s.host.Run("cat /etc/group")

	// Step 2: Set up NSS extrausers
	// We use libnss-extrausers which reads from /var/lib/extrausers
	// This works through nsswitch.conf without needing environment variables

	// Install libnss-extrausers
	if s.host.GetPkgManager() == "apt" {
		s.host.Run("sudo apt-get update && sudo apt-get install -y libnss-extrausers")
	} else if s.host.GetPkgManager() == "yum" {
		_, err := s.Env().RemoteHost.Execute("sudo yum install -y libnss-extrausers")
		if err != nil {
			s.T().Skip("libnss-extrausers not available on this system")
			return
		}
	} else if s.host.GetPkgManager() == "zypper" {
		_, err := s.Env().RemoteHost.Execute("sudo zypper install -y libnss-extrausers")
		if err != nil {
			s.T().Skip("libnss-extrausers not available on this system")
			return
		}
	}

	// Create the extrausers directory structure
	s.host.Run("sudo mkdir -p /var/lib/extrausers")
	s.host.Run("sudo touch /var/lib/extrausers/passwd /var/lib/extrausers/group")
	s.host.Run("sudo chmod 644 /var/lib/extrausers/passwd /var/lib/extrausers/group")

	// Find an available UID/GID that doesn't conflict with existing users/groups
	// Check UIDs/GIDs from 900-999 to find unused ones
	var uid, gid int
	for id := 900; id < 1000; id++ {
		// Check if GID is available
		_, err := s.Env().RemoteHost.Execute(fmt.Sprintf("getent group %d", id))
		if err != nil {
			// GID is available
			gid = id
			break
		}
	}
	if gid == 0 {
		s.T().Fatal("Could not find available GID in range 900-999")
	}

	for id := 900; id < 1000; id++ {
		// Check if UID is available
		_, err := s.Env().RemoteHost.Execute(fmt.Sprintf("getent passwd %d", id))
		if err != nil {
			// UID is available
			uid = id
			break
		}
	}
	if uid == 0 {
		s.T().Fatal("Could not find available UID in range 900-999")
	}

	s.T().Logf("Using UID=%d GID=%d for dd-agent user/group", uid, gid)

	// Create dd-agent group in extrausers
	s.host.Run(fmt.Sprintf("echo 'dd-agent:x:%d:' | sudo tee /var/lib/extrausers/group", gid))

	// Create dd-agent user in extrausers (without home directory)
	s.host.Run(fmt.Sprintf("echo 'dd-agent:x:%d:%d:Datadog Agent:/nonexistent:/usr/sbin/nologin' | sudo tee /var/lib/extrausers/passwd", uid, gid))

	// Backup and update nsswitch.conf
	s.host.Run("sudo cp /etc/nsswitch.conf /etc/nsswitch.conf.backup")
	s.host.Run("sudo sed -i 's/^passwd:.*/passwd:         files extrausers/' /etc/nsswitch.conf")
	s.host.Run("sudo sed -i 's/^group:.*/group:          files extrausers/' /etc/nsswitch.conf")

	// Set up cleanup in defer to remove extrausers configuration
	// This MUST run regardless of test outcome
	defer func() {
		s.T().Log("Cleaning up NSS extrausers configuration")
		// Restore nsswitch.conf
		s.host.Run("sudo mv /etc/nsswitch.conf.backup /etc/nsswitch.conf")
		// Clean up the extrausers directory
		s.host.Run("sudo rm -rf /var/lib/extrausers")
		// Also clean up the dd-agent user/group if they were created
		s.host.Run("sudo userdel dd-agent 2>/dev/null || true")
		s.host.Run("sudo groupdel dd-agent 2>/dev/null || true")
		// Restart nscd if it's running to clear NSS cache
		s.host.Run("sudo systemctl restart nscd 2>/dev/null || true")
	}()

	// Step 3: Verify that getent can find the user/group (confirming NSS is working)
	getentUser := s.host.Run("getent passwd dd-agent")
	assert.Contains(s.T(), getentUser, "dd-agent", "dd-agent user should be visible via getent")

	getentGroup := s.host.Run("getent group dd-agent")
	assert.Contains(s.T(), getentGroup, "dd-agent", "dd-agent group should be visible via getent")

	// Verify user is NOT in /etc/passwd
	etcPasswd := s.host.Run("cat /etc/passwd")
	assert.NotContains(s.T(), etcPasswd, "dd-agent", "dd-agent should NOT be in /etc/passwd before install")

	// Verify group is NOT in /etc/group
	etcGroup := s.host.Run("cat /etc/group")
	assert.NotContains(s.T(), etcGroup, "dd-agent", "dd-agent should NOT be in /etc/group before install")

	// Step 4: Install the agent
	s.RunInstallScript("DD_REMOTE_UPDATES=true")
	defer s.Purge()

	// Step 5: Verify no new entries were added to /etc/passwd or /etc/group
	finalPasswd := s.host.Run("cat /etc/passwd")
	finalGroup := s.host.Run("cat /etc/group")

	assert.Equal(s.T(), initialPasswd, finalPasswd, "/etc/passwd should not change during installation")
	assert.Equal(s.T(), initialGroup, finalGroup, "/etc/group should not change during installation")

	// Step 6: Verify the agent is installed and running
	s.host.AssertPackageInstalledByInstaller("datadog-agent")
	s.host.WaitForUnitActive(s.T(), agentUnit, traceUnit)

	state := s.host.State()
	state.AssertUserExists("dd-agent")
	state.AssertGroupExists("dd-agent")

	// Verify agent files have correct ownership
	state.AssertDirExists("/opt/datadog-agent", 0755, "dd-agent", "dd-agent")
	state.AssertFileExists("/etc/datadog-agent/datadog.yaml", 0640, "dd-agent", "dd-agent")

	s.T().Log("Successfully installed agent with NSS-managed user/group")
}

func (s *packageAgentSuite) TestInstallFips() {
	if s.installMethod == InstallMethodAnsible {
		s.T().Skip("Can't install datadog-fips-agent test version with Ansible")
	}

	s.RunInstallScript("DD_REMOTE_UPDATES=true", "DD_AGENT_FLAVOR=datadog-fips-agent")
	defer s.Purge()
	s.host.AssertPackageInstalledByPackageManager("datadog-fips-agent")
	s.host.WaitForUnitActive(s.T(), agentUnit, traceUnit)
	s.host.WaitForUnitExited(s.T(), 0, processUnit, dataPlaneUnit)

	// Important: the installer daemon shouldn't start if FIPS is enabled. Remote Config will be disabled and the unit will exit with code 255.
	s.host.WaitForUnitExited(s.T(), 255, installerUnit)

	state := s.host.State()
	s.assertUnits(state, true)

	state.AssertFileExistsAnyUser("/etc/datadog-agent/install_info", 0644)
	state.AssertFileExists("/etc/datadog-agent/datadog.yaml", 0640, "dd-agent", "dd-agent")

	agentVersion := s.host.AgentStableVersion()
	agentDir := "/opt/datadog-agent"
	agentRunSymlink := "/opt/datadog-packages/run/datadog-agent/" + agentVersion
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
