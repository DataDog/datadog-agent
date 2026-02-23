// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && clusterchecks

//nolint:revive // TODO(PLINT) Fix revive linter
package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"

	"code.cloudfoundry.org/bbs"
	"github.com/cloudfoundry-community/go-cfclient/v2"
	"github.com/gorilla/mux"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/cluster-agent-cloudfoundry/command"
	"github.com/DataDog/datadog-agent/cmd/cluster-agent/api"
	dcav1 "github.com/DataDog/datadog-agent/cmd/cluster-agent/api/v1"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/collector/collector/collectorimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	"github.com/DataDog/datadog-agent/comp/core/config"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	diagnosefx "github.com/DataDog/datadog-agent/comp/core/diagnose/fx"
	healthprobe "github.com/DataDog/datadog-agent/comp/core/healthprobe/def"
	healthprobefx "github.com/DataDog/datadog-agent/comp/core/healthprobe/fx"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/core/settings/settingsimpl"
	"github.com/DataDog/datadog-agent/comp/core/status"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	localTaggerfx "github.com/DataDog/datadog-agent/comp/core/tagger/fx"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadfilterfx "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx"
	wmcatalog "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/catalog"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	workloadmetainit "github.com/DataDog/datadog-agent/comp/core/workloadmeta/init"
	filterlistfx "github.com/DataDog/datadog-agent/comp/filterlist/fx"
	"github.com/DataDog/datadog-agent/comp/forwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/eventplatformreceiverimpl"
	orchestratorForwarderImpl "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorimpl"
	haagentfx "github.com/DataDog/datadog-agent/comp/haagent/fx"
	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/def"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	dcametadata "github.com/DataDog/datadog-agent/comp/metadata/clusteragent/def"
	dcametadatafx "github.com/DataDog/datadog-agent/comp/metadata/clusteragent/fx"
	clusterchecksmetadata "github.com/DataDog/datadog-agent/comp/metadata/clusterchecks/def"
	clusterchecksmetadatafx "github.com/DataDog/datadog-agent/comp/metadata/clusterchecks/fx"

	metadatarunnerimpl "github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	logscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx"
	metricscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx"
	"github.com/DataDog/datadog-agent/pkg/clusteragent"
	clusterchecksHandler "github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks"
	pkgcollector "github.com/DataDog/datadog-agent/pkg/collector"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders/cloudfoundry"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
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
					LogParams:    log.ForDaemon(command.LoggerName, "log_file", defaultpaths.DCALogFile),
				}),
				core.Bundle(true),
				hostnameimpl.Module(),
				forwarder.Bundle(defaultforwarder.NewParams(defaultforwarder.WithResolvers())),
				filterlistfx.Module(),
				demultiplexerimpl.Module(demultiplexerimpl.NewDefaultParams()),
				orchestratorForwarderImpl.Module(orchestratorForwarderImpl.NewDisabledParams()),
				eventplatformimpl.Module(eventplatformimpl.NewDisabledParams()),
				eventplatformreceiverimpl.Module(),

				// setup workloadmeta
				wmcatalog.GetCatalog(),
				workloadmetafx.Module(workloadmeta.Params{
					InitHelper: workloadmetainit.GetWorkloadmetaInit(),
				}), // TODO(components): check what this must be for cluster-agent-cloudfoundry
				localTaggerfx.Module(),
				workloadfilterfx.Module(),
				collectorimpl.Module(),
				fx.Provide(func() option.Option[serializer.MetricSerializer] {
					return option.None[serializer.MetricSerializer]()
				}),
				fx.Provide(func() option.Option[integrations.Component] {
					return option.None[integrations.Component]()
				}),
				fx.Provide(func() option.Option[agenttelemetry.Component] {
					return option.None[agenttelemetry.Component]()
				}),
				fx.Provide(func() option.Option[healthplatform.Component] {
					return option.None[healthplatform.Component]()
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
				// InitSharedContainerProvider must be called before the application starts so the workloadmeta collector can be initiailized correctly.
				// Since the tagger depends on the workloadmeta collector, we can not make the tagger a dependency of workloadmeta as it would create a circular dependency.
				// TODO: (component) - once we remove the dependency of workloadmeta component from the tagger component
				// we can include the tagger as part of the workloadmeta component.
				fx.Invoke(func(wmeta workloadmeta.Component, tagger tagger.Component, filterStore workloadfilter.Component) {
					proccontainers.InitSharedContainerProvider(wmeta, tagger, filterStore)
				}),
				haagentfx.Module(),
				logscompressionfx.Module(),
				metricscompressionfx.Module(),
				diagnosefx.Module(),
				fx.Provide(func(demuxInstance demultiplexer.Component) serializer.MetricSerializer {
					return demuxInstance.Serializer()
				}),
				metadatarunnerimpl.Module(),
				dcametadatafx.Module(),

				clusterchecksmetadatafx.Module(),
				ipcfx.ModuleReadWrite(),
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
	filterStore workloadfilter.Component,
	wmeta workloadmeta.Component,
	ac autodiscovery.Component,
	secretResolver secrets.Component,
	collector collector.Component,
	statusComponent status.Component,
	_ healthprobe.Component,
	settings settings.Component,
	logReceiver option.Option[integrations.Component],
	ipc ipc.Component,
	diagonseComp diagnose.Component,
	dcametadataComp dcametadata.Component,
	clusterChecksMetadataComp clusterchecksmetadata.Component,
	telemetry telemetry.Component,
) error {
	mainCtx, mainCtxCancel := context.WithCancel(context.Background())
	defer mainCtxCancel() // Calling cancel twice is safe

	if !pkgconfigsetup.Datadog().IsSet("api_key") {
		pkglog.Critical("no API key configured, exiting")
		return nil
	}

	// get hostname
	hname, err := hostname.Get(context.TODO())
	if err != nil {
		return pkglog.Errorf("Error while getting hostname, exiting: %v", err)
	}
	pkglog.Infof("Hostname is: %s", hname)

	demultiplexer.AddAgentStartupTelemetry(version.AgentVersion + " - Datadog Cluster Agent")

	pkglog.Infof("Datadog Cluster Agent is now running.")

	// Setup a channel to catch OS signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	// initialize CC Cache
	var ccCache cloudfoundry.CCCacheI
	ccCache, err = initializeCCCache(mainCtx)
	if err != nil {
		_ = pkglog.Errorf("Error initializing Cloud Foundry CCAPI cache, some advanced tagging features may be missing: %v", err)
	}

	// initialize BBS Cache before starting provider/listener, passing the CC cache for enrichment
	if err = initializeBBSCache(mainCtx, ccCache); err != nil {
		return err
	}

	common.LoadComponents(secretResolver, wmeta, taggerComp, filterStore, ac, pkgconfigsetup.Datadog().GetString("confd_path"))

	// Set up check collector
	ac.AddScheduler("check", pkgcollector.InitCheckScheduler(option.New(collector), demultiplexer, logReceiver, taggerComp, filterStore), true)

	// start the autoconfig, this will immediately run any configured check
	ac.LoadAndRun(mainCtx)

	if err = api.StartServer(mainCtx, wmeta, taggerComp, ac, statusComponent, settings, config, ipc, diagonseComp, dcametadataComp, clusterChecksMetadataComp, telemetry); err != nil {
		return log.Errorf("Error while starting agent API, exiting: %v", err)
	}

	var clusterCheckHandler *clusterchecksHandler.Handler
	clusterCheckHandler, err = setupClusterCheck(mainCtx, ac, taggerComp)
	if err == nil {
		api.ModifyAPIRouter(func(r *mux.Router) {
			dcav1.InstallChecksEndpoints(r, clusteragent.ServerContext{ClusterCheckHandler: clusterCheckHandler})
		})

		// Set cluster checks handler in clusterchecks component
		clusterChecksMetadataComp.SetClusterHandler(clusterCheckHandler)
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

func initializeCCCache(ctx context.Context) (cloudfoundry.CCCacheI, error) {
	pollInterval := time.Second * time.Duration(pkgconfigsetup.Datadog().GetInt("cloud_foundry_cc.poll_interval"))

	// Create the CF client
	ccClient, err := cloudfoundry.NewCFClient(&cfclient.Config{
		ApiAddress:        pkgconfigsetup.Datadog().GetString("cloud_foundry_cc.url"),
		ClientID:          pkgconfigsetup.Datadog().GetString("cloud_foundry_cc.client_id"),
		ClientSecret:      pkgconfigsetup.Datadog().GetString("cloud_foundry_cc.client_secret"),
		SkipSslValidation: pkgconfigsetup.Datadog().GetBool("cloud_foundry_cc.skip_ssl_validation"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create CC client: %v", err)
	}

	ccCache, err := cloudfoundry.ConfigureGlobalCCCache(ctx, cloudfoundry.CCCacheConfig{
		CCAPIClient:        ccClient,
		PollInterval:       pollInterval,
		AppsBatchSize:      pkgconfigsetup.Datadog().GetInt("cloud_foundry_cc.apps_batch_size"),
		RefreshCacheOnMiss: pkgconfigsetup.Datadog().GetBool("cluster_agent.refresh_on_cache_miss"),
		ServeNozzleData:    pkgconfigsetup.Datadog().GetBool("cluster_agent.serve_nozzle_data"),
		SidecarsTags:       pkgconfigsetup.Datadog().GetBool("cluster_agent.sidecars_tags"),
		SegmentsTags:       pkgconfigsetup.Datadog().GetBool("cluster_agent.isolation_segments_tags"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize CC Cache: %v", err)
	}
	return ccCache, nil
}

func initializeBBSCache(ctx context.Context, ccCache cloudfoundry.CCCacheI) error {
	pollInterval := time.Second * time.Duration(pkgconfigsetup.Datadog().GetInt("cloud_foundry_bbs.poll_interval"))
	// NOTE: we can't use GetPollInterval in ConfigureGlobalBBSCache, as that causes import cycle

	includeListString := pkgconfigsetup.Datadog().GetStringSlice("cloud_foundry_bbs.env_include")
	excludeListString := pkgconfigsetup.Datadog().GetStringSlice("cloud_foundry_bbs.env_exclude")

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

	// Create the BBS client
	bbsClient, err := bbs.NewClient(
		pkgconfigsetup.Datadog().GetString("cloud_foundry_bbs.url"),
		pkgconfigsetup.Datadog().GetString("cloud_foundry_bbs.ca_file"),
		pkgconfigsetup.Datadog().GetString("cloud_foundry_bbs.cert_file"),
		pkgconfigsetup.Datadog().GetString("cloud_foundry_bbs.key_file"),
		0, // clientSessionCacheSize
		0, // maxIdleConnsPerHost
	)
	if err != nil {
		return fmt.Errorf("failed to create BBS client: %s", err.Error())
	}

	bc, err := cloudfoundry.ConfigureGlobalBBSCache(ctx, cloudfoundry.BBSCacheConfig{
		BBSClient:    bbsClient,
		PollInterval: pollInterval,
		IncludeList:  includeList,
		ExcludeList:  excludeList,
		CCCache:      ccCache,
	})
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
			return errors.New("BBS Cache failed to warm up. Misconfiguration error? Inspect logs")
		}
	}
}

func setupClusterCheck(ctx context.Context, ac autodiscovery.Component, tagger tagger.Component) (*clusterchecksHandler.Handler, error) {
	handler, err := clusterchecksHandler.NewHandler(ac, tagger)
	if err != nil {
		return nil, err
	}
	go handler.Run(ctx)

	pkglog.Info("Started cluster check Autodiscovery")
	return handler, nil
}
