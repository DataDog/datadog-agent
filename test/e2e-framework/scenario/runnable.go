// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package scenario

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/standalone"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
)

// Runnable is the type-erased interface the CLI and service drive.
type Runnable interface {
	Name() string
	Description() string
	ParamsSchema() (Schema, error)
	ActionSchemas() (map[string]Schema, error)
	Create(ctx common.Context, stack string, cfg map[string]string) error
	RunAction(ctx common.Context, stack, action string, cfg map[string]string) error
	Destroy(ctx common.Context, stack string) error
}

type genericRunnable[Env any] struct{ s Scenario[Env] }

func (g genericRunnable[Env]) Name() string        { return g.s.Name }
func (g genericRunnable[Env]) Description() string { return g.s.Description }

func (g genericRunnable[Env]) ParamsSchema() (Schema, error) {
	return BuildSchema(g.s.NewParams())
}

func (g genericRunnable[Env]) ActionSchemas() (map[string]Schema, error) {
	out := map[string]Schema{}
	for name, a := range g.s.Actions {
		if a.NewParams == nil {
			out[name] = Schema{}
			continue
		}
		sc, err := BuildSchema(a.NewParams())
		if err != nil {
			return nil, fmt.Errorf("action %q: %w", name, err)
		}
		out[name] = sc
	}
	return out, nil
}

func (g genericRunnable[Env]) decodeParams(cfg map[string]string) (any, error) {
	p := g.s.NewParams()
	sc, err := BuildSchema(p)
	if err != nil {
		return nil, err
	}
	if err := Decode(sc, cfg, p); err != nil {
		return nil, err
	}
	return p, nil
}

func (g genericRunnable[Env]) buildProvisioner(cfg map[string]string) (provisioners.TypedProvisioner[Env], error) {
	p, err := g.decodeParams(cfg)
	if err != nil {
		return nil, err
	}
	return g.s.Provisioner(p)
}

func (g genericRunnable[Env]) Create(ctx common.Context, stack string, cfg map[string]string) error {
	prov, err := g.buildProvisioner(cfg)
	if err != nil {
		return err
	}
	// ProvisionWithResources returns the raw outputs and the hydrated env directly
	// — no second Pulumi call needed.
	env, res, err := standalone.ProvisionWithResources[Env](ctx, stack, prov)
	if err != nil {
		return err
	}

	// Snapshot the import keys from the provisioned env so they can be replayed
	// at action time without running the Pulumi program again.
	keys := environments.ImportKeys(env)

	// Persist the outputs and keys so future actions can hydrate without Pulumi.
	ps := ProvisionedStack{
		Scenario:  g.s.Name,
		Stack:     stack,
		Config:    cfg,
		Resources: toRawMessage(res),
		Keys:      keys,
		CreatedAt: time.Now(),
	}
	if err := SaveProvisionedStack(ps); err != nil {
		return fmt.Errorf("provisioned but failed to record state: %w", err)
	}
	return nil
}

func (g genericRunnable[Env]) RunAction(ctx common.Context, stack, action string, cfg map[string]string) error {
	return DispatchAction(ctx, g.s, stack, action, cfg, StateResolver[Env]{})
}

func (g genericRunnable[Env]) Destroy(ctx common.Context, stack string) error {
	prov, err := g.buildProvisioner(nil)
	if err != nil {
		return err
	}
	if err := standalone.Destroy(ctx, stack, prov); err != nil {
		return err
	}
	// Best-effort cleanup of local state — log but do not fail the destroy.
	if err := DeleteProvisionedStack(stack); err != nil {
		ctx.Logf("scenariorun: warning: failed to delete local state for stack %q: %v", stack, err)
	}
	return nil
}
