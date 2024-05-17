// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/internal/oci"
)

const (
	envSite                            = "DD_SITE"
	envInstallerDefaultPackages        = "DD_INSTALLER_DEFAULT_PACKAGES"
	envInstallerDefaultPackagesEnabled = "DD_INSTALLER_DEFAULT_PACKAGES_ENABLED"
	envApmInstrumentationEnabled       = "DD_APM_INSTRUMENTATION_ENABLED"
)

// DefaultPackages resolves the default packages URLs to install based on the environment.
func DefaultPackages() []string {
	if !featureEnabled() {
		return nil
	}

	var packages = make(map[string]string)

	switch os.Getenv(envApmInstrumentationEnabled) {
	case "all", "docker", "host":
		packages["datadog-apm-inject"] = "latest"
	}

	for p, v := range parseForcedPackages() {
		packages[p] = v
	}
	return resolvePackageURLs(packages)
}

func featureEnabled() bool {
	return os.Getenv(envInstallerDefaultPackagesEnabled) == "true"
}

func resolvePackageURLs(packages map[string]string) []string {
	site := "datadoghq.com"
	if os.Getenv(envSite) != "" {
		site = os.Getenv(envSite)
	}
	var packageURLs []string
	for p, v := range packages {
		packageURLs = append(packageURLs, oci.PackageURL(site, p, v))
	}
	return packageURLs
}

func parseForcedPackages() map[string]string {
	var packages = make(map[string]string)
	rawForcedPackages := os.Getenv(envInstallerDefaultPackages)
	if rawForcedPackages == "" {
		return packages
	}
	for _, rawPackage := range strings.Split(rawForcedPackages, ",") {
		if strings.Contains(rawPackage, ":") {
			parts := strings.Split(rawPackage, ":")
			packages[parts[0]] = parts[1]
		} else {
			packages[rawPackage] = "latest"
		}
	}
	return packages
}
