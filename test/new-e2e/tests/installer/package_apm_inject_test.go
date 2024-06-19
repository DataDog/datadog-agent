// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"fmt"
	"math/rand"
	"path/filepath"
	"strings"
	"time"

	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/stretchr/testify/assert"
)

const (
	injectOCIPath = "/opt/datadog-packages/datadog-apm-inject"
	injectDebPath = "/opt/datadog/apm"
)

type packageApmInjectSuite struct {
	packageBaseSuite
}

func testApmInjectAgent(os e2eos.Descriptor, arch e2eos.Architecture) packageSuite {
	return &packageApmInjectSuite{
		packageBaseSuite: newPackageSuite("apm-inject", os, arch),
	}
}

func (s *packageApmInjectSuite) TestInstall() {
	s.host.InstallDocker()
	s.RunInstallScript("DD_APM_INSTRUMENTATION_ENABLED=all", "DD_APM_INSTRUMENTATION_LIBRARIES=python", envForceInstall("datadog-agent"), envForceInstall("datadog-apm-inject"), envForceInstall("datadog-apm-library-python"))
	defer s.Purge()
	s.host.WaitForUnitActive("datadog-agent.service", "datadog-agent-trace.service")

	s.host.StartExamplePythonApp()
	defer s.host.StopExamplePythonApp()
	s.host.StartExamplePythonAppInDocker()
	defer s.host.StopExamplePythonAppInDocker()

	s.host.AssertPackageInstalledByInstaller("datadog-agent", "datadog-apm-inject", "datadog-apm-library-python")
	s.host.AssertPackageNotInstalledByPackageManager("datadog-agent", "datadog-apm-inject", "datadog-apm-library-python")
	state := s.host.State()
	state.AssertFileExists("/var/run/datadog-installer/environment", 0644, "root", "root")
	state.AssertDirExists("/var/log/datadog/dotnet", 0777, "root", "root")
	state.AssertFileExists("/etc/ld.so.preload", 0644, "root", "root")
	state.AssertFileExists("/usr/bin/dd-host-install", 0755, "root", "root")
	state.AssertFileExists("/usr/bin/dd-container-install", 0755, "root", "root")
	s.assertLDPreloadInstrumented(injectOCIPath)
	s.assertSocketPath("/var/run/datadog-installer/apm.socket")
	s.assertDockerdInstrumented(injectOCIPath)

	traceID := rand.Uint64()
	s.host.CallExamplePythonApp(fmt.Sprint(traceID))
	traceIDDocker := rand.Uint64()
	s.host.CallExamplePythonAppInDocker(fmt.Sprint(traceIDDocker))

	s.assertTraceReceived(traceID)
	s.assertTraceReceived(traceIDDocker)
}

func (s *packageApmInjectSuite) TestUninstall() {
	s.host.InstallDocker()
	s.RunInstallScript("DD_APM_INSTRUMENTATION_ENABLED=all", "DD_APM_INSTRUMENTATION_LIBRARIES=python", envForceInstall("datadog-agent"), envForceInstall("datadog-apm-inject"), envForceInstall("datadog-apm-library-python"))
	s.Purge()

	s.assertLDPreloadNotInstrumented()
	s.assertDockerdNotInstrumented()

	state := s.host.State()
	state.AssertPathDoesNotExist("/usr/bin/dd-host-install")
	state.AssertPathDoesNotExist("/usr/bin/dd-container-install")
}

func (s *packageApmInjectSuite) TestDockerAdditionalFields() {
	s.host.InstallDocker()
	// Broken /etc/docker/daemon.json syntax
	s.host.SetBrokenDockerConfig()
	defer s.host.RemoveBrokenDockerConfig()
	err := s.RunInstallScriptWithError("DD_APM_INSTRUMENTATION_ENABLED=all", "DD_APM_INSTRUMENTATION_LIBRARIES=python", envForceInstall("datadog-agent"), envForceInstall("datadog-apm-inject"), envForceInstall("datadog-apm-library-python"))
	defer s.Purge()

	assert.Error(s.T(), err)
	s.assertLDPreloadNotInstrumented()
	s.assertDockerdNotInstrumented()
}

func (s *packageApmInjectSuite) TestDockerBrokenJSON() {
	s.host.InstallDocker()
	// Additional fields in /etc/docker/daemon.json
	s.host.SetBrokenDockerConfigAdditionalFields()
	defer s.host.RemoveBrokenDockerConfig()
	err := s.RunInstallScriptWithError("DD_APM_INSTRUMENTATION_ENABLED=all", "DD_APM_INSTRUMENTATION_LIBRARIES=python", envForceInstall("datadog-agent"), envForceInstall("datadog-apm-inject"), envForceInstall("datadog-apm-library-python"))
	defer s.Purge()

	assert.Error(s.T(), err)
	s.assertLDPreloadNotInstrumented()
	s.assertDockerdNotInstrumented()
}

func (s *packageApmInjectSuite) TestInstrumentDocker() {
	s.host.InstallDocker()
	err := s.RunInstallScriptWithError("DD_APM_INSTRUMENTATION_ENABLED=docker", "DD_APM_INSTRUMENTATION_LIBRARIES=python", envForceInstall("datadog-agent"), envForceInstall("datadog-apm-inject"), envForceInstall("datadog-apm-library-python"))
	defer s.Purge()

	assert.NoError(s.T(), err)
	s.assertLDPreloadNotInstrumented()
	s.assertDockerdInstrumented(injectOCIPath)
}

func (s *packageApmInjectSuite) TestInstrumentHost() {
	err := s.RunInstallScriptWithError("DD_APM_INSTRUMENTATION_ENABLED=host", "DD_APM_INSTRUMENTATION_LIBRARIES=python", envForceInstall("datadog-agent"), envForceInstall("datadog-apm-inject"), envForceInstall("datadog-apm-library-python"))
	defer s.Purge()

	assert.NoError(s.T(), err)
	s.assertLDPreloadInstrumented(injectOCIPath)
	s.assertDockerdNotInstrumented()
}

func (s *packageApmInjectSuite) TestInstrumentDefault() {
	s.host.InstallDocker()
	err := s.RunInstallScriptWithError("DD_APM_INSTRUMENTATION_LIBRARIES=python", envForceInstall("datadog-agent"), envForceInstall("datadog-apm-inject"), envForceInstall("datadog-apm-library-python"))
	defer s.Purge()

	assert.NoError(s.T(), err)
	s.assertLDPreloadInstrumented(injectOCIPath)
	s.assertDockerdInstrumented(injectOCIPath)
}

// TestUpgrade_InjectorDeb_To_InjectorOCI tests the upgrade from the DEB injector to the OCI injector.
// Library package is OCI.
func (s *packageApmInjectSuite) TestUpgrade_InjectorDeb_To_InjectorOCI() {
	s.host.InstallDocker()

	// Deb install using today's defaults
	s.RunInstallScript(
		"DD_APM_INSTRUMENTATION_ENABLED=all",
		"DD_APM_INSTRUMENTATION_LIBRARIES=python",
		envForceNoInstall("datadog-apm-inject"),
		envForceNoInstall("datadog-apm-library-python"),
		envForceInstall("datadog-agent"),
		"TESTING_APT_URL=",
		"TESTING_APT_REPO_VERSION=",
		"TESTING_YUM_URL=",
		"TESTING_YUM_VERSION_PATH=",
		"DD_REPO_URL=datadoghq.com",
	)
	defer s.Purge()
	defer s.purgeInjectorDebInstall()

	s.assertLDPreloadInstrumented(injectDebPath)
	s.assertSocketPath("/opt/datadog/apm/inject/run/apm.socket")
	s.assertDockerdInstrumented(injectDebPath)

	// OCI install
	s.RunInstallScriptWithError(
		"DD_APM_INSTRUMENTATION_ENABLED=all",
		"DD_APM_INSTRUMENTATION_LIBRARIES=python",
		envForceInstall("datadog-apm-inject"),
		envForceInstall("datadog-apm-library-python"),
		envForceInstall("datadog-agent"),
	)

	s.assertLDPreloadInstrumented(injectOCIPath)
	s.assertSocketPath("/opt/datadog/apm/inject/run/apm.socket") // Socket path mustn't change
	s.assertDockerdInstrumented(injectOCIPath)

}

// TestUpgrade_InjectorOCI_To_InjectorDeb tests the upgrade from the OCI injector to the DEB injector.
// Library package is OCI.
func (s *packageApmInjectSuite) TestUpgrade_InjectorOCI_To_InjectorDeb() {
	s.host.InstallDocker()

	// OCI install
	s.RunInstallScriptWithError(
		"DD_APM_INSTRUMENTATION_ENABLED=all",
		"DD_APM_INSTRUMENTATION_LIBRARIES=python",
		envForceInstall("datadog-apm-inject"),
		envForceInstall("datadog-apm-library-python"),
		envForceInstall("datadog-agent"),
	)
	defer s.Purge()

	s.assertLDPreloadInstrumented(injectOCIPath)
	s.assertSocketPath("/var/run/datadog-installer/apm.socket")
	s.assertDockerdInstrumented(injectOCIPath)

	// Deb install using today's defaults
	s.RunInstallScriptWithError(
		"DD_APM_INSTRUMENTATION_ENABLED=all",
		"DD_APM_INSTRUMENTATION_LIBRARIES=python",
		envForceNoInstall("datadog-apm-inject"),
		envForceNoInstall("datadog-apm-library-python"),
		envForceInstall("datadog-agent"),
		"TESTING_APT_URL=",
		"TESTING_APT_REPO_VERSION=",
		"TESTING_YUM_URL=",
		"TESTING_YUM_VERSION_PATH=",
		"DD_REPO_URL=datadoghq.com",
	)
	defer s.purgeInjectorDebInstall()

	// OCI musn't be overridden
	s.assertLDPreloadInstrumented(injectOCIPath)
	s.assertSocketPath("/var/run/datadog-installer/apm.socket")
	s.assertDockerdInstrumented(injectOCIPath)
}

func (s *packageApmInjectSuite) TestVersionBump() {
	s.host.InstallDocker()
	s.RunInstallScript(
		"DD_APM_INSTRUMENTATION_ENABLED=all",
		"DD_APM_INSTRUMENTATION_LIBRARIES=python",
		envForceInstall("datadog-agent"),
		envForceInstall("datadog-apm-inject"),
		envForceVersion("datadog-apm-inject", "0.14.0-beta1-dev.b0d6e40.glci528580195.g068abe2b-1"),
		envForceInstall("datadog-apm-library-python"),
		envForceVersion("datadog-apm-library-python", "2.8.2-dev-1"),
	)
	defer s.Purge()
	s.host.WaitForUnitActive("datadog-agent.service", "datadog-agent-trace.service")

	state := s.host.State()
	state.AssertDirExists("/opt/datadog-packages/datadog-apm-library-python/2.8.2-dev", 0755, "root", "root")
	state.AssertSymlinkExists("/opt/datadog-packages/datadog-apm-library-python/stable", "/opt/datadog-packages/datadog-apm-library-python/2.8.2-dev", "root", "root")

	state.AssertDirExists("/opt/datadog-packages/datadog-apm-inject/0.14.0-beta1-dev.b0d6e40.glci528580195.g068abe2b-1", 0755, "root", "root")
	state.AssertSymlinkExists("/opt/datadog-packages/datadog-apm-inject/stable", "/opt/datadog-packages/datadog-apm-inject/0.14.0-beta1-dev.b0d6e40.glci528580195.g068abe2b-1", "root", "root")

	s.host.StartExamplePythonApp()
	defer s.host.StopExamplePythonApp()

	traceID := rand.Uint64()
	s.host.CallExamplePythonApp(fmt.Sprint(traceID))
	s.assertTraceReceived(traceID)

	// Re-run the install script with the latest tracer version
	s.RunInstallScript(
		"DD_APM_INSTRUMENTATION_ENABLED=all",
		"DD_APM_INSTRUMENTATION_LIBRARIES=python",
		envForceInstall("datadog-agent"),
		envForceInstall("datadog-apm-inject"),
		envForceVersion("datadog-apm-inject", "0.13.2-beta1-dev.b0d6e40.glci530444874.g6d8b7576-1"),
		envForceInstall("datadog-apm-library-python"),
		envForceVersion("datadog-apm-library-python", "2.8.5-1"),
	)

	// Today we expect the previous dir to be fully removed and the new one to be symlinked
	state = s.host.State()
	state.AssertPathDoesNotExist("/opt/datadog-packages/datadog-apm-library-python/2.8.2-dev")
	state.AssertDirExists("/opt/datadog-packages/datadog-apm-library-python/2.8.5", 0755, "root", "root")
	state.AssertSymlinkExists("/opt/datadog-packages/datadog-apm-library-python/stable", "/opt/datadog-packages/datadog-apm-library-python/2.8.5", "root", "root")

	state.AssertPathDoesNotExist("/opt/datadog-packages/datadog-apm-inject/0.14.0-beta1-dev.b0d6e40.glci528580195.g068abe2b-1")
	state.AssertDirExists("/opt/datadog-packages/datadog-apm-inject/0.13.2-beta1-dev.b0d6e40.glci530444874.g6d8b7576-1", 0755, "root", "root")
	state.AssertSymlinkExists("/opt/datadog-packages/datadog-apm-inject/stable", "/opt/datadog-packages/datadog-apm-inject/0.13.2-beta1-dev.b0d6e40.glci530444874.g6d8b7576-1", "root", "root")

	s.host.StartExamplePythonAppInDocker()
	defer s.host.StopExamplePythonAppInDocker()

	traceID = rand.Uint64()
	s.host.CallExamplePythonApp(fmt.Sprint(traceID))
	traceIDDocker := rand.Uint64()
	s.host.CallExamplePythonAppInDocker(fmt.Sprint(traceIDDocker))

	s.assertTraceReceived(traceID)
	s.assertTraceReceived(traceIDDocker)
}

func (s *packageApmInjectSuite) TestInstrument() {
	s.RunInstallScript(
		"DD_APM_INSTRUMENTATION_ENABLED=host",
		"DD_APM_INSTRUMENTATION_LIBRARIES=python",
		envForceInstall("datadog-agent"),
		envForceInstall("datadog-apm-inject"),
		envForceInstall("datadog-apm-library-python"),
	)
	defer s.Purge()
	s.assertLDPreloadInstrumented(injectOCIPath)
	s.assertSocketPath("/var/run/datadog-installer/apm.socket")
	s.assertDockerdNotInstrumented()

	s.host.InstallDocker()

	_, err := s.Env().RemoteHost.Execute("sudo datadog-installer apm instrument docker")
	assert.NoError(s.T(), err)

	s.assertLDPreloadInstrumented(injectOCIPath)
	s.assertSocketPath("/var/run/datadog-installer/apm.socket")
	s.assertDockerdInstrumented(injectOCIPath)
}

func (s *packageApmInjectSuite) TestPackagePinning() {
	// Deb install using today's defaults
	err := s.RunInstallScriptWithError(
		"DD_APM_INSTRUMENTATION_ENABLED=all",
		"DD_APM_INSTRUMENTATION_LIBRARIES=python:2.8.2-dev,dotnet",
		envForceInstall("datadog-apm-inject"),
		envForceInstall("datadog-apm-library-python"),
		envForceInstall("datadog-apm-library-dotnet"),
		envForceInstall("datadog-agent"),
	)
	defer s.Purge()
	defer s.purgeInjectorDebInstall()
	assert.NoError(s.T(), err)

	s.assertLDPreloadInstrumented(injectOCIPath)
	s.assertSocketPath("/var/run/datadog-installer/apm.socket")
	s.assertDockerdInstrumented(injectOCIPath)

	s.host.AssertPackageInstalledByInstaller("datadog-apm-library-python", "datadog-apm-library-dotnet")
	s.host.AssertPackageVersion("datadog-apm-library-python", "2.8.2-dev")
}

func (s *packageApmInjectSuite) TestUninstrument() {
	s.host.InstallDocker()
	s.RunInstallScriptWithError(
		"DD_APM_INSTRUMENTATION_ENABLED=all",
		"DD_APM_INSTRUMENTATION_LIBRARIES=python",
		envForceInstall("datadog-agent"),
		envForceInstall("datadog-apm-inject"),
		envForceInstall("datadog-apm-library-python"),
	)
	defer s.Purge()

	s.assertLDPreloadInstrumented(injectOCIPath)
	s.assertSocketPath("/var/run/datadog-installer/apm.socket")
	s.assertDockerdInstrumented(injectOCIPath)

	_, err := s.Env().RemoteHost.Execute("sudo datadog-installer apm uninstrument")
	assert.NoError(s.T(), err)

	state := s.host.State()
	state.AssertDirExists("/opt/datadog-packages/datadog-apm-inject/stable", 0755, "root", "root")
	s.assertLDPreloadNotInstrumented()
	s.assertDockerdNotInstrumented()
}

func (s *packageApmInjectSuite) TestInstrumentScripts() {
	s.host.InstallDocker()

	// Deb install using today's defaults
	s.RunInstallScript(
		"DD_APM_INSTRUMENTATION_ENABLED=host",
		"DD_APM_INSTRUMENTATION_LIBRARIES=python",
		envForceNoInstall("datadog-apm-inject"),
		envForceNoInstall("datadog-apm-library-python"),
		envForceInstall("datadog-agent"),
		"TESTING_APT_URL=",
		"TESTING_APT_REPO_VERSION=",
		"TESTING_YUM_URL=",
		"TESTING_YUM_VERSION_PATH=",
		"DD_REPO_URL=datadoghq.com",
	)
	defer s.Purge()
	defer s.purgeInjectorDebInstall()

	state := s.host.State()
	state.AssertFileExists("/usr/bin/dd-host-install", 0755, "root", "root")
	state.AssertFileExists("/usr/bin/dd-container-install", 0755, "root", "root")
	state.AssertFileExists("/usr/bin/dd-cleanup", 0755, "root", "root")

	// OCI install
	s.RunInstallScript(
		"DD_APM_INSTRUMENTATION_ENABLED=host",
		"DD_APM_INSTRUMENTATION_LIBRARIES=python",
		envForceInstall("datadog-agent"),
		envForceInstall("datadog-apm-inject"),
		envForceInstall("datadog-apm-library-python"),
	)

	// Old commands still work
	s.Env().RemoteHost.MustExecute("dd-host-install --uninstall")
	s.assertLDPreloadNotInstrumented()
	s.Env().RemoteHost.MustExecute("dd-container-install --uninstall")
	s.assertDockerdNotInstrumented()

	// Remove the deb injector, we should still be instrumented
	s.Env().RemoteHost.MustExecute("sudo datadog-installer apm instrument")
	s.Env().RemoteHost.MustExecute("sudo apt-get remove -y --purge datadog-apm-inject || sudo yum remove -y datadog-apm-inject")
	s.assertLDPreloadInstrumented(injectOCIPath)
	s.assertSocketPath("/opt/datadog/apm/inject/run/apm.socket")
	s.assertDockerdInstrumented(injectOCIPath)
}

func (s *packageApmInjectSuite) TestInstrumentDockerInactive() {
	s.host.InstallDocker()
	s.Env().RemoteHost.MustExecute("sudo systemctl stop docker")

	s.RunInstallScript("DD_APM_INSTRUMENTATION_ENABLED=all", "DD_APM_INSTRUMENTATION_LIBRARIES=python", envForceInstall("datadog-agent"), envForceInstall("datadog-apm-inject"), envForceInstall("datadog-apm-library-python"))
	defer s.Purge()

	s.Env().RemoteHost.MustExecute("sudo systemctl start docker")

	s.assertLDPreloadInstrumented(injectOCIPath)
	s.assertSocketPath("/var/run/datadog-installer/apm.socket")
	s.assertDockerdInstrumented(injectOCIPath)
}

func (s *packageApmInjectSuite) TestInstallDependencies() {
	s.RunInstallScript()
	defer s.Purge()
	s.host.AssertPackageNotInstalledByPackageManager("datadog-apm-inject")
	s.Env().RemoteHost.MustExecute("sudo datadog-installer install oci://datadoghq.com/datadog-apm-library-python:2.8.2-dev")
	s.host.AssertPackageNotInstalledByPackageManager("datadog-apm-library-python")
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

func (s *packageApmInjectSuite) assertLDPreloadInstrumented(libPath string) {
	content, err := s.host.ReadFile("/etc/ld.so.preload")
	assert.NoError(s.T(), err)
	assert.Contains(s.T(), string(content), libPath)
}

func (s *packageApmInjectSuite) assertSocketPath(path string) {
	output := s.host.Run("sh -c 'DD_APM_INSTRUMENTATION_DEBUG=true python3 --version 2>&1'")
	assert.Contains(s.T(), output, "DD_INJECTION_ENABLED=tracer") // this is an env var set by the injector, it should always be in the debug logs
	assert.Contains(s.T(), output, fmt.Sprintf("\"DD_TRACE_AGENT_URL=unix://%s\"", path))
}

func (s *packageApmInjectSuite) assertLDPreloadNotInstrumented() {
	exists, err := s.host.FileExists("/etc/ld.so.preload")
	assert.NoError(s.T(), err)
	if exists {
		content, err := s.host.ReadFile("/etc/ld.so.preload")
		assert.NoError(s.T(), err)
		assert.NotContains(s.T(), string(content), injectOCIPath)
		assert.NotContains(s.T(), string(content), injectDebPath)
	}
	output := s.host.Run("sh -c 'DD_APM_INSTRUMENTATION_DEBUG=true python3 --version 2>&1'")
	assert.NotContains(s.T(), output, "DD_INJECTION_ENABLED=tracer")
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

func (s *packageApmInjectSuite) purgeInjectorDebInstall() {
	s.Env().RemoteHost.MustExecute("sudo rm -f /var/run/datadog-installer/environment")
	s.Env().RemoteHost.MustExecute("sudo rm -f /etc/datadog-agent/datadog.yaml")

	packageList := []string{
		"datadog-agent",
		"datadog-apm-inject",
		"datadog-apm-library-java",
		"datadog-apm-library-ruby",
		"datadog-apm-library-js",
		"datadog-apm-library-dotnet",
		"datadog-apm-library-python",
	}
	s.Env().RemoteHost.Execute(fmt.Sprintf("sudo apt-get remove -y --purge %[1]s || sudo yum remove -y %[1]s", strings.Join(packageList, " ")))
}
