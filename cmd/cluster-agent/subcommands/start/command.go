// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

// Package start implements 'cluster-agent start'.
package start

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/agent/common/path"
	admissioncmd "github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/cmd/cluster-agent/api"
	dcav1 "github.com/DataDog/datadog-agent/cmd/cluster-agent/api/v1"
	"github.com/DataDog/datadog-agent/cmd/cluster-agent/command"
	"github.com/DataDog/datadog-agent/cmd/cluster-agent/custommetrics"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/forwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/api/healthprobe"
	"github.com/DataDog/datadog-agent/pkg/clusteragent"
	admissionpkg "github.com/DataDog/datadog-agent/pkg/clusteragent/admission"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate"
	admissionpatch "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patch"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks"
	"github.com/DataDog/datadog-agent/pkg/collector"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/remote"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	remoteconfig "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	commonsettings "github.com/DataDog/datadog-agent/pkg/config/settings"
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
)

// Commands returns a slice of subcommands for the 'cluster-agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start the Cluster Agent",
		Long:  `Runs Datadog Cluster agent in the foreground`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO: once the cluster-agent is represented as a component, and
			// not a function (start), this will use `fxutil.Run` instead of
			// `fxutil.OneShot`.
			return fxutil.OneShot(start,
				fx.Supply(globalParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewClusterAgentParams(globalParams.ConfFilePath, config.WithConfigLoadSecrets(true)),
					LogParams:    log.LogForDaemon(command.LoggerName, "log_file", path.DefaultDCALogFile),
				}),
				core.Bundle,
				forwarder.Bundle,
				fx.Provide(func(config config.Component, log log.Component) defaultforwarder.Params {
					params := defaultforwarder.NewParamsWithResolvers(config, log)
					params.Options.DisableAPIKeyChecking = true
					return params
				}),
			)
		},
	}

	return []*cobra.Command{startCmd}
}

func start(log log.Component, config config.Component, telemetry telemetry.Component, forwarder defaultforwarder.Component, cliParams *command.GlobalParams) error {
	stopCh := make(chan struct{})

	mainCtx, mainCtxCancel := context.WithCancel(context.Background())
	defer mainCtxCancel()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	// Starting Cluster Agent sequence
	// Initialization order is important for multiple reasons, see comments

	if err := util.SetupCoreDump(config); err != nil {
		pkglog.Warnf("Can't setup core dumps: %v, core dumps might not be available after a crash", err)
	}

	// Init settings that can be changed at runtime
	if err := initRuntimeSettings(); err != nil {
		pkglog.Warnf("Can't initiliaze the runtime settings: %v", err)
	}

	// Setup Internal Profiling
	common.SetupInternalProfiling(pkgconfig.Datadog, "")

	if !pkgconfig.Datadog.IsSet("api_key") {
		return fmt.Errorf("no API key configured, exiting")
	}

	// Expose the registered metrics via HTTP.
	http.Handle("/metrics", telemetry.Handler())
	metricsPort := pkgconfig.Datadog.GetInt("metrics_port")
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

	// Setup healthcheck port
	var healthPort = pkgconfig.Datadog.GetInt("health_port")
	if healthPort > 0 {
		err := healthprobe.Serve(mainCtx, healthPort)
		if err != nil {
			return fmt.Errorf("Error starting health port, exiting: %v", err)
		}

		pkglog.Debugf("Health check listening on port %d", healthPort)
	}

	// Initialize remote configuration
	var rcClient *remote.Client
	if pkgconfig.IsRemoteConfigEnabled(pkgconfig.Datadog) {
		var err error
		rcClient, err = initializeRemoteConfig(mainCtx)
		if err != nil {
			log.Errorf("Failed to start remote-configuration: %v", err)
		} else {
			rcClient.Start()
			defer rcClient.Close()
		}
	}

	// Starting server early to ease investigations
	if err := api.StartServer(aggregator.GetSenderManager()); err != nil {
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
	opts := aggregator.DefaultAgentDemultiplexerOptions()
	opts.UseEventPlatformForwarder = false
	demux := aggregator.InitAndStartAgentDemultiplexer(log, forwarder, opts, hname)
	demux.AddAgentStartupTelemetry(fmt.Sprintf("%s - Datadog Cluster Agent", version.AgentVersion))

	le, err := leaderelection.GetLeaderEngine()
	if err != nil {
		return err
	}

	// Create event recorder
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(pkglog.Infof)
	eventBroadcaster.StartRecordingToSink(&corev1.EventSinkImpl{Interface: apiCl.Cl.CoreV1().Events("")})
	eventRecorder := eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "datadog-cluster-agent"})

	ctx := apiserver.ControllerContext{
		InformerFactory:    apiCl.InformerFactory,
		WPAClient:          apiCl.WPAClient,
		WPAInformerFactory: apiCl.WPAInformerFactory,
		DDClient:           apiCl.DDClient,
		DDInformerFactory:  apiCl.DynamicInformerFactory,
		Client:             apiCl.Cl,
		IsLeaderFunc:       le.IsLeader,
		EventRecorder:      eventRecorder,
		StopCh:             stopCh,
	}

	if aggErr := apiserver.StartControllers(ctx); aggErr != nil {
		for _, err := range aggErr.Errors() {
			pkglog.Warnf("Error while starting controller: %v", err)
		}
	}

	clusterName := clustername.GetRFC1123CompliantClusterName(context.TODO(), hname)
	// Generate and persist a cluster ID
	// this must be a UUID, and ideally be stable for the lifetime of a cluster,
	// so we store it in a configmap that we try and read before generating a new one.
	coreClient := apiCl.Cl.CoreV1().(*corev1.CoreV1Client)
	clusterId, err := apicommon.GetOrCreateClusterID(coreClient)
	if err != nil {
		pkglog.Errorf("Failed to generate or retrieve the cluster ID")
	}

	if clusterName == "" {
		pkglog.Warn("Failed to auto-detect a Kubernetes cluster name. We recommend you set it manually via the cluster_name config option")
	}

	// FIXME: move LoadComponents and AC.LoadAndRun in their own package so we
	// don't import cmd/agent

	// create and setup the Autoconfig instance
	common.LoadComponents(mainCtx, aggregator.GetSenderManager(), pkgconfig.Datadog.GetString("confd_path"))

	// Set up check collector
	common.AC.AddScheduler("check", collector.InitCheckScheduler(common.Coll, aggregator.GetSenderManager()), true)
	common.Coll.Start()

	// start the autoconfig, this will immediately run any configured check
	common.AC.LoadAndRun(mainCtx)

	if pkgconfig.Datadog.GetBool("cluster_checks.enabled") {
		// Start the cluster check Autodiscovery
		clusterCheckHandler, err := setupClusterCheck(mainCtx)
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
	if pkgconfig.Datadog.GetBool("external_metrics_provider.enabled") {
		// Start the k8s custom metrics server. This is a blocking call
		wg.Add(1)
		go func() {
			defer wg.Done()

			errServ := custommetrics.RunServer(mainCtx, apiCl)
			if errServ != nil {
				pkglog.Errorf("Error in the External Metrics API Server: %v", errServ)
			}
		}()
	}

	// Compliance
	if pkgconfig.Datadog.GetBool("compliance_config.enabled") {
		wg.Add(1)
		go func() {
			defer wg.Done()

			if err := runCompliance(mainCtx, aggregator.GetSenderManager(), apiCl, le.IsLeader); err != nil {
				pkglog.Errorf("Error while running compliance agent: %v", err)
			}
		}()
	}

	if pkgconfig.Datadog.GetBool("admission_controller.enabled") {
		if pkgconfig.Datadog.GetBool("admission_controller.auto_instrumentation.patcher.enabled") {
			patchCtx := admissionpatch.ControllerContext{
				IsLeaderFunc:        le.IsLeader,
				LeaderSubscribeFunc: le.Subscribe,
				K8sClient:           apiCl.Cl,
				RcClient:            rcClient,
				ClusterName:         clusterName,
				ClusterID:           clusterId,
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
			WebhookInformers:    apiCl.WebhookConfigInformerFactory,
			Client:              apiCl.Cl,
			DiscoveryClient:     apiCl.DiscoveryCl,
			StopCh:              stopCh,
		}

		err = admissionpkg.StartControllers(admissionCtx)
		if err != nil {
			pkglog.Errorf("Could not start admission controller: %v", err)
		} else {
			// Webhook and secret controllers are started successfully
			// Setup the k8s admission webhook server
			server := admissioncmd.NewServer()
			server.Register(pkgconfig.Datadog.GetString("admission_controller.inject_config.endpoint"), mutate.InjectConfig, apiCl.DynamicCl)
			server.Register(pkgconfig.Datadog.GetString("admission_controller.inject_tags.endpoint"), mutate.InjectTags, apiCl.DynamicCl)
			server.Register(pkgconfig.Datadog.GetString("admission_controller.auto_instrumentation.endpoint"), mutate.InjectAutoInstrumentation, apiCl.DynamicCl)

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

	demux.Stop(true)
	if err := metricsServer.Shutdown(context.Background()); err != nil {
		pkglog.Errorf("Error shutdowning metrics server on port %d: %v", metricsPort, err)
	}

	pkglog.Info("See ya!")
	pkglog.Flush()

	return nil
}

// initRuntimeSettings builds the map of runtime Cluster Agent settings configurable at runtime.
func initRuntimeSettings() error {
	if err := commonsettings.RegisterRuntimeSetting(commonsettings.NewLogLevelRuntimeSetting()); err != nil {
		return err
	}

	if err := commonsettings.RegisterRuntimeSetting(commonsettings.NewRuntimeMutexProfileFraction()); err != nil {
		return err
	}

	if err := commonsettings.RegisterRuntimeSetting(commonsettings.NewRuntimeBlockProfileRate()); err != nil {
		return err
	}

	if err := commonsettings.RegisterRuntimeSetting(commonsettings.NewProfilingGoroutines()); err != nil {
		return err
	}

	return commonsettings.RegisterRuntimeSetting(commonsettings.NewProfilingRuntimeSetting("internal_profiling", "datadog-cluster-agent"))
}

func setupClusterCheck(ctx context.Context) (*clusterchecks.Handler, error) {
	handler, err := clusterchecks.NewHandler(common.AC)
	if err != nil {
		return nil, err
	}
	go handler.Run(ctx)

	pkglog.Info("Started cluster check Autodiscovery")
	return handler, nil
}

func initializeRemoteConfig(ctx context.Context) (*remote.Client, error) {
	configService, err := remoteconfig.NewService()
	if err != nil {
		return nil, fmt.Errorf("unable to create remote-config service: %w", err)
	}

	if err := configService.Start(ctx); err != nil {
		return nil, fmt.Errorf("unable to start remote-config service: %w", err)
	}

	rcClient, err := remote.NewClient("cluster-agent", configService, version.AgentVersion, []data.Product{data.ProductAPMTracing}, time.Second*5)
	if err != nil {
		return nil, fmt.Errorf("unable to create local remote-config client: %w", err)
	}

	return rcClient, nil
}
