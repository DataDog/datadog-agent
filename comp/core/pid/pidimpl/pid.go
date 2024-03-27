// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package pidimpl writes the current PID to a file, ensuring that the file
package pidimpl

import (
	"context"
	"os"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/pid"
	"github.com/DataDog/datadog-agent/pkg/pidfile"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newPID),
	)
}

// Params are the input parameters for the component.
type Params struct {
	PIDfilePath string
}

// NewParams returns a new Params with the given values.
func NewParams(pidfilePath string) Params {
	return Params{
		PIDfilePath: pidfilePath,
	}
}

type dependencies struct {
	fx.In
	Lc     fx.Lifecycle
	Log    log.Component
	Params Params
}

func newPID(deps dependencies) (pid.Component, error) {
	pidfilePath := deps.Params.PIDfilePath
	if pidfilePath != "" {
		err := pidfile.WritePID(pidfilePath)
		if err != nil {
			return struct{}{}, deps.Log.Errorf("Error while writing PID file, exiting: %v", err)
		}
		deps.Log.Infof("pid '%d' written to pid file '%s'", os.Getpid(), pidfilePath)

		deps.Lc.Append(fx.Hook{
			OnStop: func(context.Context) error {
				_ = os.Remove(pidfilePath)
				return nil
			}})
	}
	return struct{}{}, nil
}
