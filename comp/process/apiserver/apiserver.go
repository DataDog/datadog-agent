// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apiserver

import (
	"context"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/process-agent/api"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
)

var _ Component = (*apiserver)(nil)

type apiserver struct{}

type dependencies struct {
	fx.In

	Lc fx.Lifecycle

	Log log.Component
}

func newApiServer(deps dependencies) Component {
	initRuntimeSettings(deps.Log)

	deps.Lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			err := api.StartServer()
			if err != nil {
				return err
			}

			return nil
		},
	})

	return &apiserver{}
}

// initRuntimeSettings registers settings to be added to the runtime config.
func initRuntimeSettings(logger log.Component) {
	// NOTE: Any settings you want to register should simply be added here
	processRuntimeSettings := []settings.RuntimeSetting{
		settings.LogLevelRuntimeSetting{},
		settings.RuntimeMutexProfileFraction{},
		settings.RuntimeBlockProfileRate{},
		settings.ProfilingGoroutines{},
		settings.ProfilingRuntimeSetting{SettingName: "internal_profiling", Service: "process-agent"},
	}

	// Before we begin listening, register runtime settings
	for _, setting := range processRuntimeSettings {
		err := settings.RegisterRuntimeSetting(setting)
		if err != nil {
			_ = logger.Warnf("Cannot initialize the runtime setting %s: %v", setting.Name(), err)
		}
	}
}
