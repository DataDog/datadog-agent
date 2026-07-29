// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package ec2host

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenario"
)

func TestScenarioSchemaExposesAgentFlags(t *testing.T) {
	s := Scenario()
	sc, err := scenario.BuildSchema(s.NewParams())
	if err != nil {
		t.Fatalf("BuildSchema: %v", err)
	}
	names := map[string]bool{}
	for _, f := range sc.Fields {
		names[f.Name] = true
	}
	for _, want := range []string{"os", "arch", "agent-version", "agent-flavor", "agent-config-path", "use-fakeintake"} {
		if !names[want] {
			t.Errorf("expected flag %q in schema, missing (got %v)", want, names)
		}
	}
}

func TestProvisionerBuilds(t *testing.T) {
	p := NewParams()
	prov, err := Provisioner(p)
	if err != nil {
		t.Fatalf("Provisioner: %v", err)
	}
	if prov == nil {
		t.Fatal("nil provisioner")
	}
}

func TestProvisionerBuildsWithAgent(t *testing.T) {
	p := NewParams()
	if !p.Agent.Install {
		t.Fatal("NewParams should set Agent.Install=true")
	}
	prov, err := Provisioner(p)
	if err != nil {
		t.Fatalf("Provisioner with agent enabled: %v", err)
	}
	if prov == nil {
		t.Fatal("nil provisioner with agent enabled")
	}
}

func TestProvisionerBuildsArm64(t *testing.T) {
	p := NewParams()
	p.Arch = "arm64"
	prov, err := Provisioner(p)
	if err != nil {
		t.Fatalf("Provisioner with arm64: %v", err)
	}
	if prov == nil {
		t.Fatal("nil provisioner for arm64")
	}
}

func TestProvisionerRejectsUnknownArch(t *testing.T) {
	p := NewParams()
	p.Arch = "mips64"
	_, err := Provisioner(p)
	if err == nil {
		t.Fatal("expected error for unknown arch, got nil")
	}
}

func TestActionsRegistered(t *testing.T) {
	s := Scenario()
	if _, ok := s.Actions["restart-agent"]; !ok {
		t.Error("restart-agent action missing")
	}
	if _, ok := s.Actions["run-command"]; !ok {
		t.Error("run-command action missing")
	}
}
