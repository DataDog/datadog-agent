// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/DataDog/datadog-agent/cmd/manager"
	remotecfg "github.com/DataDog/datadog-agent/cmd/trace-agent/config/remote"
	"github.com/DataDog/datadog-agent/comp/trace/config"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	rc "github.com/DataDog/datadog-agent/pkg/config/remote"
	agentrt "github.com/DataDog/datadog-agent/pkg/runtime"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/local"
	"github.com/DataDog/datadog-agent/pkg/tagger/remote"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	tracecfg "github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
	"github.com/DataDog/datadog-agent/pkg/trace/metrics/timing"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/profiling"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	// register all workloadmeta collectors
	_ "github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors"
)

// runAgentSidekicks is the entrypoint for running non-components that run along the agent.
func runAgentSidekicks(ctx context.Context, cfg config.Component, telemetryCollector telemetry.TelemetryCollector) error {
	tracecfg := cfg.Object()
	err := info.InitInfo(tracecfg) // for expvar & -info option
	if err != nil {
		return err
	}

	defer watchdog.LogOnPanic()

	if err := util.SetupCoreDump(coreconfig.Datadog); err != nil {
		log.Warnf("Can't setup core dumps: %v, core dumps might not be available after a crash", err)
	}

	err = manager.ConfigureAutoExit(ctx, coreconfig.Datadog)
	if err != nil {
		telemetryCollector.SendStartupError(telemetry.CantSetupAutoExit, err)
		return fmt.Errorf("Unable to configure auto-exit, err: %v", err)
	}

	err = metrics.Configure(tracecfg, []string{"version:" + version.AgentVersion})
	if err != nil {
		telemetryCollector.SendStartupError(telemetry.CantConfigureDogstatsd, err)
		return fmt.Errorf("cannot configure dogstatsd: %v", err)
	}

	metrics.Count("datadog.trace_agent.started", 1, nil, 1)

	rand.Seed(time.Now().UTC().UnixNano())

	remoteTagger := coreconfig.Datadog.GetBool("apm_config.remote_tagger")
	if remoteTagger {
		options, err := remote.NodeAgentOptions()
		if err != nil {
			log.Errorf("Unable to configure the remote tagger: %s", err)
			remoteTagger = false
		} else {
			tagger.SetDefaultTagger(remote.NewTagger(options))
			if err := tagger.Init(ctx); err != nil {
				log.Infof("Starting remote tagger failed. Falling back to local tagger: %s", err)
				remoteTagger = false
			}
		}
	}

	// starts the local tagger if apm_config says so, or if starting the
	// remote tagger has failed.
	if !remoteTagger {
		store := workloadmeta.CreateGlobalStore(workloadmeta.NodeAgentCatalog)
		store.Start(ctx)

		tagger.SetDefaultTagger(local.NewTagger(store))
		if err := tagger.Init(ctx); err != nil {
			log.Errorf("failed to start the tagger: %s", err)
		}
	}

	if coreconfig.IsRemoteConfigEnabled(coreconfig.Datadog) {
		// Auth tokens are handled by the rcClient
		rcClient, err := rc.NewAgentGRPCConfigFetcher()
		if err != nil {
			telemetryCollector.SendStartupError(telemetry.CantCreateRCCLient, err)
			return fmt.Errorf("could not instantiate the tracer remote config client: %v", err)
		}
		api.AttachEndpoint(api.Endpoint{
			Pattern: "/v0.7/config",
			Handler: func(r *api.HTTPReceiver) http.Handler { return remotecfg.ConfigHandler(r, rcClient, tracecfg) },
		})
	}

	api.AttachEndpoint(api.Endpoint{
		Pattern: "/config/set",
		Handler: func(r *api.HTTPReceiver) http.Handler {
			return cfg.SetHandler()
		},
	})

	// prepare go runtime
	cgsetprocs := agentrt.SetMaxProcs()
	if !cgsetprocs {
		if mp, ok := os.LookupEnv("GOMAXPROCS"); ok {
			log.Infof("GOMAXPROCS manually set to %v", mp)
		} else if tracecfg.MaxCPU > 0 {
			allowedCores := int(tracecfg.MaxCPU / 100)
			if allowedCores < 1 {
				allowedCores = 1
			}
			if allowedCores < runtime.GOMAXPROCS(0) {
				log.Infof("apm_config.max_cpu is less than current GOMAXPROCS. Setting GOMAXPROCS to (%v) %d\n", allowedCores, (allowedCores))
				runtime.GOMAXPROCS(int(allowedCores))
			}
		} else {
			log.Infof("apm_config.max_cpu is disabled. leaving GOMAXPROCS at current value.")
		}
	}
	log.Infof("Trace Agent final GOMAXPROCS: %v", runtime.GOMAXPROCS(0))

	// prepare go runtime
	cgmem, err := agentrt.SetGoMemLimit(coreconfig.IsContainerized())
	if err != nil {
		log.Infof("Couldn't set Go memory limit from cgroup: %s", err)
	}
	if cgmem == 0 {
		// memory limit not set from cgroups
		if lim, ok := os.LookupEnv("GOMEMLIMIT"); ok {
			log.Infof("GOMEMLIMIT manually set to: %v", lim)
		} else if tracecfg.MaxMemory > 0 {
			// We have apm_config.max_memory, and no cgroup memory limit is in place.
			//log.Infof("apm_config.max_memory: %vMiB", int64(tracecfg.MaxMemory)/(1024*1024))
			finalmem := int64(tracecfg.MaxMemory * 0.9)
			debug.SetMemoryLimit(finalmem)
			log.Infof("apm_config.max_memory set to: %vMiB. Setting GOMEMLIMIT to 90%% of max: %vMiB", int64(tracecfg.MaxMemory)/(1024*1024), finalmem/(1024*1024))
		} else {
			// There are no memory constraints
			log.Infof("GOMEMLIMIT unconstrained.")
		}
	} else {
		log.Infof("Memory constrained by cgroup. GOMEMLIMIT is: %vMiB", cgmem/(1024*1024))
	}

	log.Infof("Trace agent running on host %s", tracecfg.Hostname)
	if pcfg := profilingConfig(tracecfg); pcfg != nil {
		if err := profiling.Start(*pcfg); err != nil {
			log.Warn(err)
		} else {
			log.Infof("Internal profiling enabled: %s.", pcfg)
		}
	}
	go func() {
		time.Sleep(time.Second * 30)
		telemetryCollector.SendStartupSuccess()
	}()

	return nil
}

func stopAgentSidekicks(cfg config.Component) {
	defer watchdog.LogOnPanic()

	log.Flush()
	metrics.Flush()

	timing.Stop()
	err := tagger.Stop()
	if err != nil {
		log.Error(err)
	}
	tracecfg := cfg.Object()
	if pcfg := profilingConfig(tracecfg); pcfg != nil {
		profiling.Stop()
	}
}

func profilingConfig(tracecfg *tracecfg.AgentConfig) *profiling.Settings {
	if !coreconfig.Datadog.GetBool("apm_config.internal_profiling.enabled") {
		return nil
	}
	endpoint := coreconfig.Datadog.GetString("internal_profiling.profile_dd_url")
	if endpoint == "" {
		endpoint = fmt.Sprintf(profiling.ProfilingURLTemplate, tracecfg.Site)
	}
	return &profiling.Settings{
		ProfilingURL: endpoint,

		// remaining configuration parameters use the top-level `internal_profiling` config
		Period:               coreconfig.Datadog.GetDuration("internal_profiling.period"),
		CPUDuration:          coreconfig.Datadog.GetDuration("internal_profiling.cpu_duration"),
		MutexProfileFraction: coreconfig.Datadog.GetInt("internal_profiling.mutex_profile_fraction"),
		BlockProfileRate:     coreconfig.Datadog.GetInt("internal_profiling.block_profile_rate"),
		WithGoroutineProfile: coreconfig.Datadog.GetBool("internal_profiling.enable_goroutine_stacktraces"),
		Tags:                 []string{fmt.Sprintf("version:%s", version.AgentVersion)},
	}
}
