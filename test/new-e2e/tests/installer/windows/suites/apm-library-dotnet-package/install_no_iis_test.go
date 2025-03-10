// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dotnettests

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host/windows"
	installer "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/unix"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"

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
		installer.WithVersion("3.12.0-pipeline.56978102.beta.sha-91fb55b4-1"),
		installer.WithRegistry("install.datad0g.com"),
	)
	s.Require().Error(err, "Installing the dotnet library package without IIS should fail")
	// TODO today the package does not get deleted but I think it should
	// s.Require().Host(s.Env().RemoteHost).
	// 	NoDirExists(consts.GetStableDirFor("datadog-apm-library-dotnet"),
	// 		"the package directory should not exist")
}
