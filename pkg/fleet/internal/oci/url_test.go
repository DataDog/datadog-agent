// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package oci

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/fleet/env"
)

func TestPackageURL(t *testing.T) {
	type test struct {
		registryOverride          string
		registryByPackageOverride map[string]string
		site                      string
		pkg                       string
		version                   string
		expected                  string
	}

	tests := []test{
		{site: "datad0g.com", pkg: "datadog-agent", version: "latest", expected: "oci://docker.io/datadog/agent-package-dev:latest"},
		{site: "datadoghq.com", pkg: "datadog-agent", version: "1.2.3", expected: "oci://gcr.io/datadoghq/agent-package:1.2.3"},
		{site: "datadoghq.com", pkg: "datadog-agent", version: "1.2.3", registryOverride: "kebab.io/sauceharissa", expected: "oci://kebab.io/sauceharissa/agent-package:1.2.3"},
		{site: "datadoghq.com", pkg: "datadog-agent", version: "1.2.3", registryByPackageOverride: map[string]string{"datadog-agent": "kebab.io/sauceharissa"}, expected: "oci://kebab.io/sauceharissa/agent-package:1.2.3"},
		{site: "datadoghq.com", pkg: "datadog-apm-inect", version: "1.2.3", registryByPackageOverride: map[string]string{"datadog-agent": "kebab.io/sauceharissa"}, expected: "oci://gcr.io/datadoghq/apm-inect-package:1.2.3"},
		{site: "datadoghq.com", pkg: "datadog-agent", version: "1.2.3", registryByPackageOverride: map[string]string{"datadog-apm-inject": "kebab.io/sauceharissa"}, registryOverride: "kebab.io/sauceblanche", expected: "oci://kebab.io/sauceblanche/agent-package:1.2.3"},
	}

	for _, tt := range tests {
		t.Run(tt.site, func(t *testing.T) {
			env := env.Env{
				Site:                      tt.site,
				RegistryOverride:          tt.registryOverride,
				RegistryOverrideByPackage: tt.registryByPackageOverride,
			}
			actual := PackageURL(&env, tt.pkg, tt.version)
			if actual != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, actual)
			}
		})
	}
}
