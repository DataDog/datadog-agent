// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package profilerimpl implements a component to handle starting and stopping the internal profiler.
package profilerimpl

import (
	"context"
	"fmt"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	profilecomp "github.com/DataDog/datadog-agent/comp/process/profiler"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/profiling"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newProfiler))
}

type dependencies struct {
	fx.In
	Lc fx.Lifecycle

	Config config.Component
}

var _ profilecomp.Component = (*profiler)(nil)

type profiler struct{}

func newProfiler(deps dependencies) profilecomp.Component {
	p := &profiler{}

	settings := getProfilingSettings(deps.Config)
	deps.Lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			if deps.Config.GetBool("process_config.internal_profiling.enabled") {
				err := profiling.Start(settings)
				if err != nil {
					_ = log.Warn("Failed to enable profiling:", err.Error())
				} else {
					log.Info("Started process-agent profiler")
				}
			}

			// Even if there is an error setting up the profiler, we don't want to block
			// starting the process-agent.
			return nil
		},
		OnStop: func(context.Context) error {
			profiling.Stop()
			return nil
		},
	})
	return p
}

func getProfilingSettings(cfg config.Component) profiling.Settings {
	// allow full url override for development use
	site := cfg.GetString("internal_profiling.profile_dd_url")
	if site == "" {
		s := cfg.GetString("site")
		if s == "" {
			s = pkgconfigsetup.DefaultSite
		}
		site = fmt.Sprintf(profiling.ProfilingURLTemplate, s)
	}

	tags := cfg.GetStringSlice("internal_profiling.extra_tags")
	tags = append(tags, fmt.Sprintf("version:%v", version.AgentVersion))

	return profiling.Settings{
		ProfilingURL:         site,
		Env:                  cfg.GetString("env"),
		Service:              "process-agent",
		Period:               cfg.GetDuration("internal_profiling.period"),
		CPUDuration:          cfg.GetDuration("internal_profiling.cpu_duration"),
		MutexProfileFraction: cfg.GetInt("internal_profiling.mutex_profile_fraction"),
		BlockProfileRate:     cfg.GetInt("internal_profiling.block_profile_rate"),
		WithGoroutineProfile: cfg.GetBool("internal_profiling.enable_goroutine_stacktraces"),
		WithBlockProfile:     cfg.GetBool("internal_profiling.enable_block_profiling"),
		WithMutexProfile:     cfg.GetBool("internal_profiling.enable_mutex_profiling"),
		WithDeltaProfiles:    cfg.GetBool("internal_profiling.delta_profiles"),
		Socket:               cfg.GetString("internal_profiling.unix_socket"),
		Tags:                 tags,
	}
}
