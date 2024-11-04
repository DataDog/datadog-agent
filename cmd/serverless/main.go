// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(SERV) Fix revive linter
package main

import (
	"context"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/tagger"
	taggernoop "github.com/DataDog/datadog-agent/comp/core/tagger/noopimpl"
	logConfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/serverless"
	"github.com/DataDog/datadog-agent/pkg/serverless/apikey"
	"github.com/DataDog/datadog-agent/pkg/serverless/appsec"
	appsecConfig "github.com/DataDog/datadog-agent/pkg/serverless/appsec/config"
	"github.com/DataDog/datadog-agent/pkg/serverless/appsec/httpsec"
	"github.com/DataDog/datadog-agent/pkg/serverless/daemon"
	"github.com/DataDog/datadog-agent/pkg/serverless/debug"
	"github.com/DataDog/datadog-agent/pkg/serverless/flush"
	"github.com/DataDog/datadog-agent/pkg/serverless/invocationlifecycle"
	serverlessLogs "github.com/DataDog/datadog-agent/pkg/serverless/logs"
	"github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/otlp"
	"github.com/DataDog/datadog-agent/pkg/serverless/proxy"
	"github.com/DataDog/datadog-agent/pkg/serverless/random"
	"github.com/DataDog/datadog-agent/pkg/serverless/registration"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace/inferredspan"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	pkglogsetup "github.com/DataDog/datadog-agent/pkg/util/log/setup"
)

// AWS Lambda is writing the Lambda function files in /var/task, we want the
// configuration file to be at the root of this directory.
var datadogConfigPath = "/var/task/datadog.yaml"

const (
	loggerName                   pkglogsetup.LoggerName = "DD_EXTENSION"
	logLevelEnvVar                                      = "DD_LOG_LEVEL"
	flushStrategyEnvVar                                 = "DD_SERVERLESS_FLUSH_STRATEGY"
	logsLogsTypeSubscribed                              = "DD_LOGS_CONFIG_LAMBDA_LOGS_TYPE"
	extensionRegistrationRoute                          = "/2020-01-01/extension/register"
	extensionRegistrationTimeout                        = 5 * time.Second

	// httpServerAddr will be the default addr used to run the HTTP server listening
	// to calls from the client libraries and to logs from the AWS environment.
	httpServerAddr = ":8124"

	logsAPIRegistrationRoute   = "/2022-07-01/telemetry"
	logsAPIRegistrationTimeout = 5 * time.Second
	logsAPIHttpServerPort      = 8124
	logsAPICollectionRoute     = "/lambda/logs"
	logsAPITimeout             = 25
	logsAPIMaxBytes            = 262144
	logsAPIMaxItems            = 1000
)

func main() {
	// run the agent
	err := fxutil.OneShot(
		runAgent,
		taggernoop.Module(),
	)

	if err != nil {
		log.Error(err)
		os.Exit(-1)
	}
}

func runAgent(tagger tagger.Component) {
	startTime := time.Now()

	setupLambdaAgentOverrides()
	setupLogger()
	debug.OutputDatadogEnvVariablesForDebugging()
	loadConfig()
	if !setupApiKey() {
		return
	}
	serverlessDaemon := startCommunicationServer(startTime)

	// serverless parts
	// ----------------

	serverlessID, done := registerExtension(serverlessDaemon)
	if !done {
		return
	}

	configureAdaptiveFlush(serverlessDaemon)

	logChannel := make(chan *logConfig.ChannelMessage)
	// Channels for ColdStartCreator
	lambdaSpanChan := make(chan *pb.Span)
	lambdaInitMetricChan := make(chan *serverlessLogs.LambdaInitMetric)
	//nolint:revive // TODO(SERV) Fix revive linter
	coldStartSpanId := random.Random.Uint64()
	metricAgent := startMetricAgent(serverlessDaemon, logChannel, lambdaInitMetricChan, tagger)

	// Concurrently start heavyweight features
	var wg sync.WaitGroup
	wg.Add(3)

	go startTraceAgent(&wg, lambdaSpanChan, coldStartSpanId, serverlessDaemon, tagger)
	go startOtlpAgent(&wg, metricAgent, serverlessDaemon)
	go startTelemetryCollection(&wg, serverlessID, logChannel, serverlessDaemon, tagger)

	// start appsec
	appsecProxyProcessor := startAppSec(serverlessDaemon)

	wg.Wait()

	startColdStartSpanCreator(lambdaSpanChan, lambdaInitMetricChan, serverlessDaemon, coldStartSpanId)

	ta := serverlessDaemon.TraceAgent
	if ta == nil {
		log.Error("Unexpected nil instance of the trace-agent")
		return
	}

	// set up invocation processor in the serverless Daemon to be used for the proxy and/or lifecycle API
	serverlessDaemon.InvocationProcessor = &invocationlifecycle.LifecycleProcessor{
		ExtraTags:            serverlessDaemon.ExtraTags,
		Demux:                serverlessDaemon.MetricAgent.Demux,
		ProcessTrace:         ta.Process,
		DetectLambdaLibrary:  serverlessDaemon.IsLambdaLibraryDetected,
		InferredSpansEnabled: inferredspan.IsInferredSpansEnabled(),
	}

	setupProxy(appsecProxyProcessor, ta, serverlessDaemon)

	serverlessDaemon.ComputeGlobalTags(configUtils.GetConfiguredTags(pkgconfigsetup.Datadog(), true))

	stopCh := startInvocationLoop(serverlessDaemon, serverlessID)

	// this log line is used for performance checks during CI
	// please be careful before modifying/removing it
	log.Debugf("serverless agent ready in %v", time.Since(startTime))

	// block here until we receive a stop signal
	<-stopCh
	//nolint:gosimple // TODO(SERV) Fix gosimple linter
}

func startInvocationLoop(serverlessDaemon *daemon.Daemon, serverlessID registration.ID) chan struct{} {
	// run the invocation loop in a routine
	// we don't want to start this mainloop before because once we're waiting on
	// the invocation route, we can't report init errors anymore.
	stopCh := make(chan struct{})
	go func() {
		for {
			if err := serverless.WaitForNextInvocation(stopCh, serverlessDaemon, serverlessID); err != nil {
				log.Error(err)
			}
		}
	}()
	go handleTerminationSignals(serverlessDaemon, stopCh, signal.Notify)

	return stopCh
}

func setupProxy(appsecProxyProcessor *httpsec.ProxyLifecycleProcessor, ta trace.ServerlessTraceAgent, serverlessDaemon *daemon.Daemon) {
	if appsecProxyProcessor != nil {
		// AppSec runs as a Runtime API proxy. The reverse proxy was already
		// started by appsec.New(). A span modifier needs to be added in order
		// to detect the finished request spans and run the complete AppSec
		// monitoring logic, and ultimately adding the AppSec events to them.
		ta.SetSpanModifier(appsecProxyProcessor.WrapSpanModifier(serverlessDaemon.ExecutionContext, ta.GetSpanModifier()))
		// Set the default rate limiting to approach 1 trace/min in live circumstances to limit non ASM related traces as much as possible.
		// This limit is decided in the Standalone ASM Billing RFC and ensures reducing non ASM-related trace throughput
		// while keeping billing and service catalog running correctly.
		// In case of ASM event, the trace priority will be set to manual keep
		if appsecConfig.IsStandalone() {
			ta.SetTargetTPS(1. / 120)
		}
	}
	if enabled, _ := strconv.ParseBool(os.Getenv("DD_EXPERIMENTAL_ENABLE_PROXY")); enabled {
		if appsecProxyProcessor != nil {
			log.Debugf("AppSec is enabled, the experimental proxy will not be started")
		} else {
			// start the experimental proxy if enabled
			log.Debug("Starting the experimental runtime api proxy")
			proxy.Start(
				"127.0.0.1:9000",
				"127.0.0.1:9001",
				serverlessDaemon.InvocationProcessor,
			)
		}
	}
}

func startMetricAgent(serverlessDaemon *daemon.Daemon, logChannel chan *logConfig.ChannelMessage, lambdaInitMetricChan chan *serverlessLogs.LambdaInitMetric, tagger tagger.Component) *metrics.ServerlessMetricAgent {
	metricAgent := &metrics.ServerlessMetricAgent{
		SketchesBucketOffset: time.Second * 10,
		Tagger:               tagger,
	}
	metricAgent.Start(daemon.FlushTimeout, &metrics.MetricConfig{}, &metrics.MetricDogStatsD{})
	serverlessDaemon.SetStatsdServer(metricAgent)
	serverlessDaemon.SetupLogCollectionHandler(logsAPICollectionRoute, logChannel, pkgconfigsetup.Datadog().GetBool("serverless.logs_enabled"), pkgconfigsetup.Datadog().GetBool("enhanced_metrics"), lambdaInitMetricChan)
	return metricAgent
}

func configureAdaptiveFlush(serverlessDaemon *daemon.Daemon) {
	if v, exists := os.LookupEnv(flushStrategyEnvVar); exists {
		if flushStrategy, err := flush.StrategyFromString(v); err != nil {
			log.Debugf("Invalid flush strategy %s, will use adaptive flush instead. Err: %s", v, err)
		} else {
			serverlessDaemon.UseAdaptiveFlush(false) // we're forcing the flush strategy, we won't be using the adaptive flush
			serverlessDaemon.SetFlushStrategy(flushStrategy)
		}
	} else {
		serverlessDaemon.UseAdaptiveFlush(true) // already initialized to true, but let's be explicit just in case
	}
}

func registerExtension(serverlessDaemon *daemon.Daemon) (registration.ID, bool) {
	serverlessID, functionArn, err := registration.RegisterExtension(extensionRegistrationRoute, extensionRegistrationTimeout)
	if err != nil {
		// at this point, we were not even able to register, thus, we don't have
		// any ID assigned, thus, we can't report an error to the init error route
		// which needs an Id.
		log.Errorf("Can't register as a serverless agent: %s", err)
		return "", false
	}
	if len(functionArn) > 0 {
		serverlessDaemon.ExecutionContext.SetArnFromExtensionResponse(string(functionArn))
	}
	return serverlessID, true
}

func startCommunicationServer(startTime time.Time) *daemon.Daemon {
	serverlessDaemon := daemon.StartDaemon(httpServerAddr)
	serverlessDaemon.ExecutionContext.SetInitializationTime(startTime)
	err := serverlessDaemon.ExecutionContext.RestoreCurrentStateFromFile()
	if err != nil {
		log.Debug("Unable to restore the state from file %s", err)
	} else {
		serverlessDaemon.StartLogCollection()
	}
	return serverlessDaemon
}

func setupLambdaAgentOverrides() {
	flavor.SetFlavor(flavor.ServerlessAgent)
	pkgconfigsetup.Datadog().Set("use_v2_api.series", false, model.SourceAgentRuntime)

	// TODO(duncanista): figure out how this is used and if it's necessary for Serverless
	pkgconfigsetup.Datadog().Set("dogstatsd_socket", "", model.SourceAgentRuntime)

	// Disable remote configuration for now as it just spams the debug logs
	// and provides no value.
	os.Setenv("DD_REMOTE_CONFIGURATION_ENABLED", "false")
}

func startColdStartSpanCreator(lambdaSpanChan chan *pb.Span, lambdaInitMetricChan chan *serverlessLogs.LambdaInitMetric, serverlessDaemon *daemon.Daemon, coldStartSpanId uint64) {
	coldStartSpanCreator := &trace.ColdStartSpanCreator{
		LambdaSpanChan:       lambdaSpanChan,
		LambdaInitMetricChan: lambdaInitMetricChan,
		TraceAgent:           serverlessDaemon.TraceAgent,
		StopChan:             make(chan struct{}),
		ColdStartSpanId:      coldStartSpanId,
	}

	log.Debug("Starting ColdStartSpanCreator")
	coldStartSpanCreator.Run()
	log.Debug("Setting ColdStartSpanCreator on Daemon")
	serverlessDaemon.SetColdStartSpanCreator(coldStartSpanCreator)
}

func startAppSec(serverlessDaemon *daemon.Daemon) *httpsec.ProxyLifecycleProcessor {
	appsecProxyProcessor, err := appsec.New(serverlessDaemon.MetricAgent.Demux)
	if err != nil {
		log.Error("appsec: could not start: ", err)
	}
	return appsecProxyProcessor
}

func startTelemetryCollection(wg *sync.WaitGroup, serverlessID registration.ID, logChannel chan *logConfig.ChannelMessage, serverlessDaemon *daemon.Daemon, tagger tagger.Component) {
	defer wg.Done()
	if os.Getenv(daemon.LocalTestEnvVar) == "true" || os.Getenv(daemon.LocalTestEnvVar) == "1" {
		log.Debug("Running in local test mode. Telemetry collection HTTP route won't be enabled")
		return
	}
	log.Debug("Enabling telemetry collection HTTP route")
	logRegistrationURL := registration.BuildURL(logsAPIRegistrationRoute)
	logRegistrationError := registration.EnableTelemetryCollection(
		registration.EnableTelemetryCollectionArgs{
			ID:                  serverlessID,
			RegistrationURL:     logRegistrationURL,
			RegistrationTimeout: logsAPIRegistrationTimeout,
			LogsType:            os.Getenv(logsLogsTypeSubscribed),
			Port:                logsAPIHttpServerPort,
			CollectionRoute:     logsAPICollectionRoute,
			Timeout:             logsAPITimeout,
			MaxBytes:            logsAPIMaxBytes,
			MaxItems:            logsAPIMaxItems,
		})

	if logRegistrationError != nil {
		log.Error("Can't subscribe to logs:", logRegistrationError)
	} else {
		logsAgent, err := serverlessLogs.SetupLogAgent(logChannel, "AWS Logs", "lambda", tagger)
		if err != nil {
			log.Errorf("Error setting up the logs agent: %s", err)
		}
		serverlessDaemon.SetLogsAgent(logsAgent)
	}
}

func startOtlpAgent(wg *sync.WaitGroup, metricAgent *metrics.ServerlessMetricAgent, serverlessDaemon *daemon.Daemon) {
	defer wg.Done()
	if !otlp.IsEnabled() {
		log.Debug("otlp endpoint disabled")
		return
	}
	otlpAgent := otlp.NewServerlessOTLPAgent(metricAgent.Demux.Serializer())
	otlpAgent.Start()
	serverlessDaemon.SetOTLPAgent(otlpAgent)

}

func startTraceAgent(wg *sync.WaitGroup, lambdaSpanChan chan *pb.Span, coldStartSpanId uint64, serverlessDaemon *daemon.Daemon, tagger tagger.Component) {
	defer wg.Done()
	traceAgent := trace.StartServerlessTraceAgent(trace.StartServerlessTraceAgentArgs{
		Enabled:         pkgconfigsetup.Datadog().GetBool("apm_config.enabled"),
		LoadConfig:      &trace.LoadConfig{Path: datadogConfigPath, Tagger: tagger},
		LambdaSpanChan:  lambdaSpanChan,
		ColdStartSpanID: coldStartSpanId,
	})
	serverlessDaemon.SetTraceAgent(traceAgent)
}

func setupApiKey() bool {
	if err := apikey.HandleEnv(); err != nil {
		log.Errorf("Can't start the Datadog extension as no API Key has been detected, or API Key could not be decrypted. Data will not be sent to Datadog.")
		ctx := context.Background()

		_, shutdownAppSec, err := appsec.NewWithShutdown(nil)
		if err != nil {
			log.Errorf("Can't start Lambda Runtime API Proxy for AppSec: %v", err)
		}
		if shutdownAppSec != nil {
			defer func() {
				if err := shutdownAppSec(ctx); err != nil {
					log.Warnf("Failed to shut down AppSec proxy: %v", err)
				}
			}()
		}

		// we still need to register the extension but let's return after (no-op)
		id, _, registrationError := registration.RegisterExtension(extensionRegistrationRoute, extensionRegistrationTimeout)
		if registrationError != nil {
			log.Errorf("Can't register as a serverless agent: %s", registrationError)
		}

		processError := registration.NoOpProcessEvent(ctx, id)
		if processError != nil {
			log.Errorf("Can't process events: %s", processError)
		}
		return false
	}
	return true
}

func loadConfig() {
	pkgconfigsetup.Datadog().SetConfigFile(datadogConfigPath)
	// Load datadog.yaml file into the config, so that metricAgent can pick these configurations
	if _, err := pkgconfigsetup.LoadWithoutSecret(pkgconfigsetup.Datadog(), nil); err != nil {
		log.Errorf("Error happened when loading configuration from datadog.yaml for metric agent: %s", err)
	}
}

// handleTerminationSignals handles OS termination signals.
// If a specified signal is received the serverless agent stops.
func handleTerminationSignals(serverlessDaemon *daemon.Daemon, stopCh chan struct{}, notify func(c chan<- os.Signal, sig ...os.Signal)) {
	signalCh := make(chan os.Signal, 1)
	notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	signo := <-signalCh
	log.Infof("Received signal '%s', shutting down...", signo)
	serverlessDaemon.Stop()
	stopCh <- struct{}{}
}

func setupLogger() {
	logLevel := "error"
	if userLogLevel := os.Getenv(logLevelEnvVar); len(userLogLevel) > 0 {
		if seelogLogLevel, err := log.ValidateLogLevel(userLogLevel); err == nil {
			logLevel = seelogLogLevel
		} else {
			log.Errorf("Invalid log level '%s', using default log level '%s'", userLogLevel, logLevel)
		}
	}

	// init the logger configuring it to not log in a file (the first empty string)
	if err := pkglogsetup.SetupLogger(
		loggerName,
		logLevel,
		"",    // logFile -> by setting this to an empty string, we don't write the logs to any file
		"",    // syslog URI
		false, // syslog_rfc
		true,  // log_to_console
		false, // log_format_json
		pkgconfigsetup.Datadog(),
	); err != nil {
		log.Errorf("Unable to setup logger: %s", err)
	}
}
