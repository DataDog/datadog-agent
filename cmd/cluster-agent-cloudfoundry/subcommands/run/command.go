// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && clusterchecks

//nolint:revive // TODO(PLINT) Fix revive linter
package run

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/agent/common/path"
	"github.com/DataDog/datadog-agent/cmd/cluster-agent-cloudfoundry/command"
	"github.com/DataDog/datadog-agent/cmd/cluster-agent/api"
	dcav1 "github.com/DataDog/datadog-agent/cmd/cluster-agent/api/v1"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/collector/collector/collectorimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	"github.com/DataDog/datadog-agent/comp/core/config"
	healthprobe "github.com/DataDog/datadog-agent/comp/core/healthprobe/def"
	healthprobefx "github.com/DataDog/datadog-agent/comp/core/healthprobe/fx"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/core/settings/settingsimpl"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	"github.com/DataDog/datadog-agent/comp/forwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/eventplatformreceiverimpl"
	orchestratorForwarderImpl "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorimpl"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl"
	"github.com/DataDog/datadog-agent/pkg/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks"
	pkgcollector "github.com/DataDog/datadog-agent/pkg/collector"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/cloudfoundry"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/DataDog/datadog-agent/pkg/version"

	"go.uber.org/fx"
)

// Commands returns a slice of subcommands for the 'cluster-agent-cloudfoundry' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	startCmd := &cobra.Command{
		Use:   "run",
		Short: "Run the Cluster Agent for Cloud Foundry",
		Long:  `Runs Datadog Cluster Agent for Cloud Foundry in the foreground`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(run,
				fx.Supply(globalParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewClusterAgentParams(globalParams.ConfFilePath),
					SecretParams: secrets.NewEnabledParams(),
					LogParams:    logimpl.ForDaemon(command.LoggerName, "log_file", path.DefaultDCALogFile),
				}),
				core.Bundle(),
				forwarder.Bundle(),
				fx.Provide(defaultforwarder.NewParamsWithResolvers),
				compressionimpl.Module(),
				demultiplexerimpl.Module(),
				orchestratorForwarderImpl.Module(),
				fx.Supply(orchestratorForwarderImpl.NewDisabledParams()),
				eventplatformimpl.Module(),
				fx.Supply(eventplatformimpl.NewDisabledParams()),
				eventplatformreceiverimpl.Module(),
				fx.Supply(demultiplexerimpl.NewDefaultParams()),
				// setup workloadmeta
				collectors.GetCatalog(),
				fx.Supply(workloadmeta.Params{
					InitHelper: common.GetWorkloadmetaInit(),
				}), // TODO(components): check what this must be for cluster-agent-cloudfoundry
				workloadmetafx.Module(),
				fx.Provide(tagger.NewTaggerParams),
				taggerimpl.Module(),
				collectorimpl.Module(),
				fx.Provide(func() optional.Option[serializer.MetricSerializer] {
					return optional.NewNoneOption[serializer.MetricSerializer]()
				}),
				// The cluster-agent-cloudfoundry agent do not have a status command
				// so there is no need to initialize the status component
				fx.Provide(func() status.Component { return nil }),
				// The cluster-agent-cloudfoundry agent do not have settings that change are runtime
				// still, we need to pass it to ensure the API server is proprely initialized
				settingsimpl.Module(),
				fx.Supply(settings.Params{}),
				autodiscoveryimpl.Module(),
				fx.Provide(func(config config.Component) healthprobe.Options {
					return healthprobe.Options{
						Port:           config.GetInt("health_port"),
						LogsGoroutines: config.GetBool("log_all_goroutines_when_unhealthy"),
					}
				}),
				healthprobefx.Module(),
			)
		},
	}

	return []*cobra.Command{startCmd}
}

func run(
	config config.Component,
	log log.Component,
	taggerComp tagger.Component,
	demultiplexer demultiplexer.Component,
	wmeta workloadmeta.Component,
	ac autodiscovery.Component,
	secretResolver secrets.Component,
	collector collector.Component,
	statusComponent status.Component,
	_ healthprobe.Component,
	settings settings.Component,
) error {
	mainCtx, mainCtxCancel := context.WithCancel(context.Background())
	defer mainCtxCancel() // Calling cancel twice is safe

	if !pkgconfig.Datadog().IsSet("api_key") {
		pkglog.Critical("no API key configured, exiting")
		return nil
	}

	// get hostname
	hname, err := hostname.Get(context.TODO())
	if err != nil {
		return pkglog.Errorf("Error while getting hostname, exiting: %v", err)
	}
	pkglog.Infof("Hostname is: %s", hname)

	demultiplexer.AddAgentStartupTelemetry(fmt.Sprintf("%s - Datadog Cluster Agent", version.AgentVersion))

	pkglog.Infof("Datadog Cluster Agent is now running.")

	// Setup a channel to catch OS signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	// initialize CC Cache
	if err = initializeCCCache(mainCtx); err != nil {
		_ = pkglog.Errorf("Error initializing Cloud Foundry CCAPI cache, some advanced tagging features may be missing: %v", err)
	}

	// initialize BBS Cache before starting provider/listener
	if err = initializeBBSCache(mainCtx); err != nil {
		return err
	}

	common.LoadComponents(secretResolver, wmeta, ac, pkgconfig.Datadog().GetString("confd_path"))

	// Set up check collector
	ac.AddScheduler("check", pkgcollector.InitCheckScheduler(optional.NewOption(collector), demultiplexer), true)

	// start the autoconfig, this will immediately run any configured check
	ac.LoadAndRun(mainCtx)

	if err = api.StartServer(mainCtx, wmeta, taggerComp, ac, statusComponent, settings, config); err != nil {
		return log.Errorf("Error while starting agent API, exiting: %v", err)
	}

	var clusterCheckHandler *clusterchecks.Handler
	clusterCheckHandler, err = setupClusterCheck(mainCtx, ac)
	if err == nil {
		api.ModifyAPIRouter(func(r *mux.Router) {
			dcav1.InstallChecksEndpoints(r, clusteragent.ServerContext{ClusterCheckHandler: clusterCheckHandler})
		})
	} else {
		log.Errorf("Error while setting up cluster check Autodiscovery %v", err)
	}

	// Block here until we receive the interrupt signal
	<-signalCh

	// retrieve the agent health before stopping the components
	// GetReadyNonBlocking has a 100ms timeout to avoid blocking
	health, err := health.GetReadyNonBlocking()
	if err != nil {
		pkglog.Warnf("Cluster Agent health unknown: %s", err)
	} else if len(health.Unhealthy) > 0 {
		pkglog.Warnf("Some components were unhealthy: %v", health.Unhealthy)
	}

	// Cancel the main context to stop components
	mainCtxCancel()

	pkglog.Info("See ya!")
	pkglog.Flush()
	return nil
}

func initializeCCCache(ctx context.Context) error {
	pollInterval := time.Second * time.Duration(pkgconfig.Datadog().GetInt("cloud_foundry_cc.poll_interval"))
	_, err := cloudfoundry.ConfigureGlobalCCCache(
		ctx,
		pkgconfig.Datadog().GetString("cloud_foundry_cc.url"),
		pkgconfig.Datadog().GetString("cloud_foundry_cc.client_id"),
		pkgconfig.Datadog().GetString("cloud_foundry_cc.client_secret"),
		pkgconfig.Datadog().GetBool("cloud_foundry_cc.skip_ssl_validation"),
		pollInterval,
		pkgconfig.Datadog().GetInt("cloud_foundry_cc.apps_batch_size"),
		pkgconfig.Datadog().GetBool("cluster_agent.refresh_on_cache_miss"),
		pkgconfig.Datadog().GetBool("cluster_agent.serve_nozzle_data"),
		pkgconfig.Datadog().GetBool("cluster_agent.sidecars_tags"),
		pkgconfig.Datadog().GetBool("cluster_agent.isolation_segments_tags"),
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to initialize CC Cache: %v", err)
	}
	return nil
}

func initializeBBSCache(ctx context.Context) error {
	pollInterval := time.Second * time.Duration(pkgconfig.Datadog().GetInt("cloud_foundry_bbs.poll_interval"))
	// NOTE: we can't use GetPollInterval in ConfigureGlobalBBSCache, as that causes import cycle

	includeListString := pkgconfig.Datadog().GetStringSlice("cloud_foundry_bbs.env_include")
	excludeListString := pkgconfig.Datadog().GetStringSlice("cloud_foundry_bbs.env_exclude")

	includeList := make([]*regexp.Regexp, len(includeListString))
	excludeList := make([]*regexp.Regexp, len(excludeListString))

	for i, pattern := range includeListString {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("failed to compile cloud_foundry_bbs.env_include regex pattern %s: %s", pattern, err.Error())
		}
		includeList[i] = re
	}

	for i, pattern := range excludeListString {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("failed to compile cloud_foundry_bbs.env_exclude regex pattern %s: %s", pattern, err.Error())
		}
		excludeList[i] = re
	}

	bc, err := cloudfoundry.ConfigureGlobalBBSCache(
		ctx,
		pkgconfig.Datadog().GetString("cloud_foundry_bbs.url"),
		pkgconfig.Datadog().GetString("cloud_foundry_bbs.ca_file"),
		pkgconfig.Datadog().GetString("cloud_foundry_bbs.cert_file"),
		pkgconfig.Datadog().GetString("cloud_foundry_bbs.key_file"),
		pollInterval,
		includeList,
		excludeList,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to initialize BBS Cache: %s", err.Error())
	}
	pkglog.Info("Waiting for initial warmup of BBS Cache")
	ticker := time.NewTicker(time.Second)
	timer := time.NewTimer(pollInterval * 5)
	for {
		select {
		case <-ticker.C:
			if bc.LastUpdated().After(time.Time{}) {
				return nil
			}
		case <-timer.C:
			ticker.Stop()
			return fmt.Errorf("BBS Cache failed to warm up. Misconfiguration error? Inspect logs")
		}
	}
}

func setupClusterCheck(ctx context.Context, ac autodiscovery.Component) (*clusterchecks.Handler, error) {
	handler, err := clusterchecks.NewHandler(ac)
	if err != nil {
		return nil, err
	}
	go handler.Run(ctx)

	pkglog.Info("Started cluster check Autodiscovery")
	return handler, nil
}
