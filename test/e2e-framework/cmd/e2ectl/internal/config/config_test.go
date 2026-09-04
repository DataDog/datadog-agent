// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present, Datadog, Inc.

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
  install: helm
  image: gcr.io/datadoghq/agent:7.99.0-e2ectl
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
  install: script
  version: "7.69.0"
`

func TestParseValid(t *testing.T) {
	for name, content := range map[string]string{"kind": validKind, "ec2": validEC2} {
		t.Run(name, func(t *testing.T) {
			f, errs := Parse([]byte(content))
			if len(errs) > 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}
			if !f.FakeIntakeEnabled() {
				t.Error("fakeintake should default to true when not set")
			}
			if name == "kind" && f.Environment.Section == nil {
				t.Error("the kind section should be preserved raw for the driver")
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

func TestParseSectionBeforeBaseIsRejected(t *testing.T) {
	bad := `
schema: 1
environment:
  kind:
    nodes: 1
  base: kind
agent:
  install: helm
`
	_, errs := Parse([]byte(bad))
	if len(errs) == 0 {
		t.Fatal("expected an error (base must come before its section)")
	}
}

func TestParseErrorsAreAccumulated(t *testing.T) {
	bad := `
schema: 2
environment: {}
agent:
  install: nope
  version: "7.x"
`
	_, errs := Parse([]byte(bad))
	joined := ""
	for _, e := range errs {
		joined += e.Error() + "\n"
	}
	for _, want := range []string{"schema:", "environment.base:", "agent.version:"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected an error anchored at %q, got: %v", want, errs)
		}
	}
}

func TestParseGenericAgentRules(t *testing.T) {
	bad := strings.Replace(validKind, "image: gcr.io/datadoghq/agent:7.99.0-e2ectl",
		"image: gcr.io/datadoghq/agent:e2ectl-dev", 1)
	_, errs := Parse([]byte(bad))
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %v", errs)
	}
	if !strings.Contains(errs[0].Error(), "semver-shaped") {
		t.Errorf("expected the semver-shaped tag rule, got: %v", errs[0])
	}
}
