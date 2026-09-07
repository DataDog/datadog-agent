// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package rcclient

import (
	"testing"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

func TestServiceParamsPreserveDefaultRemoteConfigWhenUnset(t *testing.T) {
	cfg := configmock.New(t)
	params := serviceParams(cfg)

	if params.BaseURLOverride != "" || len(params.Options) != 0 {
		t.Fatalf("expected no Remote Config overrides when PAR backend settings are unset")
	}
}

func TestServiceParamsUsePrivateActionRunnerSite(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("site", "datadoghq.eu")
	cfg.SetInTest("api_key", "uk1-api-key")
	cfg.SetInTest(setup.PARSite, "datadoghq.com")
	cfg.SetInTest(setup.PARAPIKey, "us1-api-key")

	params := serviceParams(cfg)
	if params.BaseURLOverride != "https://config.datadoghq.com." {
		t.Fatalf("expected US1 PAR Remote Config URL, got %q", params.BaseURLOverride)
	}
	if len(params.Options) == 0 {
		t.Fatal("expected PAR Remote Config authentication and root overrides")
	}
}

type testComponent struct{}

func (*testComponent) SubscribeAgentTask() {}

func (*testComponent) Subscribe(data.Product, func(map[string]state.RawConfig, func(string, state.ApplyStatus))) {
}

type testProofComponent struct {
	testComponent
	proof     state.ConfigTUFProof
	requested string
}

func (c *testProofComponent) GetConfigTUFProof(targetPath string) (state.ConfigTUFProof, bool) {
	c.requested = targetPath
	return c.proof, true
}

func TestGetConfigTUFProofUnavailable(t *testing.T) {
	proof, ok := NewAdapter(&testComponent{}).GetConfigTUFProof("target")
	if ok {
		t.Fatal("expected proof lookup to fail when the RC component does not provide proofs")
	}
	if proof.TargetPath != "" {
		t.Fatalf("expected an empty proof, got target path %q", proof.TargetPath)
	}
}

func TestGetConfigTUFProof(t *testing.T) {
	want := state.ConfigTUFProof{TargetPath: "target"}
	component := &testProofComponent{proof: want}
	proof, ok := NewAdapter(component).GetConfigTUFProof("target")
	if !ok {
		t.Fatal("expected proof lookup to succeed")
	}
	if proof.TargetPath != want.TargetPath {
		t.Fatalf("expected target path %q, got %q", want.TargetPath, proof.TargetPath)
	}
	if component.requested != "target" {
		t.Fatalf("expected target path to be forwarded, got %q", component.requested)
	}
}
