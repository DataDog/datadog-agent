// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package fxutil

import (
	"context"

	"go.uber.org/fx"
)

// StartupGate delays lifecycle startup until an external condition is satisfied.
type StartupGate interface {
	Wait(context.Context) error
	Close() error
}

// StartupGateOption installs the gate as the first Fx invoke. Fx providers are
// lazy, so later constructors and invokes are not evaluated until Wait returns.
func StartupGateOption[T StartupGate](ctx context.Context, captured *StartupGate) fx.Option {
	return fx.Invoke(func(gate T) error {
		*captured = gate
		return gate.Wait(ctx)
	})
}
