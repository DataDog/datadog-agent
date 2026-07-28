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

// Run runs an fx.App using the supplied options, returning any errors.
//
// This differs from fx.App#Run in that it returns errors instead of exiting
// the process.
func Run(opts ...fx.Option) error {
	if fxAppTestOverride != nil {
		return fxAppTestOverride(func() {}, opts)
	}
	return run(nil, opts...)
}

// RunWithStartupGate constructs an Fx application, waits for the supplied gate before
// starting its lifecycle, and releases it after shutdown.
func RunWithStartupGate[T StartupGate](opts ...fx.Option) error {
	if fxAppTestOverride != nil {
		return fxAppTestOverride(func() {}, opts)
	}

	var gate StartupGate
	opts = append([]fx.Option{StartupGateOption[T](context.Background(), &gate)}, opts...)
	return run(func() StartupGate { return gate }, opts...)
}

func run(gateProvider func() StartupGate, opts ...fx.Option) error {

	opts = append(opts, FxAgentBase())
	// Temporarily increase timeout for all fxutil.Run calls until we can better characterize our
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

	startCtx, cancel := context.WithTimeout(context.Background(), app.StartTimeout())
	defer cancel()

	if err := app.Start(startCtx); err != nil {
		return errors.Join(UnwrapIfErrArgumentsFailed(err), stopAppAndCloseGate(app, gate))
	}
	<-app.Done()

	return stopAppAndCloseGate(app, gate)
}
