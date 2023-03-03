package profiler

import (
	"context"
	"fmt"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/profiling"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type dependencies struct {
	fx.In
	Lc fx.Lifecycle

	Config config.Component
}

var _ Component = (*profiler)(nil)

type profiler struct{}

func newProfiler(deps dependencies) Component {
	p := &profiler{}

	settings := getProfilingSettings(deps.Config)
	deps.Lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			if deps.Config.GetBool("process_config.internal_profiling") {
				err := p.Start(settings)
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
			p.Stop()
			return nil
		},
	})
	return p
}

func (*profiler) Start(settings profiling.Settings) error {
	return profiling.Start(settings)
}

func (*profiler) Stop() {
	profiling.Stop()
}

func getProfilingSettings(cfg config.Component) profiling.Settings {
	// allow full url override for development use
	site := cfg.GetString("internal_profiling.profile_dd_url")
	if site == "" {
		s := cfg.GetString("site")
		if s == "" {
			s = ddconfig.DefaultSite
		}
		site = fmt.Sprintf(profiling.ProfilingURLTemplate, s)
	}

	v, _ := version.Agent()
	return profiling.Settings{
		ProfilingURL:         site,
		Env:                  cfg.GetString("env"),
		Service:              "process-agent",
		Period:               cfg.GetDuration("internal_profiling.period"),
		CPUDuration:          cfg.GetDuration("internal_profiling.cpu_duration"),
		MutexProfileFraction: cfg.GetInt("internal_profiling.mutex_profile_fraction"),
		BlockProfileRate:     cfg.GetInt("internal_profiling.block_profile_rate"),
		WithGoroutineProfile: cfg.GetBool("internal_profiling.enable_goroutine_stacktraces"),
		Tags:                 []string{fmt.Sprintf("version:%v", v)},
	}
}
