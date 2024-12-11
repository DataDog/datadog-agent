// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import "fmt"

const (
	DatadogAgentPackage            string = "datadog-agent"
	DatadogInstallerPackage        string = "datadog-installer"
	DatadogAPMInjectPackage        string = "datadog-apm-inject"
	DatadogAPMLibraryJavaPackage   string = "datadog-apm-library-java"
	DatadogAPMLibraryPythonPackage string = "datadog-apm-library-python"
	DatadogAPMLibraryRubyPackage   string = "datadog-apm-library-ruby"
	DatadogAPMLibraryJSPackage     string = "datadog-apm-library-js"
	DatadogAPMLibraryDotNetPackage string = "datadog-apm-library-dotnet"
	DatadogAPMLibraryPHPPackage    string = "datadog-apm-library-php"
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
)

func resolvePackages(packages Packages) []packageWithVersion {
	var resolved []packageWithVersion
	for _, pkg := range order {
		if p, ok := packages.install[pkg]; ok {
			resolved = append(resolved, p)
		}
	}
	if len(resolved) != len(packages.install) {
		panic(fmt.Sprintf("unknown package requested: %v", packages.install))
	}
	return resolved
}

type Packages struct {
	install map[string]packageWithVersion
}

type packageWithVersion struct {
	name    string
	version string
}

func (p *Packages) Install(pkg string, version string) {
	p.install[pkg] = packageWithVersion{
		name:    pkg,
		version: version,
	}
}
