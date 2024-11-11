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

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/controllers"
	"github.com/DataDog/datadog-agent/pkg/util/optional"

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
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	"github.com/DataDog/datadog-agent/comp/core/config"
	healthprobe "github.com/DataDog/datadog-agent/comp/core/healthprobe/def"
	healthprobefx "github.com/DataDog/datadog-agent/comp/core/healthprobe/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/core/settings/settingsimpl"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/status/statusimpl"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	wmcatalog "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/catalog"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	"github.com/DataDog/datadog-agent/comp/forwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/eventplatformreceiverimpl"
	orchestratorForwarderImpl "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorimpl"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	rccomp "github.com/DataDog/datadog-agent/comp/remote-config/rcservice"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservice/rcserviceimpl"
	"github.com/DataDog/datadog-agent/comp/remote-config/rctelemetryreporter/rctelemetryreporterimpl"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl"
	"github.com/DataDog/datadog-agent/pkg/clusteragent"
	admissionpkg "github.com/DataDog/datadog-agent/pkg/clusteragent/admission"
	admissionpatch "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patch"
	apidca "github.com/DataDog/datadog-agent/pkg/clusteragent/api"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks"
	clusteragentMetricsStatus "github.com/DataDog/datadog-agent/pkg/clusteragent/metricsstatus"
	orchestratorStatus "github.com/DataDog/datadog-agent/pkg/clusteragent/orchestrator"
	pkgcollector "github.com/DataDog/datadog-agent/pkg/collector"
	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	commonsettings "github.com/DataDog/datadog-agent/pkg/config/settings"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	hostnameStatus "github.com/DataDog/datadog-agent/pkg/status/clusteragent/hostname"
	endpointsStatus "github.com/DataDog/datadog-agent/pkg/status/endpoints"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	apicommon "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"

	"github.com/gorilla/mux"
	"github.com/spf13/cobra"
	"go.uber.org/fx"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/languagedetection"

	// Core checks

	corecheckLoader "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/helm"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/ksm"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/kubernetesapiserver"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/cpu/cpu"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/disk/disk"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/disk/io"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/filehandles"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/memory"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/uptime"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/system/winproc"
	"github.com/DataDog/datadog-agent/pkg/collector/python"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
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
					ConfigParams: config.NewClusterAgentParams(globalParams.ConfFilePath),
					SecretParams: secrets.NewEnabledParams(),
					LogParams:    log.ForDaemon(command.LoggerName, "log_file", defaultpaths.DCALogFile),
				}),
				core.Bundle(),
				forwarder.Bundle(defaultforwarder.NewParams(defaultforwarder.WithResolvers(), defaultforwarder.WithDisableAPIKeyChecking())),
				compressionimpl.Module(),
				demultiplexerimpl.Module(demultiplexerimpl.NewDefaultParams()),
				orchestratorForwarderImpl.Module(orchestratorForwarderImpl.NewDefaultParams()),
				eventplatformimpl.Module(eventplatformimpl.NewDisabledParams()),
				eventplatformreceiverimpl.Module(),
				// setup workloadmeta
				wmcatalog.GetCatalog(),
				workloadmetafx.Module(workloadmeta.Params{
					InitHelper: common.GetWorkloadmetaInit(),
					AgentType:  workloadmeta.ClusterAgent,
				}), // TODO(components): check what this must be for cluster-agent-cloudfoundry
				fx.Supply(context.Background()),
				fx.Provide(tagger.NewTaggerParams),
				taggerimpl.Module(),
				fx.Supply(
					status.Params{
						PythonVersionGetFunc: python.GetPythonVersion,
					},
					status.NewInformationProvider(leaderelection.Provider{}),
					status.NewInformationProvider(clusteragentMetricsStatus.Provider{}),
					status.NewInformationProvider(admissionpkg.Provider{}),
					status.NewInformationProvider(endpointsStatus.Provider{}),
					status.NewInformationProvider(clusterchecks.Provider{}),
					status.NewInformationProvider(orchestratorStatus.Provider{}),
				),
				fx.Provide(func(config config.Component) status.HeaderInformationProvider {
					return status.NewHeaderInformationProvider(hostnameStatus.NewProvider(config))
				}),
				fx.Provide(func() optional.Option[integrations.Component] {
					return optional.NewNoneOption[integrations.Component]()
				}),
				statusimpl.Module(),
				collectorimpl.Module(),
				fx.Provide(func() optional.Option[serializer.MetricSerializer] {
					return optional.NewNoneOption[serializer.MetricSerializer]()
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
				fx.Invoke(func(wmeta workloadmeta.Component, tagger tagger.Component) {
					proccontainers.InitSharedContainerProvider(wmeta, tagger)
				}),
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
	wmeta workloadmeta.Component,
	ac autodiscovery.Component,
	dc optional.Option[datadogclient.Component],
	secretResolver secrets.Component,
	statusComponent status.Component,
	collector collector.Component,
	rcService optional.Option[rccomp.Component],
	logReceiver optional.Option[integrations.Component],
	_ healthprobe.Component,
	settings settings.Component,
	datadogConfig config.Component,
) error {
	stopCh := make(chan struct{})
	validatingStopCh := make(chan struct{})

	mainCtx, mainCtxCancel := context.WithCancel(context.Background())
	defer mainCtxCancel()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	// Starting Cluster Agent sequence
	// Initialization order is important for multiple reasons, see comments

	if err := util.SetupCoreDump(config); err != nil {
		pkglog.Warnf("Can't setup core dumps: %v, core dumps might not be available after a crash", err)
	}

	// Setup Internal Profiling
	common.SetupInternalProfiling(settings, config, "")

	if !config.IsSet("api_key") {
		return fmt.Errorf("no API key configured, exiting")
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

	// Setup the leader forwarder for language detection and cluster checks
	if config.GetBool("cluster_checks.enabled") || (config.GetBool("language_detection.enabled") && config.GetBool("language_detection.reporting.enabled")) {
		apidca.NewGlobalLeaderForwarder(
			config.GetInt("cluster_agent.cmd_port"),
			config.GetInt("cluster_agent.max_connections"),
		)
	}

	// Starting server early to ease investigations
	if err := api.StartServer(mainCtx, wmeta, taggerComp, ac, statusComponent, settings, config); err != nil {
		return fmt.Errorf("Error while starting agent API, exiting: %v", err)
	}

	// Getting connection to APIServer, it's done before Hostname resolution
	// as hostname resolution may call APIServer
	pkglog.Info("Waiting to obtain APIClient connection")
	apiCl, err := apiserver.WaitForAPIClient(context.Background()) // make sure we can connect to the apiserver
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
	demultiplexer.AddAgentStartupTelemetry(fmt.Sprintf("%s - Datadog Cluster Agent", version.AgentVersion))

	// Create the Leader election engine and initialize it
	leaderelection.CreateGlobalLeaderEngine(mainCtx)
	le, err := leaderelection.GetLeaderEngine()
	if err != nil {
		return err
	}

	// Create event recorder
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(pkglog.Infof)
	eventBroadcaster.StartRecordingToSink(&corev1.EventSinkImpl{Interface: apiCl.Cl.CoreV1().Events("")})
	eventRecorder := eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "datadog-cluster-agent"})

	ctx := controllers.ControllerContext{
		InformerFactory:        apiCl.InformerFactory,
		DynamicClient:          apiCl.DynamicInformerCl,
		DynamicInformerFactory: apiCl.DynamicInformerFactory,
		Client:                 apiCl.InformerCl,
		IsLeaderFunc:           le.IsLeader,
		EventRecorder:          eventRecorder,
		WorkloadMeta:           wmeta,
		StopCh:                 stopCh,
		DatadogClient:          dc,
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
		pkglog.Warn("Failed to auto-detect a Kubernetes cluster name. We recommend you set it manually via the cluster_name config option")
	}
	pkglog.Infof("Cluster ID: %s, Cluster Name: %s", clusterID, clusterName)

	// Initialize and start remote configuration client
	var rcClient *rcclient.Client
	rcserv, isSet := rcService.Get()
	if pkgconfigsetup.IsRemoteConfigEnabled(config) && isSet {
		var products []string
		if config.GetBool("admission_controller.auto_instrumentation.patcher.enabled") {
			products = append(products, state.ProductAPMTracing)
		}
		if config.GetBool("autoscaling.workload.enabled") {
			products = append(products, state.ProductContainerAutoscalingSettings, state.ProductContainerAutoscalingValues)
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
	common.LoadComponents(secretResolver, wmeta, ac, config.GetString("confd_path"))

	// Set up check collector
	registerChecks(wmeta, taggerComp, config)
	ac.AddScheduler("check", pkgcollector.InitCheckScheduler(optional.NewOption(collector), demultiplexer, logReceiver, taggerComp), true)

	// start the autoconfig, this will immediately run any configured check
	ac.LoadAndRun(mainCtx)

	if config.GetBool("cluster_checks.enabled") {
		// Start the cluster check Autodiscovery
		clusterCheckHandler, err := setupClusterCheck(mainCtx, ac)
		if err == nil {
			api.ModifyAPIRouter(func(r *mux.Router) {
				dcav1.InstallChecksEndpoints(r, clusteragent.ServerContext{ClusterCheckHandler: clusterCheckHandler})
			})
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
			return fmt.Errorf("Remote config is disabled or failed to initialize, remote config is a required dependency for autoscaling")
		}

		if !config.GetBool("admission_controller.enabled") {
			log.Error("Admission controller is disabled, vertical autoscaling requires the admission controller to be enabled. Vertical scaling will be disabled.")
		}

		if adapter, err := workload.StartWorkloadAutoscaling(mainCtx, clusterID, apiCl, rcClient, wmeta, demultiplexer); err != nil {
			pkglog.Errorf("Error while starting workload autoscaling: %v", err)
		} else {
			pa = adapter
		}
	}

	// Compliance
	if config.GetBool("compliance_config.enabled") {
		wg.Add(1)
		go func() {
			defer wg.Done()

			if err := runCompliance(mainCtx, demultiplexer, wmeta, apiCl, le.IsLeader); err != nil {
				pkglog.Errorf("Error while running compliance agent: %v", err)
			}
		}()
	}

	if config.GetBool("language_detection.enabled") && config.GetBool("cluster_agent.language_detection.patcher.enabled") {
		if err = languagedetection.Start(mainCtx, wmeta, log, config); err != nil {
			log.Errorf("Cannot start language detection patcher: %v", err)
		}
	}

	if config.GetBool("admission_controller.enabled") {
		if config.GetBool("admission_controller.auto_instrumentation.patcher.enabled") {
			patchCtx := admissionpatch.ControllerContext{
				IsLeaderFunc:        le.IsLeader,
				LeaderSubscribeFunc: le.Subscribe,
				K8sClient:           apiCl.Cl,
				RcClient:            rcClient,
				ClusterName:         clusterName,
				ClusterID:           clusterID,
				StopCh:              stopCh,
			}
			if err := admissionpatch.StartControllers(patchCtx); err != nil {
				log.Errorf("Cannot start auto instrumentation patcher: %v", err)
			}
		} else {
			log.Info("Auto instrumentation patcher is disabled")
		}

		admissionCtx := admissionpkg.ControllerContext{
			IsLeaderFunc:        le.IsLeader,
			LeaderSubscribeFunc: le.Subscribe,
			SecretInformers:     apiCl.CertificateSecretInformerFactory,
			ValidatingInformers: apiCl.WebhookConfigInformerFactory,
			MutatingInformers:   apiCl.WebhookConfigInformerFactory,
			Client:              apiCl.Cl,
			StopCh:              stopCh,
			ValidatingStopCh:    validatingStopCh,
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
			server := admissioncmd.NewServer()

			for _, webhookConf := range webhooks {
				server.Register(webhookConf.Endpoint(), webhookConf.Name(), webhookConf.WebhookType(), webhookConf.WebhookFunc(), apiCl.DynamicCl, apiCl.Cl)
			}

			// Start the k8s admission webhook server
			wg.Add(1)
			go func() {
				defer wg.Done()

				errServ := server.Run(mainCtx, apiCl.Cl)
				if errServ != nil {
					pkglog.Errorf("Error in the Admission Controller Webhook Server: %v", errServ)
				}
			}()
		}
	} else {
		pkglog.Info("Admission controller is disabled")
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

func setupClusterCheck(ctx context.Context, ac autodiscovery.Component) (*clusterchecks.Handler, error) {
	handler, err := clusterchecks.NewHandler(ac)
	if err != nil {
		return nil, err
	}
	go handler.Run(ctx)

	pkglog.Info("Started cluster check Autodiscovery")
	return handler, nil
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
	// Required checks
	corecheckLoader.RegisterCheck(cpu.CheckName, cpu.Factory())
	corecheckLoader.RegisterCheck(memory.CheckName, memory.Factory())
	corecheckLoader.RegisterCheck(uptime.CheckName, uptime.Factory())
	corecheckLoader.RegisterCheck(io.CheckName, io.Factory())
	corecheckLoader.RegisterCheck(filehandles.CheckName, filehandles.Factory())

	// Flavor specific checks
	corecheckLoader.RegisterCheck(kubernetesapiserver.CheckName, kubernetesapiserver.Factory(tagger))
	corecheckLoader.RegisterCheck(ksm.CheckName, ksm.Factory())
	corecheckLoader.RegisterCheck(helm.CheckName, helm.Factory())
	corecheckLoader.RegisterCheck(disk.CheckName, disk.Factory())
	corecheckLoader.RegisterCheck(orchestrator.CheckName, orchestrator.Factory(wlm, cfg, tagger))
	corecheckLoader.RegisterCheck(winproc.CheckName, winproc.Factory())
}
