// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	logConfig "github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/serverless"
	"github.com/DataDog/datadog-agent/pkg/serverless/daemon"
	"github.com/DataDog/datadog-agent/pkg/serverless/flush"
	"github.com/DataDog/datadog-agent/pkg/serverless/invocationlifecycle"
	serverlessLogs "github.com/DataDog/datadog-agent/pkg/serverless/logs"
	"github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/proxy"
	"github.com/DataDog/datadog-agent/pkg/serverless/registration"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	kmsAPIKeyEnvVar            = "DD_KMS_API_KEY"
	secretsManagerAPIKeyEnvVar = "DD_API_KEY_SECRET_ARN"
	apiKeyEnvVar               = "DD_API_KEY"
	logLevelEnvVar             = "DD_LOG_LEVEL"
	flushStrategyEnvVar        = "DD_SERVERLESS_FLUSH_STRATEGY"
	logsLogsTypeSubscribed     = "DD_LOGS_CONFIG_LAMBDA_LOGS_TYPE"

	// AWS Lambda is writing the Lambda function files in /var/task, we want the
	// configuration file to be at the root of this directory.
	datadogConfigPath = "/var/task/datadog.yaml"
)

const (
	loggerName config.LoggerName = "DD_EXTENSION"

	runtimeAPIEnvVar = "AWS_LAMBDA_RUNTIME_API"

	extensionRegistrationRoute   = "/2020-01-01/extension/register"
	extensionRegistrationTimeout = 5 * time.Second

	// httpServerAddr will be the default addr used to run the HTTP server listening
	// to calls from the client libraries and to logs from the AWS environment.
	httpServerAddr = ":8124"

	logsAPIRegistrationRoute   = "/2020-08-15/logs"
	logsAPIRegistrationTimeout = 5 * time.Second
	logsAPIHttpServerPort      = 8124
	logsAPICollectionRoute     = "/lambda/logs"
	logsAPITimeout             = 25
	logsAPIMaxBytes            = 262144
	logsAPIMaxItems            = 1000
)

func main() {
	flavor.SetFlavor(flavor.ServerlessAgent)
	stopCh := make(chan struct{})

	// run the agent
	serverlessDaemon, err := runAgent(stopCh)
	if err != nil {
		log.Error(err)
		os.Exit(-1)
	}

	// handle SIGTERM signal
	go handleSignals(serverlessDaemon, stopCh)

	// block here until we receive a stop signal
	<-stopCh
}

func runAgent(stopCh chan struct{}) (serverlessDaemon *daemon.Daemon, err error) {

	startTime := time.Now()

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

	outputDatadogEnvVariablesForDebugging()

	// immediately starts the communication server
	serverlessDaemon = daemon.StartDaemon(httpServerAddr)
	err = serverlessDaemon.ExecutionContext.RestoreCurrentStateFromFile()
	if err != nil {
		log.Debug("Unable to restore the state from file")
	} else {
		serverlessDaemon.ComputeGlobalTags(config.GetConfiguredTags(true))
	}
	// serverless parts
	// ----------------

	// extension registration
	serverlessID, err := registration.RegisterExtension(os.Getenv(runtimeAPIEnvVar), extensionRegistrationRoute, extensionRegistrationTimeout)
	if err != nil {
		// at this point, we were not even able to register, thus, we don't have
		// any ID assigned, thus, we can't report an error to the init error route
		// which needs an Id.
		log.Errorf("Can't register as a serverless agent: %s", err)
		return
	}

	// api key reading
	// ---------------

	// API key reading priority:
	// KSM > Secrets Manager > Plaintext API key
	// If one is set but failing, the next will be tried

	// some useful warnings first

	var apikeySetIn = []string{}
	if os.Getenv(kmsAPIKeyEnvVar) != "" {
		apikeySetIn = append(apikeySetIn, "KMS")
	}
	if os.Getenv(secretsManagerAPIKeyEnvVar) != "" {
		apikeySetIn = append(apikeySetIn, "SSM")
	}
	if os.Getenv(apiKeyEnvVar) != "" {
		apikeySetIn = append(apikeySetIn, "environment variable")
	}

	if len(apikeySetIn) > 1 {
		log.Warn("An API Key has been set in multiple places:", strings.Join(apikeySetIn, ", "))
	}

	// try to read API key from KMS

	var apiKey string
	if apiKey, err = readAPIKeyFromKMS(); err != nil {
		log.Errorf("Error while trying to read an API Key from KMS: %s", err)
	} else if apiKey != "" {
		log.Info("Using deciphered KMS API Key.")
		os.Setenv(apiKeyEnvVar, apiKey)
	}

	// try to read the API key from Secrets Manager, only if not set from KMS

	if apiKey == "" {
		if apiKey, err = readAPIKeyFromSecretsManager(); err != nil {
			log.Errorf("Error while trying to read an API Key from Secrets Manager: %s", err)
		} else if apiKey != "" {
			log.Info("Using API key set in Secrets Manager.")
			os.Setenv(apiKeyEnvVar, apiKey)
		}
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

	// validate that an apikey has been set, either by the env var, read from KMS or Secrets Manager.
	// ---------------------------
	if !config.Datadog.IsSet("api_key") {
		// we're not reporting the error to AWS because we don't want the function
		// execution to be stopped. TODO(remy): discuss with AWS if there is way
		// of reporting non-critical init errors.
		// serverless.ReportInitError(serverlessID, serverless.FatalNoAPIKey)
		log.Error("No API key configured, exiting")
	}
	config.Datadog.SetConfigFile(datadogConfigPath)

	logChannel := make(chan *logConfig.ChannelMessage)

	metricAgent := &metrics.ServerlessMetricAgent{}
	metricAgent.Start(daemon.FlushTimeout, &metrics.MetricConfig{}, &metrics.MetricDogStatsD{})
	serverlessDaemon.SetStatsdServer(metricAgent)
	serverlessDaemon.SetupLogCollectionHandler(logsAPICollectionRoute, logChannel, config.Datadog.GetBool("serverless.logs_enabled"), config.Datadog.GetBool("enhanced_metrics"))

	wg := sync.WaitGroup{}
	wg.Add(1)
	wg.Add(1)

	// starts trace agent
	go func() {
		defer wg.Done()
		traceAgent := &trace.ServerlessTraceAgent{}
		traceAgent.Start(config.Datadog.GetBool("apm_config.enabled"), &trace.LoadConfig{Path: datadogConfigPath})
		serverlessDaemon.SetTraceAgent(traceAgent)
	}()

	// enable logs collection
	go func() {
		defer wg.Done()
		log.Debug("Enabling logs collection HTTP route")
		logRegistrationURL := registration.BuildURL(os.Getenv(runtimeAPIEnvVar), logsAPIRegistrationRoute)
		logRegistrationError := registration.EnableLogsCollection(
			serverlessID,
			logRegistrationURL,
			logsAPIRegistrationTimeout,
			os.Getenv(logsLogsTypeSubscribed),
			logsAPIHttpServerPort,
			logsAPICollectionRoute,
			logsAPITimeout,
			logsAPIMaxBytes,
			logsAPIMaxItems)

		if logRegistrationError != nil {
			log.Error("Can't subscribe to logs:", logRegistrationError)
		} else {
			serverlessLogs.SetupLogAgent(logChannel)
		}
	}()

	wg.Wait()

	// set up invocation processor in the serverless Daemon to be used for the proxy and/or lifecycle API
	serverlessDaemon.InvocationProcessor = &invocationlifecycle.LifecycleProcessor{
		ExtraTags:           serverlessDaemon.ExtraTags,
		Demux:               serverlessDaemon.MetricAgent.Demux,
		ProcessTrace:        serverlessDaemon.TraceAgent.Get().Process,
		DetectLambdaLibrary: func() bool { return serverlessDaemon.LambdaLibraryDetected },
	}

	// start the experimental proxy if enabled
	_ = proxy.Start(
		"127.0.0.1:9000",
		"127.0.0.1:9001",
		serverlessDaemon.InvocationProcessor,
	)

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

	// this log line is used for performance checks during CI
	// please be careful before modifying/removing it
	log.Debugf("serverless agent ready in %v", time.Since(startTime))
	return
}

// handleSignals handles OS signals, if a SIGTERM is received,
// the serverless agent stops.
func handleSignals(serverlessDaemon *daemon.Daemon, stopCh chan struct{}) {
	// setup a channel to catch OS signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	// block here until we receive the interrupt signal
	// when received, shutdown the serverless agent.
	for signo := range signalCh {
		switch signo {
		default:
			log.Infof("Received signal '%s', shutting down...", signo)
			serverlessDaemon.Stop()
			stopCh <- struct{}{}
			return
		}
	}
}
