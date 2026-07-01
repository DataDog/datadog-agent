// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package scenario

import (
	"context"
	"errors"
	"fmt"
	"time"

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
	// ProvisionWithResources returns the raw outputs directly — no second Pulumi call needed.
	_, res, err := standalone.ProvisionWithResources[Env](ctx, stack, prov)
	if err != nil {
		return err
	}

	// Persist the outputs so future actions can hydrate without Pulumi.
	ps := ProvisionedStack{
		Scenario:  g.s.Name,
		Stack:     stack,
		Config:    cfg,
		Resources: toRawMessage(res),
		CreatedAt: time.Now(),
	}
	if err := SaveProvisionedStack(ps); err != nil {
		return fmt.Errorf("provisioned but failed to record state: %w", err)
	}
	return nil
}

func (g genericRunnable[Env]) RunAction(ctx common.Context, stack, action string, cfg map[string]string) error {
	a, ok := g.s.Actions[action]
	if !ok {
		return fmt.Errorf("unknown action %q for scenario %q", action, g.s.Name)
	}
	// cfg is the ACTION's params — decode them before touching provisioning config.
	var ap any
	if a.NewParams != nil {
		ap = a.NewParams()
		sc, err := BuildSchema(ap)
		if err != nil {
			return err
		}
		if err := Decode(sc, cfg, ap); err != nil {
			return err
		}
	}

	// Hydrate the environment from the local state cache (no Pulumi call).
	// Fall back to a read-only Pulumi outputs read when no local record exists.
	var env *Env
	ps, loadErr := LoadProvisionedStack(stack)
	if loadErr == nil {
		env, loadErr = standalone.HydrateFromResources[Env](ctx, fromRawMessage(ps.Resources))
		if loadErr != nil {
			return fmt.Errorf("hydrate env for action %q from cached state: %w", action, loadErr)
		}
	} else if errors.Is(loadErr, ErrNoProvisionedStack) {
		// No cached state — fall back to a Pulumi read of the live stack outputs.
		var hydrateErr error
		env, hydrateErr = standalone.Hydrate[Env](ctx, stack)
		if hydrateErr != nil {
			return fmt.Errorf("hydrate env for action %q: %w", action, hydrateErr)
		}
	} else {
		return fmt.Errorf("load provisioned stack state for action %q: %w", action, loadErr)
	}

	return a.Run(context.Background(), env, ap)
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
