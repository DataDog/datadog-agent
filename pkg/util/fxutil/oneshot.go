// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fxutil

import (
	"context"
	"errors"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil/lifecycle"
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
	recorder := lifecycle.Default()
	recorder.BeforeFxNew()
	app := fx.New(opts...)
	recorder.FxConstructed()

	// start the app
	startCtx, cancel := context.WithTimeout(context.Background(), app.StartTimeout())
	defer cancel()
	startBegin := time.Now()
	if err := app.Start(startCtx); err != nil {
		recorder.BeginStop(lifecycle.TriggerError)
		return errors.Join(UnwrapIfErrArgumentsFailed(err), stopApp(app))
	}
	recorder.FxStarted(startBegin)

	// call the original oneShotFunc with the args captured during app startup
	err := delayedCall.call()
	if err != nil {
		recorder.BeginStop(lifecycle.TriggerError)
		return errors.Join(err, stopApp(app))
	}

	return stopApp(app)
}

func stopApp(app *fx.App) error {
	// Record the Fx stop as a shutdown phase. It begins the shutdown if the process did not do it
	// already, and completes it once the Fx application is stopped.
	recorder := lifecycle.Default()
	phaseDone := recorder.StartShutdownPhase(lifecycle.PhaseFxStop)

	// stop the app
	stopCtx, cancel := context.WithTimeout(context.Background(), app.StopTimeout())
	defer cancel()
	if err := app.Stop(stopCtx); err != nil {
		// Keep the hook that was running when the stop failed, e.g. on timeout.
		recorder.Finish(err)
		return err
	}

	phaseDone()
	recorder.Finish(nil)
	return nil
}
