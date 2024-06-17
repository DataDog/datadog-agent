// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/fleet/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/oci"
	"github.com/stretchr/testify/assert"
)

func TestDefaultPackagesDefaultInstall(t *testing.T) {
	env := &env.Env{}
	packages := DefaultPackages(env)

	assert.Empty(t, packages)
}

func TestDefaultPackagesAPMInjectEnabled(t *testing.T) {
	env := &env.Env{
		InstallScript: env.InstallScriptEnv{
			APMInstrumentationEnabled: env.APMInstrumentationEnabledAll,
		},
	}
	packages := DefaultPackages(env)

	// APM inject packages are not released by default today
	assert.Equal(t, []string{"oci://gcr.io/datadoghq/apm-inject-package:latest"}, packages)
}

func TestDefaultPackages(t *testing.T) {
	type pkg struct {
		n string
		v string
	}
	type testCase struct {
		name     string
		packages []Package
		env      *env.Env
		expected []pkg
	}

	tests := []testCase{
		{
			name:     "No packages",
			packages: []Package{},
			env:      &env.Env{},
			expected: nil,
		},
		{
			name:     "Package not released",
			packages: []Package{{Name: "datadog-agent", released: false}},
			env:      &env.Env{},
			expected: nil,
		},
		{
			name:     "Package released",
			packages: []Package{{Name: "datadog-agent", released: true}},
			env:      &env.Env{},
			expected: []pkg{{n: "datadog-agent", v: "latest"}},
		},
		{
			name:     "Package released with remote updates",
			packages: []Package{{Name: "datadog-agent", released: false, releasedWithRemoteUpdates: true}},
			env:      &env.Env{RemoteUpdates: true},
			expected: []pkg{{n: "datadog-agent", v: "latest"}},
		},
		{
			name:     "Package released to another site",
			packages: []Package{{Name: "datadog-agent", releasedBySite: []string{"datadoghq.eu"}}},
			env:      &env.Env{Site: "datadoghq.com"},
			expected: nil,
		},
		{
			name:     "Package released to the right site",
			packages: []Package{{Name: "datadog-agent", releasedBySite: []string{"datadoghq.eu"}}, {Name: "datadog-package-2", releasedBySite: []string{"datadoghq.com"}}},
			env:      &env.Env{Site: "datadoghq.eu"},
			expected: []pkg{{n: "datadog-agent", v: "latest"}},
		},
		{
			name:     "Package not released but forced install",
			packages: []Package{{Name: "datadog-agent", released: false}},
			env:      &env.Env{DefaultPackagesInstallOverride: map[string]bool{"datadog-agent": true}},
			expected: []pkg{{n: "datadog-agent", v: "latest"}},
		},
		{
			name:     "Package released but condition not met",
			packages: []Package{{Name: "datadog-agent", released: true, condition: func(Package, *env.Env) bool { return false }}},
			env:      &env.Env{},
			expected: nil,
		},
		{
			name:     "Package forced to install and version override",
			packages: []Package{{Name: "datadog-agent", released: false}},
			env:      &env.Env{DefaultPackagesInstallOverride: map[string]bool{"datadog-agent": true}, DefaultPackagesVersionOverride: map[string]string{"datadog-agent": "1.2.3"}},
			expected: []pkg{{n: "datadog-agent", v: "1.2.3"}},
		},
		{
			name:     "APM inject before agent",
			packages: []Package{{Name: "datadog-apm-inject", released: true}, {Name: "datadog-agent", released: true}},
			env:      &env.Env{},
			expected: []pkg{{n: "datadog-apm-inject", v: "latest"}, {n: "datadog-agent", v: "latest"}},
		},
		{
			name:     "Package released but forced not to install",
			packages: []Package{{Name: "datadog-agent", released: true}},
			env:      &env.Env{DefaultPackagesInstallOverride: map[string]bool{"datadog-agent": false}},
			expected: nil,
		},
		{
			name: "Package is a language with a pinned version",
			packages: []Package{
				{Name: "datadog-apm-library-java", released: true, condition: apmLanguageEnabled},
				{Name: "datadog-apm-library-ruby", released: true, condition: apmLanguageEnabled},
				{Name: "datadog-apm-library-js", released: true, condition: apmLanguageEnabled},
			},
			env: &env.Env{
				ApmLibraries: map[env.ApmLibLanguage]env.ApmLibVersion{
					"java": "1.0",
					"ruby": "",
				},
				InstallScript: env.InstallScriptEnv{
					APMInstrumentationEnabled: "all",
				},
			},
			expected: []pkg{{n: "datadog-apm-library-java", v: "1.0-1"}, {n: "datadog-apm-library-ruby", v: "latest"}},
		},
		{
			name: "Package is a language with a pinned version",
			packages: []Package{
				{Name: "datadog-apm-library-java", released: true, condition: apmLanguageEnabled},
				{Name: "datadog-apm-library-ruby", released: true, condition: apmLanguageEnabled},
				{Name: "datadog-apm-library-js", released: true, condition: apmLanguageEnabled},
			},
			env: &env.Env{
				ApmLibraries: map[env.ApmLibLanguage]env.ApmLibVersion{
					"all": "",
				},
				InstallScript: env.InstallScriptEnv{
					APMInstrumentationEnabled: "all",
				},
			},
			expected: []pkg{
				{n: "datadog-apm-library-java", v: "latest"},
				{n: "datadog-apm-library-ruby", v: "latest"},
				{n: "datadog-apm-library-js", v: "latest"},
			},
		},
		{
			name: "Override ignore package pin",
			packages: []Package{
				{Name: "datadog-apm-library-java", released: false, condition: apmLanguageEnabled},
				{Name: "datadog-apm-library-ruby", released: false, condition: apmLanguageEnabled},
				{Name: "datadog-apm-library-js", released: false, condition: apmLanguageEnabled},
			},
			env: &env.Env{
				ApmLibraries: map[env.ApmLibLanguage]env.ApmLibVersion{
					"java": "1.2",
				},
				InstallScript: env.InstallScriptEnv{
					APMInstrumentationEnabled: "all",
				},
				DefaultPackagesInstallOverride: map[string]bool{
					"datadog-apm-library-java": true,
					"datadog-apm-library-ruby": true,
				},
			},
			expected: []pkg{
				{n: "datadog-apm-library-java", v: "1.2-1"},
				{n: "datadog-apm-library-ruby", v: "latest"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packages := defaultPackages(tt.env, tt.packages)
			var expected []string
			for _, p := range tt.expected {
				expected = append(expected, oci.PackageURL(tt.env, p.n, p.v))
			}
			assert.Equal(t, expected, packages)
		})
	}
}
