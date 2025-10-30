// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package injecttests

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host/windows"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"

	"testing"
)

type testAgentScriptInstallsAPMInject struct {
	baseSuite
}

// TestAgentScriptInstallsAPMInject tests the usage of the install script to install the apm-inject package.
func TestAgentScriptInstallsAPMInject(t *testing.T) {
	e2e.Run(t, &testAgentScriptInstallsAPMInject{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake()))
}

func (s *testAgentScriptInstallsAPMInject) SetupSuite() {
	s.baseSuite.SetupSuite()
}

func (s *testAgentScriptInstallsAPMInject) AfterTest(suiteName, testName string) {
	s.Installer().Purge()
	s.baseSuite.AfterTest(suiteName, testName)
}

// TestInstallFromScript tests the Agent script can install the APM inject package with host instrumentation
func (s *testAgentScriptInstallsAPMInject) TestInstallFromScript() {
	// Act
	s.installCurrentAgentVersionWithAPMInject(
		installerwindows.WithExtraEnvVars(map[string]string{
			"DD_APM_INSTRUMENTATION_ENABLED": "host",
			// TODO: remove override once image is published in prod
			"DD_INSTALLER_REGISTRY_URL":                           "install.datad0g.com.internal.dda-testing.com",
			"DD_INSTALLER_DEFAULT_PKG_VERSION_DATADOG_APM_INJECT": "38f904a56919109d573951718671bdd72f1b4b87",
			"DD_INSTALLER_REGISTRY_URL_DATADOG_APM_INJECT":        "installtesting.datad0g.com",
		}),
	)

	// Verify the package is installed
	s.assertSuccessfulInstall()
}

// installCurrentAgentVersionWithAPMInject installs the current agent version with APM inject via script
func (s *testAgentScriptInstallsAPMInject) installCurrentAgentVersionWithAPMInject(opts ...installerwindows.Option) {
	output, err := s.InstallScript().Run(opts...)
	if s.NoError(err) {
		fmt.Printf("%s\n", output)
	}
	s.Require().NoErrorf(err, "failed to install the Datadog Agent package: %s", output)
	s.Require().NoError(s.WaitForInstallerService("Running"))
	s.Require().Host(s.Env().RemoteHost).
		HasARunningDatadogInstallerService().
		HasARunningDatadogAgentService().
		WithVersionMatchPredicate(func(version string) {
			s.Require().Contains(version, s.CurrentAgentVersion().Version())
		})
}
