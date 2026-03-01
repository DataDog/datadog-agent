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
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/pid"
	localapi "github.com/DataDog/datadog-agent/comp/updater/localapi/def"
	"github.com/DataDog/datadog-agent/comp/updater/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type windowsService struct {
	ctx                  context.Context
	cancel               context.CancelFunc
	remoteUpdatesEnabled bool
	*fx.App
}

// fail build if windowsService does not implement Context,
// it's needed to trigger shutdown when disabled, see gracefullyExitIfDisabled,
// else feature just won't work b/c go-svc won't have access to the context.
// We have E2E test for it but this will make it noticeable earlier.
var _ svc.Context = &windowsService{}

func getFxOptions(global *command.GlobalParams, s *windowsService) []fx.Option {
	return []fx.Option{
		getCommonFxOption(global),
		fxutil.FxAgentBase(),
		// Collect the remote_updates flag so we can exit gracefully if it's disabled
		// This feels less hacky than exposing the config component outside of fx.
		fx.Invoke(func(cfg config.Component) { s.remoteUpdatesEnabled = cfg.GetBool("remote_updates") }),
		// Force the instantiation of some components
		fx.Invoke(func(_ pid.Component) {}),
		fx.Invoke(func(_ localapi.Component) {}),
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
	s.App = fx.New(getFxOptions(global, s)...)

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
	if err != nil {
		// Startup failed (either fx constructors or OnStart hooks), return error immediately.
		// Current state should be SERVICE_START_PENDING so SCM will report the error.
		return err
	}
	// fx App successfully started, we won't enter SERVICE_RUNNING until this function returns.
	//
	// We want the service to exit gracefully if remote_updates is disabled.
	// In order to exit gracefully, we have to delay exit for some time after entering SERVICE_RUNNING.
	// See runTimeExitGate in servicemain.go for an explanation of why this is necessary.
	//
	// This is a little tricky to manage between fx, go-svc, and Windows SCM requirements.
	// fx doesn't provide a way to run code after the App has fully started, it schedules things based on
	// dependency order only. Our custom fxutil.OneShot provides this, but its runFunc must block or else
	// the App will be stopped, which contradicts the go-svc/Windows SCM background execution model.
	//
	// To accommodate all of this, we use fx.Invoke to collect the remote_updates flag from the config component,
	// and then we start a background goroutine to check if the service is enabled and exit if it's not.
	// This should give us enough time to enter SERVICE_RUNNING before the service is stopped.
	// This is a bit racy, but it should be fine in practice given the relatively long shutdown delay.
	// To fix this race completely, we'll have to transition to servicemain.go, which could use some
	// refactoring itself to make it fit better with fx.
	go s.gracefullyExitIfDisabled()
	// go-svc enters SERVICE_RUNNING once we return from Start()
	return nil
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

// gracefullyExitIfDisabled exits the service if remote_updates is disabled
func (s *windowsService) gracefullyExitIfDisabled() {
	if !s.remoteUpdatesEnabled {
		log.Infof("Datadog installer is not enabled, exiting")
		// Delay shutdown to prevent Windows SCM from marking the service as failed
		// when the service stops too quickly.
		// For more information see runTimeExitGate in servicemain.go
		time.Sleep(5 * time.Second)
		// go-svc will call Stop() once we cancel the context
		if s != nil && s.cancel != nil {
			s.cancel()
		}
	}
}
