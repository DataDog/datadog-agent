// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentplatform

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/install"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/install/installparams"
)

const (
	// macosInstallerBinary is the installer the .dmg postinstall script runs to install the
	// Agent's LaunchDaemons.
	macosInstallerBinary = "/opt/datadog-agent/embedded/bin/installer"
	// macosShippedLaunchDaemonsDir holds the root-owned copies of the LaunchDaemon definitions
	// the postinstall script falls back to when the installer cannot install them.
	macosShippedLaunchDaemonsDir = "/opt/datadog-agent/embedded/share/launchd"
	macosPostinstallLogPath      = "/opt/datadog-agent/logs/postinstall.log"

	// Lines omnibus/package-scripts/agent-dmg/postinst writes to its log.
	macosPostinstInstallerSucceeded = "# The installer postinst succeeded"
	macosPostinstInstallerFailed    = "The installer postinst failed; falling back"
	macosPostinstInstallerMissing   = macosInstallerBinary + " missing; falling back"
	macosPostinstFallbackMarker     = "(fallback)"
	macosPostinstFallbackFailed     = "ERROR: Could not install these LaunchDaemons"
)

// macosStableLaunchDaemons are the Agent's stable LaunchDaemons, in load order.
var macosStableLaunchDaemons = []string{
	"com.datadoghq.procmgr",
	"com.datadoghq.agent",
	"com.datadoghq.sysprobe",
	"com.datadoghq.data-plane",
}

// macosUnexpectedLaunchDaemons are jobs no package install may leave behind: the installer does
// not run as a daemon, and the experiment jobs only exist while a configuration experiment runs.
var macosUnexpectedLaunchDaemons = []string{
	"com.datadoghq.installer",
	"com.datadoghq.installer-exp",
	"com.datadoghq.agent-exp",
	"com.datadoghq.sysprobe-exp",
	"com.datadoghq.data-plane-exp",
}

func macosLaunchDaemonPath(label string) string {
	return "/Library/LaunchDaemons/" + label + ".plist"
}

func macosShippedLaunchDaemonPath(label string) string {
	return macosShippedLaunchDaemonsDir + "/" + label + ".plist"
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
	out, _ = client.Execute("ls -le /Library/LaunchDaemons/ " + macosShippedLaunchDaemonsDir + " " + macosInstallerBinary + "* 2>&1")
	t.Logf("job definitions and installer:\n%s", out)
	for _, label := range append(append([]string{}, macosStableLaunchDaemons...), macosUnexpectedLaunchDaemons...) {
		out, _ = client.Execute("sudo launchctl print system/" + label + " 2>&1 | head -n 40")
		t.Logf("launchctl print system/%s:\n%s", label, out)
	}
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

// assertMacosInstallerPostinstSucceeded checks the postinstall log shows the installer installed
// the LaunchDaemons, and that the script did not fall back to installing them itself.
func assertMacosInstallerPostinstSucceeded(t *testing.T, client *common.MacOSTestClient) {
	postinstallLog := macosReadRemoteFile(t, client, macosPostinstallLogPath)
	assert.Contains(t, postinstallLog, macosPostinstInstallerSucceeded)
	assert.NotContains(t, postinstallLog, "falling back", "the postinstall script fell back to installing the LaunchDaemons itself")
	assert.NotContains(t, postinstallLog, macosPostinstFallbackMarker, "the postinstall script fell back to installing the LaunchDaemons itself")
}

// assertMacosStableLaunchDaemons checks each stable LaunchDaemon is installed from the definition
// the package ships, owned by root:wheel with mode 644, and loaded, and that the shipped definitions
// are root-owned, since launchd runs whatever is copied from them as root.
func assertMacosStableLaunchDaemons(t *testing.T, client *common.MacOSTestClient) {
	owner, err := client.Execute("sudo stat -f '%Su:%Sg' " + macosShippedLaunchDaemonsDir)
	require.NoError(t, err)
	assert.Equal(t, "root:wheel", strings.TrimSpace(owner), "%s must not be writable by the Agent's account", macosShippedLaunchDaemonsDir)

	for _, label := range macosStableLaunchDaemons {
		plist := macosLaunchDaemonPath(label)
		shipped := macosReadRemoteFile(t, client, macosShippedLaunchDaemonPath(label))
		installed := macosReadRemoteFile(t, client, plist)
		assert.Equal(t, shipped, installed, "%s differs from the definition the package ships", plist)

		shippedAttributes, err := client.Execute("sudo stat -f '%Su:%Sg' " + macosShippedLaunchDaemonPath(label))
		require.NoError(t, err)
		assert.Equal(t, "root:wheel", strings.TrimSpace(shippedAttributes), "the shipped definition of %s must be root-owned", label)

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
}

// assertNoMacosUnexpectedLaunchDaemons checks neither the installer daemon nor any experiment job
// is loaded or has a job definition.
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
// the host. A reinstall cannot be used to exercise the fallback: the .pkg payload, which replaces
// the installer binary, is laid down before the postinstall script runs.
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
scripts=$(find %[1]s/expanded -name Scripts -type f | head -n 1)
cd %[1]s/scripts && gzip -dc "$scripts" | cpio -i --quiet
test -f %[1]s/scripts/postinstall
`, workDir, mountPoint, dmgURL))
	return workDir + "/scripts/postinstall"
}

// TestZYPostinstFallsBackWhenTheInstallerFails pins omnibus/package-scripts/agent-dmg/postinst's
// fallback: when the installer is missing or `installer postinst datadog-agent dmg` fails, the
// script installs the Agent's LaunchDaemons itself, from the root-owned copies the package ships,
// so an install never leaves the Agent stopped.
//
// The pipeline's own postinstall script is re-run on its own after the installer is removed or
// replaced with one that always fails. Beforehand, the job definitions are put in a state only the
// fallback can repair: two are unloaded and removed, one is left loaded with the wrong owner and
// mode, and one is left loaded and untouched (the fallback must reload it anyway).
//
// The ZY prefix runs this after the read-only tests and before TestZZUninstallAgent. Each case
// restores the real installer and has it reinstall the LaunchDaemons, and leaves the Agent healthy.
func (m *macosInstallSuite) TestZYPostinstFallsBackWhenTheInstallerFails() {
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
				if out, err := macosTestClient.Execute("sudo " + macosInstallerBinary + " postinst datadog-agent dmg 2>&1"); err != nil {
					t.Logf("cleanup: the restored installer could not reinstall the LaunchDaemons: %v\n%s", err, out)
				}
				waitForHealthyMacosAgent(t, macosTestClient, 60*time.Second)
			})
			macosLogDiagnosticsOnFailure(t, macosTestClient)

			agentPIDBefore := macosLaunchdPID(t, macosTestClient, "system/com.datadoghq.agent")

			macosTestClient.MustExecuteOn(t, fmt.Sprintf("sudo mv %s %s", macosInstallerBinary, savedInstaller))
			macosTestClient.MustExecuteOn(t, tc.breakInstaller)

			for _, label := range []string{"com.datadoghq.procmgr", "com.datadoghq.agent"} {
				macosTestClient.MustExecuteOn(t, fmt.Sprintf("sudo launchctl bootout system/%s 2>/dev/null; sudo rm -f %s", label, macosLaunchDaemonPath(label)))
			}
			sysprobePlist := macosLaunchDaemonPath("com.datadoghq.sysprobe")
			macosTestClient.MustExecuteOn(t, fmt.Sprintf("sudo chown %[1]s:staff %[2]s && sudo chmod 600 %[2]s", m.Env().RemoteHost.Username, sysprobePlist))

			// The postinstall script exits 0 even when the fallback fails, so its exit status is
			// not what this test checks.
			macosTestClient.MustExecuteOn(t, "sudo /bin/sh "+postinstall)

			postinstallLog := macosReadRemoteFile(t, macosTestClient, macosPostinstallLogPath)
			require.Contains(t, postinstallLog, tc.expectedLog, "the installer was made unusable, so the postinstall script should have fallen back")
			assert.NotContains(t, postinstallLog, macosPostinstInstallerSucceeded)
			assert.NotContains(t, postinstallLog, macosPostinstFallbackFailed)
			for _, label := range macosStableLaunchDaemons {
				assert.Contains(t, postinstallLog, "# Installing "+label+" as a system-wide LaunchDaemon "+macosPostinstFallbackMarker,
					"the fallback should have (re)installed %s", label)
			}

			assertMacosStableLaunchDaemons(t, macosTestClient)
			waitForHealthyMacosAgent(t, macosTestClient, 60*time.Second)
			assert.EventuallyWithT(t, func(c *assert.CollectT) {
				assert.NotEqual(c, agentPIDBefore, macosLaunchdPID(c, macosTestClient, "system/com.datadoghq.agent"),
					"the Agent should be running from the job the fallback loaded")
			}, 30*time.Second, 2*time.Second)
			assertNoMacosUnexpectedLaunchDaemons(t, macosTestClient)
		})
	}
}

// TestZYReinstallSameVersion reinstalls the same package over itself through the install script and
// checks the installer reinstalls the LaunchDaemons idempotently:
//   - the installer postinst succeeds, with no fallback;
//   - each job definition is byte-for-byte what it was before, and still the shipped definition,
//     root:wheel 644;
//   - all stable jobs are loaded and the Agent is healthy;
//   - the Agent's pid changes: the preinstall script unloads every job before the payload is laid
//     down, so a running Agent with the old pid would mean the reinstall left the previous process
//     behind instead of starting the reinstalled binaries;
//   - the configuration in etc/ is kept.
//
// Runs after TestZYPostinstFallsBackWhenTheInstallerFails (alphabetical order), so it also proves a
// regular install recovers a host whose LaunchDaemons were last installed by the fallback.
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
	assertMacosInstallerPostinstSucceeded(t, macosTestClient)
	for _, label := range macosStableLaunchDaemons {
		assert.Equal(t, definitionsBefore[label], macosReadRemoteFile(t, macosTestClient, macosLaunchDaemonPath(label)),
			"reinstalling the same version should not change %s", macosLaunchDaemonPath(label))
	}
	assertMacosStableLaunchDaemons(t, macosTestClient)
	waitForHealthyMacosAgent(t, macosTestClient, 60*time.Second)
	assert.NotEqual(t, agentPIDBefore, macosLaunchdPID(t, macosTestClient, "system/com.datadoghq.agent"),
		"the Agent should have been restarted from the reinstalled binaries")
	assert.Contains(t, macosReadRemoteFile(t, macosTestClient, confFilePath), marker, "the reinstall should keep datadog.yaml")
	assertNoMacosUnexpectedLaunchDaemons(t, macosTestClient)
}
