// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package daemon

import (
	"context"
	"syscall"
	"time"

	"github.com/judwhite/go-svc"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/installer/command"
	pid "github.com/DataDog/datadog-agent/comp/core/pid/def"
	localapi "github.com/DataDog/datadog-agent/comp/updater/localapi/def"
	telemetry "github.com/DataDog/datadog-agent/comp/updater/telemetry/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type windowsService struct {
	ctx    context.Context
	cancel context.CancelFunc
	*fx.App
}

// fail build if windowsService does not implement Context,
// it's needed to trigger shutdown when disabled, see gracefullyExit,
// else feature just won't work b/c go-svc won't have access to the context.
// We have E2E test for it but this will make it noticeable earlier.
var _ svc.Context = &windowsService{}

func getFxOptions(global *command.GlobalParams) []fx.Option {
	return []fx.Option{
		getCommonFxOption(global),
		fxutil.FxAgentBase(),
		// Force the instantiation of some components
		fx.Invoke(func(_ localapi.Component) {}),
		fx.Invoke(func(_ pid.Component) {}),
		fx.Invoke(func(_ telemetry.Component) {}),
	}
}

func runFxWrapper(global *command.GlobalParams) error {
	ctx, cancel := context.WithCancel(context.Background())
	s := &windowsService{
		ctx:    ctx,
		cancel: cancel,
	}

	// fx.New runs component constructors/Invoke but doesn't report errors directly, any errors will be returned by App.Start()
	s.App = fx.New(getFxOptions(global)...)

	// svc.Run must be called as early as possible in the app to prevent SCM errors.
	// See Run() in servicemain.go for more information.
	// svc.Run will block until the service is stopped.
	return svc.Run(s, syscall.SIGINT, syscall.SIGTERM)
}

func (s *windowsService) Init(_ svc.Environment) error {
	return nil
}

func (s *windowsService) Start() error {
	// Default start timeout is 15s, which is fine for us.
	startCtx, cancel := context.WithTimeout(context.Background(), s.StartTimeout())
	defer cancel()

	// Returns errors from fx.New if any, if not then runs fx lifecycle OnStart hooks
	err := s.App.Start(startCtx)
	if isDaemonDisabled(err) {
		log.Infof("Datadog installer is not enabled, exiting")
		go s.gracefullyExit()
		return nil
	}
	return err
}

func (s *windowsService) Stop() error {
	// Default stop timeout is 15s, which is fine for us.
	stopCtx, cancel := context.WithTimeout(context.Background(), s.StopTimeout())
	defer cancel()
	return s.App.Stop(stopCtx)
}

func (s *windowsService) Context() context.Context {
	return s.ctx
}

func (s *windowsService) gracefullyExit() {
	// Allow go-svc to enter SERVICE_RUNNING before stopping, so SCM reports success.
	// See runTimeExitGate in servicemain.go.
	time.Sleep(5 * time.Second)
	if s.cancel != nil {
		s.cancel()
	}
}
