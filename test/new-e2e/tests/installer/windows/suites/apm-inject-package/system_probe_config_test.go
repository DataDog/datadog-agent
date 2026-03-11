// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package injecttests

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
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	windowsAgent "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/agent"
)

type testSystemProbeConfig struct {
	baseSuite
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
	s.baseSuite.AfterTest(suiteName, testName)
}

// TestInstallScriptEnablesSystemProbe tests the setup script path: installing
// via the install script with DD_APM_INSTRUMENTATION_ENABLED=host writes
// system_probe_config.enabled: true in system-probe.yaml before the agent starts.
func (s *testSystemProbeConfig) TestInstallScriptEnablesSystemProbe() {
	output, err := s.InstallScript().Run(
		installerwindows.WithExtraEnvVars(map[string]string{
			"DD_APM_INSTRUMENTATION_ENABLED": "host",
			// TODO: remove override once image is published in prod
			"DD_INSTALLER_REGISTRY_URL":                           "install.datad0g.com",
			"DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_APM_INJECT": s.currentAPMInjectVersion.PackageVersion(),
			"DD_APM_INSTRUMENTATION_LIBRARIES":                    "dotnet:3",
		}),
	)
	if s.NoError(err) {
		fmt.Printf("%s\n", output)
	}
	s.Require().NoErrorf(err, "failed to install: %s", output)
	s.Require().NoError(s.WaitForInstallerService("Running"))
	s.Require().NoError(s.WaitForServicesWithBackoff("Running", []string{"ddinjector"}, backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second))))

	s.assertSystemProbeEnabled()
}

// TestStandaloneInstallEnablesSystemProbe tests the standalone install path:
// the agent is already running, then apm-inject is installed separately with
// DD_APM_INSTRUMENTATION_ENABLED=host. The postInstall hook must write
// system_probe_config.enabled: true into system-probe.yaml.
func (s *testSystemProbeConfig) TestStandaloneInstallEnablesSystemProbe() {
	// Install agent without APM inject
	output, err := s.InstallScript().Run()
	if s.NoError(err) {
		fmt.Printf("%s\n", output)
	}
	s.Require().NoErrorf(err, "failed to install agent: %s", output)
	s.Require().NoError(s.WaitForInstallerService("Running"))
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogInstallerService().
		HasARunningDatadogAgentService()

	// Install apm-inject package separately with host instrumentation
	s.Env().RemoteHost.MustExecute(
		`[Environment]::SetEnvironmentVariable("DD_APM_INSTRUMENTATION_ENABLED", "host", "Process")`,
	)
	pkgOutput, err := s.Installer().InstallPackage("apm-inject-package",
		installer.WithVersion(s.currentAPMInjectVersion.PackageVersion()),
		installer.WithRegistry("install.datad0g.com"),
	)
	s.Require().NoError(err, "failed to install apm-inject package: %s", pkgOutput)
	s.Require().NoError(s.WaitForServicesWithBackoff("Running", []string{"ddinjector"}, backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second))))

	s.assertSystemProbeEnabled()
}

// assertSystemProbeEnabled reads system-probe.yaml and asserts that
// system_probe_config.enabled is true.
func (s *testSystemProbeConfig) assertSystemProbeEnabled() {
	host := s.Env().RemoteHost
	configRoot, err := windowsAgent.GetConfigRootFromRegistry(host)
	s.Require().NoError(err)
	configPath := filepath.Join(configRoot, "system-probe.yaml")

	configBytes, err := host.ReadFile(configPath)
	s.Require().NoErrorf(err, "failed to read system-probe.yaml")

	cfg := map[string]interface{}{}
	s.Require().NoError(yaml.Unmarshal(configBytes, &cfg), "failed to unmarshal system-probe.yaml")

	spCfg, ok := cfg["system_probe_config"].(map[string]interface{})
	s.Require().True(ok, "system_probe_config key missing in system-probe.yaml, got: %v", cfg)
	s.Require().Equal(true, spCfg["enabled"], "system_probe_config.enabled should be true")
}
