// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
)

const (
	// DatadogInstallerPackage is the datadog installer package
	DatadogInstallerPackage string = "datadog-installer"
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
		DatadogInstallerPackage,
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
	install map[string]packageWithVersion
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

// InstallInstaller marks the installer package to be installed
func (p *Packages) InstallInstaller() {
	p.install[DatadogInstallerPackage] = packageWithVersion{
		name: DatadogInstallerPackage,
		// HACK: There is an assumption that the parrent install-*.sh script will set the version.
		// We will fail if the version is not set.
		version: "unset",
	}
}
