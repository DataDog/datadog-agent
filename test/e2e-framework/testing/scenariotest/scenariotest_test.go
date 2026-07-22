// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package scenariotest

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenario"
)

type bridgeEnv struct{ ran bool }

func bridgeScenario() scenario.Scenario[bridgeEnv] {
	return scenario.Scenario[bridgeEnv]{
		Name: "bridge",
		Actions: map[string]scenario.Action[bridgeEnv]{
			"go": {Run: func(_ context.Context, e *bridgeEnv, _ any) error { e.ran = true; return nil }},
		},
	}
}

func TestRunActionAgainstProvidedEnv(t *testing.T) {
	env := &bridgeEnv{}
	if err := RunAction(env, bridgeScenario(), "go", nil); err != nil {
		t.Fatalf("RunAction: %v", err)
	}
	if !env.ran {
		t.Fatal("action did not run against provided env")
	}
}
