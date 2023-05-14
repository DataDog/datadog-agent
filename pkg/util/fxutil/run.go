// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fxutil

import (
	"context"

	"go.uber.org/fx"
)

// Run runs an fx.App using the supplied options, returning any errors.
//
// This differs from fx.App#Run in that it returns errors instead of exiting
// the process.
func Run(opts ...fx.Option) error {
	opts = append(opts, FxLoggingOption())
	app := fx.New(opts...)

	startCtx, cancel := context.WithTimeout(context.Background(), app.StartTimeout())
	defer cancel()

	if err := app.Start(startCtx); err != nil {
		return UnwrapIfErrArgumentsFailed(err)
	}

	_ = <-app.Done()

	stopCtx, cancel := context.WithTimeout(context.Background(), app.StopTimeout())
	defer cancel()

	if err := app.Stop(stopCtx); err != nil {
		return err
	}

	return nil
}
