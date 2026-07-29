// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package scenario

import (
	"context"
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/standalone"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
)

// EnvResolver produces the typed environment an action runs against. The CLI
// resolves from local state; a test can resolve to a live suite env.
type EnvResolver[Env any] interface {
	Resolve(ctx common.Context, stack string) (*Env, error)
}

// DispatchAction is the single action code path: decode the action's params,
// resolve the environment, run the handler. Only env resolution is pluggable.
func DispatchAction[Env any](ctx common.Context, s Scenario[Env], stack, action string, cfg map[string]string, resolver EnvResolver[Env]) error {
	a, ok := s.Actions[action]
	if !ok {
		return fmt.Errorf("unknown action %q for scenario %q", action, s.Name)
	}
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
	env, err := resolver.Resolve(ctx, stack)
	if err != nil {
		return fmt.Errorf("action %q: %w", action, err)
	}
	return a.Run(context.Background(), env, ap)
}

// StateResolver hydrates the env from the local provisioned-stack record
// (cached outputs + import keys), with no Pulumi call. This is the CLI default.
type StateResolver[Env any] struct{}

func (StateResolver[Env]) Resolve(ctx common.Context, stack string) (*Env, error) {
	ps, err := LoadProvisionedStack(stack)
	if errors.Is(err, ErrNoProvisionedStack) {
		return nil, fmt.Errorf("no local record for stack %q; actions require a stack created via 'scenariorun create'", stack)
	}
	if err != nil {
		return nil, fmt.Errorf("load provisioned stack state: %w", err)
	}
	env, err := standalone.HydrateFromResources[Env](ctx, fromRawMessage(ps.Resources), ps.Keys)
	if err != nil {
		return nil, fmt.Errorf("hydrate env from cached state: %w", err)
	}
	return env, nil
}
