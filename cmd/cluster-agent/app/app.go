// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver
// +build kubeapiserver

package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/fatih/color"
	"github.com/gorilla/mux"
	"github.com/spf13/cobra"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	admissioncmd "github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/cmd/cluster-agent/api"
	dcav1 "github.com/DataDog/datadog-agent/cmd/cluster-agent/api/v1"
	"github.com/DataDog/datadog-agent/cmd/cluster-agent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/api/healthprobe"
	"github.com/DataDog/datadog-agent/pkg/clusteragent"
	admissionpkg "github.com/DataDog/datadog-agent/pkg/clusteragent/admission"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/resolver"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	apicommon "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// loggerName is the name of the cluster agent logger
const loggerName config.LoggerName = "CLUSTER"

// FIXME: move LoadComponents and AC.LoadAndRun in their own package so we don't import cmd/agent
var (
	ClusterAgentCmd = &cobra.Command{
		Use:   "datadog-cluster-agent [command]",
		Short: "Datadog Cluster Agent at your service.",
		Long: `
Datadog Cluster Agent takes care of running checks that need run only once per cluster.
It also exposes an API for other Datadog agents that provides them with cluster-level
metadata for their metrics.`,
	}

	startCmd = &cobra.Command{
		Use:   "start",
		Short: "Start the Cluster Agent",
		Long:  `Runs Datadog Cluster agent in the foreground`,
		RunE:  start,
	}

	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print the version info",
		Long:  ``,
		Run: func(cmd *cobra.Command, args []string) {
			if flagNoColor {
				color.NoColor = true
			}
			av, _ := version.Agent()
			meta := ""
			if av.Meta != "" {
				meta = fmt.Sprintf("- Meta: %s ", color.YellowString(av.Meta))
			}
			fmt.Fprintln(
				color.Output,
				fmt.Sprintf("Cluster agent %s %s- Commit: '%s' - Serialization version: %s",
					color.BlueString(av.GetNumberAndPre()),
					meta,
					color.GreenString(version.Commit),
					color.MagentaString(serializer.AgentPayloadVersion),
				),
			)
		},
	}

	confPath    string
	flagNoColor bool
	stopCh      chan struct{}
)

func init() {
	// attach the command to the root
	ClusterAgentCmd.AddCommand(startCmd)
	ClusterAgentCmd.AddCommand(versionCmd)

	ClusterAgentCmd.PersistentFlags().StringVarP(&confPath, "cfgpath", "c", "", "path to directory containing datadog.yaml")
	ClusterAgentCmd.PersistentFlags().BoolVarP(&flagNoColor, "no-color", "n", false, "disable color output")
}

func start(cmd *cobra.Command, args []string) error {
	// Starting Cluster Agent sequence
	// Initialization order is important for multiple reasons, see comments

	// Reading configuration as mostly everything can depend on config variables
	config.Datadog.SetConfigName("datadog-cluster")
	err := common.SetupConfig(confPath)
	if err != nil {
		return fmt.Errorf("unable to set up global agent configuration: %v", err)
	}

	// Setup logger
	syslogURI := config.GetSyslogURI()
	logFile := config.Datadog.GetString("log_file")
	if logFile == "" {
		logFile = common.DefaultDCALogFile
	}
	if config.Datadog.GetBool("disable_file_logging") {
		// this will prevent any logging on file
		logFile = ""
	}

	err = config.SetupLogger(
		loggerName,
		config.Datadog.GetString("log_level"),
		logFile,
		syslogURI,
		config.Datadog.GetBool("syslog_rfc"),
		config.Datadog.GetBool("log_to_console"),
		config.Datadog.GetBool("log_format_json"),
	)
	if err != nil {
		log.Criticalf("Unable to setup logger: %s", err)
		return nil
	}

	if err := util.SetupCoreDump(); err != nil {
		log.Warnf("Can't setup core dumps: %v, core dumps might not be available after a crash", err)
	}

	// Init settings that can be changed at runtime
	if err := initRuntimeSettings(); err != nil {
		log.Warnf("Can't initiliaze the runtime settings: %v", err)
	}

	// Setup Internal Profiling
	common.SetupInternalProfiling()

	if !config.Datadog.IsSet("api_key") {
		log.Critical("no API key configured, exiting")
		return nil
	}

	mainCtx, mainCtxCancel := context.WithCancel(context.Background())
	defer mainCtxCancel() // Calling cancel twice is safe

	// Expose the registered metrics via HTTP.
	http.Handle("/metrics", telemetry.Handler())
	metricsPort := config.Datadog.GetInt("metrics_port")
	metricsServer := &http.Server{
		Addr:    fmt.Sprintf("0.0.0.0:%d", metricsPort),
		Handler: http.DefaultServeMux,
	}
	go func() {
		err := metricsServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.Errorf("Error creating expvar server on port %v: %v", metricsPort, err)
		}
	}()

	// Setup healthcheck port
	var healthPort = config.Datadog.GetInt("health_port")
	if healthPort > 0 {
		err := healthprobe.Serve(mainCtx, healthPort)
		if err != nil {
			return log.Errorf("Error starting health port, exiting: %v", err)
		}
		log.Debugf("Health check listening on port %d", healthPort)
	}

	// Starting server early to ease investigations
	if err = api.StartServer(); err != nil {
		return log.Errorf("Error while starting agent API, exiting: %v", err)
	}

	// Getting connection to APIServer, it's done before Hostname resolution
	// as hostname resolution may call APIServer
	log.Info("Waiting to obtain APIClient connection")
	apiCl, err := apiserver.WaitForAPIClient(context.Background()) // make sure we can connect to the apiserver
	if err != nil {
		return log.Errorf("Fatal error: Cannot connect to the apiserver: %v", err)
	}
	log.Infof("Got APIClient connection")

	// Get hostname as aggregator requires hostname
	hname, err := hostname.Get(context.TODO())
	if err != nil {
		return log.Errorf("Error while getting hostname, exiting: %v", err)
	}
	log.Infof("Hostname is: %s", hname)

	// setup the forwarder
	keysPerDomain, err := config.GetMultipleEndpoints()
	if err != nil {
		log.Error("Misconfiguration of agent endpoints: ", err)
	}

	// If a cluster-agent looses the connectivity to DataDog, we still want it to remain ready so that its endpoint remains in the service because:
	// * It is still able to serve metrics to the WPA controller and
	// * The metrics reported are reported as stale so that there is no "lie" about the accuracy of the reported metrics.
	// Serving stale data is better than serving no data at all.
	forwarderOpts := forwarder.NewOptionsWithResolvers(resolver.NewSingleDomainResolvers(keysPerDomain))
	forwarderOpts.DisableAPIKeyChecking = true
	opts := aggregator.DefaultDemultiplexerOptions(forwarderOpts)
	opts.UseEventPlatformForwarder = false
	opts.UseContainerLifecycleForwarder = false
	demux := aggregator.InitAndStartAgentDemultiplexer(opts, hname)
	demux.AddAgentStartupTelemetry(fmt.Sprintf("%s - Datadog Cluster Agent", version.AgentVersion))

	le, err := leaderelection.GetLeaderEngine()
	if err != nil {
		return err
	}

	// Create event recorder
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(log.Infof)
	eventBroadcaster.StartRecordingToSink(&corev1.EventSinkImpl{Interface: apiCl.Cl.CoreV1().Events("")})
	eventRecorder := eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "datadog-cluster-agent"})

	ctx := apiserver.ControllerContext{
		InformerFactory:    apiCl.InformerFactory,
		WPAClient:          apiCl.WPAClient,
		WPAInformerFactory: apiCl.WPAInformerFactory,
		DDClient:           apiCl.DDClient,
		DDInformerFactory:  apiCl.DDInformerFactory,
		Client:             apiCl.Cl,
		IsLeaderFunc:       le.IsLeader,
		EventRecorder:      eventRecorder,
		StopCh:             stopCh,
	}

	if aggErr := apiserver.StartControllers(ctx); aggErr != nil {
		for _, err := range aggErr.Errors() {
			log.Warnf("Error while starting controller: %v", err)
		}
	}

	if config.Datadog.GetBool("orchestrator_explorer.enabled") {
		// Generate and persist a cluster ID
		// this must be a UUID, and ideally be stable for the lifetime of a cluster
		// so we store it in a configmap that we try and read before generating a new one.
		coreClient := apiCl.Cl.CoreV1().(*corev1.CoreV1Client)
		_, err = apicommon.GetOrCreateClusterID(coreClient)
		if err != nil {
			log.Errorf("Failed to generate or retrieve the cluster ID")
		}

		clusterName := clustername.GetClusterName(context.TODO(), hname)
		if clusterName == "" {
			log.Warn("Failed to auto-detect a Kubernetes cluster name. We recommend you set it manually via the cluster_name config option")
		}
	} else {
		log.Info("Orchestrator explorer is disabled")
	}

	// Setup a channel to catch OS signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	// create and setup the Autoconfig instance
	common.LoadComponents(mainCtx, config.Datadog.GetString("confd_path"))
	// start the autoconfig, this will immediately run any configured check
	common.AC.LoadAndRun()

	if config.Datadog.GetBool("cluster_checks.enabled") {
		// Start the cluster check Autodiscovery
		clusterCheckHandler, err := setupClusterCheck(mainCtx)
		if err == nil {
			api.ModifyAPIRouter(func(r *mux.Router) {
				dcav1.InstallChecksEndpoints(r, clusteragent.ServerContext{ClusterCheckHandler: clusterCheckHandler})
			})
		} else {
			log.Errorf("Error while setting up cluster check Autodiscovery, CLC API endpoints won't be available, err: %v", err)
		}
	} else {
		log.Debug("Cluster check Autodiscovery disabled")
	}

	wg := sync.WaitGroup{}
	// Autoscaler Controller Goroutine
	if config.Datadog.GetBool("external_metrics_provider.enabled") {
		// Start the k8s custom metrics server. This is a blocking call
		wg.Add(1)
		go func() {
			defer wg.Done()

			errServ := custommetrics.RunServer(mainCtx, apiCl)
			if errServ != nil {
				log.Errorf("Error in the External Metrics API Server: %v", errServ)
			}
		}()
	}

	// Compliance
	if config.Datadog.GetBool("compliance_config.enabled") {
		wg.Add(1)
		go func() {
			defer wg.Done()

			if err := runCompliance(mainCtx, apiCl, le.IsLeader); err != nil {
				log.Errorf("Error while running compliance agent: %v", err)
			}
		}()
	}

	if config.Datadog.GetBool("admission_controller.enabled") {
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
			log.Errorf("Could not start admission controller: %v", err)
		} else {
			// Webhook and secret controllers are started successfully
			// Setup the the k8s admission webhook server
			server := admissioncmd.NewServer()
			server.Register(config.Datadog.GetString("admission_controller.inject_config.endpoint"), mutate.InjectConfig, apiCl.DynamicCl)
			server.Register(config.Datadog.GetString("admission_controller.inject_tags.endpoint"), mutate.InjectTags, apiCl.DynamicCl)

			// Start the k8s admission webhook server
			wg.Add(1)
			go func() {
				defer wg.Done()

				errServ := server.Run(mainCtx, apiCl.Cl)
				if errServ != nil {
					log.Errorf("Error in the Admission Controller Webhook Server: %v", errServ)
				}
			}()
		}
	} else {
		log.Info("Admission controller is disabled")
	}

	log.Infof("All components started. Cluster Agent now running.")

	// Block here until we receive the interrupt signal
	<-signalCh

	// retrieve the agent health before stopping the components
	// GetReadyNonBlocking has a 100ms timeout to avoid blocking
	health, err := health.GetReadyNonBlocking()
	if err != nil {
		log.Warnf("Cluster Agent health unknown: %s", err)
	} else if len(health.Unhealthy) > 0 {
		log.Warnf("Some components were unhealthy: %v", health.Unhealthy)
	}

	// Cancel the main context to stop components
	mainCtxCancel()

	// wait for the External Metrics Server and
	// the Admission Webhook Server to stop properly
	wg.Wait()

	if stopCh != nil {
		close(stopCh)
	}

	demux.Stop(true)
	if err := metricsServer.Shutdown(context.Background()); err != nil {
		log.Errorf("Error shutdowning metrics server on port %d: %v", metricsPort, err)
	}

	log.Info("See ya!")
	log.Flush()
	return nil
}

func setupClusterCheck(ctx context.Context) (*clusterchecks.Handler, error) {
	handler, err := clusterchecks.NewHandler(common.AC)
	if err != nil {
		return nil, err
	}
	go handler.Run(ctx)

	log.Info("Started cluster check Autodiscovery")
	return handler, nil
}
