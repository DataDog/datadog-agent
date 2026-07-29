// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package scenariotest bridges a registered scenario into an e2e BaseSuite test:
// provision via the scenario's own provisioner, and run its actions against the
// live suite env. Kept separate from the core scenario package so that package
// takes no test-harness dependency.
package scenariotest

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenario"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/standalone"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
)

// WithScenario provisions the scenario with params as a BaseSuite provisioner.
// No CLI state is written — the suite owns the env lifecycle.
func WithScenario[Env any](s scenario.Scenario[Env], params any) e2e.SuiteOption {
	prov, err := s.Provisioner(params)
	if err != nil {
		panic(fmt.Sprintf("scenariotest.WithScenario(%q): %v", s.Name, err))
	}
	return e2e.WithProvisioner(prov)
}

// fixedResolver resolves to a preset env (the live suite env), ignoring ctx/stack.
type fixedResolver[Env any] struct{ env *Env }

func (r fixedResolver[Env]) Resolve(common.Context, string) (*Env, error) { return r.env, nil }

// RunAction runs the scenario's action against env via the shared DispatchAction
// (real param decode + handler). Use sparingly — to test that a defined action
// works, not to drive test mutations.
func RunAction[Env any](env *Env, s scenario.Scenario[Env], action string, config map[string]string) error {
	ctx := standalone.NewContext(os.TempDir()) // resolver ignores it
	return scenario.DispatchAction[Env](ctx, s, "", action, config, fixedResolver[Env]{env: env})
}
