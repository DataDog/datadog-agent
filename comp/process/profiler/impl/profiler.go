// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package profilerimpl implements a component to handle starting and stopping the internal profiler.
package profilerimpl

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/config"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	profilercomp "github.com/DataDog/datadog-agent/comp/process/profiler/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/profiling"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// Requires defines the dependencies for the profiler component.
type Requires struct {
	Lc compdef.Lifecycle

	Config config.Component
}

// Provides defines the output of the profiler component.
type Provides struct {
	Comp profilercomp.Component
}

var _ profilercomp.Component = (*profiler)(nil)

type profiler struct{}

// NewComponent creates a new profiler component.
func NewComponent(reqs Requires) Provides {
	p := &profiler{}

	settings := getProfilingSettings(reqs.Config)
	reqs.Lc.Append(compdef.Hook{
		OnStart: func(context.Context) error {
			if reqs.Config.GetBool("process_config.internal_profiling.enabled") {
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
	return Provides{Comp: p}
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
	tags = append(tags, "__dd_internal_profiling:datadog-agent")

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
