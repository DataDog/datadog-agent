// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentimpl

import (
	"fmt"
	"math/rand"
	"net/http"
	"time"

	remotecfg "github.com/DataDog/datadog-agent/cmd/trace-agent/config/remote"
	"github.com/DataDog/datadog-agent/comp/trace/config"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	rc "github.com/DataDog/datadog-agent/pkg/config/remote/client"
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
func runAgentSidekicks(ag component) error {
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
		Pattern: "/config/set",
		Handler: func(_ *api.HTTPReceiver) http.Handler {
			return ag.config.SetHandler()
		},
	})

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
