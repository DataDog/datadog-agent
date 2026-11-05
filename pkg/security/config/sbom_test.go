// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// TestSBOMResolverGates checks the settings that bring up the SBOM resolver
// and its index of the host packages. CWS sets them in system-probe.yaml.
// Runtime usage enrichment sets them in datadog.yaml, where the index of the
// host also needs the host SBOM that takes its usage.
func TestSBOMResolverGates(t *testing.T) {
	for _, tc := range []struct {
		name     string
		sysprobe []string
		core     []string
		resolver bool
		host     bool
	}{
		{
			name: "defaults",
		},
		{
			name:     "CWS",
			sysprobe: []string{"runtime_security_config.sbom.enabled", "runtime_security_config.sbom.host.enabled"},
			resolver: true,
			host:     true,
		},
		{
			name:     "CWS for containers",
			sysprobe: []string{"runtime_security_config.sbom.enabled"},
			resolver: true,
		},
		{
			name:     "usage enrichment of the host",
			core:     []string{"sbom.enabled", "sbom.host.enabled", "sbom.enrichment.usage.enabled"},
			resolver: true,
			host:     true,
		},
		{
			name:     "usage enrichment of containers",
			core:     []string{"sbom.enabled", "sbom.enrichment.usage.enabled"},
			resolver: true,
		},
		{
			name:     "usage enrichment with SBOM collection off",
			core:     []string{"sbom.host.enabled", "sbom.enrichment.usage.enabled"},
			resolver: true,
		},
		{
			name: "host SBOM alone",
			core: []string{"sbom.enabled", "sbom.host.enabled"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			core := configmock.New(t)
			sysprobe := configmock.NewSystemProbe(t)
			for _, key := range tc.core {
				core.SetInTest(key, true)
			}
			for _, key := range tc.sysprobe {
				sysprobe.SetInTest(key, true)
			}

			cfg, err := NewRuntimeSecurityConfig()
			require.NoError(t, err)
			assert.Equal(t, tc.resolver, cfg.SBOMResolverEnabled, "SBOMResolverEnabled")
			assert.Equal(t, tc.host, cfg.SBOMResolverHostEnabled, "SBOMResolverHostEnabled")
		})
	}
}
