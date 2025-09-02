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
	s.Require().NoError(s.Installer().Install())
	defer s.Installer().Purge()

	// TODO: remove override once image is published in prod
	_, err := s.Installer().InstallPackage("datadog-apm-library-dotnet",
		installer.WithVersion("3.19.0-pipeline.67351320.beta.sha-c05ddfb1-1"),
		installer.WithRegistry("install.datad0g.com.internal.dda-testing.com"),
	)
	s.Require().NoError(err, "Installing the dotnet library package without IIS should not fail")
}

func (s *testDotnetLibraryInstallSuiteWithoutIIS) TestMSIInstallDotnetLibraryFailsWithoutIIS() {
	version := "3.19.0-pipeline.67351320.beta.sha-c05ddfb1-1"
	s.Require().NoError(s.Installer().Install(
		installerwindows.WithMSIArg("DD_APM_INSTRUMENTATION_ENABLED=iis"),
		// TODO: remove override once image is published in prod
		installerwindows.WithMSIArg("DD_INSTALLER_REGISTRY_URL=install.datad0g.com.internal.dda-testing.com"),
		installerwindows.WithMSIArg(fmt.Sprintf("DD_APM_INSTRUMENTATION_LIBRARIES=dotnet:%s", version)),
		installerwindows.WithMSILogFile("install-rollback.log"),
	))
	defer s.Installer().Purge()

	s.Require().Host(s.Env().RemoteHost).
		DirExists(consts.GetStableDirFor("datadog-apm-library-dotnet"),
			"the package directory should exist")
}
