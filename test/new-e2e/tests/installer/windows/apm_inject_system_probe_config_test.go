// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !e2eunit

package installer

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v5"
	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	installer "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/unix"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

type testSystemProbeConfig struct {
	baseAPMInjectSuite
}

// TestSystemProbeConfig tests that system-probe is enabled when apm-inject
// is installed with host instrumentation.
func TestSystemProbeConfig(t *testing.T) {
	e2e.Run(t, &testSystemProbeConfig{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake()))
}

func (s *testSystemProbeConfig) AfterTest(suiteName, testName string) {
	s.Installer().Purge()
	// Purge does not remove config files; wipe the config directory so that
	// leftover system-probe.yaml does not leak between tests.
	_, _ = s.Env().RemoteHost.Execute(`Remove-Item -Path 'C:\ProgramData\Datadog\*' -Recurse -Force`)
	s.BaseSuite.AfterTest(suiteName, testName)
}

func (s *testSystemProbeConfig) TestInstallScriptStartsSystemProbe() {
	output, err := s.InstallScript().Run(
		WithExtraEnvVars(map[string]string{
			"DD_APM_INSTRUMENTATION_ENABLED":                      "host",
			"DD_INSTALLER_REGISTRY_URL":                           "install.datad0g.com",
			"DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_APM_INJECT": s.currentAPMInjectVersion.PackageVersion(),
			"DD_APM_INSTRUMENTATION_LIBRARIES":                    "dotnet:3",
		}),
	)
	if s.NoError(err) {
		fmt.Printf("%s\n", output)
	}
	s.assertSystemProbeEnabled()
	s.Require().NoError(
		s.WaitForServicesWithBackoff("Running", []string{"datadog-system-probe"},
			backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second))),
		"system-probe service should be running after install script",
	)
}

func (s *testSystemProbeConfig) TestStandaloneInstallDoesNotStartSystemProbe() {
	// Install agent without APM inject
	output, err := s.InstallScript().Run()
	if s.NoError(err) {
		fmt.Printf("%s\n", output)
	}

	// Install apm-inject package separately
	pkgOutput, err := s.Installer().InstallPackage("apm-inject-package",
		installer.WithVersion(s.currentAPMInjectVersion.PackageVersion()),
		installer.WithRegistry("install.datad0g.com"),
	)
	s.Require().NoError(err, "failed to install apm-inject package: %s", pkgOutput)

	// We don't restart the agent so the system probe will only be enabled at the next agent restart
	s.assertSystemProbeEnabled()
	status, err := windowsCommon.GetServiceStatus(s.Env().RemoteHost, "datadog-system-probe")
	s.Require().NoError(err)
	s.Require().NotEqual("Running", status,
		"system-probe service should not be running after standalone apm-inject install")
}

// assertSystemProbeEnabled reads system-probe.yaml and asserts that
// windows_crash_detection.enabled is true.
func (s *testSystemProbeConfig) assertSystemProbeEnabled() {
	host := s.Env().RemoteHost
	configRoot, err := windowsAgent.GetConfigRootFromRegistry(host)
	s.Require().NoError(err)
	configPath := filepath.Join(configRoot, "system-probe.yaml")

	configBytes, err := host.ReadFile(configPath)
	s.Require().NoErrorf(err, "failed to read system-probe.yaml")

	cfg := map[string]interface{}{}
	s.Require().NoError(yaml.Unmarshal(configBytes, &cfg), "failed to unmarshal system-probe.yaml")

	wcdCfg, ok := cfg["windows_crash_detection"].(map[string]interface{})
	s.Require().True(ok, "windows_crash_detection key missing in system-probe.yaml, got: %v", cfg)
	s.Require().Equal(true, wcdCfg["enabled"], "windows_crash_detection.enabled should be true")
}
