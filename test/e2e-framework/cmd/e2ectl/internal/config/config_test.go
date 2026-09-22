// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"strings"
	"testing"
)

const validKind = `
schema: 1
environment:
  base: kind
  fakeintake: true
  kind:
    version: "1.31.0"
    nodes: 1
agent:
  version: "7.83.0"
`

const validEC2 = `
schema: 1
environment:
  base: ec2-host
  fakeintake: true
  ec2-host:
    os: ubuntu-22.04
    arch: amd64
agent:
  version: "7.69.0"
`

const validLocal = `
schema: 1
environment:
  base: local
  fakeintake: true
  local: {}
agent:
  source: true
  config: |
    log_level: debug
  integrations:
    cpu.d: |
      init_config: {}
`

func TestParseValid(t *testing.T) {
	for name, content := range map[string]string{"kind": validKind, "ec2": validEC2, "local": validLocal} {
		t.Run(name, func(t *testing.T) {
			f, errs := Parse([]byte(content))
			if len(errs) > 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}
			if !f.FakeIntakeEnabled() {
				t.Error("fakeintake should default to true when not set")
			}
			if f.Environment.Section == nil {
				t.Error("the environment section should be preserved raw for the driver")
			}
			if f.Agent.Install == "" {
				t.Error("the install mechanism should be derived")
			}
		})
	}
}

func TestParseDerivesTheInstallMechanism(t *testing.T) {
	for name, tc := range map[string]struct {
		base      string
		agent     string
		installer string
		section   string
		build     string
	}{
		"local source":      {"local", "  source: true\n  config: |\n    log_level: debug\n", "binary", "log_level: debug", "invoke-binary"},
		"local default":     {"local", "  {}\n", "binary", "{}\n", "invoke-binary"},
		"local pipeline":    {"local", "  pipeline: 138372337\n", "package", "allow-unsigned: true\n", "pipeline"},
		"kind version":      {"kind", "  version: \"7.83.0\"\n", "helm", "version: 7.83.0\n", ""},
		"kind default":      {"kind", "  {}\n", "helm", "version: 7.83.0\n", ""},
		"kind source":       {"kind", "  source: true\n", "helm", "{}\n", "invoke-image"},
		"ec2-host version":  {"ec2-host", "  version: \"7.83.0\"\n", "script", "version: 7.83.0\n", ""},
		"ec2-host pipeline": {"ec2-host", "  pipeline: 138372337\n", "package", "allow-unsigned: true\n", "pipeline"},
		"ec2-host default":  {"ec2-host", "  {}\n", "script", "version: 7.83.0\n", ""},
	} {
		t.Run(name, func(t *testing.T) {
			raw := "schema: 1\nenvironment: {base: " + tc.base + "}\nagent:\n" + tc.agent
			f, errs := Parse([]byte(raw))
			if len(errs) > 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}
			if f.Agent.Install != tc.installer {
				t.Fatalf("derived installer %q, want %q", f.Agent.Install, tc.installer)
			}
			if !strings.Contains(string(f.Agent.Section), strings.TrimSuffix(tc.section, "\n")) {
				t.Fatalf("derived section %q does not contain %q", f.Agent.Section, tc.section)
			}
			if tc.build == "" && f.Agent.Build != nil {
				t.Fatalf("unexpected derived build provider %q", f.Agent.Build.Provider)
			}
			if tc.build != "" && (f.Agent.Build == nil || f.Agent.Build.Provider != tc.build) {
				t.Fatalf("derived build provider %v, want %q", f.Agent.Build, tc.build)
			}
		})
	}
}

func TestParseExtractsTheDriverSection(t *testing.T) {
	f, errs := Parse([]byte(validKind))
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	var section map[string]any
	if err := StrictDecode(f.Environment.Section, &section); err != nil {
		t.Fatalf("section is not valid YAML: %v", err)
	}
	if section["version"] != "1.31.0" {
		t.Errorf("section version mismatch: %v", section)
	}
}

func TestParseUnknownTopLevelFieldIsRejected(t *testing.T) {
	bad := strings.Replace(validKind, "schema: 1", "schema: 1\nextra: true", 1)
	_, errs := Parse([]byte(bad))
	if len(errs) == 0 || !strings.Contains(errs[0].Error(), "unknown top-level field") {
		t.Fatalf("expected unknown top-level field error, got: %v", errs)
	}
}

func TestParseSectionNotMatchingBaseIsRejected(t *testing.T) {
	// a vm: section on a kind base: the coupling T2 removed
	bad := strings.Replace(validKind, "  kind:\n    version: \"1.31.0\"\n    nodes: 1",
		"  kind:\n    version: \"1.31.0\"\n  vm:\n    os: ubuntu-22.04", 1)
	_, errs := Parse([]byte(bad))
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), `environment.vm: not supported for base "kind"`) {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a section/base mismatch error, got: %v", errs)
	}
}

func TestParseSectionBeforeBaseIsAccepted(t *testing.T) {
	bad := `
schema: 1
environment:
  kind:
    nodes: 1
  base: kind
agent:
  version: "7.83.0"
`
	f, errs := Parse([]byte(bad))
	if len(errs) != 0 || f.Environment.Base != "kind" || f.Environment.SectionNode == nil {
		t.Fatalf("mapping key order must not affect parsing: %v", errs)
	}
}

func TestParseErrorsAreAccumulated(t *testing.T) {
	bad := `
schema: 2
environment: {}
agent:
  version: "7.x"
`
	_, errs := Parse([]byte(bad))
	joined := ""
	for _, e := range errs {
		joined += e.Error() + "\n"
	}
	for _, want := range []string{"schema:", "environment.base:"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected an error anchored at %q, got: %v", want, errs)
		}
	}
}

func TestParseExtractsTheAgentSelection(t *testing.T) {
	f, errs := Parse([]byte(validEC2))
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if f.Agent.Selection.Version != "7.69.0" {
		t.Fatalf("agent selection mismatch: %+v", f.Agent.Selection)
	}
	var section map[string]any
	if err := StrictDecode(f.Agent.Section, &section); err != nil {
		t.Fatalf("derived agent section is not valid YAML: %v", err)
	}
	if section["version"] != "7.69.0" {
		t.Errorf("derived agent section mismatch: %v", section)
	}
}

func TestParseLegacyAgentFieldsAreRejectedWithGuidance(t *testing.T) {
	for name, legacy := range map[string]string{
		"install selector": `
schema: 1
environment: {base: local}
agent:
  install: binary
  binary: {}
`,
		"build selector": `
schema: 1
environment: {base: local}
agent:
  source: true
  build:
    provider: invoke-binary
`,
		"installer section": `
schema: 1
environment: {base: kind}
agent:
  version: "7.83.0"
  helm:
    values: |
      datadog: {}
`,
	} {
		t.Run(name, func(t *testing.T) {
			_, errs := Parse([]byte(legacy))
			if len(errs) == 0 {
				t.Fatalf("legacy agent shape must be rejected: %v", errs)
			}
			joined := ""
			for _, e := range errs {
				joined += e.Error() + "\n"
			}
			if !strings.Contains(joined, "see the e2ectl README") {
				t.Fatalf("rejection must point to the new shape, got: %v", errs)
			}
		})
	}
}

func TestParseSourcesAreMutuallyExclusive(t *testing.T) {
	for name, raw := range map[string]string{
		"source and version":   "schema: 1\nenvironment: {base: local}\nagent:\n  source: true\n  version: \"7.83.0\"\n",
		"pipeline and version": "schema: 1\nenvironment: {base: local}\nagent:\n  pipeline: 1\n  version: \"7.83.0\"\n",
		"all three":            "schema: 1\nenvironment: {base: local}\nagent:\n  source: true\n  pipeline: 1\n  version: \"7.83.0\"\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, errs := Parse([]byte(raw))
			if len(errs) == 0 || !strings.Contains(errs[0].Error(), "mutually exclusive") {
				t.Fatalf("expected a mutual-exclusion error, got: %v", errs)
			}
		})
	}
}

func TestParseUnsupportedSourceBaseCombinationsAreRejected(t *testing.T) {
	for name, tc := range map[string]struct {
		base  string
		agent string
		want  string
	}{
		"source on ec2-host": {"ec2-host", "source: true\n", "not yet automated for remote targets"},
		"pipeline on kind":   {"kind", "pipeline: 138372337\n", "pipeline images are not downloadable yet"},
		"version on local":   {"local", "version: \"7.83.0\"\n", "released versions install on the Helm bases"},
	} {
		t.Run(name, func(t *testing.T) {
			raw := "schema: 1\nenvironment: {base: " + tc.base + "}\nagent:\n  " + tc.agent
			_, errs := Parse([]byte(raw))
			if len(errs) == 0 || !strings.Contains(errs[0].Error(), tc.want) {
				t.Fatalf("expected %q, got: %v", tc.want, errs)
			}
		})
	}
}

func TestParseUnknownAgentFieldIsRejected(t *testing.T) {
	raw := "schema: 1\nenvironment: {base: local}\nagent:\n  image: localhost/agent:dev\n"
	_, errs := Parse([]byte(raw))
	if len(errs) == 0 || !strings.Contains(errs[0].Error(), "unknown field (supported: source, pipeline, version, config, integrations, values, receiver)") {
		t.Fatalf("expected an actionable unknown-field error, got: %v", errs)
	}
}

func TestParseAgentRequired(t *testing.T) {
	// The agent key selects the installation source; it may be empty (the
	// base's default), but it cannot be absent — e2ectl configs install agents.
	if _, errs := Parse([]byte("schema: 1\nenvironment: {base: local, fakeintake: true}\n")); len(errs) == 0 {
		t.Fatal("expected an agent-anchored error for a missing agent section")
	}
	if f, errs := Parse([]byte("schema: 1\nenvironment: {base: local, fakeintake: true}\nagent: {}\n")); len(errs) != 0 || f.Agent.Install != "binary" {
		t.Fatalf("an empty agent section must select the base default, got: %v %v", f.Agent, errs)
	}
}

func TestParseValuesRouting(t *testing.T) {
	// values reach the helm section only on kind
	f, errs := Parse([]byte("schema: 1\nenvironment: {base: kind}\nagent:\n  version: \"7.83.0\"\n  values: |\n    datadog:\n      logLevel: DEBUG\n"))
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if !strings.Contains(string(f.Agent.Section), "logLevel") {
		t.Fatalf("values were not routed into the derived helm section: %q", f.Agent.Section)
	}
	_, errs = Parse([]byte("schema: 1\nenvironment: {base: local}\nagent:\n  source: true\n  values: |\n    datadog: {}\n"))
	if len(errs) == 0 {
		t.Fatal("values on a non-kind base must be rejected")
	}
}
