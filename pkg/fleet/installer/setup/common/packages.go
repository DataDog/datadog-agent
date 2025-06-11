// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
)

const (
	// DatadogAgentPackage is the datadog agent package
	DatadogAgentPackage string = "datadog-agent"
	// DatadogAPMInjectPackage is the datadog apm inject package
	DatadogAPMInjectPackage string = "datadog-apm-inject"
	// DatadogAPMLibraryJavaPackage is the datadog apm library java package
	DatadogAPMLibraryJavaPackage string = "datadog-apm-library-java"
	// DatadogAPMLibraryPythonPackage is the datadog apm library python package
	DatadogAPMLibraryPythonPackage string = "datadog-apm-library-python"
	// DatadogAPMLibraryRubyPackage is the datadog apm library ruby package
	DatadogAPMLibraryRubyPackage string = "datadog-apm-library-ruby"
	// DatadogAPMLibraryJSPackage is the datadog apm library js package
	DatadogAPMLibraryJSPackage string = "datadog-apm-library-js"
	// DatadogAPMLibraryDotNetPackage is the datadog apm library dotnet package
	DatadogAPMLibraryDotNetPackage string = "datadog-apm-library-dotnet"
	// DatadogAPMLibraryPHPPackage is the datadog apm library php package
	DatadogAPMLibraryPHPPackage string = "datadog-apm-library-php"
)

var (
	order = []string{
		DatadogAgentPackage,
		DatadogAPMInjectPackage,
		DatadogAPMLibraryJavaPackage,
		DatadogAPMLibraryPythonPackage,
		DatadogAPMLibraryRubyPackage,
		DatadogAPMLibraryJSPackage,
		DatadogAPMLibraryDotNetPackage,
		DatadogAPMLibraryPHPPackage,
	}

	// ApmLibraries is a list of all the apm libraries
	ApmLibraries = []string{
		DatadogAPMLibraryJavaPackage,
		DatadogAPMLibraryPythonPackage,
		DatadogAPMLibraryRubyPackage,
		DatadogAPMLibraryJSPackage,
		DatadogAPMLibraryDotNetPackage,
		DatadogAPMLibraryPHPPackage,
	}
)

func resolvePackages(env *env.Env, packages Packages) []packageWithVersion {
	var resolved []packageWithVersion
	for _, pkg := range order {
		forceInstall := env.DefaultPackagesInstallOverride[pkg]
		if p, ok := packages.install[pkg]; ok || forceInstall {
			if env.DefaultPackagesVersionOverride[pkg] != "" {
				p.version = env.DefaultPackagesVersionOverride[pkg]
			}
			resolved = append(resolved, p)
		}
	}
	if len(resolved) != len(packages.install) {
		panic(fmt.Sprintf("unknown package requested: %v", packages.install))
	}
	return resolved
}

// Packages is a list of packages to install
type Packages struct {
	install          map[string]packageWithVersion
	copyInstallerSSI bool
}

type packageWithVersion struct {
	name    string
	version string
}

// Install marks a package to be installed
func (p *Packages) Install(pkg string, version string) {
	p.install[pkg] = packageWithVersion{
		name:    pkg,
		version: version,
	}
}

// WriteSSIInstaller marks that the installer should be copied to /opt/datadog-packages/run/datadog-installer-ssi
// Use this when installing SSI without the agent, so that the installer can be used later to remove the packages.
func (p *Packages) WriteSSIInstaller() {
	p.copyInstallerSSI = true
}

func copyInstallerSSI() error {
	destinationPath := "/opt/datadog-packages/run/datadog-installer-ssi"

	// Get the current executable path
	currentExecutable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get current executable: %w", err)
	}

	// Open the current executable file
	sourceFile, err := os.Open(currentExecutable)
	if err != nil {
		return fmt.Errorf("failed to open current executable: %w", err)
	}
	defer sourceFile.Close()

	// Create /usr/bin directory if it doesn't exist (unlikely)
	err = os.MkdirAll(filepath.Dir(destinationPath), 0755)
	if err != nil {
		return fmt.Errorf("failed to create installer directory: %w", err)
	}

	// Check if the destination file already exists and remove it if it does (we don't want to overwrite a symlink)
	if _, err := os.Stat(destinationPath); err == nil {
		if err := os.Remove(destinationPath); err != nil {
			return fmt.Errorf("failed to remove existing destination file: %w", err)
		}
	}

	// Create the destination file
	destinationFile, err := os.Create(destinationPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destinationFile.Close()

	// Copy the current executable to the destination file
	_, err = io.Copy(destinationFile, sourceFile)
	if err != nil {
		return fmt.Errorf("failed to copy executable: %w", err)
	}

	// Set the permissions on the destination file to be executable
	err = destinationFile.Chmod(0755)
	if err != nil {
		return fmt.Errorf("failed to set permissions on destination file: %w", err)
	}

	return nil
}
