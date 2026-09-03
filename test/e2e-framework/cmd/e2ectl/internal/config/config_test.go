// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
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
agent:
  install: helm
  image: gcr.io/datadoghq/agent:7.99.0-e2ectl
`

const validEC2 = `
schema: 1
environment:
  base: ec2-host
  vm:
    os: ubuntu-22.04
    arch: amd64
  fakeintake: true
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
		})
	}
}

func TestParseUnknownFieldIsRejected(t *testing.T) {
	bad := strings.Replace(validKind, "base: kind", "base: kind\n  instnce-type: t3.medium", 1)
	_, errs := Parse([]byte(bad))
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %v", errs)
	}
	if !strings.Contains(errs[0].Error(), "field instnce-type") {
		t.Errorf("error should be field-anchored, got: %v", errs[0])
	}
}

func TestParseErrorsAreAccumulated(t *testing.T) {
	bad := `
schema: 2
environment:
  base: kubernetes
agent:
  install: puppet
`
	_, errs := Parse([]byte(bad))
	if len(errs) < 3 {
		t.Fatalf("expected accumulated errors for schema, base and install, got: %v", errs)
	}
}

func TestParseCrossFieldRules(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(string) string
		wantErr string
	}{
		{"helm on host", func(s string) string {
			s = strings.Replace(s, "install: script", "install: helm", 1)
			return s
		}, "agent.install"},
		{"script on kind", func(s string) string {
			s = strings.Replace(validKind, "install: helm", "install: script", 1)
			s = strings.Replace(s, "image: gcr.io/datadoghq/agent:7.99.0-e2ectl", "", 1)
			s = strings.Replace(s, "agent:", "agent:\n  version: \"7.69.0\"", 1)
			return s
		}, "agent.install"},
		{"missing version for script", func(s string) string {
			return strings.Replace(s, `  version: "7.69.0"`, "", 1)
		}, "agent.version"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, errs := Parse([]byte(tc.mutate(validEC2)))
			if len(errs) == 0 {
				t.Fatal("expected an error")
			}
			if !strings.Contains(errs[0].Error(), tc.wantErr) {
				t.Errorf("expected error anchored at %q, got: %v", tc.wantErr, errs)
			}
		})
	}
}

func TestParseImageTagMustBeSemverShaped(t *testing.T) {
	bad := strings.Replace(validKind, "7.99.0-e2ectl", "e2ectl-dev", 1)
	_, errs := Parse([]byte(bad))
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %v", errs)
	}
	if !strings.Contains(errs[0].Error(), "agent.image") || !strings.Contains(errs[0].Error(), "semver") {
		t.Errorf("expected a semver-shaped tag error, got: %v", errs[0])
	}
}

func TestParseBadOS(t *testing.T) {
	bad := strings.Replace(validEC2, "ubuntu-22.04", "ubunt-22.04", 1)
	_, errs := Parse([]byte(bad))
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %v", errs)
	}
	if !strings.Contains(errs[0].Error(), "environment.vm.os") {
		t.Errorf("expected OS error with supported list, got: %v", errs[0])
	}
}
