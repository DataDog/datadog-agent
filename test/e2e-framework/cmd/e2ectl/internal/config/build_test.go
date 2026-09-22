// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"strings"
	"testing"
)

// The build-provider selection is DERIVED, never parsed: agent.build is gone
// from user YAML (breaking change). These tests pin the derived selections the
// build-provider registries consume.
func TestDerivedBuildSelections(t *testing.T) {
	for name, tc := range map[string]struct {
		base     string
		agent    string
		provider string
		section  string
	}{
		"local source":      {"local", "source: true\n", "invoke-binary", "repository: "},
		"local default":     {"local", "  {}\n", "invoke-binary", "repository: "},
		"kind source":       {"kind", "source: true\n", "invoke-image", "reference: localhost/datadog-agent:7.83.0-e2ectl-dev"},
		"local pipeline":    {"local", "pipeline: 138372337\n", "pipeline", "pipeline: 138372337"},
		"ec2-host pipeline": {"ec2-host", "pipeline: 138372337\n", "pipeline", "pipeline: 138372337"},
	} {
		t.Run(name, func(t *testing.T) {
			raw := "schema: 1\nenvironment: {base: " + tc.base + "}\nagent:\n  " + tc.agent
			cfg, errs := Parse([]byte(raw))
			if len(errs) != 0 {
				t.Fatal(errs)
			}
			if cfg.Agent.Build == nil || cfg.Agent.Build.Provider != tc.provider {
				t.Fatalf("derived build provider %v, want %q", cfg.Agent.Build, tc.provider)
			}
			if !strings.Contains(string(cfg.Agent.Build.Section), tc.section) {
				t.Fatalf("derived build section %q does not contain %q", cfg.Agent.Build.Section, tc.section)
			}
		})
	}
	for name, raw := range map[string]string{
		"no build for released versions": "schema: 1\nenvironment: {base: kind}\nagent:\n  version: \"7.83.0\"\n",
		"no build for scripts":           "schema: 1\nenvironment: {base: ec2-host}\nagent:\n  version: \"7.83.0\"\n",
	} {
		t.Run(name, func(t *testing.T) {
			cfg, errs := Parse([]byte(raw))
			if len(errs) != 0 || cfg.Agent.Build != nil {
				t.Fatal(cfg.Agent.Build, errs)
			}
		})
	}
}

// Old configs with an agent.build block must fail with a pointer to the new
// shape, not silently drop the block.
func TestLegacyBuildBlocksAreRejected(t *testing.T) {
	for name, build := range map[string]string{
		"provider":     "  build:\n    provider: invoke-binary\n",
		"with section": "  build:\n    provider: existing-image\n    existing-image:\n      reference: localhost/agent:dev\n",
	} {
		t.Run(name, func(t *testing.T) {
			raw := "schema: 1\nenvironment: {base: local}\nagent:\n  source: true\n" + build
			if _, errs := Parse([]byte(raw)); len(errs) == 0 || !strings.Contains(errs[0].Error(), "agent.build: no longer exists") {
				t.Fatalf("expected a legacy agent.build rejection, got: %v", errs)
			}
		})
	}
}
