// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package fxutil

import (
	"context"
	"errors"

	"go.uber.org/fx"
)

// StartupGate delays lifecycle startup until an external ownership condition is satisfied.
type StartupGate interface {
	Wait(context.Context) error
	MarkActive() error
	Close() error
}

// ErrStartupGateShutdown means an Fx shutdown signal arrived while the gate was waiting.
var ErrStartupGateShutdown = errors.New("shutdown while waiting on startup gate")

// WaitForStartupGate waits for a gate and cancels it when the caller or Fx requests shutdown.
func WaitForStartupGate(ctx context.Context, app *fx.App, gate StartupGate) error {
	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	waitResult := make(chan error, 1)
	go func() {
		waitResult <- gate.Wait(waitCtx)
	}()

	select {
	case err := <-waitResult:
		return err
	case <-app.Done():
		cancel()
		<-waitResult
		return ErrStartupGateShutdown
	case <-ctx.Done():
		cancel()
		<-waitResult
		return ctx.Err()
	}
}
