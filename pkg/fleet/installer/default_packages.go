// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"slices"

	"github.com/DataDog/datadog-agent/pkg/fleet/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/oci"
)

type defaultPackage struct {
	name                      string
	released                  bool
	releasedBySite            []string
	releasedWithRemoteUpdates bool
	condition                 func(*env.Env) bool
}

var defaultPackagesList = []defaultPackage{
	{name: "datadog-apm-inject", released: false, condition: apmInjectEnabled},
	{name: "datadog-apm-library-java", released: false, condition: apmInjectEnabled},
	{name: "datadog-apm-library-ruby", released: false, condition: apmInjectEnabled},
	{name: "datadog-apm-library-js", released: false, condition: apmInjectEnabled},
	{name: "datadog-apm-library-dotnet", released: false, condition: apmInjectEnabled},
	{name: "datadog-apm-library-python", released: false, condition: apmInjectEnabled},
	{name: "datadog-agent", released: false, releasedWithRemoteUpdates: true},
}

var languageToPackage = map[string]env.ApmLibLanguage{
	"datadog-apm-library-java":   "java",
	"datadog-apm-library-ruby":   "ruby",
	"datadog-apm-library-js":     "js",
	"datadog-apm-library-dotnet": "dotnet",
	"datadog-apm-library-python": "python",
}

// DefaultPackages resolves the default packages URLs to install based on the environment.
func DefaultPackages(env *env.Env) []string {
	return defaultPackages(env, defaultPackagesList)
}

func defaultPackages(env *env.Env, defaultPackages []defaultPackage) []string {
	var packages []string
	for _, p := range defaultPackages {
		released := p.released || slices.Contains(p.releasedBySite, env.Site) || (p.releasedWithRemoteUpdates && env.RemoteUpdates)
		installOverride, isOverridden := env.DefaultPackagesInstallOverride[p.name]
		condition := p.condition == nil || p.condition(env)

		shouldInstall := released && condition
		if isOverridden {
			shouldInstall = installOverride
		}
		if !shouldInstall {
			continue
		}

		version := "latest"

		// Respect pinned version of APM packages if we don't define any overwrite
		if apmLibVersion, ok := env.ApmLibraries[languageToPackage[p.name]]; ok {
			version = apmLibVersion.AsVersionTag()
		}

		if v, ok := env.DefaultPackagesVersionOverride[p.name]; ok {
			version = v
		}
		url := oci.PackageURL(env, p.name, version)
		packages = append(packages, url)
	}
	return packages
}

func apmInjectEnabled(e *env.Env) bool {
	switch e.InstallScript.APMInstrumentationEnabled {
	case env.APMInstrumentationEnabledAll, env.APMInstrumentationEnabledDocker, env.APMInstrumentationEnabledHost:
		return true
	}
	return false
}
