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
	"go.yaml.in/yaml/v3"
)

const (
	injectOCIPath = "/opt/datadog-packages/datadog-apm-inject"
	injectDebPath = "/opt/datadog/apm"
	// injectTmpfsLauncher is the launcher entry written to /etc/ld.so.preload
	// for OCI host instrumentation on systemd hosts: a symlink on tmpfs that
	// auto-vanishes on reboot. See apminject.defaultTmpfsInjectDir.
	injectTmpfsLauncher = "/run/datadog-apm-inject/launcher.preload.so"
)

type packageApmInjectSuite struct {
	packageBaseSuite
}

func testApmInjectAgent(os e2eos.Descriptor, arch e2eos.Architecture, method InstallMethodOption) packageSuite {
	return &packageApmInjectSuite{
		packageBaseSuite: newPackageSuite("apm-inject", os, arch, method),
	}
}

func (s *packageApmInjectSuite) SetupTest() {
	// Purge() uses Execute (not MustExecute), so failures are silent.
	// A stale packages.db entry causes Install() to skip PostInstall hooks
	// (which create /etc/ld.so.preload and /etc/docker/daemon.json).
	s.Env().RemoteHost.Execute("sudo rm -f /opt/datadog-packages/packages.db")
	s.Env().RemoteHost.Execute("sudo rm -f /etc/ld.so.preload")
	s.Env().RemoteHost.Execute("sudo rm -f /etc/docker/daemon.json")
}

func (s *packageApmInjectSuite) TestInstall() {
	s.host.InstallDocker()
	s.RunInstallScript("DD_APM_INSTRUMENTATION_ENABLED=all", "DD_APM_INSTRUMENTATION_LIBRARIES=python")
	defer s.Purge()
	s.host.WaitForUnitActive(s.T(), "datadog-agent.service", "datadog-agent-trace.service")

	s.host.StartExamplePythonApp()
	defer s.host.StopExamplePythonApp()
	s.host.StartExamplePythonAppInDocker()
	defer s.host.StopExamplePythonAppInDocker()

	s.host.AssertPackageInstalledByInstaller("datadog-apm-inject", "datadog-apm-library-python")
	s.host.AssertPackageNotInstalledByPackageManager("datadog-apm-inject", "datadog-apm-library-python")
	s.host.AssertPackageInstalledByPackageManager("datadog-agent")
	state := s.host.State()
	state.AssertFileExists("/opt/datadog-packages/run/environment", 0644, "root", "root")
	state.AssertSymlinkExists("/etc/default/datadog-agent", "/opt/datadog-packages/run/environment", "root", "root")
	state.AssertSymlinkExists("/etc/default/datadog-agent-trace", "/opt/datadog-packages/run/environment", "root", "root")
	state.AssertDirExists("/var/log/datadog/dotnet", 0777, "root", "root")
	state.AssertFileExists("/etc/ld.so.preload", 0644, "root", "root")
	state.AssertFileExists("/usr/bin/dd-host-install", 0755, "root", "root")
	state.AssertFileExists("/usr/bin/dd-container-install", 0755, "root", "root")
	state.AssertDirExists("/etc/datadog-agent/inject", 0755, "root", "root")
	if s.os == e2eos.Ubuntu2404 || s.os == e2eos.Debian12 {
		state.AssertDirExists("/etc/apparmor.d/abstractions/datadog.d", 0755, "root", "root")
		state.AssertFileExists("/etc/apparmor.d/abstractions/datadog.d/injector", 0644, "root", "root")
	}
	s.assertLDPreloadInstrumented(injectOCIPath)
	s.assertSocketPath()
	s.assertDockerdInstrumented(injectOCIPath)
	s.assertStableConfig(map[string]interface{}{})

	traceID := rand.Uint64()
	s.host.CallExamplePythonApp(strconv.FormatUint(traceID, 10))
	traceIDDocker := rand.Uint64()
	s.host.CallExamplePythonAppInDocker(strconv.FormatUint(traceIDDocker, 10))

	s.assertTraceReceived(traceID)
	s.assertTraceReceived(traceIDDocker)
}

func (s *packageApmInjectSuite) TestUninstall() {
	s.host.InstallDocker()
	s.RunInstallScript("DD_APM_INSTRUMENTATION_ENABLED=all", "DD_APM_INSTRUMENTATION_LIBRARIES=python")
	s.Purge()

	s.assertLDPreloadNotInstrumented()
	s.assertDockerdNotInstrumented()

	state := s.host.State()
	state.AssertPathDoesNotExist("/usr/bin/dd-host-install")
	state.AssertPathDoesNotExist("/usr/bin/dd-container-install")
	state.AssertPathDoesNotExist("/etc/apparmor.d/abstractions/datadog.d/injector")
}

func (s *packageApmInjectSuite) TestDockerAdditionalFields() {
	s.host.InstallDocker()
	// Broken /etc/docker/daemon.json syntax
	s.host.SetBrokenDockerConfig()
	defer s.host.RemoveBrokenDockerConfig()
	_ = s.RunInstallScriptWithError("DD_APM_INSTRUMENTATION_ENABLED=all", "DD_APM_INSTRUMENTATION_LIBRARIES=python")
	defer s.Purge()

	s.assertLDPreloadNotInstrumented()
	s.assertDockerdNotInstrumented()
}

func (s *packageApmInjectSuite) TestDockerBrokenJSON() {
	s.host.InstallDocker()
	// Additional fields in /etc/docker/daemon.json
	s.host.SetBrokenDockerConfigAdditionalFields()
	defer s.host.RemoveBrokenDockerConfig()
	_ = s.RunInstallScriptWithError("DD_APM_INSTRUMENTATION_ENABLED=all", "DD_APM_INSTRUMENTATION_LIBRARIES=python")
	defer s.Purge()

	s.assertLDPreloadNotInstrumented()
	s.assertDockerdNotInstrumented()
}

func (s *packageApmInjectSuite) TestInstrumentDocker() {
	s.host.InstallDocker()
	s.RunInstallScript("DD_APM_INSTRUMENTATION_ENABLED=docker", "DD_APM_INSTRUMENTATION_LIBRARIES=python")
	defer s.Purge()

	s.assertLDPreloadNotInstrumented()
	s.assertDockerdInstrumented(injectOCIPath)
}

func (s *packageApmInjectSuite) TestInstrumentHost() {
	s.RunInstallScript("DD_APM_INSTRUMENTATION_ENABLED=host", "DD_APM_INSTRUMENTATION_LIBRARIES=python")
	defer s.Purge()

	s.assertLDPreloadInstrumented(injectOCIPath)
	s.assertDockerdNotInstrumented()
}

func (s *packageApmInjectSuite) TestInstrumentProfilingEnabled() {
	s.RunInstallScript("DD_APM_INSTRUMENTATION_ENABLED=host", "DD_APM_INSTRUMENTATION_LIBRARIES=python", "DD_PROFILING_ENABLED=auto", "DD_DATA_STREAMS_ENABLED=true")
	defer s.Purge()
	s.assertStableConfig(map[string]interface{}{
		"DD_PROFILING_ENABLED":    "auto",
		"DD_DATA_STREAMS_ENABLED": true,
	})
}

func (s *packageApmInjectSuite) TestInstrumentDefault() {
	s.host.InstallDocker()
	s.RunInstallScript("DD_APM_INSTRUMENTATION_ENABLED=all", "DD_APM_INSTRUMENTATION_LIBRARIES=python")
	defer s.Purge()

	s.assertLDPreloadInstrumented(injectOCIPath)
	s.assertDockerdInstrumented(injectOCIPath)
}

func (s *packageApmInjectSuite) TestSystemdReload() {
	s.host.InstallDocker()
	s.RunInstallScript()
	defer s.Purge()

	s.RunInstallScript("DD_APM_INSTRUMENTATION_ENABLED=all", "DD_APM_INSTRUMENTATION_LIBRARIES=python")
	s.host.WaitForUnitActive(s.T(), "datadog-agent.service", "datadog-agent-trace.service")
	s.assertLDPreloadInstrumented(injectOCIPath)
	s.assertSocketPath()
	s.assertDockerdInstrumented(injectOCIPath)
}

// TestUpgrade_InjectorDeb_To_InjectorOCI tests the upgrade from the DEB injector to the OCI injector.
// Library package is OCI.
func (s *packageApmInjectSuite) TestUpgrade_InjectorDeb_To_InjectorOCI() {
	if s.os.Flavor == e2eos.Suse {
		s.T().Skip("Can't install APM deb/rpm packages on Suse, they were never released")
	}
	if s.installMethod == InstallMethodAnsible {
		s.T().Skip("Ansible doesn't support upgrading from OCI to DEB")
	}

	s.host.InstallDocker()

	// Deb install using today's defaults
	s.RunInstallScript(
		"TESTING_APT_URL=",
		"TESTING_APT_REPO_VERSION=",
		"TESTING_YUM_URL=",
		"TESTING_YUM_VERSION_PATH=",
		"DD_REPO_URL=datadoghq.com",
	)
	s.host.Run("sudo apt-get install -y datadog-apm-inject datadog-apm-library-python || sudo yum install -y datadog-apm-inject datadog-apm-library-python")
	s.host.Run("sudo dd-container-install --no-agent-restart")
	s.host.Run("sudo dd-host-install --no-agent-restart")
	defer s.Purge()
	defer s.purgeInjectorDebInstall()

	s.assertLDPreloadInstrumented(injectDebPath)
	s.assertSocketPath()
	s.assertDockerdInstrumented(injectDebPath)

	// OCI install
	s.RunInstallScript(
		"DD_APM_INSTRUMENTATION_ENABLED=all",
		"DD_APM_INSTRUMENTATION_LIBRARIES=python",
	)

	s.assertLDPreloadInstrumented(injectOCIPath)
	s.assertSocketPath()
	s.assertDockerdInstrumented(injectOCIPath)
	s.host.AssertPackageNotInstalledByPackageManager("datadog-apm-inject", "datadog-apm-library-python")

}

// TestUpgrade_InjectorOCI_To_InjectorDeb tests the upgrade from the OCI injector to the DEB injector.
// Library package is OCI.
func (s *packageApmInjectSuite) TestUpgrade_InjectorOCI_To_InjectorDeb() {
	if s.os.Flavor == e2eos.Suse {
		s.T().Skip("Can't install APM deb/rpm packages on Suse, they were never released")
	}
	if s.installMethod == InstallMethodAnsible {
		s.T().Skip("Ansible doesn't support upgrading from OCI to DEB")
	}

	s.host.InstallDocker()

	// OCI install
	s.RunInstallScript(
		"DD_APM_INSTRUMENTATION_ENABLED=all",
		"DD_APM_INSTRUMENTATION_LIBRARIES=python",
	)
	defer s.Purge()

	s.assertLDPreloadInstrumented(injectOCIPath)
	s.assertSocketPath()
	s.assertDockerdInstrumented(injectOCIPath)

	// Deb install using today's defaults
	s.RunInstallScript(
		"TESTING_APT_URL=",
		"TESTING_APT_REPO_VERSION=",
		"TESTING_YUM_URL=",
		"TESTING_YUM_VERSION_PATH=",
		"DD_REPO_URL=datadoghq.com",
	)
	s.host.Run("sudo apt-get install -y datadog-apm-inject datadog-apm-library-python || sudo yum install -y datadog-apm-inject datadog-apm-library-python")
	defer s.purgeInjectorDebInstall()

	// OCI mustn't be overridden
	s.assertLDPreloadInstrumented(injectOCIPath)
	s.assertSocketPath()
	s.assertDockerdInstrumented(injectOCIPath)
}

func (s *packageApmInjectSuite) TestVersionBump() {
	s.host.InstallDocker()
	s.RunInstallScript(
		"DD_APM_INSTRUMENTATION_ENABLED=all",
		"DD_APM_INSTRUMENTATION_LIBRARIES=python:2.8.5",
		envForceVersion("datadog-apm-inject", "0.39.0-1"),
	)
	defer s.Purge()
	s.host.WaitForUnitActive(s.T(), "datadog-agent.service", "datadog-agent-trace.service")

	state := s.host.State()
	state.AssertDirExists("/opt/datadog-packages/datadog-apm-library-python/2.8.5", 0755, "root", "root")
	state.AssertSymlinkExists("/opt/datadog-packages/datadog-apm-library-python/stable", "/opt/datadog-packages/datadog-apm-library-python/2.8.5", "root", "root")

	state.AssertDirExists("/opt/datadog-packages/datadog-apm-inject/0.39.0", 0755, "root", "root")
	state.AssertSymlinkExists("/opt/datadog-packages/datadog-apm-inject/stable", "/opt/datadog-packages/datadog-apm-inject/0.39.0", "root", "root")

	s.host.StartExamplePythonApp()
	defer s.host.StopExamplePythonApp()

	traceID := rand.Uint64()
	s.host.CallExamplePythonApp(strconv.FormatUint(traceID, 10))
	s.assertTraceReceived(traceID)

	// Re-run the install script with the latest tracer version
	s.RunInstallScript(
		"DD_APM_INSTRUMENTATION_ENABLED=all",
		"DD_APM_INSTRUMENTATION_LIBRARIES=python:2.9.2",
		envForceVersion("datadog-apm-inject", "0.40.0-1"),
	)
	s.host.WaitForUnitActive(s.T(), "datadog-agent.service", "datadog-agent-trace.service")

	// Today we expect the previous dir to be fully removed and the new one to be symlinked
	state = s.host.State()
	state.AssertPathDoesNotExist("/opt/datadog-packages/datadog-apm-library-python/2.8.5")
	state.AssertDirExists("/opt/datadog-packages/datadog-apm-library-python/2.9.2", 0755, "root", "root")
	state.AssertSymlinkExists("/opt/datadog-packages/datadog-apm-library-python/stable", "/opt/datadog-packages/datadog-apm-library-python/2.9.2", "root", "root")

	state.AssertPathDoesNotExist("/opt/datadog-packages/datadog-apm-inject/0.39.0")
	state.AssertDirExists("/opt/datadog-packages/datadog-apm-inject/0.40.0", 0755, "root", "root")
	state.AssertSymlinkExists("/opt/datadog-packages/datadog-apm-inject/stable", "/opt/datadog-packages/datadog-apm-inject/0.40.0", "root", "root")

	s.host.StartExamplePythonAppInDocker()
	defer s.host.StopExamplePythonAppInDocker()

	traceID = rand.Uint64()
	s.host.CallExamplePythonApp(strconv.FormatUint(traceID, 10))
	traceIDDocker := rand.Uint64()
	s.host.CallExamplePythonAppInDocker(strconv.FormatUint(traceIDDocker, 10))

	s.assertTraceReceived(traceID)
	s.assertTraceReceived(traceIDDocker)
}

func (s *packageApmInjectSuite) TestInstrument() {
	s.RunInstallScript(
		"DD_APM_INSTRUMENTATION_ENABLED=host",
		"DD_APM_INSTRUMENTATION_LIBRARIES=python",
	)
	defer s.Purge()
	s.assertLDPreloadInstrumented(injectOCIPath)
	s.assertSocketPath()
	s.assertDockerdNotInstrumented()

	s.host.InstallDocker()

	_, err := s.Env().RemoteHost.Execute("sudo datadog-installer apm instrument docker")
	assert.NoError(s.T(), err)

	s.assertLDPreloadInstrumented(injectOCIPath)
	s.assertSocketPath()
	s.assertDockerdInstrumented(injectOCIPath)
}

func (s *packageApmInjectSuite) TestPackagePinning() {
	s.host.InstallDocker()

	// Deb install using today's defaults
	s.RunInstallScript(
		"DD_APM_INSTRUMENTATION_ENABLED=all",
		"DD_APM_INSTRUMENTATION_LIBRARIES=python:2.8.5,dotnet",
	)
	defer s.Purge()
	defer s.purgeInjectorDebInstall()

	s.assertLDPreloadInstrumented(injectOCIPath)
	s.assertSocketPath()
	s.assertDockerdInstrumented(injectOCIPath)

	s.host.AssertPackageInstalledByInstaller("datadog-apm-library-python", "datadog-apm-library-dotnet")
	s.host.AssertPackageVersion("datadog-apm-library-python", "2.8.5")
}

func (s *packageApmInjectSuite) TestUninstrument() {
	s.host.InstallDocker()
	s.RunInstallScript(
		"DD_APM_INSTRUMENTATION_ENABLED=all",
		"DD_APM_INSTRUMENTATION_LIBRARIES=python",
	)
	defer s.Purge()

	s.assertLDPreloadInstrumented(injectOCIPath)
	s.assertSocketPath()
	s.assertDockerdInstrumented(injectOCIPath)

	_, err := s.Env().RemoteHost.Execute("sudo datadog-installer apm uninstrument")
	assert.NoError(s.T(), err)

	state := s.host.State()
	state.AssertDirExists("/opt/datadog-packages/datadog-apm-inject/stable", 0755, "root", "root")
	s.assertLDPreloadNotInstrumented()
	s.assertDockerdNotInstrumented()
	if s.os == e2eos.Ubuntu2404 || s.os == e2eos.Debian12 {
		s.assertAppArmorProfile() // AppArmor profile should still be there
	}
}

func (s *packageApmInjectSuite) TestInstrumentScripts() {
	if s.os.Flavor == e2eos.Suse {
		s.T().Skip("Can't install APM deb/rpm packages on Suse, they were never released")
	}
	if s.installMethod == InstallMethodAnsible {
		s.T().Skip("Ansible doesn't support upgrading from OCI to DEB")
	}

	s.host.InstallDocker()

	// Deb install using today's defaults
	s.RunInstallScript(
		"TESTING_APT_URL=",
		"TESTING_APT_REPO_VERSION=",
		"TESTING_YUM_URL=",
		"TESTING_YUM_VERSION_PATH=",
		"DD_REPO_URL=datadoghq.com",
	)
	s.host.Run("sudo apt-get install -y datadog-apm-inject datadog-apm-library-python || sudo yum install -y datadog-apm-inject datadog-apm-library-python")
	defer s.purgeInjectorDebInstall()

	state := s.host.State()
	state.AssertFileExists("/usr/bin/dd-host-install", 0755, "root", "root")
	state.AssertFileExists("/usr/bin/dd-container-install", 0755, "root", "root")
	state.AssertFileExists("/usr/bin/dd-cleanup", 0755, "root", "root")

	// OCI install
	s.RunInstallScript(
		"DD_APM_INSTRUMENTATION_ENABLED=host",
		"DD_APM_INSTRUMENTATION_LIBRARIES=python",
	)
	defer s.Purge()

	// Old commands still work
	s.Env().RemoteHost.MustExecute("dd-host-install --uninstall")
	s.assertLDPreloadNotInstrumented()
	s.Env().RemoteHost.MustExecute("dd-container-install --uninstall")
	s.assertDockerdNotInstrumented()

	// Remove the deb injector, we should still be instrumented
	s.Env().RemoteHost.MustExecute("sudo datadog-installer apm instrument")
	s.Env().RemoteHost.MustExecute("sudo apt-get remove -y --purge datadog-apm-inject || sudo yum remove -y datadog-apm-inject")
	s.assertLDPreloadInstrumented(injectOCIPath)
	s.assertSocketPath()
	s.assertDockerdInstrumented(injectOCIPath)
}

func (s *packageApmInjectSuite) TestInstrumentDockerInactive() {
	s.host.InstallDocker()
	s.Env().RemoteHost.MustExecute("sudo systemctl stop docker")

	s.RunInstallScript("DD_APM_INSTRUMENTATION_ENABLED=all", "DD_APM_INSTRUMENTATION_LIBRARIES=python")
	defer s.Purge()

	s.host.InstallDocker() // Restart docker cleanly

	s.assertLDPreloadInstrumented(injectOCIPath)
	s.assertSocketPath()
	s.assertDockerdInstrumented(injectOCIPath)
}

func (s *packageApmInjectSuite) TestDefaultPackageVersion() {
	s.RunInstallScript(
		"DD_APM_INSTRUMENTATION_ENABLED=host",
		"DD_APM_INSTRUMENTATION_LIBRARIES=python",
	)
	defer s.Purge()
	s.host.AssertPackagePrefix("datadog-apm-library-python", "3")
}

func (s *packageApmInjectSuite) TestInstallWithUmask() {
	oldmask := s.host.SetUmask("0027")
	defer s.host.SetUmask(oldmask)
	s.TestInstall()
}

func (s *packageApmInjectSuite) TestAppArmor() {
	if s.os != e2eos.Ubuntu2404 && s.os != e2eos.Debian12 {
		s.T().Skip("AppArmor not installed by default")
	}
	assert.Contains(s.T(), s.Env().RemoteHost.MustExecute("sudo aa-enabled"), "Yes")
	s.RunInstallScript(
		"DD_APM_INSTRUMENTATION_ENABLED=host",
		"DD_APM_INSTRUMENTATION_LIBRARIES=python",
	)
	defer s.Purge()
	s.assertAppArmorProfile()
	assert.Contains(s.T(), s.Env().RemoteHost.MustExecute("sudo aa-enabled"), "Yes")
	s.Env().RemoteHost.MustExecute("sudo apt update && sudo apt install -y isc-dhcp-client")
	res := s.Env().RemoteHost.MustExecute("sudo DD_APM_INSTRUMENTATION_DEBUG=true /usr/sbin/dhclient 2>&1")
	assert.Contains(s.T(), res, "not injecting")
}

func (s *packageApmInjectSuite) assertTraceReceived(traceID uint64) {
	found := assert.Eventually(s.T(), func() bool {
		tracePayloads, err := s.Env().FakeIntake.Client().GetTraces()
		assert.NoError(s.T(), err)
		for _, tracePayload := range tracePayloads {
			for _, tracerPayload := range tracePayload.TracerPayloads {
				for _, chunk := range tracerPayload.Chunks {
					for _, span := range chunk.Spans {
						if span.TraceID == traceID {
							return true
						}
					}
				}
			}
		}
		return false
	}, time.Second*30, time.Second*1)
	if !found {
		tracePayloads, _ := s.Env().FakeIntake.Client().GetTraces()
		s.T().Logf("Traces received: %v", tracePayloads)
		s.T().Logf("Server logs: %v", s.Env().RemoteHost.MustExecute("cat /tmp/server.log"))
		s.T().Logf("Trace Agent logs: %v", s.Env().RemoteHost.MustExecute("cat /var/log/datadog/trace-agent.log"))
	}
}

// isSystemdPID1 reports whether systemd is the init system (PID 1) on the host.
// This — not the mere presence of the systemctl binary — is what decides whether
// the injector is systemd-managed, and hence whether it uses the reboot-safe
// tmpfs launcher path or writes /etc/ld.so.preload directly with the persistent
// path.
func (s *packageApmInjectSuite) isSystemdPID1() bool {
	_, err := s.Env().RemoteHost.Execute(`test "$(cat /proc/1/comm 2>/dev/null)" = systemd`)
	return err == nil
}

// assertLDPreloadInstrumented checks that /etc/ld.so.preload references the
// launcher for the given injector flavor (injectOCIPath or injectDebPath).
//
// On a systemd-managed host, OCI host instrumentation references the launcher
// through the reboot-safe tmpfs symlink (/run/...): the datadog-apm-inject
// service recreates the symlink on every boot. We then require the tmpfs entry
// AND the absence of the persistent path — a lingering persistent entry would
// survive a reboot and defeat the safety guarantee. Without systemd, OCI falls
// back to writing the persistent path directly. The deb injector always keeps
// its own persistent path.
func (s *packageApmInjectSuite) assertLDPreloadInstrumented(injectorRoot string) {
	content, err := s.host.ReadFile("/etc/ld.so.preload")
	assert.NoError(s.T(), err)

	if injectorRoot == injectOCIPath && s.isSystemdPID1() {
		ociPersistentLauncher := filepath.Join(injectorRoot, "stable", "inject", "launcher.preload.so")
		assert.Contains(s.T(), string(content), injectTmpfsLauncher)
		assert.NotContains(s.T(), string(content), ociPersistentLauncher,
			"systemd-managed OCI host must not keep the persistent launcher path in ld.so.preload")
		return
	}
	assert.Contains(s.T(), string(content), injectorRoot)
}

func (s *packageApmInjectSuite) assertStableConfig(expectedConfigs map[string]interface{}) {
	if len(expectedConfigs) == 0 {
		return
	}

	state := s.host.State()
	state.AssertFileExists("/etc/datadog-agent/application_monitoring.yaml", 0644, "root", "root")
	content, err := s.host.ReadFile("/etc/datadog-agent/application_monitoring.yaml")
	assert.NoError(s.T(), err)

	actualStableConfig := map[string]interface{}{}
	err = yaml.Unmarshal(content, &actualStableConfig)
	assert.NoError(s.T(), err)

	assert.Contains(s.T(), actualStableConfig, "apm_configuration_default")
	assert.Equal(s.T(), expectedConfigs, actualStableConfig["apm_configuration_default"])
}

func (s *packageApmInjectSuite) assertSocketPath() {
	output := s.host.Run("sh -c 'python3 -c \"import os; print(os.environ)\"'")
	assert.Contains(s.T(), output, "'DD_INJECTION_ENABLED': 'tracer'") // this is an env var set by the injector
}

func (s *packageApmInjectSuite) assertLDPreloadNotInstrumented() {
	exists, err := s.host.FileExists("/etc/ld.so.preload")
	assert.NoError(s.T(), err)
	if exists {
		content, err := s.host.ReadFile("/etc/ld.so.preload")
		assert.NoError(s.T(), err)
		assert.NotContains(s.T(), string(content), injectOCIPath)
		assert.NotContains(s.T(), string(content), injectDebPath)
		// Also assert the tmpfs path is gone: a dangling /run entry (e.g. left by a
		// failed instrument-start after a reboot wiped /run) does not contain
		// injectOCIPath, so without this check it would slip through — yet ld.so
		// prints a "cannot be preloaded ... ignored" warning for it on every exec.
		assert.NotContains(s.T(), string(content), injectTmpfsLauncher)
	}
	output := s.host.Run("sh -c 'python3 -c \"import os; print(os.environ)\"'")
	assert.NotContains(s.T(), output, "'DD_INJECTION_ENABLED': 'tracer'")
}

func (s *packageApmInjectSuite) assertDockerdInstrumented(path string) {
	content, err := s.host.ReadFile("/etc/docker/daemon.json")
	assert.NoError(s.T(), err)
	assert.Contains(s.T(), string(content), path)
	runtimeConfig := s.host.GetDockerRuntimePath("dd-shim")
	if path == injectOCIPath {
		assert.Equal(s.T(), runtimeConfig, filepath.Join(path, "stable", "inject", "auto_inject_runc"))
	} else {
		assert.Equal(s.T(), runtimeConfig, filepath.Join(path, "inject", "auto_inject_runc"))
	}
}

func (s *packageApmInjectSuite) assertDockerdNotInstrumented() {
	exists, err := s.host.FileExists("/etc/docker/daemon.json")
	assert.NoError(s.T(), err)
	if exists {
		content, err := s.host.ReadFile("/etc/docker/daemon.json")
		assert.NoError(s.T(), err)
		assert.NotContains(s.T(), string(content), injectOCIPath)
		assert.NotContains(s.T(), string(content), injectDebPath)
	}
	runtimeConfig := s.host.GetDockerRuntimePath("dd-shim")
	assert.Equal(s.T(), runtimeConfig, "")
}

func (s *packageApmInjectSuite) assertAppArmorProfile() {
	content, err := s.host.ReadFile("/etc/apparmor.d/abstractions/datadog.d/injector")
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), string(content), `/opt/datadog-packages/** rix,
/proc/@{pid}/** rix,
/run/datadog/apm.socket rw,`)
	assert.Contains(s.T(), s.Env().RemoteHost.MustExecute("sudo aa-enabled"), "Yes")
}

// TestSystemdService verifies that on a host with systemd, the datadog-apm-inject.service
// is installed and enabled after host instrumentation. The service is not started during
// install; direct instrumentation covers the current boot. The service's ExecStart/ExecStop
// commands (instrument-start/instrument-stop) manage /etc/ld.so.preload on every reboot.
func (s *packageApmInjectSuite) TestSystemdService() {
	if _, err := s.Env().RemoteHost.Execute("test \"$(cat /proc/1/comm 2>/dev/null)\" = systemd"); err != nil {
		s.T().Skip("systemd is not running as PID 1 on this host")
	}

	s.RunInstallScript("DD_APM_INSTRUMENTATION_ENABLED=host", "DD_APM_INSTRUMENTATION_LIBRARIES=python")
	defer s.Purge()

	// After install: service is enabled (will start on next boot) and ld.so.preload is
	// already written by direct instrumentation during install.
	state := s.host.State()
	state.AssertFileExists("/etc/systemd/system/datadog-apm-inject.service", 0644, "root", "root")
	state.AssertUnitsEnabled("datadog-apm-inject.service")
	s.assertLDPreloadInstrumented(injectOCIPath)

	// Verify ExecStop command removes from ld.so.preload
	s.Env().RemoteHost.MustExecute("sudo datadog-installer apm instrument-stop host")
	s.assertLDPreloadNotInstrumented()

	// Verify ExecStart command writes to ld.so.preload
	s.Env().RemoteHost.MustExecute("sudo datadog-installer apm instrument-start host")
	s.assertLDPreloadInstrumented(injectOCIPath)

	// Uninstrumenting removes the service file and clears ld.so.preload
	s.Env().RemoteHost.MustExecute("sudo datadog-installer apm uninstrument host")

	state = s.host.State()
	state.AssertPathDoesNotExist("/etc/systemd/system/datadog-apm-inject.service")
	state.AssertUnitsNotLoaded("datadog-apm-inject.service")
	s.assertLDPreloadNotInstrumented()
}

// crashyConstructorGuardedSrc is a tiny C source compiled into a shared
// library whose ELF constructor calls _exit(1) — but only when the
// library was loaded via the LD_PRELOAD environment variable (i.e., the
// deliberate "is this .so loadable?" probe that verifySharedLib runs via
// `LD_PRELOAD=lib echo 1`). When the library is loaded via
// /etc/ld.so.preload alone, the constructor returns without crashing,
// because /etc/ld.so.preload entries do not propagate into the loading
// process's LD_PRELOAD env var. That selectivity is what makes this
// fixture testable from userspace: every dynamically-linked program
// (bash, sudo, systemctl, ssh-spawned shells) still loads the .so at
// startup via /etc/ld.so.preload, but stays alive because LD_PRELOAD is
// not set in its environment — so the test can actually invoke
// `systemctl stop` after the bad lib is in place.
//
// The unconditional-crash variant (crashyConstructorUnconditionalSrc)
// is what the production bug actually requires for the brick to occur,
// because on the next boot init(1) is launched by the kernel with
// /etc/ld.so.preload still pointing at the bad lib. That variant is
// only safe to use in tests that go through a real reboot — see
// TestSystemdServiceRebootBrokenInjector in
// package_apm_inject_reboot_test.go.
const crashyConstructorGuardedSrc = `#include <stdlib.h>
#include <unistd.h>
__attribute__((constructor)) static void crash(void) {
    const char *p = getenv("LD_PRELOAD");
    if (p && *p) _exit(1);
}
`

// installGCC installs the C compiler used by buildCrashyInjectorSO.
func (s *packageApmInjectSuite) installGCC() {
	s.T().Helper()
	host := s.Env().RemoteHost
	switch s.os.Flavor {
	case e2eos.Ubuntu, e2eos.Debian:
		host.MustExecute("sudo apt-get update -qq && sudo apt-get install -y gcc libc6-dev")
	case e2eos.Suse:
		host.MustExecute("sudo zypper --non-interactive install -y gcc glibc-devel")
	default:
		s.T().Skipf("test does not know how to install gcc on %s", s.os.Flavor)
	}
}

// buildCrashyInjectorSO compiles src as a shared library and places the
// resulting .so at dst. The result is a real, ld.so-loadable ELF — not a
// missing file or a junk-content blob — which matches the user-facing
// failure shape: the library is on disk and looks fine to ld.so, but
// its constructor rejects the verifySharedLib probe. Callers choose
// between the guarded source (safe to leave in /etc/ld.so.preload while
// running tests in-process) and the unconditional source (only safe for
// tests that go through a real reboot before any process tries to use
// the lib).
func (s *packageApmInjectSuite) buildCrashyInjectorSO(dst, src string) {
	s.T().Helper()
	s.installGCC()
	host := s.Env().RemoteHost
	host.MustExecute("sudo tee /tmp/crashy.c >/dev/null <<'CRASHY_EOF'\n" + src + "CRASHY_EOF")
	host.MustExecute("sudo gcc -shared -fPIC -o " + dst + " /tmp/crashy.c")
	//host.MustExecute("sudo chmod 0755 " + dst)
}

// TestSystemdServiceStopBrokenInjector verifies the safety property the unit
// file's no-shell design exists to provide: `systemctl stop
// datadog-apm-inject.service` succeeds at clearing /etc/ld.so.preload even
// when the on-disk launcher.preload.so is a real, ld.so-loadable shared
// object whose ELF constructor crashes any program that LD_PRELOADs it via
// the env var (the probe verifySharedLib uses). ExecStop is
// `<installer> apm instrument-stop host` invoked directly by systemd via
// execve — the static, CGO_ENABLED=0 installer does not consult
// /etc/ld.so.preload, so a broken injector on disk never blocks cleanup.
//
// See crashyConstructorGuardedSrc's commentary for why the fixture
// crashes selectively (via LD_PRELOAD env var only, not via
// /etc/ld.so.preload) rather than unconditionally.
func (s *packageApmInjectSuite) TestSystemdServiceStopBrokenInjector() {
	if !s.isSystemdPID1() {
		s.T().Skip("systemd is not running as PID 1 on this host")
	}

	s.RunInstallScript("DD_APM_INSTRUMENTATION_ENABLED=host", "DD_APM_INSTRUMENTATION_LIBRARIES=python")
	defer s.Purge()

	s.host.WaitForUnitActive(s.T(), "datadog-apm-inject.service")
	s.assertLDPreloadInstrumented(injectOCIPath)

	// Move the original launcher aside (rename, not truncate) so any
	// process that still has it mmapped keeps a valid backing inode,
	// then write a real ELF .so at the same path. /etc/ld.so.preload
	// still references this path — the fixture's getenv guard is what
	// keeps the surrounding system runnable. Restoring the original on
	// cleanup keeps the host sane for Purge() and any retries.
	launcherPath := filepath.Join(injectOCIPath, "stable", "inject", "launcher.preload.so")
	host := s.Env().RemoteHost
	host.MustExecute(fmt.Sprintf("sudo mv %[1]s %[1]s.bak", launcherPath))
	s.buildCrashyInjectorSO(launcherPath, crashyConstructorGuardedSrc)
	defer host.Execute(fmt.Sprintf("sudo mv -f %[1]s.bak %[1]s 2>/dev/null || true", launcherPath)) //nolint:errcheck

	// Confirm the shared object actually crashes the same probe
	// verifySharedLib uses. Without this, a silently-non-crashing fixture
	// would make the rest of the test a misleading no-op (no broken
	// injector → "systemctl stop succeeded" tells us nothing about the
	// safety property).
	out, err := host.Execute(fmt.Sprintf("LD_PRELOAD=%s /bin/true; echo $?", launcherPath))
	require.NoError(s.T(), err, "could not even run the LD_PRELOAD probe — environment is unusable")
	require.Equal(s.T(), "1", strings.TrimSpace(out), "shared object did not crash LD_PRELOAD=lib /bin/true as expected; the rest of this test would be a misleading no-op")

	// ExecStop is exec'd directly against the static installer binary —
	// no /bin/sh wrapper, no dynamic-linker dependency on the broken .so —
	// so it must succeed and clear /etc/ld.so.preload despite the crashy
	// injector sitting at the path /etc/ld.so.preload still references.
	_, err = host.Execute("sudo systemctl stop datadog-apm-inject.service")
	require.NoError(s.T(), err, "systemctl stop must succeed despite the broken injector on disk")

	s.assertLDPreloadNotInstrumented()
}

// TestInstrumentHost_NoSystemd verifies that host instrumentation writes directly to
// /etc/ld.so.preload when systemd is not the init system, without creating a service file.
// This test only runs on hosts where systemd is not PID 1; TestSystemdService covers the systemd path.
//
// NOTE: every flavor in the current e2e matrix (Ubuntu/Debian/RHEL/CentOS/Amazon
// Linux/SUSE) boots systemd as PID 1, so this test SKIPS everywhere today — the
// non-systemd path is not actually exercised in CI. Covering it (and a reboot
// variant, which on a non-systemd host is near-trivial since /etc/ld.so.preload
// is a plain persistent file with no service to clear or recreate it) requires
// adding a non-systemd host to the matrix; tracked separately.
func (s *packageApmInjectSuite) TestInstrumentHost_NoSystemd() {
	if s.isSystemdPID1() {
		s.T().Skip("systemd is PID 1 on this host; TestSystemdService covers that path")
	}

	s.RunInstallScript("DD_APM_INSTRUMENTATION_ENABLED=host", "DD_APM_INSTRUMENTATION_LIBRARIES=python")
	defer s.Purge()

	s.Env().RemoteHost.MustExecute("sudo datadog-installer apm uninstrument host")
	s.assertLDPreloadNotInstrumented()

	s.Env().RemoteHost.MustExecute("sudo datadog-installer apm instrument host")
	s.assertLDPreloadInstrumented(injectOCIPath)

	state := s.host.State()
	state.AssertPathDoesNotExist("/etc/systemd/system/datadog-apm-inject.service")

	s.Env().RemoteHost.MustExecute("sudo datadog-installer apm uninstrument host")
	s.assertLDPreloadNotInstrumented()
}

func (s *packageApmInjectSuite) purgeInjectorDebInstall() {
	s.Env().RemoteHost.MustExecute("sudo rm -f /opt/datadog-packages/run/environment")
	s.Env().RemoteHost.MustExecute("sudo rm -f /etc/datadog-agent/datadog.yaml")

	packageList := []string{
		"datadog-agent",
		"datadog-apm-inject",
		"datadog-apm-library-python",
	}
	s.Env().RemoteHost.Execute(fmt.Sprintf("sudo apt-get remove -y --purge %[1]s || sudo yum remove -y %[1]s", strings.Join(packageList, " ")))
}
