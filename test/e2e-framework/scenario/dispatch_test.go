// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package scenario

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
)

type dispEnv struct{ ran string }

type dispActionParams struct {
	Msg string `scenario:"name=msg,required"`
}

// fixedResolver returns a preset env, ignoring ctx/stack.
type fixedResolver[Env any] struct{ env *Env }

func (r fixedResolver[Env]) Resolve(common.Context, string) (*Env, error) { return r.env, nil }

func dispScenario() Scenario[dispEnv] {
	return Scenario[dispEnv]{
		Name: "disp",
		Actions: map[string]Action[dispEnv]{
			"ping": {
				NewParams: func() any { return &dispActionParams{} },
				Run: func(_ context.Context, e *dispEnv, p any) error {
					e.ran = p.(*dispActionParams).Msg
					return nil
				},
			},
		},
	}
}

func TestDispatchActionRunsHandlerWithDecodedParams(t *testing.T) {
	env := &dispEnv{}
	err := DispatchAction(nil, dispScenario(), "stack", "ping",
		map[string]string{"msg": "hi"}, fixedResolver[dispEnv]{env: env})
	if err != nil {
		t.Fatalf("DispatchAction: %v", err)
	}
	if env.ran != "hi" {
		t.Fatalf("handler not run with decoded params: %q", env.ran)
	}
}

func TestDispatchActionUnknownAction(t *testing.T) {
	if err := DispatchAction(nil, dispScenario(), "stack", "nope", nil, fixedResolver[dispEnv]{env: &dispEnv{}}); err == nil {
		t.Fatal("expected error for unknown action")
	}
}
