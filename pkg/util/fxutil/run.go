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

// Run runs an fx.App using the supplied options, returning any errors.
//
// This differs from fx.App#Run in that it returns errors instead of exiting
// the process.
func Run(opts ...fx.Option) error {
	if fxAppTestOverride != nil {
		return fxAppTestOverride(func() {}, opts)
	}

	opts = append(opts, FxAgentBase())
	// Temporarily increase timeout for all fxutil.Run calls until we can better characterize our
	// start time requirements. Prepend to opts so individual calls can override the timeout.
	opts = append(
		[]fx.Option{TemporaryAppTimeouts()},
		opts...,
	)
	recorder := lifecycle.Default()
	recorder.BeforeFxNew()
	app := fx.New(opts...)
	recorder.FxConstructed()

	if err := app.Err(); err != nil {
		return err
	}

	startCtx, cancel := context.WithTimeout(context.Background(), app.StartTimeout())
	defer cancel()

	startBegin := time.Now()
	if err := app.Start(startCtx); err != nil {
		recorder.BeginStop(lifecycle.TriggerError)
		return errors.Join(UnwrapIfErrArgumentsFailed(err), stopApp(app))
	}
	recorder.FxStarted(startBegin)

	<-app.Done()
	recorder.BeginStop(lifecycle.TriggerSignal)

	return stopApp(app)
}
