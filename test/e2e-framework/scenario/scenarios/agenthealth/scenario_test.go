// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agenthealth

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenario"
)

func TestSchemaExposesAgentAndOSFlags(t *testing.T) {
	sc, err := scenario.BuildSchema(NewParams())
	if err != nil {
		t.Fatalf("BuildSchema: %v", err)
	}
	names := map[string]bool{}
	for _, f := range sc.Fields {
		names[f.Name] = true
	}
	for _, want := range []string{"os", "arch", "agent-version"} {
		if !names[want] {
			t.Errorf("missing flag %q (got %v)", want, names)
		}
	}
}

func TestProvisionerBuilds(t *testing.T) {
	prov, err := Provisioner(NewParams())
	if err != nil || prov == nil {
		t.Fatalf("Provisioner: prov=%v err=%v", prov, err)
	}
}

func TestActionsRegistered(t *testing.T) {
	a := Scenario().Actions
	if _, ok := a["connection-info"]; !ok {
		t.Error("connection-info action missing")
	}
	if _, ok := a["restart-agent"]; !ok {
		t.Error("restart-agent action missing")
	}
}
