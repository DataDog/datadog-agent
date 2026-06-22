// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !e2eunit

package installer

import (
	_ "embed"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	winawshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host/windows"
	unixinstaller "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/unix"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"

	"testing"
)

const dotnetActivationGateEnv = "DD_TEST_APM_LIBRARY_DOTNET_ACTIVATION_GATE"

var (
	//go:embed resources/dotnet/web.config
	webConfigFile []byte
	//go:embed resources/dotnet/index.aspx
	aspxFile []byte
)

type testDotnetLibraryInstallSuite struct {
	baseIISSuite
}

// TestDotnetInstalls tests the usage of the Datadog installer to install the apm-library-dotnet-package package.
func TestDotnetLibraryInstalls(t *testing.T) {
	e2e.Run(t, &testDotnetLibraryInstallSuite{},
		e2e.WithProvisioner(
			winawshost.ProvisionerNoAgentNoFakeIntake()))
}

func (s *testDotnetLibraryInstallSuite) BeforeTest(suiteName, testName string) {
	s.baseIISSuite.BeforeTest(suiteName, testName)
	s.Require().NoError(s.Installer().Install(WithMSILogFile(testName + "-msiinstall.log")))
}

func (s *testDotnetLibraryInstallSuite) AfterTest(suiteName, testName string) {
	s.Installer().Purge()
	s.baseIISSuite.AfterTest(suiteName, testName)
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
		initialVersion = "3.19.0-pipeline.67299728.beta.sha-c05ddfb1-1"
		upgradeVersion = "3.19.0-pipeline.67351320.beta.sha-c05ddfb1-1"
	)
	flake.Mark(s.T())

	// Install first version
	s.installDotnetAPMLibraryWithVersion(initialVersion)

	// Start the IIS app to load the library
	defer s.stopIISApp()
	s.startIISApp(webConfigFile, aspxFile)

	// Check that the expected version of the library is loaded
	oldLibraryPath := s.getLibraryPathFromInstrumentedIIS()
	s.Require().Contains(oldLibraryPath, initialVersion[:len(initialVersion)-2])

	// Install the new version of the library
	s.installDotnetAPMLibraryWithVersion(upgradeVersion)

	// Check that the old version of the library is still loaded since we have not restarted yet
	output := s.getLibraryPathFromInstrumentedIIS()
	s.Require().Contains(output, initialVersion[:len(initialVersion)-2])

	// Check that a garbage collection does not remove the old version of the library
	output, err := s.Installer().GarbageCollect()
	s.Require().NoErrorf(err, "failed to garbage collect: %s", output)
	s.Require().Host(s.Env().RemoteHost).DirExists(oldLibraryPath, "the old library path: %s should still exist after garbage collection", oldLibraryPath)

	// Restart the IIS application
	s.startIISApp(webConfigFile, aspxFile)

	// Check that the new version of the library is loaded
	output = s.getLibraryPathFromInstrumentedIIS()
	s.Require().Contains(output, upgradeVersion[:len(upgradeVersion)-2], "the new library path should contain the new version")

	// Check that garbage collection removes the old version of the library
	output, err = s.Installer().GarbageCollect()
	s.Require().NoErrorf(err, "failed to garbage collect: %s", output)
	s.Require().Host(s.Env().RemoteHost).NoDirExists(oldLibraryPath, "the old library path:%s should no longer exist after garbage collection", oldLibraryPath)

}

func (s *testDotnetLibraryInstallSuite) TestUpgradeDoesNotPublishStableBeforeActivation() {
	const (
		initialVersion = "3.19.0-pipeline.67299728.beta.sha-c05ddfb1-1"
		upgradeVersion = "3.19.0-pipeline.67351320.beta.sha-c05ddfb1-1"
	)

	s.installDotnetAPMLibraryWithVersion(initialVersion)
	s.assertDotnetStableTargetContains(initialVersion)

	gatePath := `C:\ProgramData\Datadog\Installer\dotnet-activation-gate`
	host := s.Env().RemoteHost
	_, _ = host.Execute(fmt.Sprintf(`Remove-Item -Force -ErrorAction SilentlyContinue '%s.ready', '%s.release'`, gatePath, gatePath))

	session, stdout, err := s.Installer().StartInstallPackage("datadog-apm-library-dotnet",
		map[string]string{dotnetActivationGateEnv: gatePath},
		unixinstaller.WithVersion(upgradeVersion),
		unixinstaller.WithRegistry("install.datad0g.com.internal.dda-testing.com"),
	)
	s.Require().NoError(err)
	defer session.Close() //nolint:errcheck

	s.waitForRemoteFile(gatePath+".ready", 2*time.Minute)
	s.assertDotnetStableTargetContains(initialVersion)

	_, err = host.Execute(fmt.Sprintf(`New-Item -ItemType File -Force -Path '%s.release' | Out-Null`, gatePath))
	s.Require().NoError(err)

	output, readErr := io.ReadAll(stdout)
	s.Require().NoError(readErr)
	s.Require().NoErrorf(session.Wait(), "failed to install upgraded dotnet library: %s", string(output))
	s.assertDotnetStableTargetContains(upgradeVersion)
}

func (s *testDotnetLibraryInstallSuite) TestRemovePackageFailsIfInUse() {
	flake.Mark(s.T())
	s.installDotnetAPMLibrary()

	defer s.stopIISApp()
	s.startIISApp(webConfigFile, aspxFile)

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
	flake.Mark(s.T())
	const (
		initialVersion = "3.19.0-pipeline.67299728.beta.sha-c05ddfb1-1"
		upgradeVersion = "3.19.0-pipeline.67351320.beta.sha-c05ddfb1-1"
	)
	// Install initial version
	s.installDotnetAPMLibraryWithVersion(initialVersion)

	// Start app using the library
	defer s.stopIISApp()
	s.startIISApp(webConfigFile, aspxFile)
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
	s.startIISApp(webConfigFile, aspxFile)

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
		unixinstaller.WithVersion("3.19.0-pipeline.67351320.beta.sha-c05ddfb1-1"),
		unixinstaller.WithRegistry("install.datad0g.com.internal.dda-testing.com"),
	)
	s.Require().NoErrorf(err, "failed to install the dotnet library package: %s", output)
}

func (s *testDotnetLibraryInstallSuite) installDotnetAPMLibraryWithVersion(version string) {
	// TODO remove override once image is published in prod
	output, err := s.Installer().InstallPackage("datadog-apm-library-dotnet",
		unixinstaller.WithVersion(version),
		unixinstaller.WithRegistry("install.datad0g.com.internal.dda-testing.com"),
	)
	s.Require().NoErrorf(err, "failed to install the dotnet library package: %s", output)
}

func (s *testDotnetLibraryInstallSuite) removeDotnetAPMLibrary() {
	output, err := s.Installer().RemovePackage("datadog-apm-library-dotnet")
	s.Require().NoErrorf(err, "failed to remove the dotnet library package: %s", output)
}

func (s *testDotnetLibraryInstallSuite) assertDotnetStableTargetContains(version string) {
	output, err := s.Env().RemoteHost.Execute(fmt.Sprintf(`(Get-Item -LiteralPath '%s').Target`, consts.GetStableDirFor("datadog-apm-library-dotnet")))
	s.Require().NoErrorf(err, "failed to read dotnet stable target: %s", output)
	s.Require().Contains(strings.TrimSpace(output), version)
}

func (s *testDotnetLibraryInstallSuite) waitForRemoteFile(path string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		output, err := s.Env().RemoteHost.Execute(fmt.Sprintf(`Test-Path -LiteralPath '%s'`, path))
		if err == nil && strings.EqualFold(strings.TrimSpace(output), "true") {
			return
		}
		time.Sleep(time.Second)
	}
	s.Require().FailNowf("timed out waiting for remote file", "path: %s", path)
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
