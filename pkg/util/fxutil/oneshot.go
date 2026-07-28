// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fxutil

import (
	"context"
	"errors"

	"go.uber.org/fx"
)

// OneShot runs the given function in an fx.App using the supplied options.
// The function's arguments are supplied by Fx and can be any provided type.
// The function must return `error` or nothing.
//
// The resulting app starts all components, then invokes the function, then
// immediately shuts down.  This is typically used for command-line tools like
// `agent status`.
func OneShot(oneShotFunc interface{}, opts ...fx.Option) error {
	if fxAppTestOverride != nil {
		return fxAppTestOverride(oneShotFunc, opts)
	}
	return oneShot(oneShotFunc, nil, opts...)
}

// OneShotWithStartupGate constructs an Fx application, waits for the supplied gate before
// starting its lifecycle, invokes oneShotFunc, and releases the gate after lifecycle shutdown.
func OneShotWithStartupGate[T StartupGate](oneShotFunc interface{}, opts ...fx.Option) error {
	if fxAppTestOverride != nil {
		return fxAppTestOverride(oneShotFunc, opts)
	}

	var gate StartupGate
	opts = append([]fx.Option{StartupGateOption[T](context.Background(), &gate)}, opts...)
	return oneShot(oneShotFunc, func() StartupGate { return gate }, opts...)
}

func oneShot(oneShotFunc interface{}, gateProvider func() StartupGate, opts ...fx.Option) error {

	// Use a delayed Fx invocation to capture arguments for oneShotFunc during
	// application setup, but not actually invoke the question until all
	// lifecycle start hooks have completed.  Order of lifecycle start hooks is
	// partially ordered by dependencies, but there is no way to guarantee "run
	// this function last".
	delayedCall := newDelayedFxInvocation(oneShotFunc)

	opts = append(opts,
		delayedCall.option(),
		FxAgentBase(),
	)
	// Temporarily increase timeout for all fxutil.OneShot calls until we can better characterize our
	// start time requirements. Prepend to opts so individual calls can override the timeout.
	opts = append(
		[]fx.Option{TemporaryAppTimeouts()},
		opts...,
	)
	app := fx.New(opts...)

	var gate StartupGate
	if gateProvider != nil {
		gate = gateProvider()
	}
	if err := app.Err(); err != nil {
		if gate != nil {
			return errors.Join(UnwrapIfErrArgumentsFailed(err), gate.Close())
		}
		return UnwrapIfErrArgumentsFailed(err)
	}

	// start the app
	startCtx, cancel := context.WithTimeout(context.Background(), app.StartTimeout())
	defer cancel()
	if err := app.Start(startCtx); err != nil {
		return errors.Join(UnwrapIfErrArgumentsFailed(err), stopAppAndCloseGate(app, gate))
	}

	// call the original oneShotFunc with the args captured during app startup
	err := delayedCall.call()
	if err != nil {
		return errors.Join(err, stopAppAndCloseGate(app, gate))
	}

	return stopAppAndCloseGate(app, gate)
}

func stopAppAndCloseGate(app *fx.App, gate StartupGate) error {
	stopErr := stopApp(app)
	if gate != nil {
		return errors.Join(stopErr, gate.Close())
	}
	return stopErr
}

func stopApp(app *fx.App) error {
	// stop the app
	stopCtx, cancel := context.WithTimeout(context.Background(), app.StopTimeout())
	defer cancel()
	if err := app.Stop(stopCtx); err != nil {
		return err
	}

	return nil
}
