// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"fmt"
	"math/rand"
	"time"

	e2eos "github.com/DataDog/test-infra-definitions/components/os"
	"github.com/stretchr/testify/assert"
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
	state.AssertFileExists("/var/run/datadog/installer/environment", 0644, "root", "root")
	state.AssertDirExists("/var/log/datadog/dotnet", 0777, "root", "root")
	state.AssertFileExists("/etc/ld.so.preload", 0644, "root", "root")
	s.assertLDPreloadInstrumented()
	s.assertDockerdInstrumented()

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
	s.assertDockerdConfigNotInstrumented()
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
	s.assertDockerdConfigNotInstrumented()
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
	s.assertDockerdConfigNotInstrumented()
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

func (s *packageApmInjectSuite) assertLDPreloadInstrumented() {
	content, err := s.host.ReadFile("/etc/ld.so.preload")
	assert.NoError(s.T(), err)
	assert.Contains(s.T(), string(content), "/opt/datadog-packages/datadog-apm-inject")
	output := s.host.Run("sh -c 'DD_APM_INSTRUMENTATION_DEBUG=true python3 --version 2>&1'")
	assert.Contains(s.T(), output, "DD_INJECTION_ENABLED=tracer") // this is an env var set by the injector, it should always be in the debug logs
}

func (s *packageApmInjectSuite) assertLDPreloadNotInstrumented() {
	exists, err := s.host.FileExists("/etc/ld.so.preload")
	assert.NoError(s.T(), err)
	if exists {
		content, err := s.host.ReadFile("/etc/ld.so.preload")
		assert.NoError(s.T(), err)
		assert.NotContains(s.T(), string(content), "/opt/datadog-packages/datadog-apm-inject")
	}
	output := s.host.Run("sh -c 'DD_APM_INSTRUMENTATION_DEBUG=true python3 --version 2>&1'")
	assert.NotContains(s.T(), output, "DD_INJECTION_ENABLED=tracer")
}

func (s *packageApmInjectSuite) assertDockerdInstrumented() {
	content, err := s.host.ReadFile("/etc/docker/daemon.json")
	assert.NoError(s.T(), err)
	assert.Contains(s.T(), string(content), "/opt/datadog-packages/datadog-apm-inject")
	runtimeConfig := s.host.Run("sudo docker system info --format '{{json .}}'")
	assert.Contains(s.T(), runtimeConfig, "/opt/datadog-packages/datadog-apm-inject")
}

func (s *packageApmInjectSuite) assertDockerdConfigNotInstrumented() {
	exists, err := s.host.FileExists("/etc/docker/daemon.json")
	assert.NoError(s.T(), err)
	if exists {
		content, err := s.host.ReadFile("/etc/docker/daemon.json")
		assert.NoError(s.T(), err)
		assert.NotContains(s.T(), string(content), "/opt/datadog-packages/datadog-apm-inject")
	}
	runtimeConfig := s.host.Run("sudo docker system info --format '{{json .}}'")
	assert.NotContains(s.T(), runtimeConfig, "/opt/datadog-packages/datadog-apm-inject")
}
