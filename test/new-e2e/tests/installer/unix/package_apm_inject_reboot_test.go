// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"fmt"
	"math/rand/v2"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// launcherPreloadPath is the path verifySharedLib() exercises and the path
// written into /etc/ld.so.preload by the datadog-apm-inject service's
// instrument-start command.
var launcherPreloadPath = filepath.Join(injectOCIPath, "stable", "inject", "launcher.preload.so")

// crashyConstructorUnconditionalSrc is a tiny C source compiled into a shared
// library whose ELF constructor calls _exit(1) unconditionally — regardless of
// whether the lib is loaded via LD_PRELOAD env var or /etc/ld.so.preload.
// This reproduces the worst-case production failure: a broken launcher that
// kills any process that loads it, even the kernel's first userspace process
// if /etc/ld.so.preload is not cleared before the reboot.
const crashyConstructorUnconditionalSrc = `#include <unistd.h>
__attribute__((constructor)) static void crash(void) { _exit(1); }
`

const (
	// agentMinorVersionBeforeAPMTmpfsSupport is passed to the public install
	// script through DD_AGENT_MINOR_VERSION to exercise a real installation of
	// an older Agent. This version supports the systemd service commands but
	// predates their tmpfs preload lifecycle.
	agentMinorVersionBeforeAPMTmpfsSupport = "80.4"

	// agentVersionBeforeAPMTmpfsSupport is the resulting installer repository
	// version (the Debian package's -1 revision is not part of this directory
	// name).
	agentVersionBeforeAPMTmpfsSupport = "7." + agentMinorVersionBeforeAPMTmpfsSupport
)

// captureBootID returns the current kernel boot id. Used as the source of
// truth for "has this host actually rebooted?" — reboot-command exit status is
// unreliable because sshd can be torn down before the SSH session returns.
func (s *packageApmInjectSuite) captureBootID() string {
	s.T().Helper()
	return strings.TrimSpace(s.Env().RemoteHost.MustExecute("cat /proc/sys/kernel/random/boot_id"))
}

// waitForBootIDChange reconnects over SSH and polls until the boot id differs
// from bootIDBefore, meaning the host has come back on a fresh boot.
func (s *packageApmInjectSuite) waitForBootIDChange(bootIDBefore string) {
	s.T().Helper()
	host := s.Env().RemoteHost

	// Give sshd time to actually go down so we don't reconnect to the old
	// boot. 30s is conservative; on fast images this is wasted, on slow
	// images (cloud-init, snapshot restores) 10s is not always enough.
	time.Sleep(30 * time.Second)

	// 20-minute ceiling: Ubuntu 24.04 VMs in CI take 12-20 minutes to reboot
	// and start accepting SSH (cloud-init, service startup). 10 minutes was
	// not enough.
	require.Eventually(s.T(), func() bool {
		if err := host.Reconnect(); err != nil {
			s.T().Logf("reconnect failed, will retry: %v", err)
			return false
		}
		out, err := host.Execute("cat /proc/sys/kernel/random/boot_id")
		if err != nil {
			s.T().Logf("boot id read failed after reconnect, will retry: %v", err)
			return false
		}
		bootIDAfter := strings.TrimSpace(out)
		if bootIDAfter == bootIDBefore {
			s.T().Logf("boot id unchanged (%s), host still on old boot", bootIDAfter)
			return false
		}
		s.T().Logf("host back up (boot id after: %s)", bootIDAfter)
		return true
	}, 20*time.Minute, 15*time.Second, "host did not reboot within timeout")
}

// reboot triggers a graceful reboot on a healthy host and waits for SSH to
// come back on a new boot id. Only use this when /etc/ld.so.preload does not
// point at an unconditionally-crashy lib — `systemctl` is dynamically linked
// and would be killed by ld.so. See TestSystemdServiceRebootBrokenInjector
// for the broken-injector variant that uses shutdown(8) to schedule the
// reboot before installing the crashy lib.
func (s *packageApmInjectSuite) reboot() {
	s.T().Helper()
	host := s.Env().RemoteHost

	bootIDBefore := s.captureBootID()
	s.T().Logf("rebooting host (boot id before: %s)", bootIDBefore)

	// `--no-block` queues the reboot job and returns immediately. We do not
	// require this call to succeed: on some images sshd is killed before the
	// SSH session returns, which surfaces as a transport / dial error here.
	// Whether the command returned cleanly or dropped the connection, the
	// boot-id check below is what tells us the host actually rebooted.
	if _, err := host.Execute("sudo systemctl --no-block reboot"); err != nil {
		s.T().Logf("reboot command returned an error (likely sshd torn down before reply): %v", err)
	}
	s.waitForBootIDChange(bootIDBefore)
}

// requireSystemd skips the test if systemd is not PID 1. The
// datadog-apm-inject.service is only installed on systemd hosts.
func (s *packageApmInjectSuite) requireSystemd() {
	s.T().Helper()
	if _, err := s.Env().RemoteHost.Execute("test \"$(cat /proc/1/comm 2>/dev/null)\" = systemd"); err != nil {
		s.T().Skip("systemd is not running as PID 1 on this host")
	}
}

// TestSystemdServiceReboot verifies the boot-time instrumentation contract on
// a healthy host: after a reboot, the systemd service runs ExecStart, restores
// /etc/ld.so.preload, and tracer injection still produces traces end-to-end.
//
// Without the service, /etc/ld.so.preload would only be written at install
// time and rebooting after a clean shutdown (which clears it via ExecStopPost)
// would leave the host uninstrumented until the next install action.
func (s *packageApmInjectSuite) TestSystemdServiceReboot() {
	s.requireSystemd()

	s.RunInstallScript("DD_APM_INSTRUMENTATION_ENABLED=host", "DD_APM_INSTRUMENTATION_LIBRARIES=python")
	defer s.Purge()

	s.host.WaitForUnitActive(s.T(), "datadog-apm-inject.service", "datadog-agent.service", "datadog-agent-trace.service")
	s.assertLDPreloadInstrumented(injectOCIPath)

	s.reboot()

	// After reboot, ExecStopPost ran during shutdown (clearing ld.so.preload) and
	// ExecStart ran on boot (re-adding the entry). The service unit must be
	// active and the file must contain the injector path again.
	s.host.WaitForUnitActive(s.T(), "datadog-apm-inject.service", "datadog-agent.service", "datadog-agent-trace.service")

	state := s.host.State()
	state.AssertFileExists("/etc/systemd/system/datadog-apm-inject.service", 0644, "root", "root")
	state.AssertUnitsEnabled("datadog-apm-inject.service")
	state.AssertUnitsActive("datadog-apm-inject.service")
	s.assertLDPreloadInstrumented(injectOCIPath)
	s.assertSocketPath()

	// End-to-end check: the tracer is injected into a freshly-spawned process
	// and the resulting trace lands in fakeintake.
	s.host.StartExamplePythonApp()
	defer s.host.StopExamplePythonApp()
	traceID := rand.Uint64()
	s.host.CallExamplePythonApp(strconv.FormatUint(traceID, 10))
	s.assertTraceReceived(traceID)
}

// TestOlderAgentAPMInjectService verifies that installing SSI with an Agent
// predating tmpfs support keeps the systemd service without using tmpfs:
//
//  1. Install host SSI with Agent 7.80.4 on a clean host.
//  2. Verify the service uses the persistent injector path.
//  3. Reboot and verify host injection still uses a valid preload entry without
//     producing loader errors.
//
// The selected Agent installer supports the service commands but predates the
// tmpfs preload lifecycle. The injector post-install hook therefore keeps the
// service while selecting the persistent injector path it understands.
//
// This is intentionally limited to one representative host. The contract is
// the installer/package/systemd lifecycle, not distro- or architecture-specific
// behavior.
func (s *packageApmInjectSuite) TestOlderAgentAPMInjectService() {
	isUbuntu2404 := s.os.Flavor == e2eos.Ubuntu2404.Flavor && s.os.Version == e2eos.Ubuntu2404.Version
	if s.installMethod != InstallMethodInstallScript || !isUbuntu2404 || s.arch != e2eos.AMD64Arch {
		s.T().Skip("older Agent installation is covered on Ubuntu 24.04 amd64 with the install script")
	}
	s.requireSystemd()

	host := s.Env().RemoteHost
	// Purge uses the older Agent installer, which predates the current APM
	// cleanup commands. Always remove the preload entry last so a same-host retry
	// does not start with a dangling /run launcher after the reboot.
	defer host.Execute("sudo rm -f /etc/ld.so.preload") //nolint:errcheck
	s.RunInstallScript(
		"DD_APM_INSTRUMENTATION_ENABLED=host",
		"DD_APM_INSTRUMENTATION_LIBRARIES=python",
		"TESTING_APT_URL=",
		"TESTING_APT_REPO_VERSION=",
		"TESTING_YUM_URL=",
		"TESTING_YUM_VERSION_PATH=",
		"DD_REPO_URL=datadoghq.com",
		"DD_AGENT_MAJOR_VERSION=7",
		"DD_AGENT_MINOR_VERSION="+agentMinorVersionBeforeAPMTmpfsSupport,
	)
	defer s.Purge()
	require.Equal(s.T(), agentVersionBeforeAPMTmpfsSupport, s.host.AgentStableVersion())

	// An older selected installer must keep the service but make the hook use the
	// persistent path understood by its pre-tmpfs implementation.
	s.host.WaitForUnitActive(s.T(), "datadog-apm-inject.service", "datadog-agent.service", "datadog-agent-trace.service")
	preload, err := s.host.ReadFile("/etc/ld.so.preload")
	require.NoError(s.T(), err)
	require.Contains(s.T(), string(preload), launcherPreloadPath)
	require.NotContains(s.T(), string(preload), injectTmpfsLauncher)

	oldInstallerPath := "/opt/datadog-packages/datadog-agent/stable/embedded/bin/installer"
	help, err := host.Execute("sudo " + oldInstallerPath + " apm --help")
	require.NoError(s.T(), err, "could not inspect the older embedded installer")
	require.Contains(s.T(), help, "instrument-start", "test precondition: 7.80.4 must support the systemd service commands")
	require.Contains(s.T(), help, "instrument-stop", "test precondition: 7.80.4 must support the systemd service commands")

	s.reboot()
	s.host.WaitForUnitActive(s.T(), "datadog-apm-inject.service", "datadog-agent.service", "datadog-agent-trace.service")

	preload, err = s.host.ReadFile("/etc/ld.so.preload")
	require.NoError(s.T(), err)
	require.Contains(s.T(), string(preload), launcherPreloadPath)
	require.NotContains(s.T(), string(preload), injectTmpfsLauncher)
	_, err = host.Execute("test -e " + launcherPreloadPath)
	require.NoError(s.T(), err, "ld.so.preload entry is dangling")
	_, err = host.Execute("test -e /etc/systemd/system/datadog-apm-inject.service")
	require.NoError(s.T(), err, "apm-inject service was not preserved")
	s.assertDockerdNotInstrumented()

	stderr, err := host.Execute(`i=0; while [ "$i" -lt 10 ]; do /bin/true; i=$((i + 1)); done 2>&1`)
	require.NoError(s.T(), err)
	require.NotContains(s.T(), stderr, "cannot be preloaded")
}

// TestAgentDowngradeReinstallsAPMInject verifies that an Agent installation
// reruns the post-install hook of an already-installed injector even when its
// requested version is unchanged:
//
//  1. Install the pipeline Agent and injector with host SSI enabled.
//  2. Downgrade only the Agent to 7.80.4 while requesting the same injector.
//  3. Verify the injector version is unchanged but its hook has switched the
//     service from tmpfs to the persistent preload path supported by 7.80.4.
//  4. Reboot and verify the refreshed service still has a valid preload entry.
//
// This is intentionally limited to one representative host. The contract is
// the install-script/package-hook/systemd lifecycle, not distro- or
// architecture-specific behavior.
func (s *packageApmInjectSuite) TestAgentDowngradeReinstallsAPMInject() {
	isUbuntu2404 := s.os.Flavor == e2eos.Ubuntu2404.Flavor && s.os.Version == e2eos.Ubuntu2404.Version
	if s.installMethod != InstallMethodInstallScript || !isUbuntu2404 || s.arch != e2eos.AMD64Arch {
		s.T().Skip("Agent downgrade hook replay is covered on Ubuntu 24.04 amd64 with the install script")
	}
	s.requireSystemd()

	host := s.Env().RemoteHost
	// Purge uses the downgraded Agent installer. Always remove the preload entry
	// last so a same-host retry cannot start with a dangling launcher.
	defer host.Execute("sudo rm -f /etc/ld.so.preload") //nolint:errcheck
	s.RunInstallScript("DD_APM_INSTRUMENTATION_ENABLED=host", "DD_APM_INSTRUMENTATION_LIBRARIES=python")
	defer s.Purge()

	s.host.WaitForUnitActive(s.T(), "datadog-apm-inject.service", "datadog-agent.service", "datadog-agent-trace.service")
	s.assertLDPreloadInstrumented(injectOCIPath)

	recentAgentVersion := s.host.AgentStableVersion()
	require.NotEqual(s.T(), agentVersionBeforeAPMTmpfsSupport, recentAgentVersion,
		"test requires a recent Agent before exercising the downgrade")
	injectorVersion := strings.TrimSpace(host.MustExecute("readlink /opt/datadog-packages/datadog-apm-inject/stable"))
	require.NotEmpty(s.T(), injectorVersion)

	// Keep SSI explicitly enabled so the same injector version is requested. Its
	// package must still be force-installed after Agent stable changes, otherwise
	// Install() skips the post-install hook as already up to date.
	s.RunInstallScript(
		"DD_APM_INSTRUMENTATION_ENABLED=host",
		"DD_APM_INSTRUMENTATION_LIBRARIES=python",
		"TESTING_APT_URL=",
		"TESTING_APT_REPO_VERSION=",
		"TESTING_YUM_URL=",
		"TESTING_YUM_VERSION_PATH=",
		"DD_REPO_URL=datadoghq.com",
		"DD_AGENT_MAJOR_VERSION=7",
		"DD_AGENT_MINOR_VERSION="+agentMinorVersionBeforeAPMTmpfsSupport,
	)
	require.Equal(s.T(), agentVersionBeforeAPMTmpfsSupport, s.host.AgentStableVersion())
	require.Equal(s.T(), injectorVersion,
		strings.TrimSpace(host.MustExecute("readlink /opt/datadog-packages/datadog-apm-inject/stable")),
		"the test requires the injector version to remain unchanged")

	// Replaying the injector hook must preserve the service while changing the
	// preload path to the persistent path understood by the older Agent installer.
	s.host.WaitForUnitActive(s.T(), "datadog-apm-inject.service", "datadog-agent.service", "datadog-agent-trace.service")
	preload, err := s.host.ReadFile("/etc/ld.so.preload")
	require.NoError(s.T(), err)
	require.Contains(s.T(), string(preload), launcherPreloadPath)
	require.NotContains(s.T(), string(preload), injectTmpfsLauncher)

	s.reboot()
	s.host.WaitForUnitActive(s.T(), "datadog-apm-inject.service", "datadog-agent.service", "datadog-agent-trace.service")

	preload, err = s.host.ReadFile("/etc/ld.so.preload")
	require.NoError(s.T(), err)
	require.Contains(s.T(), string(preload), launcherPreloadPath)
	require.NotContains(s.T(), string(preload), injectTmpfsLauncher)
	_, err = host.Execute("test -e " + launcherPreloadPath)
	require.NoError(s.T(), err, "ld.so.preload entry is dangling")

	stderr, err := host.Execute(`i=0; while [ "$i" -lt 10 ]; do /bin/true; i=$((i + 1)); done 2>&1`)
	require.NoError(s.T(), err)
	require.NotContains(s.T(), stderr, "cannot be preloaded")
}

// TestSystemdServiceRebootBrokenInjector verifies the safety property the
// tmpfs symlink exists to provide: if the injector library on disk is replaced
// by a .so that unconditionally crashes any process that loads it (via any
// mechanism), a reboot still leaves the host usable.
//
// The contract: /etc/ld.so.preload references the launcher through the
// /run/datadog-apm-inject symlink, which lives on tmpfs and is wiped on every
// boot. After the reboot that symlink is gone, so the ld.so.preload entry
// resolves to a missing file and ld.so silently skips it — init(1) and every
// other process come up normally. Crucially this holds even if shutdown-time
// cleanup (ExecStopPost) never ran: recovery does not depend on any binary
// executing successfully during shutdown. On the next boot ExecStart's
// verifySharedLib detects the broken lib (via LD_PRELOAD=lib echo 1), refuses
// to recreate the symlink, and leaves the service in failed state. Docker must
// start only after that stale preload entry has been cleaned up.
//
// Reboot mechanism: the test uses shutdown(8) to schedule a graceful reboot
// ONE MINUTE before installing the crashy lib. At T+1min systemd activates
// reboot.target internally — no new binary is exec'd when the timer fires, so
// the unconditional crashy lib cannot kill the trigger. This avoids the
// alternative `kill -39 1` (whose graceful-shutdown guarantee is unreliable on
// some CI images) and `systemctl reboot` (which would be exec'd after the
// crashy lib is installed and killed by ld.so before it could reach main()).
func (s *packageApmInjectSuite) TestSystemdServiceRebootBrokenInjector() {
	s.requireSystemd()

	s.host.InstallDocker()
	// InstallDocker starts the daemon but some packages, notably SUSE's, do
	// not enable it. Enable it explicitly so Docker and datadog-apm-inject are
	// part of the same boot transaction and exercise the service ordering.
	s.Env().RemoteHost.MustExecute("sudo systemctl enable docker.service")
	s.RunInstallScript("DD_APM_INSTRUMENTATION_ENABLED=host", "DD_APM_INSTRUMENTATION_LIBRARIES=python")
	defer s.Purge()

	host := s.Env().RemoteHost
	s.host.WaitForUnitActive(s.T(), "datadog-apm-inject.service")
	s.assertLDPreloadInstrumented(injectOCIPath)

	// Pre-compile the unconditional crashy .so (crashes any process that loads
	// it, regardless of whether it is via LD_PRELOAD env var or /etc/ld.so.preload).
	crashyStagingPath := "/tmp/crashy_preboot.so"
	s.buildCrashyInjectorSO(crashyStagingPath, crashyConstructorUnconditionalSrc)

	bootIDBefore := s.captureBootID()
	s.T().Logf("rebooting host (boot id before: %s)", bootIDBefore)

	// Defensive restore: after the reboot the tmpfs symlink is gone, so the
	// on-disk crashy .so is unreachable via ld.so.preload and inert. Restoring
	// the original keeps the host sane for Purge() and any retries.
	defer host.Execute(fmt.Sprintf("sudo mv -f %[1]s.bak %[1]s 2>/dev/null || true", launcherPreloadPath)) //nolint:errcheck

	// Schedule a graceful reboot ONE MINUTE from now, before installing the
	// crashy lib. shutdown(8) talks to systemd (D-Bus) and returns; at T+1min
	// systemd activates reboot.target internally — no new binary is exec'd at
	// that point, so the unconditional crashy lib cannot kill the trigger.
	host.MustExecute("sudo shutdown -r +1")

	// Install the unconditional crashy lib. After the second mv, any new
	// execve on this host is killed by ld.so before main() can run.
	// Between the two mv calls /etc/ld.so.preload references a missing file
	// which ld.so silently skips, so the second mv itself can exec normally.
	// No SSH commands should be issued after this point until the reboot
	// completes — waitForBootIDChange handles the reconnect loop.
	host.MustExecute(fmt.Sprintf("sudo mv %s %s.bak", launcherPreloadPath, launcherPreloadPath))
	host.MustExecute(fmt.Sprintf("sudo mv %s %s", crashyStagingPath, launcherPreloadPath))

	s.waitForBootIDChange(bootIDBefore)

	// Reaching here means SSH came back: the host booted past init(1) with a
	// crashy .so on disk. That's only possible because the /run tmpfs symlink
	// was wiped on reboot, so the ld.so.preload entry resolved to nothing and
	// ld.so skipped it — recovery did not depend on shutdown-time cleanup.
	out, err := host.Execute("uname -a && id && /bin/true")
	require.NoError(s.T(), err, "host is not usable after reboot with broken injector")
	require.NotEmpty(s.T(), out)

	// The new boot's ExecStart fired verifySharedLib, the LD_PRELOAD=lib echo
	// subprocess exited 1 (constructor _exit), the command exited non-zero,
	// the unit is failed.
	assert.Eventually(s.T(), func() bool {
		_, err := host.Execute("systemctl is-failed --quiet datadog-apm-inject.service")
		return err == nil
	}, 90*time.Second, 2*time.Second,
		"datadog-apm-inject.service did not enter failed state after reboot. status:\n%s\nlogs:\n%s",
		host.MustExecute("systemctl status datadog-apm-inject.service --no-pager || true"),
		host.MustExecute("sudo journalctl -xeu datadog-apm-inject.service --no-pager || true"),
	)

	// The host must not be instrumented: the tmpfs symlink was wiped on reboot
	// and the failed ExecStart refused to recreate it, so no process loads the
	// injector. ExecStart additionally strips the now-dangling /run entry from
	// /etc/ld.so.preload on verification failure, so ld.so does not warn about a
	// missing preload on every exec. assertLDPreloadNotInstrumented checks that
	// the tmpfs path is gone (it runs only after the unit reached failed state
	// above, i.e. after instrument-start completed its cleanup). This is the core
	// guarantee — a broken injector on disk does not brick subsequent boots.
	s.assertLDPreloadNotInstrumented()

	// The agent itself is unaffected — it does not depend on the inject
	// service for its own operation.
	// Docker is ordered after datadog-apm-inject so it starts only after the
	// failed ExecStart has removed the stale /run entry from ld.so.preload.
	s.host.WaitForUnitActive(s.T(), "datadog-agent.service", "datadog-agent-trace.service", "docker.service")
	_, err = host.Execute("sudo docker info")
	require.NoError(s.T(), err, "Docker daemon is not usable after reboot with a stale injector entry")
}
