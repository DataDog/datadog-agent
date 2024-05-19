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

	// No packages released by default today
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
	assert.Empty(t, packages)
}

func TestDefaultPackages(t *testing.T) {
	type pkg struct {
		n string
		v string
	}
	type testCase struct {
		name     string
		packages []defaultPackage
		env      *env.Env
		expected []pkg
	}

	tests := []testCase{
		{
			name:     "No packages",
			packages: []defaultPackage{},
			env:      &env.Env{},
			expected: nil,
		},
		{
			name:     "Package not released",
			packages: []defaultPackage{{name: "datadog-agent", released: false}},
			env:      &env.Env{},
			expected: nil,
		},
		{
			name:     "Package released",
			packages: []defaultPackage{{name: "datadog-agent", released: true}},
			env:      &env.Env{},
			expected: []pkg{{n: "datadog-agent", v: "latest"}},
		},
		{
			name:     "Package released to another site",
			packages: []defaultPackage{{name: "datadog-agent", releasedBySite: []string{"datadoghq.eu"}}},
			env:      &env.Env{Site: "datadoghq.com"},
			expected: nil,
		},
		{
			name:     "Package released to the right site",
			packages: []defaultPackage{{name: "datadog-agent", releasedBySite: []string{"datadoghq.eu"}}, {name: "datadog-package-2", releasedBySite: []string{"datadoghq.com"}}},
			env:      &env.Env{Site: "datadoghq.eu"},
			expected: []pkg{{n: "datadog-agent", v: "latest"}},
		},
		{
			name:     "Package not released but forced install",
			packages: []defaultPackage{{name: "datadog-agent", released: false}},
			env:      &env.Env{DefaultPackagesInstallOverride: map[string]bool{"datadog-agent": true}},
			expected: []pkg{{n: "datadog-agent", v: "latest"}},
		},
		{
			name:     "Package released but condition not met",
			packages: []defaultPackage{{name: "datadog-agent", released: true, condition: func(e *env.Env) bool { return false }}},
			env:      &env.Env{},
			expected: nil,
		},
		{
			name:     "Package forced to install and version override",
			packages: []defaultPackage{{name: "datadog-agent", released: false}},
			env:      &env.Env{DefaultPackagesInstallOverride: map[string]bool{"datadog-agent": true}, DefaultPackagesVersionOverride: map[string]string{"datadog-agent": "1.2.3"}},
			expected: []pkg{{n: "datadog-agent", v: "1.2.3"}},
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
