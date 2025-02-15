// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installer contains code for the E2E tests for the Datadog installer on Windows
package installer

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/consts"

	e2eos "github.com/DataDog/test-infra-definitions/components/os"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/optional"
	installer "github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/unix"
	windowsCommon "github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
)

// DatadogInstaller represents an interface to the Datadog Installer on the remote host.
type DatadogInstaller struct {
	binaryPath string
	env        *environments.WindowsHost
	outputDir  string
}

// NewDatadogInstaller instantiates a new instance of the Datadog Installer running
// on a remote Windows host.
func NewDatadogInstaller(env *environments.WindowsHost, outputDir string) *DatadogInstaller {
	if outputDir == "" {
		outputDir = os.TempDir()
	}

	return &DatadogInstaller{
		binaryPath: path.Join(consts.Path, consts.BinaryName),
		env:        env,
		outputDir:  outputDir,
	}
}

func (d *DatadogInstaller) execute(cmd string, options ...client.ExecuteOption) (string, error) {
	output, err := d.env.RemoteHost.Execute(fmt.Sprintf("& \"%s\" %s", d.binaryPath, cmd), options...)
	if err != nil {
		return output, err
	}
	return strings.TrimSpace(output), nil
}

// executeFromCopy executes a command using a copy of the Datadog Installer binary that is created
// outside of the install directory. This is useful for commands that may remove the installer binary
func (d *DatadogInstaller) executeFromCopy(cmd string, options ...client.ExecuteOption) (string, error) {
	// Create temp file
	tempFile, err := windowsCommon.GetTemporaryFile(d.env.RemoteHost)
	if err != nil {
		return "", err
	}
	defer d.env.RemoteHost.Remove(tempFile) //nolint:errcheck
	// ensure it has a .exe extension
	exeTempFile := tempFile + ".exe"
	defer d.env.RemoteHost.Remove(exeTempFile) //nolint:errcheck
	// must pass -Force b/c the temporary file is already created
	copyCmd := fmt.Sprintf(`Copy-Item -Force -Path "%s" -Destination "%s"`, d.binaryPath, exeTempFile)
	_, err = d.env.RemoteHost.Execute(copyCmd)
	if err != nil {
		return "", err
	}
	// Execute the command with the copied binary
	output, err := d.env.RemoteHost.Execute(fmt.Sprintf("& \"%s\" %s", exeTempFile, cmd), options...)
	if err != nil {
		return output, err
	}
	return strings.TrimSpace(output), nil
}

// Version returns the version of the Datadog Installer on the host.
func (d *DatadogInstaller) Version() (string, error) {
	return d.execute("version")
}

func (d *DatadogInstaller) runCommand(command, packageName string, opts ...installer.PackageOption) (string, error) {
	var packageConfigFound = false
	var packageConfig installer.TestPackageConfig
	for _, packageConfig = range installer.PackagesConfig {
		if packageConfig.Name == packageName {
			packageConfigFound = true
			break
		}
	}

	if !packageConfigFound {
		return "", fmt.Errorf("unknown package %s", packageName)
	}

	err := optional.ApplyOptions(&packageConfig, opts)
	if err != nil {
		return "", nil
	}

	registryTag := packageName
	if packageConfig.Alias != "" {
		registryTag = packageConfig.Alias
	}

	envVars := installer.InstallScriptEnvWithPackages(e2eos.AMD64Arch, []installer.TestPackageConfig{packageConfig})
	packageURL := fmt.Sprintf("oci://%s/%s:%s", packageConfig.Registry, registryTag, packageConfig.Version)

	return d.execute(fmt.Sprintf("%s %s", command, packageURL), client.WithEnvVariables(envVars))
}

// InstallPackage will attempt to use the Datadog Installer to install the package given in parameter.
// version: A function that returns the version of the package to install. By default, it will install
// the package matching the current pipeline. This is a function so that it can be combined with
// Note that this command is a direct command and won't go through the Daemon.
func (d *DatadogInstaller) InstallPackage(packageName string, opts ...installer.PackageOption) (string, error) {
	return d.runCommand("install", packageName, opts...)
}

// InstallExperiment will attempt to use the Datadog Installer to start an experiment for the package given in parameter.
func (d *DatadogInstaller) InstallExperiment(packageName string, opts ...installer.PackageOption) (string, error) {
	return d.runCommand("install-experiment", packageName, opts...)
}

// RemovePackage requests that the Datadog Installer removes a package on the remote host.
func (d *DatadogInstaller) RemovePackage(packageName string) (string, error) {
	return d.execute(fmt.Sprintf("remove %s", packageName))
}

// RemoveExperiment requests that the Datadog Installer removes a package on the remote host.
func (d *DatadogInstaller) RemoveExperiment(packageName string) (string, error) {
	return d.execute(fmt.Sprintf("remove-experiment %s", packageName))
}

// Status returns the status provided by the running daemon
func (d *DatadogInstaller) Status() (string, error) {
	return d.execute("status")
}

// Purge runs the purge command, removing all packages
func (d *DatadogInstaller) Purge() (string, error) {
	// executeFromCopy is used here because the installer will remove itself
	// if purge is run from the install directory it may cause an uninstall failure due
	// to the file being in use.
	return d.executeFromCopy("purge")
}
