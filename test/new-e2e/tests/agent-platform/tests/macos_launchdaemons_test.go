// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentplatform

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/install"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/install/installparams"
)

const (
	// macosInstallerBinary is the installer the .dmg postinstall script runs to do the install.
	macosInstallerBinary = "/opt/datadog-agent/embedded/bin/installer"
	// macosProcmgrCLI talks to dd-procmgrd over its socket. It runs as root because the socket
	// is mode 0660 and owned by root:daemon.
	macosProcmgrCLI         = "/opt/datadog-agent/embedded/bin/dd-procmgr"
	macosPostinstallLogPath = "/opt/datadog-agent/logs/postinstall.log"
	macosPreinstallLogPath  = "/opt/datadog-agent/logs/preinstall.log"

	// Lines omnibus/package-scripts/agent-dmg/preinst writes to its log.
	macosPreinstPreRemoveRan    = "# Running the installed version's pre-remove"
	macosPreinstPreRemoveFailed = "WARNING: The installed version's pre-remove"

	// Lines omnibus/package-scripts/agent-dmg/postinst writes to its log.
	macosPostinstInstallerSucceeded = "# The installer postinst succeeded"
	macosPostinstInstallerFailed    = "ERROR: The installer postinst failed"
	macosPostinstInstallerMissing   = "ERROR: " + macosInstallerBinary + " is missing"
)

// macosInstallerLaunchDaemon is the root installer daemon's job. The installer loads it after the
// Agent's own jobs.
const macosInstallerLaunchDaemon = "com.datadoghq.installer"

// macosInstallerOwnDirs are the directories only root may write to: the packages root, which holds
// the package entry the installer daemon reports the Agent from.
var macosInstallerOwnDirs = []string{
	"/opt/datadog-packages",
}

// macosStableLaunchDaemons are the Agent's stable LaunchDaemons, in load order.
var macosStableLaunchDaemons = []string{
	"com.datadoghq.procmgr",
	"com.datadoghq.agent",
	"com.datadoghq.sysprobe",
	"com.datadoghq.data-plane",
}

// macosUnexpectedLaunchDaemons are jobs no package install may leave behind: the experiment jobs
// only exist while a configuration experiment runs, and the process manager has no experiment
// variant at all.
var macosUnexpectedLaunchDaemons = []string{
	"com.datadoghq.installer-exp",
	"com.datadoghq.agent-exp",
	"com.datadoghq.sysprobe-exp",
	"com.datadoghq.data-plane-exp",
	"com.datadoghq.procmgr-exp",
}

func macosLaunchDaemonPath(label string) string {
	return "/Library/LaunchDaemons/" + label + ".plist"
}

// waitForHealthyMacosAgent polls until `datadog-agent status` succeeds. Safe in t.Cleanup: CollectT
// recovers failures internally.
func waitForHealthyMacosAgent(t *testing.T, client *common.MacOSTestClient, timeout time.Duration) {
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		client.MustExecuteOn(c, macosAgentStatusCmd)
	}, timeout, 1*time.Second)
}

// macosReadRemoteFile returns the content of a root-readable file on the host.
func macosReadRemoteFile(t require.TestingT, client *common.MacOSTestClient, path string) string {
	out, err := client.Execute("sudo cat " + path)
	require.NoError(t, err, "could not read %s", path)
	return out
}

// macosLogLaunchDaemonDiagnostics logs what is needed to understand a LaunchDaemon failure: the
// tail of the postinstall log, the job definitions on disk and what launchd reports for each job.
func macosLogLaunchDaemonDiagnostics(t *testing.T, client *common.MacOSTestClient) {
	out, _ := client.Execute("sudo tail -n 100 " + macosPostinstallLogPath + " 2>&1")
	t.Logf("tail of %s:\n%s", macosPostinstallLogPath, out)
	out, _ = client.Execute("ls -le /Library/LaunchDaemons/ " + macosInstallerBinary + "* 2>&1")
	t.Logf("job definitions and installer:\n%s", out)
	for _, label := range append(append(append([]string{}, macosStableLaunchDaemons...), macosInstallerLaunchDaemon), macosUnexpectedLaunchDaemons...) {
		out, _ = client.Execute("sudo launchctl print system/" + label + " 2>&1 | head -n 40")
		t.Logf("launchctl print system/%s:\n%s", label, out)
	}
	out, _ = client.Execute("sudo " + macosProcmgrCLI + " status --json 2>&1")
	t.Logf("dd-procmgr status:\n%s", out)
}

// macosLogDiagnosticsOnFailure registers a cleanup that logs LaunchDaemon diagnostics if the test
// failed. Register it after any cleanup that restores the host, so it runs first and captures the
// failing state.
func macosLogDiagnosticsOnFailure(t *testing.T, client *common.MacOSTestClient) {
	t.Cleanup(func() {
		if t.Failed() {
			macosLogLaunchDaemonDiagnostics(t, client)
		}
	})
}

// assertMacosInstallerPostinstSucceeded checks the postinstall log shows the installer did the
// install.
func assertMacosInstallerPostinstSucceeded(t *testing.T, client *common.MacOSTestClient) {
	postinstallLog := macosReadRemoteFile(t, client, macosPostinstallLogPath)
	assert.Contains(t, postinstallLog, macosPostinstInstallerSucceeded)
	assert.NotContains(t, postinstallLog, "ERROR:", "the postinstall script reported a failure")
}

// assertMacosStableLaunchDaemons checks each stable LaunchDaemon is installed from the definition
// the installer embeds, owned by root:wheel with mode 644, and loaded. It also checks that
// dd-procmgrd answers on its socket and is supervising nothing.
func assertMacosStableLaunchDaemons(t *testing.T, client *common.MacOSTestClient) {
	for _, label := range macosStableLaunchDaemons {
		plist := macosLaunchDaemonPath(label)
		expected, err := embedded.GetLaunchdJob(label)
		require.NoError(t, err)
		installed := macosReadRemoteFile(t, client, plist)
		assert.Equal(t, string(expected), installed, "%s differs from the definition the installer embeds", plist)

		attributes, err := client.Execute("sudo stat -f '%Su:%Sg %Lp' " + plist)
		require.NoError(t, err)
		assert.Equal(t, "root:wheel 644", strings.TrimSpace(attributes), "%s has the wrong owner or mode", plist)
	}

	// The jobs are bootstrapped by the time the postinstall script returns, but give launchd time
	// to settle rather than racing it.
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		for _, label := range macosStableLaunchDaemons {
			_, err := client.Execute("sudo launchctl print system/" + label)
			assert.NoError(c, err, "%s is not loaded", label)
		}
	}, 60*time.Second, 2*time.Second)

	assertMacosProcmgrAnswers(t, client)
}

// assertMacosProcmgrAnswers checks dd-procmgrd is up and its catalog is empty.
//
// status is the call that proves the socket works. describe needs a process name, and no
// processes.d entry is shipped on macOS yet, so there is nothing to describe.
func assertMacosProcmgrAnswers(t *testing.T, client *common.MacOSTestClient) {
	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		out, err := client.Execute("sudo " + macosProcmgrCLI + " status --json")
		if !assert.NoError(c, err, "dd-procmgr status failed:\n%s", out) {
			return
		}
		var status struct {
			Ready          bool `json:"ready"`
			TotalProcesses int  `json:"total_processes"`
		}
		if !assert.NoError(c, json.Unmarshal([]byte(strings.TrimSpace(out)), &status), "dd-procmgr status output:\n%s", out) {
			return
		}
		assert.True(c, status.Ready, "dd-procmgrd is not ready:\n%s", out)
		assert.Equal(c, 0, status.TotalProcesses, "dd-procmgrd should be supervising nothing yet:\n%s", out)
	}, 60*time.Second, 2*time.Second)
}

// assertMacosInstallerLaunchDaemon checks the installer loaded its own daemon's job, root:wheel 644,
// and that the directories the daemon keeps its state in are root-owned and writable by root only,
// since the daemon runs as root and trusts what they hold.
func assertMacosInstallerLaunchDaemon(t *testing.T, client *common.MacOSTestClient) {
	plist := macosLaunchDaemonPath(macosInstallerLaunchDaemon)
	attributes, err := client.Execute("sudo stat -f '%Su:%Sg %Lp' " + plist)
	require.NoError(t, err, "the installer daemon has no job definition")
	assert.Equal(t, "root:wheel 644", strings.TrimSpace(attributes), "%s has the wrong owner or mode", plist)

	for _, dir := range macosInstallerOwnDirs {
		attributes, err := client.Execute("sudo stat -f '%HT %Su:%Sg %Lp' " + dir)
		require.NoError(t, err, "%s is missing", dir)
		assert.Equal(t, "Directory root:wheel 755", strings.TrimSpace(attributes), "%s must be a root-owned directory only root can write to", dir)
	}

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := client.Execute("sudo launchctl print system/" + macosInstallerLaunchDaemon)
		assert.NoError(c, err, "%s is not loaded", macosInstallerLaunchDaemon)
	}, 60*time.Second, 2*time.Second)
}

// assertNoMacosUnexpectedLaunchDaemons checks no experiment job is loaded or has a job definition.
func assertNoMacosUnexpectedLaunchDaemons(t *testing.T, client *common.MacOSTestClient) {
	for _, label := range macosUnexpectedLaunchDaemons {
		_, err := client.Execute("sudo launchctl print system/" + label)
		assert.Error(t, err, "%s is loaded", label)
		_, err = client.Execute("sudo test -e " + macosLaunchDaemonPath(label))
		assert.Error(t, err, "%s has a job definition", label)
	}
}

// macosExtractPipelinePostinstall downloads this pipeline's .dmg and extracts the postinstall
// script of the .pkg it ships, so the script can be re-run on its own. Returns the script's path on
// the host.
func macosExtractPipelinePostinstall(t *testing.T, client *common.MacOSTestClient, dmgURL string) string {
	const (
		workDir    = "/tmp/e2e-dmg-postinstall"
		mountPoint = "/Volumes/e2e_datadog_agent"
	)
	t.Cleanup(func() {
		if _, err := client.Execute(fmt.Sprintf("sudo hdiutil detach %s -force >/dev/null 2>&1; sudo rm -rf %s", mountPoint, workDir)); err != nil {
			t.Logf("cleanup: failed to remove %s: %v", workDir, err)
		}
	})
	client.MustExecuteOn(t, fmt.Sprintf(`set -e
sudo rm -rf %[1]s
mkdir -p %[1]s/scripts
for i in 1 2 3 4 5; do curl -fsSL %[3]s -o %[1]s/agent.dmg && break || sleep $((2**i)); done
test -s %[1]s/agent.dmg
sudo hdiutil detach %[2]s >/dev/null 2>&1 || true
sudo hdiutil attach %[1]s/agent.dmg -mountpoint %[2]s -nobrowse >/dev/null
pkg=$(find %[2]s -name '*.pkg' | head -n 1)
pkgutil --expand "$pkg" %[1]s/expanded
sudo hdiutil detach %[2]s >/dev/null 2>&1 || sudo hdiutil detach %[2]s -force >/dev/null
postinstall=$(find %[1]s/expanded -path '*/Scripts/postinstall' -type f | head -n 1)
if [ -n "$postinstall" ]; then
    cp "$postinstall" %[1]s/scripts/postinstall
else
    scripts=$(find %[1]s/expanded -name Scripts -type f | head -n 1)
    if [ -n "$scripts" ]; then
        # Keep decompression separate so cpio cannot mask a gzip failure.
        gzip -dc "$scripts" > %[1]s/scripts.cpio
        (cd %[1]s/scripts && cpio -i --quiet < %[1]s/scripts.cpio)
    fi
fi
if [ ! -f %[1]s/scripts/postinstall ]; then
    echo "Could not find postinstall: expected Scripts/postinstall or a Scripts archive containing postinstall. Expanded package contents:" >&2
    find %[1]s/expanded -ls >&2
    exit 1
fi
`, workDir, mountPoint, dmgURL))
	return workDir + "/scripts/postinstall"
}

// TestZYPostinstFailsWhenTheInstallerFails pins omnibus/package-scripts/agent-dmg/postinst's
// handling of a broken installer: when the installer is missing or `installer postinst
// datadog-agent dmg` fails, the script logs why and exits non-zero, so Installer.app, MDM or
// `installer -pkg` report a failed install. It does not touch the LaunchDaemons itself, so the
// Agent left running before keeps running.
//
// The pipeline's own postinstall script is re-run on its own after the installer is removed or
// replaced with one that always fails: a reinstall cannot be used, since the .pkg payload, which
// replaces the installer binary, is laid down before the postinstall script runs.
//
// The ZY prefix runs this after the read-only tests and before TestZZUninstallAgent. Each case
// restores the real installer and leaves the Agent healthy.
func (m *macosInstallSuite) TestZYPostinstFailsWhenTheInstallerFails() {
	params := installparams.NewParams(installparams.WithArch("x64"))
	if params.PipelineID == "" {
		m.T().Skip("E2E_PIPELINE_ID is not set: there is no pipeline .dmg to extract the postinstall script from")
	}
	macosTestClient := common.NewMacOSTestClient(m.Env().RemoteHost)
	postinstall := macosExtractPipelinePostinstall(m.T(), macosTestClient, install.MacOSPipelineRepoURL(params.PipelineID, params.Arch)+"/datadog-agent-7-latest.dmg")

	const savedInstaller = macosInstallerBinary + ".e2e-orig"
	for _, tc := range []struct {
		name string
		// breakInstaller runs after the real installer has been moved to savedInstaller.
		breakInstaller string
		expectedLog    string
	}{
		{
			name: "installer fails",
			breakInstaller: fmt.Sprintf(`printf '#!/bin/sh\necho "installer made to fail by the e2e test" >&2\nexit 1\n' | sudo tee %[1]s >/dev/null
sudo chown root:wheel %[1]s
sudo chmod 755 %[1]s`, macosInstallerBinary),
			expectedLog: macosPostinstInstallerFailed,
		},
		{
			name:           "installer missing",
			breakInstaller: "true",
			expectedLog:    macosPostinstInstallerMissing,
		},
	} {
		m.Run(tc.name, func() {
			t := m.T()
			// Registered before anything is changed, so the host is restored whatever fails below.
			t.Cleanup(func() {
				if _, err := macosTestClient.Execute(fmt.Sprintf("if sudo test -e %[1]s; then sudo mv -f %[1]s %[2]s; fi", savedInstaller, macosInstallerBinary)); err != nil {
					t.Logf("cleanup: failed to restore %s: %v", macosInstallerBinary, err)
				}
				waitForHealthyMacosAgent(t, macosTestClient, 60*time.Second)
			})
			macosLogDiagnosticsOnFailure(t, macosTestClient)

			agentPIDBefore := macosLaunchdPID(t, macosTestClient, "system/com.datadoghq.agent")

			macosTestClient.MustExecuteOn(t, fmt.Sprintf("sudo mv %s %s", macosInstallerBinary, savedInstaller))
			macosTestClient.MustExecuteOn(t, tc.breakInstaller)

			_, err := macosTestClient.Execute("sudo /bin/sh " + postinstall)
			assert.Error(t, err, "the postinstall script must fail the install when the installer cannot do it")

			postinstallLog := macosReadRemoteFile(t, macosTestClient, macosPostinstallLogPath)
			assert.Contains(t, postinstallLog, tc.expectedLog)
			assert.NotContains(t, postinstallLog, macosPostinstInstallerSucceeded)

			assertMacosStableLaunchDaemons(t, macosTestClient)
			assert.Equal(t, agentPIDBefore, macosLaunchdPID(t, macosTestClient, "system/com.datadoghq.agent"),
				"the postinstall script must leave the running Agent alone")
			assertNoMacosUnexpectedLaunchDaemons(t, macosTestClient)
		})
	}
}

// TestZYReinstallSameVersion reinstalls the same package over itself through the install script and
// checks the installer reinstalls the LaunchDaemons idempotently:
//   - the preinstall script hands the teardown to the installed installer's pre-remove, which
//     succeeds;
//   - the installer postinst succeeds;
//   - each job definition is byte-for-byte what it was before, and still the embedded definition,
//     root:wheel 644;
//   - all stable jobs are loaded and the Agent is healthy;
//   - the Agent's pid changes: the preinstall script unloads every job before the payload is laid
//     down, so a running Agent with the old pid would mean the reinstall left the previous process
//     behind instead of starting the reinstalled binaries;
//   - the configuration in etc/ is kept.
//
// Runs after TestZYPostinstFailsWhenTheInstallerFails (alphabetical order), so it also proves a
// regular install recovers a host whose last install failed.
func (m *macosInstallSuite) TestZYReinstallSameVersion() {
	t := m.T()
	macosTestClient := common.NewMacOSTestClient(m.Env().RemoteHost)
	confFilePath := macosConfDefaultConfPath + "/datadog.yaml"
	const marker = "# added by e2e TestZYReinstallSameVersion"

	t.Cleanup(func() {
		if _, err := macosTestClient.Execute(fmt.Sprintf(`sudo sed -i '' "/%s/d" %s`, marker, confFilePath)); err != nil {
			t.Logf("cleanup: failed to remove %q from %s: %v", marker, confFilePath, err)
		}
	})
	macosLogDiagnosticsOnFailure(t, macosTestClient)

	versionBefore := macosTestClient.MustExecuteOn(t, macosAgentBinary+" version")
	definitionsBefore := map[string]string{}
	for _, label := range macosStableLaunchDaemons {
		definitionsBefore[label] = macosReadRemoteFile(t, macosTestClient, macosLaunchDaemonPath(label))
	}
	agentPIDBefore := macosLaunchdPID(t, macosTestClient, "system/com.datadoghq.agent")
	macosTestClient.MustExecuteOn(t, fmt.Sprintf("echo %q | sudo tee -a %s >/dev/null", marker, confFilePath))

	install.MacOS(t, macosTestClient, installparams.WithUsername(m.Env().RemoteHost.Username), installparams.WithArch("x64"))
	require.False(t, t.Failed(), "the reinstall failed")

	assert.Equal(t, versionBefore, macosTestClient.MustExecuteOn(t, macosAgentBinary+" version"), "the same version should have been reinstalled")
	preinstallLog := macosReadRemoteFile(t, macosTestClient, macosPreinstallLogPath)
	assert.Contains(t, preinstallLog, macosPreinstPreRemoveRan, "the preinstall script should hand the teardown to the installed installer")
	assert.NotContains(t, preinstallLog, macosPreinstPreRemoveFailed)
	assertMacosInstallerPostinstSucceeded(t, macosTestClient)
	for _, label := range macosStableLaunchDaemons {
		assert.Equal(t, definitionsBefore[label], macosReadRemoteFile(t, macosTestClient, macosLaunchDaemonPath(label)),
			"reinstalling the same version should not change %s", macosLaunchDaemonPath(label))
	}
	assertMacosStableLaunchDaemons(t, macosTestClient)
	assertMacosInstallerLaunchDaemon(t, macosTestClient)
	waitForHealthyMacosAgent(t, macosTestClient, 60*time.Second)
	assert.NotEqual(t, agentPIDBefore, macosLaunchdPID(t, macosTestClient, "system/com.datadoghq.agent"),
		"the Agent should have been restarted from the reinstalled binaries")
	assert.Contains(t, macosReadRemoteFile(t, macosTestClient, confFilePath), marker, "the reinstall should keep datadog.yaml")
	assertNoMacosUnexpectedLaunchDaemons(t, macosTestClient)
}
