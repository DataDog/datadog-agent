// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package actor provides basic support for building actors for use in the Agent.
//
// Methods on this component are not re-entrant.  Components using this one
// should _either_ call HookLifecycle once in their constructor or call Start
// and Stop from their lifecycle hook.
package actor

import (
	"context"

	"go.uber.org/fx"
)

// NOTE: when a health component is introduced, this package should be updated
// to automatically register with that component. See
// https://github.com/DataDog/dd-agent-comp-experiments/tree/main/pkg/util/actor

// Actor manages a component structured as an actor, supporting starting and
// later stopping the goroutine.  This is one-shot: once started and stopped,
// the goroutine cannot be started again.
type Actor struct {
	// started is true after the goroutine has been started, and remains true after
	// it has stopped.
	started bool

	// cancel cancels the context passed to the `run` function, used to signal
	// that it should stop
	cancel context.CancelFunc

	// stopped is closed once the run function returns.
	stopped chan struct{}
}

// New creates a new actor.
func New() *Actor {
	return &Actor{}
}

// RunFunc defines the function implementing the actor's event loop.  It should
// run until the passed context is cancelled.
//
// The loop should read from `alive`, discarding the results.  This is used to
// monitor the actor's health, ensuring that the component is returning to
// its main loop frequently.
type RunFunc func(ctx context.Context, alive <-chan struct{})

// HookLifecycle connects this actor to the given fx.Lifecycle, starting and
// stopping it with the lifecycle.  Use this method _or_ the Start and Stop methods,
// but not both.
func (a *Actor) HookLifecycle(lc fx.Lifecycle, runFunc RunFunc) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			a.Start(runFunc)
			return nil
		},
		OnStop: a.Stop,
	})
}

// Start starts run in a goroutine, setting up to stop it by cancelling the context
// it receives.
func (a *Actor) Start(runFunc RunFunc) {
	if a.started {
		panic("Goroutine has already been started")
	}
	a.started = true

	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.stopped = make(chan struct{})

	go a.run(ctx, runFunc)
}

// Stop stops the goroutine, waiting until it is complete, or the given context
// is cancelled, before returning.  Returns the error from context if it is
// cancelled.
func (a *Actor) Stop(ctx context.Context) error {
	if !a.started {
		panic("Goroutine has not been started")
	}
	if a.cancel == nil {
		panic("Goroutine has already been stopped")
	}
	a.cancel()
	a.cancel = nil
	select {
	case <-a.stopped:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// run executes the given function, ensuring that the stopped channel is closed
// when it finishes.  This method runs in a dedicated goroutine.
func (a *Actor) run(ctx context.Context, runFunc RunFunc) {
	defer close(a.stopped)
	alive := make(chan struct{}, 1)
	runFunc(ctx, alive)
}
