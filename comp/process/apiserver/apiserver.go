// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apiserver

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/process-agent/api"
	"github.com/DataDog/datadog-agent/comp/core/log"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
)

var _ Component = (*apiserver)(nil)

type apiserver struct {
	server *http.Server
}

type dependencies struct {
	fx.In

	Lc fx.Lifecycle

	Log log.Component

	APIServerDeps api.APIServerDeps
}

//nolint:revive // TODO(PROC) Fix revive linter
func newApiServer(deps dependencies) Component {
	initRuntimeSettings(deps.Log)

	r := mux.NewRouter()
	api.SetupAPIServerHandlers(deps.APIServerDeps, r) // Set up routes

	addr, err := ddconfig.GetProcessAPIAddressPort()
	if err != nil {
		return err
	}
	deps.Log.Infof("API server listening on %s", addr)
	timeout := time.Duration(ddconfig.Datadog.GetInt("server_timeout")) * time.Second

	apiserver := &apiserver{
		server: &http.Server{
			Handler:      r,
			Addr:         addr,
			ReadTimeout:  timeout,
			WriteTimeout: timeout,
			IdleTimeout:  timeout,
		},
	}

	deps.Lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				err := apiserver.server.ListenAndServe()
				if err != nil && !errors.Is(err, http.ErrServerClosed) {
					_ = deps.Log.Error(err)
				}
			}()

			return nil
		},
		OnStop: func(ctx context.Context) error {
			err := apiserver.server.Shutdown(ctx)
			if err != nil {
				_ = deps.Log.Error("Failed to properly shutdown api server:", err)
			}

			return nil
		},
	})

	return apiserver
}

// initRuntimeSettings registers settings to be added to the runtime config.
func initRuntimeSettings(logger log.Component) {
	// NOTE: Any settings you want to register should simply be added here
	processRuntimeSettings := []settings.RuntimeSetting{
		settings.NewLogLevelRuntimeSetting(),
		settings.NewRuntimeMutexProfileFraction(),
		settings.NewRuntimeBlockProfileRate(),
		settings.NewProfilingGoroutines(),
		settings.NewProfilingRuntimeSetting("internal_profiling", "process-agent"),
	}

	// Before we begin listening, register runtime settings
	for _, setting := range processRuntimeSettings {
		err := settings.RegisterRuntimeSetting(setting)
		if err != nil {
			_ = logger.Warnf("Cannot initialize the runtime setting %s: %v", setting.Name(), err)
		}
	}
}
