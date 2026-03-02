// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

// Package start implements 'cluster-agent start'.
package start

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	admissioncmd "github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/cmd/cluster-agent/api"
	dcav1 "github.com/DataDog/datadog-agent/cmd/cluster-agent/api/v1"
	"github.com/DataDog/datadog-agent/cmd/cluster-agent/command"
	"github.com/DataDog/datadog-agent/cmd/cluster-agent/custommetrics"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	datadogclient "github.com/DataDog/datadog-agent/comp/autoscaling/datadogclient/def"
	datadogclientmodule "github.com/DataDog/datadog-agent/comp/autoscaling/datadogclient/fx"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/collector/collector/collectorimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	agenttelemetryfx "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/fx"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	"github.com/DataDog/datadog-agent/comp/core/config"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	diagnosefx "github.com/DataDog/datadog-agent/comp/core/diagnose/fx"
	healthprobe "github.com/DataDog/datadog-agent/comp/core/healthprobe/def"
	healthprobefx "github.com/DataDog/datadog-agent/comp/core/healthprobe/fx"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/core/settings/settingsimpl"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/status/statusimpl"
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
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/eventplatformreceiverimpl"
	orchestratorForwarderImpl "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorimpl"
	haagentfx "github.com/DataDog/datadog-agent/comp/haagent/fx"
	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/def"
	traceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/def"
	remotetraceroutefx "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/fx-remote"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/appsec"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/mcp"

	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	metadatarunner "github.com/DataDog/datadog-agent/comp/metadata/runner"
	metadatarunnerimpl "github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	privateactionrunner "github.com/DataDog/datadog-agent/comp/privateactionrunner/impl"
	rccomp "github.com/DataDog/datadog-agent/comp/remote-config/rcservice"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservice/rcserviceimpl"
	"github.com/DataDog/datadog-agent/comp/remote-config/rctelemetryreporter/rctelemetryreporterimpl"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	logscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx"
	metricscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx"
	"github.com/DataDog/datadog-agent/pkg/clusteragent"
	admissionpkg "github.com/DataDog/datadog-agent/pkg/clusteragent/admission"
	admissionpatch "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patch"
	apidca "github.com/DataDog/datadog-agent/pkg/clusteragent/api"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/cluster"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/provider"
	pkgclusterchecks "github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks"
	clusteragentMetricsStatus "github.com/DataDog/datadog-agent/pkg/clusteragent/metricsstatus"
	orchestratorStatus "github.com/DataDog/datadog-agent/pkg/clusteragent/orchestrator"
	pkgcollector "github.com/DataDog/datadog-agent/pkg/collector"
	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	commonsettings "github.com/DataDog/datadog-agent/pkg/config/settings"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/diagnose/connectivity"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	hostnameStatus "github.com/DataDog/datadog-agent/pkg/status/clusteragent/hostname"
	endpointsStatus "github.com/DataDog/datadog-agent/pkg/status/endpoints"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/coredump"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	apicommon "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/controllers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/cloudprovider"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/version"

	"github.com/gorilla/mux"
	"github.com/spf13/cobra"
	"go.uber.org/fx"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"

	dcametadata "github.com/DataDog/datadog-agent/comp/metadata/clusteragent/def"
	dcametadatafx "github.com/DataDog/datadog-agent/comp/metadata/clusteragent/fx"
	clusterchecksmetadata "github.com/DataDog/datadog-agent/comp/metadata/clusterchecks/def"
	clusterchecksmetadatafx "github.com/DataDog/datadog-agent/comp/metadata/clusterchecks/fx"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/languagedetection"

	// Core checks

	corecheckLoader "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/helm"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/ksm"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/kubernetesapiserver"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/collector/python"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

// Commands returns a slice of subcommands for the 'cluster-agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start the Cluster Agent",
		Long:  `Runs Datadog Cluster agent in the foreground`,
		RunE: func(_ *cobra.Command, _ []string) error {
			// TODO: once the cluster-agent is represented as a component, and
			// not a function (start), this will use `fxutil.Run` instead of
			// `fxutil.OneShot`.
			return fxutil.OneShot(start,
				fx.Supply(globalParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewClusterAgentParams(globalParams.ConfFilePath, config.WithExtraConfFiles(globalParams.ExtraConfFilePath)),
					LogParams:    log.ForDaemon(command.LoggerName, "log_file", defaultpaths.DCALogFile),
				}),
				core.Bundle(core.WithSecrets()),
				hostnameimpl.Module(),
				forwarder.Bundle(defaultforwarder.NewParams(defaultforwarder.WithResolvers(), defaultforwarder.WithDisableAPIKeyChecking())),
				filterlistfx.Module(),
				demultiplexerimpl.Module(demultiplexerimpl.NewDefaultParams()),
				orchestratorForwarderImpl.Module(orchestratorForwarderImpl.NewDefaultParams()),
				eventplatformimpl.Module(eventplatformimpl.NewDisabledParams()),
				eventplatformreceiverimpl.Module(),
				// setup workloadmeta
				wmcatalog.GetCatalog(),
				workloadmetafx.Module(workloadmeta.Params{
					InitHelper: workloadmetainit.GetWorkloadmetaInit(),
					AgentType:  workloadmeta.ClusterAgent,
				}), // TODO(components): check what this must be for cluster-agent-cloudfoundry
				fx.Supply(context.Background()),
				localTaggerfx.Module(),
				workloadfilterfx.Module(),
				fx.Supply(
					status.Params{
						PythonVersionGetFunc: python.GetPythonVersion,
					},
					status.NewInformationProvider(leaderelection.Provider{}),
					status.NewInformationProvider(clusteragentMetricsStatus.Provider{}),
					status.NewInformationProvider(admissionpkg.Provider{}),
					status.NewInformationProvider(endpointsStatus.Provider{}),
					status.NewInformationProvider(pkgclusterchecks.Provider{}),
					status.NewInformationProvider(orchestratorStatus.Provider{}),
				),
				fx.Provide(func(config config.Component, hostname hostnameinterface.Component) status.HeaderInformationProvider {
					return status.NewHeaderInformationProvider(hostnameStatus.NewProvider(config, hostname))
				}),
				fx.Provide(func() option.Option[integrations.Component] {
					return option.None[integrations.Component]()
				}),
				agenttelemetryfx.Module(),
				fx.Provide(func() option.Option[healthplatform.Component] {
					return option.None[healthplatform.Component]()
				}),

				statusimpl.Module(),
				collectorimpl.Module(),
				fx.Provide(func() option.Option[serializer.MetricSerializer] {
					return option.None[serializer.MetricSerializer]()
				}),
				autodiscoveryimpl.Module(),
				rcserviceimpl.Module(),
				rctelemetryreporterimpl.Module(),
				fx.Provide(func(config config.Component) healthprobe.Options {
					return healthprobe.Options{
						Port:           config.GetInt("health_port"),
						LogsGoroutines: config.GetBool("log_all_goroutines_when_unhealthy"),
					}
				}),
				healthprobefx.Module(),
				fx.Provide(func(c config.Component) settings.Params {
					return settings.Params{
						Settings: map[string]settings.RuntimeSetting{
							"log_level":                      commonsettings.NewLogLevelRuntimeSetting(),
							"runtime_mutex_profile_fraction": commonsettings.NewRuntimeMutexProfileFraction(),
							"runtime_block_profile_rate":     commonsettings.NewRuntimeBlockProfileRate(),
							"internal_profiling_goroutines":  commonsettings.NewProfilingGoroutines(),
							"internal_profiling":             commonsettings.NewProfilingRuntimeSetting("internal_profiling", "datadog-cluster-agent"),
						},
						Config: c,
					}
				}),
				settingsimpl.Module(),
				datadogclientmodule.Module(),
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
				remotetraceroutefx.Module(),
			)
		},
	}

	return []*cobra.Command{startCmd}
}

func start(log log.Component,
	config config.Component,
	taggerComp tagger.Component,
	telemetry telemetry.Component,
	demultiplexer demultiplexer.Component,
	filterStore workloadfilter.Component,
	wmeta workloadmeta.Component,
	ac autodiscovery.Component,
	dc option.Option[datadogclient.Component],
	secretResolver secrets.Component,
	statusComponent status.Component,
	collector collector.Component,
	rcService option.Option[rccomp.Component],
	logReceiver option.Option[integrations.Component],
	_ healthprobe.Component,
	settings settings.Component,
	compression logscompression.Component,
	datadogConfig config.Component,
	ipc ipc.Component,
	diagnoseComp diagnose.Component,
	dcametadataComp dcametadata.Component,
	hostnameGetter hostnameinterface.Component,

	clusterChecksMetadataComp clusterchecksmetadata.Component,
	_ metadatarunner.Component,
	tracerouteComp traceroute.Component,
	eventPlatform eventplatform.Component,
) error {
	stopCh := make(chan struct{})
	validatingStopCh := make(chan struct{})

	mainCtx, mainCtxCancel := context.WithCancel(context.Background())
	defer mainCtxCancel()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	// Starting Cluster Agent sequence
	// Initialization order is important for multiple reasons, see comments

	if err := coredump.Setup(config); err != nil {
		pkglog.Warnf("Can't setup core dumps: %v, core dumps might not be available after a crash", err)
	}

	// Setup Internal Profiling
	common.SetupInternalProfiling(settings, config, "")

	if !config.IsSet("api_key") {
		return errors.New("no API key configured, exiting")
	}

	// Expose the registered metrics via HTTP.
	http.Handle("/metrics", telemetry.Handler())
	metricsPort := config.GetInt("metrics_port")
	metricsServer := &http.Server{
		Addr:    fmt.Sprintf("0.0.0.0:%d", metricsPort),
		Handler: http.DefaultServeMux,
	}

	go func() {
		err := metricsServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			pkglog.Errorf("Error creating expvar server on port %v: %v", metricsPort, err)
		}
	}()

	// Create the Leader election engine and initialize it
	leaderelection.CreateGlobalLeaderEngine(mainCtx)
	le, err := leaderelection.GetLeaderEngine()
	if err != nil {
		return err
	}

	// Setup the leader forwarder for autoscaling failover store, language detection and cluster checks
	if config.GetBool("cluster_checks.enabled") ||
		(config.GetBool("language_detection.enabled") && config.GetBool("language_detection.reporting.enabled")) ||
		config.GetBool("autoscaling.failover.enabled") {
		apidca.NewGlobalLeaderForwarder(
			config.GetInt("cluster_agent.cmd_port"),
			config.GetInt("cluster_agent.max_leader_connections"),
		)
	}

	// Register Diagnose functions
	diagnoseCatalog := diagnose.GetCatalog()

	diagnoseCatalog.Register(diagnose.AutodiscoveryConnectivity, func(_ diagnose.Config) []diagnose.Diagnosis {
		return connectivity.DiagnoseMetadataAutodiscoveryConnectivity()
	})

	// Starting server early to ease investigations
	if err := api.StartServer(mainCtx, wmeta, taggerComp, ac, statusComponent, settings, config, ipc, diagnoseComp, dcametadataComp, clusterChecksMetadataComp, telemetry); err != nil {
		return fmt.Errorf("Error while starting agent API, exiting: %v", err)
	}

	// Getting connection to APIServer, it's done before Hostname resolution
	// as hostname resolution may call APIServer
	pkglog.Info("Waiting to obtain APIClient connection")
	apiCl, err := apiserver.WaitForAPIClient(mainCtx) // make sure we can connect to the apiserver
	if err != nil {
		return fmt.Errorf("Fatal error: Cannot connect to the apiserver: %v", err)
	}
	pkglog.Infof("Got APIClient connection")

	// Get hostname as aggregator requires hostname
	hname, err := hostname.Get(mainCtx)
	if err != nil {
		return fmt.Errorf("Error while getting hostname, exiting: %v", err)
	}
	pkglog.Infof("Hostname is: %s", hname)

	// If a cluster-agent looses the connectivity to DataDog, we still want it to remain ready so that its endpoint remains in the service because:
	// * It is still able to serve metrics to the WPA controller and
	// * The metrics reported are reported as stale so that there is no "lie" about the accuracy of the reported metrics.
	// Serving stale data is better than serving no data at all.
	demultiplexer.AddAgentStartupTelemetry(version.AgentVersion + " - Datadog Cluster Agent")

	// Create event recorder
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(pkglog.Infof)
	eventBroadcaster.StartRecordingToSink(&corev1.EventSinkImpl{Interface: apiCl.Cl.CoreV1().Events("")})
	eventRecorder := eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "datadog-cluster-agent"})

	ctx := controllers.ControllerContext{
		InformerFactory:             apiCl.InformerFactory,
		APIExentionsInformerFactory: apiCl.APIExentionsInformerFactory,
		DynamicClient:               apiCl.DynamicInformerCl,
		DynamicInformerFactory:      apiCl.DynamicInformerFactory,
		Client:                      apiCl.InformerCl,
		IsLeaderFunc:                le.IsLeader,
		EventRecorder:               eventRecorder,
		WorkloadMeta:                wmeta,
		StopCh:                      stopCh,
		DatadogClient:               dc,
	}

	if aggErr := controllers.StartControllers(&ctx); aggErr != nil {
		for _, err := range aggErr.Errors() {
			pkglog.Warnf("Error while starting controller: %v", err)
		}
	}

	clusterName := clustername.GetRFC1123CompliantClusterName(context.TODO(), hname)
	// Generate and persist a cluster ID
	// this must be a UUID, and ideally be stable for the lifetime of a cluster,
	// so we store it in a configmap that we try and read before generating a new one.
	clusterID, err := apicommon.GetOrCreateClusterID(apiCl.Cl.CoreV1())
	if err != nil {
		pkglog.Errorf("Failed to generate or retrieve the cluster ID, err: %v", err)
	}
	if clusterName == "" {
		if config.GetBool("autoscaling.workload.enabled") || config.GetBool("autoscaling.cluster.enabled") {
			return errors.New("Failed to start: autoscaling is enabled but no cluster name detected, exiting")
		}
		pkglog.Warn("Failed to auto-detect a Kubernetes cluster name. We recommend you set it manually via the cluster_name config option")
	}
	// determine kube distribution for that node.
	kubeDistro := cloudprovider.DCAGetName(mainCtx)

	pkglog.Infof("Cluster ID: %s, Cluster Name: %s, Kube Distribution: %s", clusterID, clusterName, kubeDistro)

	// Setup APM tracing
	if config.GetBool("cluster_agent.tracing.enabled") {
		sampleRate := config.GetFloat64("cluster_agent.tracing.sample_rate")
		if sampleRate < 0.0 || sampleRate > 1.0 {
			pkglog.Warnf("Invalid cluster_agent.tracing.sample_rate: %f (must be between 0.0 and 1.0), using default 0.1", sampleRate)
			sampleRate = 0.1
		}

		opts := []tracer.StartOption{
			tracer.WithService("datadog-cluster-agent"),
			tracer.WithServiceVersion(version.AgentVersion),
			tracer.WithSampler(tracer.NewRateSampler(sampleRate)),
			tracer.WithGlobalTag("cluster_name", clusterName),
			tracer.WithLogStartup(false),
		}
		if env := config.GetString("cluster_agent.tracing.env"); env != "" {
			opts = append(opts, tracer.WithEnv(env))
		}
		if clusterID != "" {
			opts = append(opts, tracer.WithGlobalTag("cluster_id", clusterID))
		}
		tracer.Start(opts...)
		pkglog.Infof("APM tracing enabled for Cluster Agent (sample_rate=%.2f)", sampleRate)
		defer tracer.Stop()
	}

	// Initialize and start remote configuration client
	var rcClient *rcclient.Client
	rcserv, isSet := rcService.Get()
	if configUtils.IsRemoteConfigEnabled(config) && isSet {
		var products []string
		if config.GetBool("admission_controller.auto_instrumentation.patcher.enabled") {
			products = append(products, state.ProductAPMTracing)
		}
		if config.GetBool("autoscaling.workload.enabled") {
			products = append(products, state.ProductContainerAutoscalingSettings, state.ProductContainerAutoscalingValues)
		}
		if config.GetBool("autoscaling.cluster.enabled") {
			products = append(products, state.ProductClusterAutoscalingValues)
		}
		if config.GetBool("admission_controller.auto_instrumentation.enabled") || config.GetBool("apm_config.instrumentation.enabled") {
			products = append(products, state.ProductGradualRollout)
		}
		// Add private action runner product if enabled
		if config.GetBool("private_action_runner.enabled") {
			products = append(products, state.ProductActionPlatformRunnerKeys)
		}

		if len(products) > 0 {
			var err error
			rcClient, err = initializeRemoteConfigClient(rcserv, config, clusterName, clusterID, products...)
			if err != nil {
				log.Errorf("Failed to start remote-configuration: %v", err)
			} else {
				rcClient.Start()
				defer func() {
					rcClient.Close()
				}()
			}
		}
	}

	// FIXME: move LoadComponents and AC.LoadAndRun in their own package so we
	// don't import cmd/agent

	// create and setup the autoconfig instance
	// The autoconfig instance setup happens in the workloadmeta start hook
	// create and setup the Collector and others.
	common.LoadComponents(secretResolver, wmeta, taggerComp, filterStore, ac, config.GetString("confd_path"))

	// Set up check collector
	registerChecks(wmeta, taggerComp, config)
	ac.AddScheduler("check", pkgcollector.InitCheckScheduler(option.New(collector), demultiplexer, logReceiver, taggerComp, filterStore), true)

	// start the autoconfig, this will immediately run any configured check
	ac.LoadAndRun(mainCtx)

	if config.GetBool("cluster_checks.enabled") {
		// Start the cluster check Autodiscovery
		clusterCheckHandler, err := setupClusterCheck(mainCtx, ac, taggerComp)
		if err == nil {
			api.ModifyAPIRouter(func(r *mux.Router) {
				dcav1.InstallChecksEndpoints(r, clusteragent.ServerContext{ClusterCheckHandler: clusterCheckHandler})
			})

			// Set cluster checks handler in clusterchecks component
			clusterChecksMetadataComp.SetClusterHandler(clusterCheckHandler)
		} else {
			pkglog.Errorf("Error while setting up cluster check Autodiscovery, CLC API endpoints won't be available, err: %v", err)
		}
	} else {
		pkglog.Debug("Cluster check Autodiscovery disabled")
	}

	wg := sync.WaitGroup{}
	// Autoscaler Controller Goroutine
	if config.GetBool("external_metrics_provider.enabled") {
		// Start the k8s custom metrics server. This is a blocking call
		wg.Add(1)
		go func() {
			defer wg.Done()

			errServ := custommetrics.RunServer(mainCtx, apiCl, dc)
			if errServ != nil {
				pkglog.Errorf("Error in the External Metrics API Server: %v", errServ)
			}
		}()
	}

	// Autoscaling Product
	var pa workload.PodPatcher
	if config.GetBool("autoscaling.workload.enabled") {
		if rcClient == nil {
			return errors.New("Remote config is disabled or failed to initialize, remote config is a required dependency for autoscaling")
		}

		if !config.GetBool("admission_controller.enabled") {
			log.Error("Admission controller is disabled, vertical autoscaling requires the admission controller to be enabled. Vertical scaling will be disabled.")
		}

		if adapter, err := provider.StartWorkloadAutoscaling(mainCtx, clusterID, clusterName, le.IsLeader, apiCl, rcClient, wmeta, taggerComp, demultiplexer); err == nil {
			pa = adapter
		} else {
			return fmt.Errorf("Error while starting workload autoscaling: %v", err)
		}
	}

	if config.GetBool("autoscaling.cluster.enabled") {
		if rcClient == nil {
			return errors.New("Remote config is disabled or failed to initialize, remote config is a required dependency for autoscaling")
		}

		if err := cluster.StartClusterAutoscaling(mainCtx, clusterID, clusterName, le.IsLeader, apiCl, rcClient, demultiplexer); err != nil {
			return fmt.Errorf("Error while starting cluster autoscaling: %w", err)
		}
	}

	// Compliance
	if config.GetBool("compliance_config.enabled") {
		wg.Add(1)
		go func() {
			defer wg.Done()

			if err := runCompliance(mainCtx, demultiplexer, wmeta, filterStore, apiCl, compression, le.IsLeader); err != nil {
				pkglog.Errorf("Error while running compliance agent: %v", err)
			}
		}()
	}

	if config.GetBool("language_detection.enabled") && config.GetBool("cluster_agent.language_detection.patcher.enabled") {
		if err = languagedetection.Start(mainCtx, le.IsLeader, wmeta, log, config); err != nil {
			log.Errorf("Cannot start language detection patcher: %v", err)
		}
	}

	if config.GetBool("appsec.proxy.enabled") && config.GetBool("cluster_agent.appsec.injector.enabled") {
		// Should be run before admissionpkg.StartControllers
		if err := appsec.Start(mainCtx, log, config, le.Subscribe); err != nil {
			log.Errorf("Cannot start appsec injector: %v", err)
		}
	} else {
		appsec.Cleanup(mainCtx, log, config, le.Subscribe)
	}

	if config.GetBool("private_action_runner.enabled") {
		drain, err := startPrivateActionRunner(mainCtx, config, hostnameGetter, rcClient, le, log, taggerComp, tracerouteComp, eventPlatform)
		if err != nil {
			log.Errorf("Cannot start private action runner: %v", err)
		} else {
			defer drain()
		}
	}

	if config.GetBool("admission_controller.enabled") {
		if config.GetBool("admission_controller.auto_instrumentation.patcher.enabled") {
			patchCtx := admissionpatch.ControllerContext{
				LeadershipStateSubscribeFunc: le.Subscribe,
				K8sClient:                    apiCl.Cl,
				RcClient:                     rcClient,
				ClusterName:                  clusterName,
				ClusterID:                    clusterID,
				StopCh:                       stopCh,
			}
			if err := admissionpatch.StartControllers(patchCtx); err != nil {
				log.Errorf("Cannot start auto instrumentation patcher: %v", err)
			}
		} else {
			log.Info("Auto instrumentation patcher is disabled")
		}

		admissionCtx := admissionpkg.ControllerContext{
			LeadershipStateSubscribeFunc: le.Subscribe,
			SecretInformers:              apiCl.CertificateSecretInformerFactory,
			ValidatingInformers:          apiCl.WebhookConfigInformerFactory,
			MutatingInformers:            apiCl.WebhookConfigInformerFactory,
			Client:                       apiCl.Cl,
			StopCh:                       stopCh,
			ValidatingStopCh:             validatingStopCh,
			Demultiplexer:                demultiplexer,
		}

		webhooks, err := admissionpkg.StartControllers(admissionCtx, wmeta, pa, datadogConfig)
		// Ignore the error if it's related to the validatingwebhookconfigurations.
		var syncInformerError *apiserver.SyncInformersError
		if err != nil && !(errors.As(err, &syncInformerError) && syncInformerError.Name == apiserver.ValidatingWebhooksInformer) {
			pkglog.Errorf("Could not start admission controller: %v", err)
		} else {
			if err != nil {
				pkglog.Warnf("Admission controller started with errors: %v", err)
				pkglog.Debugf("Closing ValidatingWebhooksInformer channel")
				close(validatingStopCh)
			}
			// Webhook and secret controllers are started successfully
			// Set up the k8s admission webhook server
			secretsLister := apiCl.CertificateSecretInformerFactory.Core().V1().Secrets().Lister()
			server := admissioncmd.NewServer(secretsLister)

			for _, webhookConf := range webhooks {
				server.Register(webhookConf.Endpoint(), webhookConf.Name(), webhookConf.WebhookType(), webhookConf.WebhookFunc(), apiCl.DynamicCl, apiCl.Cl)
			}

			// Start the k8s admission webhook server
			wg.Add(1)
			go func() {
				defer wg.Done()

				errServ := server.Run(mainCtx)
				if errServ != nil {
					pkglog.Errorf("Error in the Admission Controller Webhook Server: %v", errServ)
				}
			}()
		}
	} else {
		pkglog.Info("Admission controller is disabled")
	}

	if config.GetBool("cluster_agent.mcp.enabled") {
		// Get MCP configured endpoint
		mcpEndpoint := config.GetString("cluster_agent.mcp.endpoint")
		// Register MCP handler on the HTTP metrics server via HTTP
		mcpHandler := mcp.CreateMCPHandler()
		http.Handle(mcpEndpoint, mcpHandler)
		pkglog.Infof("MCP endpoint registered with HTTP metrics server on port %d: %s", metricsPort, mcpEndpoint)
	} else {
		pkglog.Debug("MCP server is disabled")
	}

	pkglog.Infof("All components started. Cluster Agent now running.")

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

	// wait for the External Metrics Server and the Admission Webhook Server to
	// stop properly
	wg.Wait()

	close(stopCh)
	if validatingStopCh != nil {
		close(validatingStopCh)
	}

	if err := metricsServer.Shutdown(context.Background()); err != nil {
		pkglog.Errorf("Error shutdowning metrics server on port %d: %v", metricsPort, err)
	}

	pkglog.Info("See ya!")
	pkglog.Flush()

	return nil
}

func setupClusterCheck(ctx context.Context, ac autodiscovery.Component, tagger tagger.Component) (*pkgclusterchecks.Handler, error) {
	handler, err := pkgclusterchecks.NewHandler(ac, tagger)
	if err != nil {
		return nil, err
	}
	go handler.Run(ctx)

	pkglog.Info("Started cluster check Autodiscovery")
	return handler, nil
}

func startPrivateActionRunner(
	ctx context.Context,
	config config.Component,
	hostnameGetter hostnameinterface.Component,
	rcClient *rcclient.Client,
	le *leaderelection.LeaderEngine,
	log log.Component,
	tagger tagger.Component,
	tracerouteComp traceroute.Component,
	eventPlatform eventplatform.Component,
) (func(), error) {
	if rcClient == nil {
		return nil, errors.New("Remote config is disabled or failed to initialize, remote config is a required dependency for private action runner")
	}
	if !config.GetBool("leader_election") {
		return nil, errors.New("leader election is not enabled on the Cluster Agent. The private action runner needs leader election for identity coordination across replicas")
	}
	le.StartLeaderElectionRun()
	app, err := privateactionrunner.NewPrivateActionRunner(ctx, config, hostnameGetter, rcClient, log, tagger, tracerouteComp, eventPlatform)
	if err != nil {
		return nil, err
	}
	// Start the private action runner asynchronously
	errChan := app.StartAsync(ctx)

	go func() {
		// We could ignore this error but it's better to log it for debugging purposes
		if err := <-errChan; err != nil {
			log.Errorf("Failed to start private action runner: %v", err)
		}
	}()
	return func() {
		if err := app.Stop(context.Background()); err != nil {
			log.Errorf("Error stopping private action runner: %v", err)
		}
	}, nil
}

func initializeRemoteConfigClient(rcService rccomp.Component, config config.Component, clusterName, clusterID string, products ...string) (*rcclient.Client, error) {
	if clusterName == "" {
		pkglog.Warn("cluster-name won't be set for remote-config client")
	}

	if clusterID == "" {
		pkglog.Warn("Error retrieving cluster ID: cluster-id won't be set for remote-config client")
	}

	pkglog.Debugf("Initializing remote-config client with cluster-name: '%s', cluster-id: '%s', products: %v", clusterName, clusterID, products)
	rcClient, err := rcclient.NewClient(rcService,
		rcclient.WithAgent("cluster-agent", version.AgentVersion),
		rcclient.WithCluster(clusterName, clusterID),
		rcclient.WithProducts(products...),
		rcclient.WithPollInterval(5*time.Second),
		rcclient.WithDirectorRootOverride(config.GetString("site"), config.GetString("remote_configuration.director_root")),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to create local remote-config client: %w", err)
	}

	return rcClient, nil
}

func registerChecks(wlm workloadmeta.Component, tagger tagger.Component, cfg config.Component) {
	corecheckLoader.RegisterCheck(kubernetesapiserver.CheckName, kubernetesapiserver.Factory(tagger))
	corecheckLoader.RegisterCheck(ksm.CheckName, ksm.Factory(tagger, nil)) // wmeta is not used in KSM when running from cluster-agent, so we can pass nil here
	corecheckLoader.RegisterCheck(helm.CheckName, helm.Factory())
	corecheckLoader.RegisterCheck(orchestrator.CheckName, orchestrator.Factory(wlm, cfg, tagger))
}
