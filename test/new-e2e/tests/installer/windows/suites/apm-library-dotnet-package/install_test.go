// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dotnettests

import (
	_ "embed"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host/windows"
	installer "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/unix"
	installerwindows "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"

	"testing"
)

var (
	//go:embed resources/web.config
	webConfigFile []byte
	//go:embed resources/index.aspx
	aspxFile []byte
)

type testDotnetLibraryInstallSuite struct {
	installerwindows.BaseSuite
}

// TestDotnetInstalls tests the usage of the Datadog installer to install the apm-library-dotnet-package package.
func TestDotnetLibraryInstalls(t *testing.T) {
	e2e.Run(t, &testDotnetLibraryInstallSuite{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake()))
}

func (s *testDotnetLibraryInstallSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	s.installIIS()
	s.installAspNet()
}

func (s *testDotnetLibraryInstallSuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)
	s.Require().NoError(s.Installer().Install(installerwindows.WithMSILogFile(testName + "-msiinstall.log")))
}

func (s *testDotnetLibraryInstallSuite) AfterTest(suiteName, testName string) {
	s.Installer().Purge()
	s.BaseSuite.AfterTest(suiteName, testName)
}

// TestInstallUninstallDotnetLibraryPackage tests installing and uninstalling the Datadog APM Library for .NET using the Datadog installer.
func (s *testDotnetLibraryInstallSuite) TestInstallUninstallDotnetLibraryPackage() {
	s.installDotnetAPMLibrary()

	s.removeDotnetAPMLibrary()

	s.Require().Host(s.Env().RemoteHost).
		NoDirExists(consts.GetStableDirFor("datadog-apm-library-dotnet"),
			"the package directory should be removed")
}

func (s *testDotnetLibraryInstallSuite) TestReinstall() {
	s.installDotnetAPMLibrary()

	s.installDotnetAPMLibrary()
}

func (s *testDotnetLibraryInstallSuite) TestUpdate() {
	const (
		oldVersion = "3.12.0-pipeline.56978102.beta.sha-91fb55b4-1"
		newVersion = "3.11.0-pipeline.56515513.beta.sha-d6a0900f-1"
	)
	flake.Mark(s.T())

	// Install first version
	s.installDotnetAPMLibraryWithVersion(oldVersion)

	// Start the IIS app to load the library
	defer s.stopIISApp()
	s.startIISApp()

	// Check that the expected version of the library is loadedi
	oldLibraryPath := s.getLibraryPathFromInstrumentedIIS()
	s.Require().Contains(oldLibraryPath, oldVersion[:len(oldVersion)-2])

	// Install the new version of the library
	s.installDotnetAPMLibraryWithVersion(newVersion)

	// Check that the old version of the library is still loaded since we have not restarted yet
	output := s.getLibraryPathFromInstrumentedIIS()
	s.Require().Contains(output, oldVersion[:len(oldVersion)-2])

	// Check that a garbage collection does not remove the old version of the library
	output, err := s.Installer().GarbageCollect()
	s.Require().NoErrorf(err, "failed to garbage collect: %s", output)
	s.Require().Host(s.Env().RemoteHost).DirExists(oldLibraryPath, "the old library path: %s should still exist after garbage collection", oldLibraryPath)

	// Restart the IIS application
	s.startIISApp()

	// Check that the new version of the library is loaded
	output = s.getLibraryPathFromInstrumentedIIS()
	s.Require().Contains(output, newVersion[:len(newVersion)-2], "the new library path should contain the new version")

	// Check that garbage collection removes the old version of the library
	output, err = s.Installer().GarbageCollect()
	s.Require().NoErrorf(err, "failed to garbage collect: %s", output)
	s.Require().Host(s.Env().RemoteHost).NoDirExists(oldLibraryPath, "the old library path:%s should no longer exist after garbage collection", oldLibraryPath)

}

func (s *testDotnetLibraryInstallSuite) TestRemovePackageFailsIfInUse() {
	s.installDotnetAPMLibrary()

	defer s.stopIISApp()
	s.startIISApp()

	output, err := s.Installer().RemovePackage("datadog-apm-library-dotnet")
	s.Require().Error(err, "Removing the package while the native profiler is used by another process should fail: %s", output)

	// Check that an arbitrary file from the package still exists to make sure
	// that the files were not deleted when attempting to remove the package
	libraryPath := s.getLibraryPathFromInstrumentedIIS()
	versionPath := pathJoin(pathDir(libraryPath), "version")
	s.Require().Host(s.Env().RemoteHost).FileExists(versionPath, "the package files should still exist, %s is missing", versionPath)

	s.stopIISApp()

	s.removeDotnetAPMLibrary()

	s.Require().Host(s.Env().RemoteHost).NoDirExists(pathDir(libraryPath), "the package directory should no longer exist")
}

func (s *testDotnetLibraryInstallSuite) TestUpgradeAndDowngradePackage() {
	const (
		initialVersion = "3.12.0-pipeline.56978102.beta.sha-91fb55b4-1"
		upgradeVersion = "3.11.0-pipeline.56515513.beta.sha-d6a0900f-1"
	)
	// Install initial version
	s.installDotnetAPMLibraryWithVersion(initialVersion)

	// Start app using the library
	defer s.stopIISApp()
	s.startIISApp()
	initialLibraryPath := s.getLibraryPathFromInstrumentedIIS()
	s.Require().Contains(initialLibraryPath, initialVersion[:len(initialVersion)-2], "library path should contain initial version")

	// Upgrade to newer version
	s.installDotnetAPMLibraryWithVersion(upgradeVersion)

	// Check that an arbitrary file from the package still exists to make sure
	// that the files were not deleted when attempting to remove the package
	output, err := s.Installer().GarbageCollect()
	s.Require().NoErrorf(err, "failed to garbage collect: %s", output)
	libraryPath := s.getLibraryPathFromInstrumentedIIS()
	s.Require().Contains(libraryPath, initialVersion[:len(initialVersion)-2], "library path should contain initial version")
	versionPath := pathJoin(pathDir(libraryPath), "version")
	s.Require().Host(s.Env().RemoteHost).FileExists(versionPath, "the package files should still exist, %s is missing", versionPath)

	// Downgrade back to initial version
	s.installDotnetAPMLibraryWithVersion(initialVersion)

	// Restart app and verify downgrade
	s.stopIISApp()
	s.startIISApp()

	downgradedLibraryPath := s.getLibraryPathFromInstrumentedIIS()
	s.Require().Contains(downgradedLibraryPath, initialVersion[:len(initialVersion)-2], "library path should contain initial version after downgrade")
}

func (s *testDotnetLibraryInstallSuite) TestRemoveCorruptedPackageFails() {
	s.installDotnetAPMLibrary()

	s.Env().RemoteHost.Remove(pathJoin(consts.GetStableDirFor("datadog-apm-library-dotnet"), "installer", "Datadog.FleetInstaller.exe"))

	output, err := s.Installer().RemovePackage("datadog-apm-library-dotnet")
	s.Require().Error(err, "Removing the package when the dotnet installer binary is missing should fail: %s", output)
}

func (s *testDotnetLibraryInstallSuite) installDotnetAPMLibrary() {
	// TODO remove override once image is published in prod
	output, err := s.Installer().InstallPackage("datadog-apm-library-dotnet",
		installer.WithVersion("3.12.0-pipeline.56978102.beta.sha-91fb55b4-1"),
		installer.WithRegistry("install.datad0g.com"),
	)
	s.Require().NoErrorf(err, "failed to install the dotnet library package: %s", output)
}

func (s *testDotnetLibraryInstallSuite) installDotnetAPMLibraryWithVersion(version string) {
	// TODO remove override once image is published in prod
	output, err := s.Installer().InstallPackage("datadog-apm-library-dotnet",
		installer.WithVersion(version),
		installer.WithRegistry("install.datad0g.com"),
	)
	s.Require().NoErrorf(err, "failed to install the dotnet library package: %s", output)
}

func (s *testDotnetLibraryInstallSuite) removeDotnetAPMLibrary() {
	output, err := s.Installer().RemovePackage("datadog-apm-library-dotnet")
	s.Require().NoErrorf(err, "failed to remove the dotnet library package: %s", output)
}

func (s *testDotnetLibraryInstallSuite) installIIS() {
	host := s.Env().RemoteHost
	err := windows.InstallIIS(host)
	s.Require().NoError(err)
}

func (s *testDotnetLibraryInstallSuite) installAspNet() {
	host := s.Env().RemoteHost
	output, err := host.Execute("Install-WindowsFeature Web-Asp-Net45")
	s.Require().NoErrorf(err, "failed to install Asp.Net: %s", output)
}

func (s *testDotnetLibraryInstallSuite) startIISApp() {
	host := s.Env().RemoteHost
	err := host.MkdirAll("C:\\inetpub\\wwwroot\\DummyApp")
	s.Require().NoError(err, "failed to create directory for DummyApp")
	_, err = host.WriteFile("C:\\inetpub\\wwwroot\\DummyApp\\web.config", webConfigFile)
	s.Require().NoError(err, "failed to write web.config file")
	_, err = host.WriteFile("C:\\inetpub\\wwwroot\\DummyApp\\index.aspx", aspxFile)
	s.Require().NoError(err, "failed to write index.aspx file")
	script := `
$SitePath = "C:\inetpub\wwwroot\DummyApp"
New-WebSite -Name DummyApp -PhysicalPath $SitePath -Port 8080 -ApplicationPool "DefaultAppPool" -Force
Stop-WebSite -Name "DummyApp"
Start-WebSite -Name "DummyApp"
$state = (Get-WebAppPoolState -Name "DefaultAppPool").Value
if ($state -eq "Stopped") {
    Start-WebAppPool -Name "DefaultAppPool"
}
Restart-WebAppPool -Name "DefaultAppPool"
Invoke-WebRequest -Uri "http://localhost:8080/index.aspx" -UseBasicParsing
	`
	output, err := host.Execute(script)
	s.Require().NoErrorf(err, "failed to start site: %s", output)
}

func (s *testDotnetLibraryInstallSuite) stopIISApp() {
	script := `
Stop-WebSite -Name "DummyApp"
$state = (Get-WebAppPoolState -Name "DefaultAppPool").Value
if ($state -ne "Stopped") {
	Stop-WebAppPool -Name "DefaultAppPool"
	$retryCount = 0
	do {
		Start-Sleep -Seconds 1
		$status = (Get-WebAppPoolState -Name DefaultAppPool).Value
		$retryCount++
	} while ($status -ne "Stopped" -and $retryCount -lt 60)
	if ($status -ne "Stopped") {
		exit -1
	}
}
	`
	host := s.Env().RemoteHost
	output, err := host.Execute(script)
	s.Require().NoErrorf(err, "failed to stop site: %s", output)
}

func (s *testDotnetLibraryInstallSuite) getLibraryPathFromInstrumentedIIS() string {
	host := s.Env().RemoteHost
	output, err := host.Execute("(Invoke-WebRequest -Uri \"http://localhost:8080/index.aspx\" -UseBasicParsing).Content")
	s.Require().NoErrorf(err, "failed to get content from site: %s", output)
	return strings.TrimSpace(output)
}

// pathJoin and pathDir are helper functions to work with paths in Windows.
// since the test code is running on a Linux machine filepath functions
// will not work as expected
func pathJoin(elements ...string) string {
	return strings.Join(elements, "\\")
}

func pathDir(path string) string {
	sep := "\\"
	path = strings.TrimRight(path, sep)
	if path == "" {
		return ""
	}
	lastSep := strings.LastIndex(path, sep)
	if lastSep == -1 {
		return ""
	}
	return path[:lastSep]
}
