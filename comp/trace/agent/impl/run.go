// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentimpl

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	remotecfg "github.com/DataDog/datadog-agent/cmd/trace-agent/config/remote"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/comp/trace/config"
	rc "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/trace/api"
	tracecfg "github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/telemetry"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
	"github.com/DataDog/datadog-agent/pkg/util/coredump"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/profiling"

	"github.com/DataDog/datadog-go/v5/statsd"
)

// runAgentSidekicks is the entrypoint for running non-components that run along the agent.
func runAgentSidekicks(ag component) error {
	// Configure the Trace Agent Debug server to use the IPC certificate
	ag.Agent.DebugServer.SetTLSConfig(ag.ipc.GetTLSServerConfig())

	tracecfg := ag.config.Object()
	err := info.InitInfo(tracecfg) // for expvar & -info option
	if err != nil {
		return err
	}

	defer watchdog.LogOnPanic(ag.Statsd)

	if err := coredump.Setup(pkgconfigsetup.Datadog()); err != nil {
		log.Warnf("Can't setup core dumps: %v, core dumps might not be available after a crash", err)
	}

	if pkgconfigsetup.IsRemoteConfigEnabled(pkgconfigsetup.Datadog()) {
		cf, err := newConfigFetcher(ag.ipc)
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
	ag.Agent.DebugServer.AddRoute("/config", ag.config.GetConfigHandler())
	ag.Agent.DebugServer.AddRoute("/config/set", ag.config.SetHandler())
	// The below endpoint is deprecated and has been replaced with /config/set on the debug server.
	// It will be removed in a future version.
	api.AttachEndpoint(api.Endpoint{
		Pattern: "/config/set",
		Handler: func(_ *api.HTTPReceiver) http.Handler {
			log.Warnf("The /config/set endpoint on this port is deprecated and will be removed. The same endpoint is available on the debug server at 127.0.0.1:%d", tracecfg.DebugServerPort)
			return ag.config.SetHandler()
		},
	})

	if secrets, ok := ag.secrets.Get(); ok {
		// Adding a route to trigger a secrets refresh from the CLI.
		// TODO - components: the secrets comp already export a route but it requires the API component which is not
		// used by the trace agent. This should be removed once the trace-agent is fully componentize.
		ag.Agent.DebugServer.AddRoute("/secret/refresh",
			// Adding IPC middleware to the secrets refresh endpoint to check validity of auth token Header.
			ag.ipc.HTTPMiddleware(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					res, err := secrets.Refresh()
					if err != nil {
						log.Errorf("error while refresing secrets: %s", err)
						w.Header().Set("Content-Type", "application/json")
						body, _ := json.Marshal(map[string]string{"error": err.Error()})
						http.Error(w, string(body), http.StatusInternalServerError)
						return
					}
					w.Write([]byte(res))
				}),
			),
		)
	}

	log.Infof("Trace agent running on host %s", tracecfg.Hostname)
	if pcfg := profilingConfig(tracecfg, ag.params.DisableInternalProfiling); pcfg != nil {
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

func stopAgentSidekicks(cfg config.Component, statsd statsd.ClientInterface, disableInternalProfiling bool) {
	defer watchdog.LogOnPanic(statsd)

	log.Flush()

	tracecfg := cfg.Object()
	if pcfg := profilingConfig(tracecfg, disableInternalProfiling); pcfg != nil {
		profiling.Stop()
	}
}

func profilingConfig(tracecfg *tracecfg.AgentConfig, disableInternalProfiling bool) *profiling.Settings {
	if !pkgconfigsetup.Datadog().GetBool("apm_config.internal_profiling.enabled") || disableInternalProfiling {
		return nil
	}
	endpoint := pkgconfigsetup.Datadog().GetString("internal_profiling.profile_dd_url")
	if endpoint == "" {
		endpoint = fmt.Sprintf(profiling.ProfilingURLTemplate, tracecfg.Site)
	}
	tags := pkgconfigsetup.Datadog().GetStringSlice("internal_profiling.extra_tags")
	tags = profiling.GetBaseProfilingTags(tags)
	return &profiling.Settings{
		ProfilingURL: endpoint,

		// remaining configuration parameters use the top-level `internal_profiling` config
		Period:               pkgconfigsetup.Datadog().GetDuration("internal_profiling.period"),
		Service:              "trace-agent",
		CPUDuration:          pkgconfigsetup.Datadog().GetDuration("internal_profiling.cpu_duration"),
		MutexProfileFraction: pkgconfigsetup.Datadog().GetInt("internal_profiling.mutex_profile_fraction"),
		BlockProfileRate:     pkgconfigsetup.Datadog().GetInt("internal_profiling.block_profile_rate"),
		WithGoroutineProfile: pkgconfigsetup.Datadog().GetBool("internal_profiling.enable_goroutine_stacktraces"),
		WithBlockProfile:     pkgconfigsetup.Datadog().GetBool("internal_profiling.enable_block_profiling"),
		WithMutexProfile:     pkgconfigsetup.Datadog().GetBool("internal_profiling.enable_mutex_profiling"),
		WithDeltaProfiles:    pkgconfigsetup.Datadog().GetBool("internal_profiling.delta_profiles"),
		Socket:               pkgconfigsetup.Datadog().GetString("internal_profiling.unix_socket"),
		Tags:                 tags,
	}
}

func newConfigFetcher(ipc ipc.Component) (rc.ConfigFetcher, error) {
	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return nil, err
	}

	// Auth tokens are handled by the rcClient
	return rc.NewAgentGRPCConfigFetcher(ipcAddress, pkgconfigsetup.GetIPCPort(), ipc.GetAuthToken(), ipc.GetTLSClientConfig()) // TODO IPC: GRPC client will be provided by the IPC component
}
