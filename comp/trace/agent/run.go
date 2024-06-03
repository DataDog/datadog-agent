// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"runtime"
	"runtime/debug"
	"time"

	remotecfg "github.com/DataDog/datadog-agent/cmd/trace-agent/config/remote"
	"github.com/DataDog/datadog-agent/comp/trace/config"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	rc "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	agentrt "github.com/DataDog/datadog-agent/pkg/runtime"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	tracecfg "github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/profiling"
	"github.com/DataDog/datadog-agent/pkg/version"

	"github.com/DataDog/datadog-go/v5/statsd"
)

// runAgentSidekicks is the entrypoint for running non-components that run along the agent.
func runAgentSidekicks(ag *agent) error {
	tracecfg := ag.config.Object()
	err := info.InitInfo(tracecfg) // for expvar & -info option
	if err != nil {
		return err
	}

	defer watchdog.LogOnPanic(ag.Statsd)

	if err := util.SetupCoreDump(coreconfig.Datadog()); err != nil {
		log.Warnf("Can't setup core dumps: %v, core dumps might not be available after a crash", err)
	}

	rand.Seed(time.Now().UTC().UnixNano())

	if coreconfig.IsRemoteConfigEnabled(coreconfig.Datadog()) {
		cf, err := newConfigFetcher()
		if err != nil {
			ag.telemetryCollector.SendStartupError(telemetry.CantCreateRCCLient, err)
			return fmt.Errorf("could not instantiate the tracer remote config client: %v", err)
		}

		api.AttachEndpoint(api.Endpoint{
			Pattern: "/v0.7/config",
			Handler: func(r *api.HTTPReceiver) http.Handler {
				return remotecfg.ConfigHandler(r, cf, tracecfg, ag.Statsd, ag.Timing)
			},
		})
	}

	// We're adding the /config endpoint from the comp side of the trace agent to avoid linking with pkg/config from
	// the trace agent.
	// pkg/config is not a go-module yet and pulls a large chunk of Agent code base with it. Using it within the
	// trace-agent would largely increase the number of module pulled by OTEL when using the pkg/trace go-module.
	if err := apiutil.CreateAndSetAuthToken(coreconfig.Datadog()); err != nil {
		log.Errorf("could not set auth token: %s", err)
	} else {
		ag.Agent.DebugServer.AddRoute("/config", ag.config.GetConfigHandler())
	}

	api.AttachEndpoint(api.Endpoint{
		Pattern: "/alpha/instrumentation/pod-container-metadata",
		Handler: func(r *api.HTTPReceiver) http.Handler {
			return ag.containerinpsector.PodContainerMetadataHandlerFunc()
		},
	})

	api.AttachEndpoint(api.Endpoint{
		Pattern: "/config/set",
		Handler: func(r *api.HTTPReceiver) http.Handler {
			return ag.config.SetHandler()
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
			// log.Infof("apm_config.max_memory: %vMiB", int64(tracecfg.MaxMemory)/(1024*1024))
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
		ag.telemetryCollector.SendStartupSuccess()
	}()

	return nil
}

func stopAgentSidekicks(cfg config.Component, statsd statsd.ClientInterface) {
	defer watchdog.LogOnPanic(statsd)

	log.Flush()

	tracecfg := cfg.Object()
	if pcfg := profilingConfig(tracecfg); pcfg != nil {
		profiling.Stop()
	}
}

func profilingConfig(tracecfg *tracecfg.AgentConfig) *profiling.Settings {
	if !coreconfig.Datadog().GetBool("apm_config.internal_profiling.enabled") {
		return nil
	}
	endpoint := coreconfig.Datadog().GetString("internal_profiling.profile_dd_url")
	if endpoint == "" {
		endpoint = fmt.Sprintf(profiling.ProfilingURLTemplate, tracecfg.Site)
	}
	tags := coreconfig.Datadog().GetStringSlice("internal_profiling.extra_tags")
	tags = append(tags, fmt.Sprintf("version:%s", version.AgentVersion))
	return &profiling.Settings{
		ProfilingURL: endpoint,

		// remaining configuration parameters use the top-level `internal_profiling` config
		Period:               coreconfig.Datadog().GetDuration("internal_profiling.period"),
		Service:              "trace-agent",
		CPUDuration:          coreconfig.Datadog().GetDuration("internal_profiling.cpu_duration"),
		MutexProfileFraction: coreconfig.Datadog().GetInt("internal_profiling.mutex_profile_fraction"),
		BlockProfileRate:     coreconfig.Datadog().GetInt("internal_profiling.block_profile_rate"),
		WithGoroutineProfile: coreconfig.Datadog().GetBool("internal_profiling.enable_goroutine_stacktraces"),
		WithBlockProfile:     coreconfig.Datadog().GetBool("internal_profiling.enable_block_profiling"),
		WithMutexProfile:     coreconfig.Datadog().GetBool("internal_profiling.enable_mutex_profiling"),
		Tags:                 tags,
	}
}

func newConfigFetcher() (rc.ConfigFetcher, error) {
	ipcAddress, err := coreconfig.GetIPCAddress()
	if err != nil {
		return nil, err
	}

	// Auth tokens are handled by the rcClient
	return rc.NewAgentGRPCConfigFetcher(ipcAddress, coreconfig.GetIPCPort(), func() (string, error) { return security.FetchAuthToken(coreconfig.Datadog()) })
}
