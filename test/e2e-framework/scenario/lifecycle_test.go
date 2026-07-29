// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package scenario

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/standalone"
)

// fakeComp is an importable component whose Import records the value.
type fakeComp struct {
	components.JSONImporter
	Value string `json:"value"`
}

// lifeEnv is a fake environment with one importable component.
type lifeEnv struct {
	Comp *fakeComp
}

const lifeKey = "comp" // export key the fake provisioner assigns

// lifeProvisioner implements TypedProvisioner[lifeEnv]: it sets the component key
// and returns a matching RawResources map (mirrors what a Pulumi Export does).
type lifeProvisioner struct{}

func (lifeProvisioner) ID() string                                       { return "life-fake" }
func (lifeProvisioner) Destroy(context.Context, string, io.Writer) error { return nil }
func (lifeProvisioner) ProvisionEnv(_ context.Context, _ string, _ io.Writer, env *lifeEnv) (provisioners.RawResources, error) {
	env.Comp.SetKey(lifeKey)
	blob, _ := json.Marshal(map[string]string{"value": "hello"})
	return provisioners.RawResources{lifeKey: blob}, nil
}

func lifeScenario(ran *string) Scenario[lifeEnv] {
	return Scenario[lifeEnv]{
		Name:      "life",
		NewParams: func() any { return &struct{}{} },
		Provisioner: func(any) (provisioners.TypedProvisioner[lifeEnv], error) {
			return lifeProvisioner{}, nil
		},
		Actions: map[string]Action[lifeEnv]{
			"observe": {Run: func(_ context.Context, e *lifeEnv, _ any) error { *ran = e.Comp.Value; return nil }},
		},
	}
}

func TestFullLifecycleNoCloud(t *testing.T) {
	t.Setenv("SCENARIORUN_STATE_DIR", t.TempDir())
	resetRegistry()
	var ran string
	Register(lifeScenario(&ran))
	ctx := standalone.NewContext(t.TempDir())

	// create -> provision + persist state
	if err := Create(ctx, "life", "st1", map[string]string{}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	ps, err := LoadProvisionedStack("st1")
	if err != nil {
		t.Fatalf("expected state record: %v", err)
	}
	if ps.Scenario != "life" || ps.Keys["Comp"] != lifeKey || len(ps.Resources) == 0 {
		t.Fatalf("state record wrong: %+v", ps)
	}

	// action -> hydrate from cached state (no provisioner), run handler
	if err := RunAction(ctx, "life", "st1", "observe", nil); err != nil {
		t.Fatalf("RunAction: %v", err)
	}
	if ran != "hello" {
		t.Fatalf("action saw wrong hydrated value: %q", ran)
	}

	// destroy -> state removed
	if err := Destroy(ctx, "life", "st1"); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if _, err := LoadProvisionedStack("st1"); !errors.Is(err, ErrNoProvisionedStack) {
		t.Fatalf("expected state removed, got %v", err)
	}
}
