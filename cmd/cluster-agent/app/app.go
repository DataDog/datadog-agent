// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	admissioncmd "github.com/DataDog/datadog-agent/cmd/cluster-agent/admission"
	"github.com/DataDog/datadog-agent/cmd/cluster-agent/api"
	"github.com/DataDog/datadog-agent/cmd/cluster-agent/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/api/healthprobe"
	"github.com/DataDog/datadog-agent/pkg/clusteragent"
	admissionpkg "github.com/DataDog/datadog-agent/pkg/clusteragent/admission"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	apicommon "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// loggerName is the name of the cluster agent logger
const loggerName config.LoggerName = "CLUSTER"

// FIXME: move SetupAutoConfig and StartAutoConfig in their own package so we don't import cmd/agent
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
	// we'll search for a config file named `datadog-cluster.yaml`
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

	mainCtx, mainCtxCancel := context.WithCancel(context.Background())
	defer mainCtxCancel() // Calling cancel twice is safe

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

	if !config.Datadog.IsSet("api_key") {
		log.Critical("no API key configured, exiting")
		return nil
	}

	// Setup healthcheck port
	var healthPort = config.Datadog.GetInt("health_port")
	if healthPort > 0 {
		err := healthprobe.Serve(mainCtx, healthPort)
		if err != nil {
			return log.Errorf("Error starting health port, exiting: %v", err)
		}
		log.Debugf("Health check listening on port %d", healthPort)
	}

	// get hostname
	hostname, err := util.GetHostname()
	if err != nil {
		return log.Errorf("Error while getting hostname, exiting: %v", err)
	}
	log.Infof("Hostname is: %s", hostname)

	// setup the forwarder
	keysPerDomain, err := config.GetMultipleEndpoints()
	if err != nil {
		log.Error("Misconfiguration of agent endpoints: ", err)
	}
	forwarderOpts := forwarder.NewOptions(keysPerDomain)
	// If a cluster-agent looses the connectivity to DataDog, we still want it to remain ready so that its endpoint remains in the service because:
	// * It is still able to serve metrics to the WPA controller and
	// * The metrics reported are reported as stale so that there is no "lie" about the accuracy of the reported metrics.
	// Serving stale data is better than serving no data at all.
	forwarderOpts.DisableAPIKeyChecking = true
	f := forwarder.NewDefaultForwarder(forwarderOpts)
	f.Start() //nolint:errcheck
	s := serializer.NewSerializer(f)

	aggregatorInstance := aggregator.InitAggregator(s, hostname, aggregator.ClusterAgentName)
	aggregatorInstance.AddAgentStartupTelemetry(fmt.Sprintf("%s - Datadog Cluster Agent", version.AgentVersion))

	log.Infof("Datadog Cluster Agent is now running.")

	apiCl, err := apiserver.GetAPIClient() // make sure we can connect to the apiserver
	if err != nil {
		log.Errorf("Could not connect to the apiserver: %v", err)
	} else {
		le, err := leaderelection.GetLeaderEngine()
		if err != nil {
			return err
		}

		// Create event recorder
		eventBroadcaster := record.NewBroadcaster()
		eventBroadcaster.StartLogging(log.Infof)
		eventBroadcaster.StartRecordingToSink(&corev1.EventSinkImpl{Interface: apiCl.Cl.CoreV1().Events("")})
		eventRecorder := eventBroadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: "datadog-cluster-agent"})

		stopCh := make(chan struct{})
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

		// Generate and persist a cluster ID
		// this must be a UUID, and ideally be stable for the lifetime of a cluster
		// so we store it in a configmap that we try and read before generating a new one.
		coreClient := apiCl.Cl.CoreV1().(*corev1.CoreV1Client)
		_, err = apicommon.GetOrCreateClusterID(coreClient)
		if err != nil {
			log.Errorf("Failed to generate or retrieve the cluster ID")
		}

		// TODO: move rest of the controllers out of the apiserver package
		orchestratorCtx := orchestrator.ControllerContext{
			IsLeaderFunc:                 le.IsLeader,
			UnassignedPodInformerFactory: apiCl.UnassignedPodInformerFactory,
			Client:                       apiCl.Cl,
			StopCh:                       stopCh,
			Hostname:                     hostname,
			ClusterName:                  clustername.GetClusterName(),
			ConfigPath:                   confPath,
		}
		err = orchestrator.StartController(orchestratorCtx)
		if err != nil {
			log.Errorf("Could not start orchestrator controller: %v", err)
		}

		if config.Datadog.GetBool("admission_controller.enabled") {
			admissionCtx := admissionpkg.ControllerContext{
				IsLeaderFunc:     le.IsLeader,
				SecretInformers:  apiCl.CertificateSecretInformerFactory,
				WebhookInformers: apiCl.WebhookConfigInformerFactory,
				Client:           apiCl.Cl,
				StopCh:           stopCh,
			}
			err = admissionpkg.StartControllers(admissionCtx)
			if err != nil {
				log.Errorf("Could not start admission controller: %v", err)
			}
		} else {
			log.Info("Admission controller is disabled")
		}
	}

	// Setup a channel to catch OS signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	// create and setup the Autoconfig instance
	common.SetupAutoConfig(config.Datadog.GetString("confd_path"))
	// start the autoconfig, this will immediately run any configured check
	common.StartAutoConfig()

	var clusterCheckHandler *clusterchecks.Handler
	if config.Datadog.GetBool("cluster_checks.enabled") {
		// Start the cluster check Autodiscovery
		clusterCheckHandler, err = setupClusterCheck(mainCtx)
		if err != nil {
			log.Errorf("Error while setting up cluster check Autodiscovery %v", err)
		}
	} else {
		log.Debug("Cluster check Autodiscovery disabled")
	}

	// Start the cmd HTTPS server
	// We always need to start it, even with nil clusterCheckHandler
	// as it's also used to perform the agent commands (e.g. agent status)
	sc := clusteragent.ServerContext{
		ClusterCheckHandler: clusterCheckHandler,
	}
	if err = api.StartServer(sc); err != nil {
		return log.Errorf("Error while starting agent API, exiting: %v", err)
	}

	wg := sync.WaitGroup{}

	// Autoscaler Controller Goroutine
	if config.Datadog.GetBool("external_metrics_provider.enabled") {
		// Start the k8s custom metrics server. This is a blocking call
		wg.Add(1)
		go func() {
			defer wg.Done()

			errServ := custommetrics.RunServer(mainCtx)
			if errServ != nil {
				log.Errorf("Error in the External Metrics API Server: %v", errServ)
			}
		}()
	}

	// Admission Controller Goroutine
	if config.Datadog.GetBool("admission_controller.enabled") {
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
