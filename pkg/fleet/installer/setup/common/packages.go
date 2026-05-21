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
	// DatadogAgentPackage is the datadog agent package
	DatadogAgentPackage string = "datadog-agent"
	// DatadogAgentDDOTPackage is the datadog agent ddot package
	DatadogAgentDDOTPackage string = "datadog-ddot"
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
	// DatadogAPMLibraryNginxPackage is the datadog apm library nginx package
	DatadogAPMLibraryNginxPackage string = "datadog-apm-library-nginx"
	// DatadogAPMLibraryIISPackage is the datadog apm library iis package
	DatadogAPMLibraryIISPackage string = "datadog-apm-library-iis"
	// DatadogAPMLibraryIISRumPackage is the datadog apm library iis rum package
	DatadogAPMLibraryIISRumPackage string = "datadog-apm-library-iis-rum"
	// DatadogAPMLibraryHttpdPackage is the datadog apm library httpd package
	DatadogAPMLibraryHttpdPackage string = "datadog-apm-library-httpd"
	// DatadogAPMLibraryCPackage is the datadog apm library c package
	DatadogAPMLibraryCPackage string = "datadog-apm-library-c"
)

var (
	order = []string{
		DatadogAgentPackage,
		DatadogAgentDDOTPackage,
		DatadogAPMInjectPackage,
		DatadogAPMLibraryJavaPackage,
		DatadogAPMLibraryPythonPackage,
		DatadogAPMLibraryRubyPackage,
		DatadogAPMLibraryJSPackage,
		DatadogAPMLibraryDotNetPackage,
		DatadogAPMLibraryPHPPackage,
		DatadogAPMLibraryNginxPackage,
		DatadogAPMLibraryIISPackage,
		DatadogAPMLibraryIISRumPackage,
		DatadogAPMLibraryHttpdPackage,
		DatadogAPMLibraryCPackage,
	}

	// ApmLibraries is the list of apm libraries selectable via the setup
	// default script (DD_APM_INSTRUMENTATION_LIBRARIES), including by the
	// "all" / install-all-when-empty fallback.
	ApmLibraries = []string{
		DatadogAPMLibraryJavaPackage,
		DatadogAPMLibraryPythonPackage,
		DatadogAPMLibraryRubyPackage,
		DatadogAPMLibraryJSPackage,
		DatadogAPMLibraryDotNetPackage,
		DatadogAPMLibraryPHPPackage,
		DatadogAPMLibraryNginxPackage,
	}

	// ExplicitOnlyApmLibraries are apm libraries that must be named in
	// DD_APM_INSTRUMENTATION_LIBRARIES to be installed via the setup default
	// script. They are excluded from "all" and the empty-libraries fallback
	// because the underlying packages are pre-registered and gated on remote
	// updates.
	ExplicitOnlyApmLibraries = []string{
		DatadogAPMLibraryIISPackage,
		DatadogAPMLibraryIISRumPackage,
		DatadogAPMLibraryHttpdPackage,
		DatadogAPMLibraryCPackage,
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
