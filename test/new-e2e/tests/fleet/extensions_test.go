// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"path/filepath"
	"strings"
	"testing"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/fleet/suite"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/fixtures"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
)

type extensionsSuite struct {
	suite.FleetSuite
	fixtureServer *fixtures.Server
	packageURL    string // Remote package URL with extension fixture
}

func newExtensionsSuite() e2e.Suite[environments.Host] {
	return &extensionsSuite{}
}

func TestFleetExtensions(t *testing.T) {
	suite.Run(t, newExtensionsSuite, suite.AllPlatforms)
}

func (s *extensionsSuite) SetupSuite() {
	s.FleetSuite.SetupSuite()
	s.fixtureServer = fixtures.NewServer(s.T())

	// Copy fixture to remote VM once for all tests
	localLayoutPath := s.fixtureServer.PackageLayoutURL(fixtures.FixtureSimpleV1WithExtension)
	localLayoutPath = strings.TrimPrefix(localLayoutPath, "file://")

	remoteLayoutPath := "/tmp/oci-layout-simple-v1-with-extension"
	err := s.Env().RemoteHost.CopyFolder(localLayoutPath, remoteLayoutPath)
	s.Require().NoError(err, "Failed to copy fixture to VM")

	s.packageURL = "file://" + remoteLayoutPath
}

// TestExtensionInstallAndRemove tests installing and removing an extension
func (s *extensionsSuite) TestExtensionInstallAndRemove() {
	// Install agent with datadog-installer
	s.Agent.MustInstall()
	defer s.Agent.MustUninstall()

	// Install package with extension directly from file:// URL (without using catalog)
	output, err := s.Installer.Install(s.packageURL)
	s.Require().NoError(err, "Failed to install package: %s", output)
	defer func() {
		// Cleanup: remove package
		_, _ = s.Installer.Remove("simple")
	}()

	// Install extension
	output, err = s.Installer.InstallExtension(s.packageURL, "simple-extension")
	s.Require().NoError(err, "Failed to install extension: %s", output)

	// Verify extension was installed
	extensionPath := s.getExtensionPath("simple", "v1", "simple-extension")
	exists, err := s.Host.DirExists(extensionPath)
	s.Require().NoError(err, "Failed to check if extension exists")
	s.Require().True(exists, "Extension should be installed at %s", extensionPath)

	// Verify extension script file exists
	scriptPath := filepath.Join(extensionPath, "extension.sh")
	exists, err = s.Env().RemoteHost.FileExists(scriptPath)
	s.Require().NoError(err, "Failed to check if extension script exists")
	s.Require().True(exists, "Extension script should exist at %s", scriptPath)

	// Remove extension
	output, err = s.Installer.RemoveExtension("simple", "simple-extension")
	s.Require().NoError(err, "Failed to remove extension: %s", output)

	// Verify extension was removed
	exists, err = s.Host.DirExists(extensionPath)
	s.Require().NoError(err, "Failed to check if extension exists")
	s.Require().False(exists, "Extension should be removed from %s", extensionPath)
}

// TestExtensionSaveAndRestore tests saving and restoring extensions
func (s *extensionsSuite) TestExtensionSaveAndRestore() {
	// Install agent with datadog-installer
	s.Agent.MustInstall()
	defer s.Agent.MustUninstall()

	// Install package with extension directly from file:// URL (without using catalog)
	output, err := s.Installer.Install(s.packageURL)
	s.Require().NoError(err, "Failed to install package: %s", output)
	defer func() {
		// Cleanup: remove package
		_, _ = s.Installer.Remove("simple")
	}()

	// Install extension
	output, err = s.Installer.InstallExtension(s.packageURL, "simple-extension")
	s.Require().NoError(err, "Failed to install extension: %s", output)

	// Verify extension was installed
	extensionPath := s.getExtensionPath("simple", "v1", "simple-extension")
	exists, err := s.Host.DirExists(extensionPath)
	s.Require().NoError(err, "Failed to check if extension exists")
	s.Require().True(exists, "Extension should be installed at %s", extensionPath)

	// Create save directory in temp folder
	tmpFolder, err := s.Env().RemoteHost.GetTmpFolder()
	s.Require().NoError(err, "Failed to get temp folder")
	saveDir := s.Env().RemoteHost.JoinPath(tmpFolder, "extensions-save-test")
	err = s.Env().RemoteHost.MkdirAll(saveDir)
	s.Require().NoError(err, "Failed to create save directory")

	// Save extensions to temp directory
	output, err = s.Installer.SaveExtensions("simple", saveDir)
	s.Require().NoError(err, "Failed to save extensions: %s", output)
	defer func() {
		// Cleanup: remove save directory
		_ = s.Env().RemoteHost.RemoveAll(saveDir)
	}()

	// Verify save directory was created
	saveExists, err := s.Host.DirExists(saveDir)
	s.Require().NoError(err, "Failed to check if save directory exists")
	s.Require().True(saveExists, "Save directory should be created at %s", saveDir)

	// Remove extension
	output, err = s.Installer.RemoveExtension("simple", "simple-extension")
	s.Require().NoError(err, "Failed to remove extension: %s", output)

	// Verify extension was removed
	exists, err = s.Host.DirExists(extensionPath)
	s.Require().NoError(err, "Failed to check if extension exists after removal")
	s.Require().False(exists, "Extension should be removed from %s", extensionPath)

	// Restore extensions from save directory
	output, err = s.Installer.RestoreExtensions(s.packageURL, saveDir)
	s.Require().NoError(err, "Failed to restore extensions: %s", output)

	// Verify extension was restored
	exists, err = s.Host.DirExists(extensionPath)
	s.Require().NoError(err, "Failed to check if extension exists after restore")
	s.Require().True(exists, "Extension should be restored at %s", extensionPath)
}

// Helper methods

// getExtensionPath returns the path to an extension directory.
// It uses the same logic as pkg/fleet/installer/packages/extensions/extensions.go:getExtensionsPath
func (s *extensionsSuite) getExtensionPath(pkg, version, extensionName string) string {
	var basePath string
	switch s.Env().RemoteHost.OSFamily {
	case e2eos.LinuxFamily:
		basePath = filepath.Join(paths.PackagesPath, pkg, version)
	case e2eos.WindowsFamily:
		// On Windows: C:\ProgramData\Datadog\Installer\packages\<pkg>\<version>
		basePath = filepath.Join(`C:\ProgramData\Datadog\Installer\packages`, pkg, version)
	default:
		s.T().Fatalf("unsupported OS family: %v", s.Env().RemoteHost.OSFamily)
		return ""
	}
	return filepath.Join(basePath, "ext", extensionName)
}
