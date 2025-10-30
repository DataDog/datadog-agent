// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package injecttests

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host/windows"
	installer "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/unix"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"

	"testing"
)

type testAPMInjectInstallSuite struct {
	baseSuite
}

// TestAPMInjectInstalls tests the usage of the Datadog installer to install the apm-inject package.
func TestAPMInjectInstalls(t *testing.T) {
	e2e.Run(t, &testAPMInjectInstallSuite{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake()))
}

func (s *testAPMInjectInstallSuite) BeforeTest(suiteName, testName string) {
	s.baseSuite.BeforeTest(suiteName, testName)
	s.Require().NoError(s.Installer().Install(installerwindows.WithMSILogFile(testName + "-msiinstall.log")))
}

func (s *testAPMInjectInstallSuite) AfterTest(suiteName, testName string) {
	s.Installer().Purge()
	s.baseSuite.AfterTest(suiteName, testName)
}

// TestInstallUninstallAPMInjectPackage tests installing and uninstalling the Datadog APM Inject package using the Datadog installer.
func (s *testAPMInjectInstallSuite) TestInstallUninstallAPMInjectPackage() {
	s.installAPMInject()

	// Verify the driver is installed
	s.verifyDriverInstalled()

	s.removeAPMInject()

	// Verify the driver is uninstalled
	s.verifyDriverNotInstalled()

	s.Require().Host(s.Env().RemoteHost).
		NoDirExists(consts.GetStableDirFor("datadog-apm-inject"),
			"the package directory should be removed")
}

func (s *testAPMInjectInstallSuite) TestReinstall() {
	s.installAPMInject()

	s.verifyDriverInstalled()

	s.installAPMInject()

	s.verifyDriverInstalled()
}

func (s *testAPMInjectInstallSuite) installAPMInject() {
	// TODO: Update version when package is published
	output, err := s.Installer().InstallPackage("datadog-apm-inject",
		installer.WithVersion("latest"),
		installer.WithRegistry("install.datad0g.com.internal.dda-testing.com"),
	)
	s.Require().NoErrorf(err, "failed to install the apm-inject package: %s", output)
}

func (s *testAPMInjectInstallSuite) removeAPMInject() {
	output, err := s.Installer().RemovePackage("datadog-apm-inject")
	s.Require().NoErrorf(err, "failed to remove the apm-inject package: %s", output)
}
