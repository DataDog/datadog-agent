// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package config

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	schemanoopimpl "github.com/DataDog/datadog-agent/comp/core/schema/noop-impl"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

// TestSBOMUsageDefault checks that the runtime usage enrichment of SBOMs turns
// on, while its setting is unset, where the Agent scans SBOMs and the eBPF
// probe of CWS runs, which a sidecar runs without eBPF unless told otherwise,
// and that the event monitor and the open sampler follow.
func TestSBOMUsageDefault(t *testing.T) {
	const openSampling = "runtime_security_config.event_sampling.open.enabled"
	cws := []string{"runtime_security_config.enabled"}
	containers := []string{"sbom.enabled", "sbom.container_image.enabled"}
	host := []string{"sbom.enabled", "sbom.host.enabled"}

	tests := []struct {
		name     string
		sysprobe []string // set to true, each of them runs the probe
		core     []string // set to true
		explicit *bool    // sbom.enrichment.usage.enabled, unset when nil
		sidecar  bool     // runs as an ECS Fargate sidecar
		ebpf     bool     // runtime_security_config.ebpfless.enabled set to false
		expected bool     // on Linux
	}{
		{name: "CWS and container image SBOMs", sysprobe: cws, core: containers, expected: true},
		{name: "CWS and host SBOM", sysprobe: cws, core: host, expected: true},
		{name: "FIM alone and host SBOM", sysprobe: []string{"runtime_security_config.fim_enabled"}, core: host, expected: true},
		{name: "CWS without SBOM scanning", sysprobe: cws},
		{name: "CWS and SBOM scans without sbom.enabled", sysprobe: cws, core: []string{"sbom.container_image.enabled", "sbom.host.enabled"}},
		{name: "SBOM scanning without CWS", core: []string{"sbom.enabled", "sbom.container_image.enabled", "sbom.host.enabled"}},
		{name: "eBPF-less CWS", sysprobe: []string{"runtime_security_config.enabled", "runtime_security_config.ebpfless.enabled"}, core: host},
		{name: "CWS in a sidecar", sysprobe: cws, core: host, sidecar: true},
		{name: "CWS with eBPF in a sidecar", sysprobe: cws, core: host, sidecar: true, ebpf: true, expected: true},
		{name: "explicit false with CWS and SBOM scanning", sysprobe: cws, core: host, explicit: new(false)},
		{name: "explicit true without CWS", explicit: new(true), expected: true},
	}

	for _, te := range tests {
		t.Run(te.name, func(t *testing.T) {
			// Fresh configs, as Adjust runs once per system-probe config.
			coreCfg := mock.New(t)
			sysprobeCfg := mock.NewSystemProbe(t)
			// SetInTest, as New reloads the file layer of the system-probe config.
			for _, key := range te.sysprobe {
				sysprobeCfg.SetInTest(key, true)
			}
			for _, key := range te.core {
				coreCfg.SetInTest(key, true)
			}
			if te.explicit != nil {
				coreCfg.SetInTest(sbomUsageKey, *te.explicit)
			}
			if te.sidecar {
				env.SetFeatures(t, env.ECSFargate)
			}
			if te.ebpf {
				sysprobeCfg.SetInTest("runtime_security_config.ebpfless.enabled", false)
			}
			// Off, so that only the usage enrichment turns it on.
			sysprobeCfg.SetInTest(openSampling, false)
			require.Equal(t, te.explicit != nil, coreCfg.IsConfigured(sbomUsageKey), "%s is set", sbomUsageKey)

			// Unset, the setting resolves on Linux alone.
			expected := te.expected && (te.explicit != nil || runtime.GOOS == "linux")
			source := model.SourceDefault
			switch {
			case te.explicit != nil:
				source = model.SourceUnknown
			case expected:
				source = model.SourceAgentRuntime
			}

			cfg, err := New("/doesnotexist", "", &schemanoopimpl.SchemaNoop{})
			require.NoError(t, err)

			assert.Equal(t, expected, coreCfg.GetBool(sbomUsageKey))
			assert.Equal(t, source, coreCfg.GetSource(sbomUsageKey))
			assert.Equal(t, expected, sysprobeCfg.GetBool(openSampling), openSampling)
			assert.Equal(t, len(te.sysprobe) > 0 || expected, cfg.ModuleIsEnabled(types.EventMonitorModule), "event monitor")
		})
	}
}
