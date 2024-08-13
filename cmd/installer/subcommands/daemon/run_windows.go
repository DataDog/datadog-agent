// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package daemon

import (
	"context"
	"github.com/DataDog/datadog-agent/cmd/installer/command"
	"github.com/DataDog/datadog-agent/comp/core/pid"
	"github.com/DataDog/datadog-agent/comp/updater/localapi"
	"github.com/DataDog/datadog-agent/comp/updater/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/judwhite/go-svc"
	"go.uber.org/fx"
	"syscall"
)

type windowsService struct {
	*fx.App
}

func runFxWrapper(global *command.GlobalParams) error {
	return svc.Run(&windowsService{
		App: fx.New(
			getCommonFxOption(global),
			fxutil.FxAgentBase(),
			// Force the instantiation of some components
			fx.Invoke(func(_ pid.Component) {}),
			fx.Invoke(func(_ localapi.Component) {}),
			fx.Invoke(func(_ telemetry.Component) {}),
		),
	}, syscall.SIGINT, syscall.SIGTERM)
}

func (s *windowsService) Init(_ svc.Environment) error {
	return nil
}

func (s *windowsService) Start() error {
	// Default start timeout is 15s, which is fine for us.
	startCtx, cancel := context.WithTimeout(context.Background(), s.StartTimeout())
	defer cancel()
	return s.App.Start(startCtx)
}

func (s *windowsService) Stop() error {
	// Default stop timeout is 15s, which is fine for us.
	stopCtx, cancel := context.WithTimeout(context.Background(), s.StopTimeout())
	defer cancel()
	return s.App.Stop(stopCtx)
}
