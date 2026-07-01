// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package scenario

import (
	"context"
	"io"
	"log"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
)

// fakeEnv is a stand-in environment; actions record that they ran.
type fakeEnv struct{ ran string }

type fakeParams struct {
	Region string `scenario:"name=region,default=us-east-1,enum=us-east-1|eu-west-1"`
}
type actParams struct {
	Msg string `scenario:"name=msg,required"`
}

// fakeProvisioner satisfies provisioners.TypedProvisioner[fakeEnv] without cloud calls.
type fakeProvisioner struct{}

func (fakeProvisioner) ID() string                                     { return "fake" }
func (fakeProvisioner) Destroy(context.Context, string, io.Writer) error { return nil }
func (fakeProvisioner) ProvisionEnv(_ context.Context, _ string, _ io.Writer, e *fakeEnv) (provisioners.RawResources, error) {
	return provisioners.RawResources{}, nil
}

func newFakeScenario() Scenario[fakeEnv] {
	return Scenario[fakeEnv]{
		Name:        "fake",
		Description: "fake scenario",
		NewParams:   func() any { return &fakeParams{} },
		Provisioner: func(any) (provisioners.TypedProvisioner[fakeEnv], error) {
			return fakeProvisioner{}, nil
		},
		Actions: map[string]Action[fakeEnv]{
			"ping": {
				Description: "ping",
				NewParams:   func() any { return &actParams{} },
				Run: func(_ context.Context, e *fakeEnv, p any) error {
					e.ran = p.(*actParams).Msg
					return nil
				},
			},
		},
	}
}

func TestRegisterDescribeAndDrive(t *testing.T) {
	resetRegistry()
	Register(newFakeScenario())

	// Describe
	d, err := Describe()
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if d.ProtocolVersion != 1 || len(d.Scenarios) != 1 {
		t.Fatalf("describe wrong: %+v", d)
	}
	if d.Scenarios[0].Params.Fields[0].Name != "region" {
		t.Fatalf("params schema wrong: %+v", d.Scenarios[0].Params)
	}
	if _, ok := d.Scenarios[0].Actions["ping"]; !ok {
		t.Fatalf("action schema missing")
	}

	// Drive create + action (fake provisioner, standalone with a temp dir)
	r, ok := Lookup("fake")
	if !ok {
		t.Fatal("scenario not found")
	}
	ctx := newTestContext(t)
	if err := r.Create(ctx, "fake-stack", map[string]string{"region": "eu-west-1"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := r.RunAction(ctx, "fake-stack", "ping", map[string]string{"msg": "hi"}); err != nil {
		t.Fatalf("RunAction: %v", err)
	}
	// bad config rejected before provisioning
	if err := r.Create(ctx, "fake-stack", map[string]string{"region": "mars"}); err == nil {
		t.Fatal("expected enum validation error")
	}
}

type testCtx struct{ dir string }

func newTestContext(t *testing.T) *testCtx { return &testCtx{dir: t.TempDir()} }
func (c *testCtx) T() *testing.T              { return nil }
func (c *testCtx) Logf(f string, a ...any)    { log.Printf(f, a...) }
func (c *testCtx) FailNow(f string, a ...any) { log.Fatalf(f, a...) }
func (c *testCtx) SessionOutputDir() string   { return c.dir }
