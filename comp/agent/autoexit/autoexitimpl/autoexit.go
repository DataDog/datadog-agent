// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package autoexitimpl implements autoexit.Component
package autoexitimpl

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/agent/autoexit"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Supply(params{exitTicker: 30 * time.Second}),
		fx.Provide(newAutoExit),
	)
}

type dependencies struct {
	fx.In

	Config config.Component
	Logger log.Component
	Lc     fx.Lifecycle
	Params params
}

type params struct {
	exitTicker time.Duration
}

// ExitDetector is common interface for shutdown mechanisms
type ExitDetector interface {
	check() bool
}

// ConfigureAutoExit starts automatic shutdown mechanism if necessary
func newAutoExit(deps dependencies) (autoexit.Component, error) {
	var sd ExitDetector
	var err error

	if deps.Config.GetBool("auto_exit.noprocess.enabled") {
		sd, err = DefaultNoProcessExit(deps.Config)
	}

	if err != nil {
		return nil, deps.Logger.Errorf("Unable to configure auto-exit, err: %v", err)
	}

	if sd == nil {
		return nil, nil
	}

	validationPeriod := time.Duration(deps.Config.GetInt("auto_exit.validation_period")) * time.Second

	ctx, cancel := context.WithCancel(context.Background())

	deps.Lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			return start(ctx, sd, deps.Params.exitTicker, validationPeriod, deps.Logger)
		},
		OnStop: func(_ context.Context) error {
			cancel()
			return nil
		},
	})

	return struct{}{}, nil
}

func start(ctx context.Context, sd ExitDetector, tickerPeriod, validationPeriod time.Duration, logger log.Component) error {
	if sd == nil {
		return fmt.Errorf("a shutdown detector must be provided")
	}

	selfProcess, err := os.FindProcess(os.Getpid())
	if err != nil {
		return fmt.Errorf("cannot find own process, err: %w", err)
	}

	logger.Info("Starting auto-exit watcher")
	lastConditionNotMet := time.Now()
	ticker := time.NewTicker(tickerPeriod)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return

			case <-ticker.C:
				shutdownConditionFound := sd.check()
				if shutdownConditionFound {
					if lastConditionNotMet.Add(validationPeriod).Before(time.Now()) {
						logger.Info("Conditions met for automatic exit: triggering stop sequence")
						if err := selfProcess.Signal(os.Interrupt); err != nil {
							logger.Errorf("Unable to send termination signal - will use os.exit, err: %v", err)
							os.Exit(1)
						}
						return
					}
				} else {
					lastConditionNotMet = time.Now()
				}
			}
		}
	}()

	return nil
}
