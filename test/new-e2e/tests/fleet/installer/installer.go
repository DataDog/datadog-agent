// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installer contains a wrapper around the datadog-installer commands for use in tests.
package installer

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
)

// Installer is a cross-platform wrapper around the datadog-installer commands for use in tests.
type Installer struct {
	t    func() *testing.T
	host *environments.Host
}

// New creates a new instance of Installer.
func New(t func() *testing.T, host *environments.Host) *Installer {
	return &Installer{t: t, host: host}
}

// Run executes a datadog-installer command with the given arguments.
// Example: Run("install", "file:///path/to/package")
func (i *Installer) Run(args ...string) (string, error) {
	var baseCommand string
	switch i.host.RemoteHost.OSFamily {
	case e2eos.LinuxFamily:
		baseCommand = "sudo datadog-installer"
	case e2eos.WindowsFamily:
		baseCommand = `& "C:\Program Files\Datadog\Datadog Agent\bin\datadog-installer.exe"`
	default:
		return "", fmt.Errorf("unsupported OS family: %v", i.host.RemoteHost.OSFamily)
	}

	cmd := fmt.Sprintf("%s %s", baseCommand, strings.Join(args, " "))
	return i.host.RemoteHost.Execute(cmd)
}

// Install installs a package from the given URL.
func (i *Installer) Install(packageURL string) (string, error) {
	return i.Run("install", packageURL)
}

// Remove removes a package by name.
func (i *Installer) Remove(packageName string) (string, error) {
	return i.Run("remove", packageName)
}

// InstallExtension installs an extension from a package.
func (i *Installer) InstallExtension(packageURL, extensionName string) (string, error) {
	return i.Run("extension", "install", packageURL, extensionName)
}

// MustInstallExtension installs an extension from a package and fails the test if it returns an error.
func (i *Installer) MustInstallExtension(packageURL, extensionName string) {
	output, err := i.InstallExtension(packageURL, extensionName)
	assert.NoError(i.t(), err, "Failed to install extension: %s", output)
}

// RemoveExtension removes an extension from a package.
func (i *Installer) RemoveExtension(packageName, extensionName string) (string, error) {
	return i.Run("extension", "remove", packageName, extensionName)
}

// MustRemoveExtension removes an extension from a package and fails the test if it returns an error.
func (i *Installer) MustRemoveExtension(packageName, extensionName string) {
	output, err := i.RemoveExtension(packageName, extensionName)
	assert.NoError(i.t(), err, "Failed to remove extension: %s", output)
}

// SaveExtensions saves extensions to a directory.
// packageName is the name of the package (e.g., "simple").
// path is the directory where extensions will be saved.
func (i *Installer) SaveExtensions(packageName, path string) (string, error) {
	return i.Run("extension", "save", packageName, path)
}

// RestoreExtensions restores extensions from a directory.
// packageURL is the URL to the package (e.g., "file:///path/to/package").
// path is the directory containing the saved extensions.
func (i *Installer) RestoreExtensions(packageURL, path string) (string, error) {
	return i.Run("extension", "restore", packageURL, path)
}
