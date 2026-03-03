// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package injecttests

import (
	"time"

	"github.com/cenkalti/backoff/v5"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	installer "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/unix"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"

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
	s.Require().NoError(s.Installer().Install(
		installerwindows.WithMSILogFile(testName+"-msiinstall.log"),
		installerwindows.WithMSIArg("DD_REMOTE_UPDATES=true"),
	))
}

func (s *testAPMInjectInstallSuite) AfterTest(suiteName, testName string) {
	s.Installer().Purge()
	s.baseSuite.AfterTest(suiteName, testName)
}

// TestInstallAPMInjectPackage tests the usage of the Datadog installer to install the apm-inject package.
func (s *testAPMInjectInstallSuite) TestExperiment() {
	initialVersion := s.previousAPMInjectVersion.PackageVersion()
	upgradeVersion := s.currentAPMInjectVersion.PackageVersion()

	// install initial version
	output, err := s.Installer().InstallPackage("apm-inject-package",
		installer.WithVersion(initialVersion),
		installer.WithRegistry("install.datad0g.com"),
	)
	s.Require().NoError(err, "failed to install the apm-inject package: %s", output)

	// verify the driver is running
	s.Require().NoError(s.WaitForServicesWithBackoff("Running", []string{"ddinjector"}, backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second))))

	// start experiment
	output, err = s.Installer().InstallExperiment("apm-inject-package",
		installer.WithVersion(upgradeVersion),
		installer.WithRegistry("install.datad0g.com"),
	)
	s.Require().NoError(err, "failed to start the apm-inject experiment: %s", output)

	// promote experiment
	output, err = s.Installer().PromoteExperiment("datadog-apm-inject")
	s.Require().NoError(err, "failed to promote the apm-inject experiment: %s", output)

	// verify the driver is running post promote
	s.Require().NoError(s.WaitForServicesWithBackoff("Running", []string{"ddinjector"}, backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second))))
}

// TestInstallAPMInjectPackage tests the usage of the Datadog installer to install the apm-inject package.
func (s *testAPMInjectInstallSuite) TestStopExperiment() {
	initialVersion := s.previousAPMInjectVersion.PackageVersion()
	upgradeVersion := s.currentAPMInjectVersion.PackageVersion()

	// install initial version
	output, err := s.Installer().InstallPackage("apm-inject-package",
		installer.WithVersion(initialVersion),
		installer.WithRegistry("install.datad0g.com"),
	)
	s.Require().NoError(err, "failed to install the apm-inject package: %s", output)

	// verify the driver is running
	s.Require().NoError(s.WaitForServicesWithBackoff("Running", []string{"ddinjector"}, backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second))))

	// start experiment
	output, err = s.Installer().InstallExperiment("apm-inject-package",
		installer.WithVersion(upgradeVersion),
		installer.WithRegistry("install.datad0g.com"),
	)
	s.Require().NoError(err, "failed to start the apm-inject experiment: %s", output)

	// stop experiment
	output, err = s.Installer().StopExperiment("datadog-apm-inject")
	s.Require().NoError(err, "failed to stop the apm-inject experiment: %s", output)

	// verify the driver is running as we should be on stable now
	s.Require().NoError(s.WaitForServicesWithBackoff("Running", []string{"ddinjector"}, backoff.WithBackOff(backoff.NewConstantBackOff(30*time.Second))))
}
