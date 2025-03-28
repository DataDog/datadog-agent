// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dotnettests

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host/windows"
	installer "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/unix"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"

	"testing"
)

type testDotnetLibraryInstallSuiteWithoutIIS struct {
	installerwindows.BaseSuite
}

// TestDotnetInstalls tests the usage of the Datadog installer to install the apm-library-dotnet-package package.
func TestDotnetLibraryInstallsWithoutIIS(t *testing.T) {
	e2e.Run(t, &testDotnetLibraryInstallSuiteWithoutIIS{},
		e2e.WithProvisioner(winawshost.ProvisionerNoAgentNoFakeIntake()))
}

// TestInstallDotnetLibraryPackageWithoutIIS tests installing the Datadog APM Library for .NET using the Datadog installer without IIS installed.
func (s *testDotnetLibraryInstallSuiteWithoutIIS) TestInstallDotnetLibraryPackageWithoutIIS() {
	s.Require().NoError(s.Installer().Install(
		installerwindows.WithMSIDevEnvOverrides("CURRENT_AGENT"),
	))
	defer s.Installer().Purge()

	// TODO: remove override once image is published in prod
	_, err := s.Installer().InstallPackage("datadog-apm-library-dotnet",
		installer.WithVersion("3.13.0-pipeline.58951229.beta.sha-af5a1fab-1"),
		installer.WithRegistry("install.datad0g.com"),
	)
	s.Require().Error(err, "Installing the dotnet library package without IIS should fail")
	// TODO today the package does not get deleted but I think it should
	// s.Require().Host(s.Env().RemoteHost).
	// 	NoDirExists(consts.GetStableDirFor("datadog-apm-library-dotnet"),
	// 		"the package directory should not exist")
}

func (s *testDotnetLibraryInstallSuiteWithoutIIS) TestMSIInstallDotnetLibraryFailsWithoutIIS() {
	version := "3.13.0-pipeline.58951229.beta.sha-af5a1fab-1"
	s.Require().Error(s.Installer().Install(
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_ENABLED=iis"),
		// TODO: remove override once image is published in prod
		// TODO: support DD_INSTALLER_REGISTRY_URL
		installerwindows.WithMSIArg("SITE=datad0g.com"),
		installerwindows.WithMSIArg(fmt.Sprintf("DD_APM_INSTRUMENTATION_LIBRARIES=dotnet:%s", version)),
		installerwindows.WithMSILogFile("install-rollback.log"),
		installerwindows.WithMSIDevEnvOverrides("CURRENT_AGENT"),
	))
	defer s.Installer().Purge()

	s.Require().Host(s.Env().RemoteHost).
		NoDirExists(consts.GetStableDirFor("datadog-apm-library-dotnet"),
			"the package directory should not exist")
}
