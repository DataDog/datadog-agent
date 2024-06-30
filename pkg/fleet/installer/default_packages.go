// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"slices"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/fleet/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/oci"
)

// Package represents a package known to the installer
type Package struct {
	Name                      string
	released                  bool
	releasedBySite            []string
	releasedWithRemoteUpdates bool
	condition                 func(Package, *env.Env) bool
}

// PackagesList lists all known packages. Not all of them are installable
var PackagesList = []Package{
	{Name: "datadog-apm-inject", released: true, condition: apmInjectEnabled},
	{Name: "datadog-apm-library-java", released: true, condition: apmLanguageEnabled},
	{Name: "datadog-apm-library-ruby", released: false, condition: apmLanguageEnabled},
	{Name: "datadog-apm-library-js", released: true, condition: apmLanguageEnabled},
	{Name: "datadog-apm-library-dotnet", released: true, condition: apmLanguageEnabled},
	{Name: "datadog-apm-library-python", released: true, condition: apmLanguageEnabled},
	{Name: "datadog-agent", released: false, releasedWithRemoteUpdates: true},
}

var packageDependencies = map[string][]string{
	"datadog-apm-library-java":   {"datadog-apm-inject"},
	"datadog-apm-library-ruby":   {"datadog-apm-inject"},
	"datadog-apm-library-js":     {"datadog-apm-inject"},
	"datadog-apm-library-dotnet": {"datadog-apm-inject"},
	"datadog-apm-library-python": {"datadog-apm-inject"},
}

// DefaultPackages resolves the default packages URLs to install based on the environment.
func DefaultPackages(env *env.Env) []string {
	return defaultPackages(env, PackagesList)
}

func defaultPackages(env *env.Env, defaultPackages []Package) []string {
	var packages []string
	for _, p := range defaultPackages {
		released := p.released || slices.Contains(p.releasedBySite, env.Site) || (p.releasedWithRemoteUpdates && env.RemoteUpdates)
		installOverride, isOverridden := env.DefaultPackagesInstallOverride[p.Name]
		condition := p.condition == nil || p.condition(p, env)

		shouldInstall := released && condition
		if isOverridden {
			shouldInstall = installOverride
		}
		if !shouldInstall {
			continue
		}

		version := "latest"

		// Respect pinned version of APM packages if we don't define any overwrite
		if apmLibVersion, ok := env.ApmLibraries[packageToLanguage(p.Name)]; ok {
			version = apmLibVersion.AsVersionTag()
			// TODO(paullgdc): Emit a warning here if APM packages are not pinned to at least a major
		}

		if v, ok := env.DefaultPackagesVersionOverride[p.Name]; ok {
			version = v
		}
		url := oci.PackageURL(env, p.Name, version)
		packages = append(packages, url)
	}
	return packages
}

func apmInjectEnabled(_ Package, e *env.Env) bool {
	switch e.InstallScript.APMInstrumentationEnabled {
	case env.APMInstrumentationEnabledAll, env.APMInstrumentationEnabledDocker, env.APMInstrumentationEnabledHost:
		return true
	}
	return false
}

func apmLanguageEnabled(p Package, e *env.Env) bool {
	if !apmInjectEnabled(p, e) {
		return false
	}
	if _, ok := e.ApmLibraries[packageToLanguage(p.Name)]; ok {
		return true
	}
	if _, ok := e.ApmLibraries["all"]; ok {
		return true
	}
	return false
}

func packageToLanguage(packageName string) env.ApmLibLanguage {
	lang, found := strings.CutPrefix(packageName, "datadog-apm-library-")
	if !found {
		return ""
	}
	return env.ApmLibLanguage(lang)
}
