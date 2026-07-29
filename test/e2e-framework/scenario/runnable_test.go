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

func (fakeProvisioner) ID() string                                       { return "fake" }
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

func TestRegisterAndDescribe(t *testing.T) {
	resetRegistry()
	Register(newFakeScenario())

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
}

func TestCreateRejectsInvalidParams(t *testing.T) {
	resetRegistry()
	Register(newFakeScenario())

	r, ok := Lookup("fake")
	if !ok {
		t.Fatal("scenario not found")
	}
	ctx := newTestContext(t)

	// Create with fake provisioner works (returns no error from fakeProvisioner.ProvisionEnv).
	if err := r.Create(ctx, "fake-stack", map[string]string{"region": "eu-west-1"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Bad config is rejected before provisioning.
	if err := r.Create(ctx, "fake-stack", map[string]string{"region": "mars"}); err == nil {
		t.Fatal("expected enum validation error")
	}
}

func TestActionParamDecoding(t *testing.T) {
	// Verify that action param decoding works correctly without requiring
	// a real Pulumi stack. RunAction's full flow (Hydrate) needs real infra;
	// this test validates the action schema and decode path in isolation.
	resetRegistry()
	s := newFakeScenario()
	Register(s)

	r, ok := Lookup("fake")
	if !ok {
		t.Fatal("scenario not found")
	}

	// Verify action schemas are built correctly.
	schemas, err := r.ActionSchemas()
	if err != nil {
		t.Fatalf("ActionSchemas: %v", err)
	}
	pingSchema, ok := schemas["ping"]
	if !ok {
		t.Fatal("ping action schema missing")
	}
	if len(pingSchema.Fields) != 1 || pingSchema.Fields[0].Name != "msg" {
		t.Fatalf("ping schema unexpected: %+v", pingSchema)
	}

	// Verify unknown action returns an error immediately (before any provisioning).
	gr := r.(genericRunnable[fakeEnv])
	a, ok := s.Actions["ping"]
	if !ok {
		t.Fatal("ping action missing from scenario")
	}
	// Decode action params directly.
	ap := a.NewParams()
	sc, err := BuildSchema(ap)
	if err != nil {
		t.Fatalf("BuildSchema: %v", err)
	}
	if err := Decode(sc, map[string]string{"msg": "hello"}, ap); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if ap.(*actParams).Msg != "hello" {
		t.Fatalf("expected msg=hello, got %q", ap.(*actParams).Msg)
	}

	// Verify unknown action returns error.
	ctx := newTestContext(t)
	if err := gr.RunAction(ctx, "fake-stack", "no-such-action", nil); err == nil {
		t.Fatal("expected error for unknown action")
	}
}

type testCtx struct{ dir string }

func newTestContext(t *testing.T) *testCtx { return &testCtx{dir: t.TempDir()} }
func (c *testCtx) T() *testing.T              { return nil }
func (c *testCtx) Logf(f string, a ...any)    { log.Printf(f, a...) }
func (c *testCtx) FailNow(f string, a ...any) { log.Fatalf(f, a...) }
func (c *testCtx) SessionOutputDir() string   { return c.dir }
