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

	logConfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
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
)

var (
	logLevelEnvVar         = "DD_LOG_LEVEL"
	flushStrategyEnvVar    = "DD_SERVERLESS_FLUSH_STRATEGY"
	logsLogsTypeSubscribed = "DD_LOGS_CONFIG_LAMBDA_LOGS_TYPE"

	// AWS Lambda is writing the Lambda function files in /var/task, we want the
	// configuration file to be at the root of this directory.
	datadogConfigPath = "/var/task/datadog.yaml"
)

const (
	loggerName config.LoggerName = "DD_EXTENSION"

	extensionRegistrationRoute   = "/2020-01-01/extension/register"
	extensionRegistrationTimeout = 5 * time.Second

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
	err := fxutil.OneShot(runAgent)

	if err != nil {
		log.Error(err)
		os.Exit(-1)
	}
}

func runAgent() {
	var err error

	startTime := time.Now()

	stopCh := make(chan struct{})

	flavor.SetFlavor(flavor.ServerlessAgent)
	config.Datadog.Set("use_v2_api.series", false, model.SourceAgentRuntime)

	// Disable remote configuration for now as it just spams the debug logs
	// and provides no value.
	os.Setenv("DD_REMOTE_CONFIGURATION_ENABLED", "false")

	// setup logger
	// -----------

	// init the logger configuring it to not log in a file (the first empty string)
	if err = config.SetupLogger(
		loggerName,
		"error", // will be re-set later with the value from the env var
		"",      // logFile -> by setting this to an empty string, we don't write the logs to any file
		"",      // syslog URI
		false,   // syslog_rfc
		true,    // log_to_console
		false,   // log_format_json
	); err != nil {
		log.Errorf("Unable to setup logger: %s", err)
	}

	if logLevel := os.Getenv(logLevelEnvVar); len(logLevel) > 0 {
		if err := config.ChangeLogLevel(logLevel); err != nil {
			log.Errorf("While changing the loglevel: %s", err)
		}
	}

	debug.OutputDatadogEnvVariablesForDebugging()

	if !apikey.HasAPIKey() {
		log.Errorf("Can't start the Datadog extension as no API Key has been detected, or API Key could not be decrypted. Data will not be sent to Datadog.")
		// we still need to register the extension but let's return after (no-op)
		id, _, registrationError := registration.RegisterExtension(extensionRegistrationRoute, extensionRegistrationTimeout)
		if registrationError != nil {
			log.Errorf("Can't register as a serverless agent: %s", registrationError)
		}
		ctx := context.Background()
		processError := registration.NoOpProcessEvent(ctx, id)
		if processError != nil {
			log.Errorf("Can't process events: %s", processError)
		}
		return
	}

	// immediately starts the communication server
	serverlessDaemon := daemon.StartDaemon(httpServerAddr)
	serverlessDaemon.ExecutionContext.SetInitializationTime(startTime)
	err = serverlessDaemon.ExecutionContext.RestoreCurrentStateFromFile()
	if err != nil {
		log.Debug("Unable to restore the state from file")
	} else {
		serverlessDaemon.StartLogCollection()
	}
	// serverless parts
	// ----------------

	// extension registration
	serverlessID, functionArn, err := registration.RegisterExtension(extensionRegistrationRoute, extensionRegistrationTimeout)
	if err != nil {
		// at this point, we were not even able to register, thus, we don't have
		// any ID assigned, thus, we can't report an error to the init error route
		// which needs an Id.
		log.Errorf("Can't register as a serverless agent: %s", err)
		return
	}
	if len(functionArn) > 0 {
		serverlessDaemon.ExecutionContext.SetArnFromExtensionResponse(string(functionArn))
	}

	// adaptive flush configuration
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

	config.Datadog.SetConfigFile(datadogConfigPath)
	// Load datadog.yaml file into the config, so that metricAgent can pick these configurations
	if _, err := config.LoadWithoutSecret(); err != nil {
		log.Errorf("Error happened when loading configuration from datadog.yaml for metric agent: %s", err)
	}

	apikey.HandleEnv()

	logChannel := make(chan *logConfig.ChannelMessage)
	// Channels for ColdStartCreator
	lambdaSpanChan := make(chan *pb.Span)
	lambdaInitMetricChan := make(chan *serverlessLogs.LambdaInitMetric)
	//nolint:revive // TODO(SERV) Fix revive linter
	coldStartSpanId := random.Random.Uint64()
	metricAgent := &metrics.ServerlessMetricAgent{
		SketchesBucketOffset: time.Second * 10,
	}
	metricAgent.Start(daemon.FlushTimeout, &metrics.MetricConfig{}, &metrics.MetricDogStatsD{})
	serverlessDaemon.SetStatsdServer(metricAgent)
	serverlessDaemon.SetupLogCollectionHandler(logsAPICollectionRoute, logChannel, config.Datadog.GetBool("serverless.logs_enabled"), config.Datadog.GetBool("enhanced_metrics"), lambdaInitMetricChan)

	// Concurrently start heavyweight features
	var wg sync.WaitGroup

	// starts trace agent
	wg.Add(1)
	go func() {
		defer wg.Done()
		traceAgent := &trace.ServerlessTraceAgent{}
		traceAgent.Start(config.Datadog.GetBool("apm_config.enabled"), &trace.LoadConfig{Path: datadogConfigPath}, lambdaSpanChan, coldStartSpanId)
		serverlessDaemon.SetTraceAgent(traceAgent)
	}()

	// starts otlp agent
	wg.Add(1)
	go func() {
		defer wg.Done()
		if !otlp.IsEnabled() {
			log.Debug("otlp endpoint disabled")
			return
		}
		otlpAgent := otlp.NewServerlessOTLPAgent(metricAgent.Demux.Serializer())
		otlpAgent.Start()
		serverlessDaemon.SetOTLPAgent(otlpAgent)
	}()

	// enable telemetry collection
	wg.Add(1)
	go func() {
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
			logsAgent, err := serverlessLogs.SetupLogAgent(logChannel, "AWS Logs", "lambda")
			if err != nil {
				log.Errorf("Error setting up the logs agent: %s", err)
			}
			serverlessDaemon.SetLogsAgent(logsAgent)
		}
	}()

	// start appsec
	var appsecProxyProcessor *httpsec.ProxyLifecycleProcessor
	wg.Add(1)
	go func() {
		defer wg.Done()
		var err error
		appsecProxyProcessor, err = appsec.New()
		if err != nil {
			log.Error("appsec: could not start: ", err)
		}
	}()

	wg.Wait()

	coldStartSpanCreator := &trace.ColdStartSpanCreator{
		LambdaSpanChan:       lambdaSpanChan,
		LambdaInitMetricChan: lambdaInitMetricChan,
		TraceAgent:           serverlessDaemon.TraceAgent,
		StopChan:             make(chan struct{}),
		ColdStartSpanId:      coldStartSpanId,
		ColdStartRequestID:   serverlessDaemon.ExecutionContext.GetCurrentState().ColdstartRequestID,
	}

	log.Debug("Starting ColdStartSpanCreator")
	coldStartSpanCreator.Run()
	log.Debug("Setting ColdStartSpanCreator on Daemon")
	serverlessDaemon.SetColdStartSpanCreator(coldStartSpanCreator)

	ta := serverlessDaemon.TraceAgent.Get()
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

	if appsecProxyProcessor != nil {
		// AppSec runs as a Runtime API proxy. The reverse proxy was already
		// started by appsec.New(). A span modifier needs to be added in order
		// to detect the finished request spans and run the complete AppSec
		// monitoring logic, and ultimately adding the AppSec events to them.
		ta.ModifySpan = appsecProxyProcessor.WrapSpanModifier(serverlessDaemon.ExecutionContext, ta.ModifySpan)
		// Set the default rate limiting to approach 1 trace/min in live circumstances to limit non ASM related traces as much as possible.
		// This limit is decided in the Standalone ASM Billing RFC and ensures reducing non ASM-related trace throughput
		// while keeping billing and service catalog running correctly.
		// In case of ASM event, the trace priority will be set to manual keep
		if appsecConfig.IsStandalone() {
			ta.PrioritySampler.UpdateTargetTPS(1. / 120)
		}
	} else if enabled, _ := strconv.ParseBool(os.Getenv("DD_EXPERIMENTAL_ENABLE_PROXY")); enabled {
		// start the experimental proxy if enabled
		log.Debug("Starting the experimental runtime api proxy")
		proxy.Start(
			"127.0.0.1:9000",
			"127.0.0.1:9001",
			serverlessDaemon.InvocationProcessor,
		)
	}

	serverlessDaemon.ComputeGlobalTags(configUtils.GetConfiguredTags(config.Datadog, true))

	// run the invocation loop in a routine
	// we don't want to start this mainloop before because once we're waiting on
	// the invocation route, we can't report init errors anymore.
	go func() {
		for {
			if err := serverless.WaitForNextInvocation(stopCh, serverlessDaemon, serverlessID); err != nil {
				log.Error(err)
			}
		}
	}()

	go handleTerminationSignals(serverlessDaemon, stopCh, signal.Notify)

	// this log line is used for performance checks during CI
	// please be careful before modifying/removing it
	log.Debugf("serverless agent ready in %v", time.Since(startTime))

	// block here until we receive a stop signal
	<-stopCh
	//nolint:gosimple // TODO(SERV) Fix gosimple linter
	return
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
