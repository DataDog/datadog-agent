// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(SERV) Fix revive linter
package main

import (
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/hostname"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggernoop "github.com/DataDog/datadog-agent/comp/core/tagger/fx-noop"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	logscompressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx"
	"github.com/DataDog/datadog-agent/pkg/agentless/apikey"
	"github.com/DataDog/datadog-agent/pkg/agentless/daemon"
	"github.com/DataDog/datadog-agent/pkg/agentless/debug"
	"github.com/DataDog/datadog-agent/pkg/agentless/metrics"
	serverlessRemoteConfig "github.com/DataDog/datadog-agent/pkg/agentless/remoteconfig"
	"github.com/DataDog/datadog-agent/pkg/agentless/trace"
	remoteconfig "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	configUtils "github.com/DataDog/datadog-agent/pkg/config/utils"
	serverlessLogs "github.com/DataDog/datadog-agent/pkg/serverless/logs"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	pkglogsetup "github.com/DataDog/datadog-agent/pkg/util/log/setup"
)

// Configuration file path - can be overridden via DD_CONFIG_FILE env var
var datadogConfigPath string

const (
	loggerName     pkglogsetup.LoggerName = "AGENTLESS"
	logLevelEnvVar                        = "DD_LOG_LEVEL"

	// Default HTTP server address for the agentless agent
	httpServerAddr = ":8124"
)

func main() {
	// run the agent
	err := fxutil.OneShot(
		runAgent,
		taggernoop.Module(),
		hostnameimpl.Module(),
		logscompressionfx.Module(),
	)

	if err != nil {
		log.Error(err)
		os.Exit(-1)
	}
}

func runAgent(tagger tagger.Component, hostname hostname.Component, compression logscompression.Component) {

	startTime := time.Now()

	setupAgentlessOverrides()
	setupLogger()
	debug.OutputDatadogEnvVariablesForDebugging()
	loadConfig()
	if !setupApiKey() {
		return
	}
	// Start the agentless daemon
	agentlessDaemon := startCommunicationServer(startTime)

	// Start the metric agent
	_ = startMetricAgent(agentlessDaemon, tagger)

	// Start RC service if remote configuration is enabled
	// Use a valid hostname identifier for the RC service
	agentHostname, err := os.Hostname()
	if err != nil || agentHostname == "" {
		agentHostname = "agentless-agent"
	}
	rcService := serverlessRemoteConfig.StartRCService(agentHostname)

	// Start the trace agent and logs agent concurrently
	var wg sync.WaitGroup
	wg.Add(2)

	go startTraceAgent(&wg, agentlessDaemon, tagger, rcService)
	go startLogsAgent(&wg, agentlessDaemon, tagger, compression, hostname)

	wg.Wait()

	// Verify trace agent started
	if agentlessDaemon.TraceAgent == nil {
		log.Error("Failed to start trace agent")
		return
	}

	// Set up signal handling for graceful shutdown
	stopCh := make(chan struct{})
	go handleTerminationSignals(agentlessDaemon, stopCh, signal.Notify)

	// this log line is used for performance checks during CI
	// please be careful before modifying/removing it
	log.Infof("agentless agent ready in %v", time.Since(startTime))

	// block here until we receive a stop signal
	<-stopCh
	//nolint:gosimple // TODO(SERV) Fix gosimple linter
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

func setupAgentlessOverrides() {
	// Enable remote configuration by default for agentless agent
	if strings.ToLower(os.Getenv("DD_REMOTE_CONFIGURATION_ENABLED")) != "false" {
		os.Setenv("DD_REMOTE_CONFIGURATION_ENABLED", "true")
	}

	// APM (Traces) - prefer Unix socket/named pipe over TCP
	// Set APM receiver port to 0 (disable TCP listener) if not explicitly set
	if os.Getenv("DD_APM_RECEIVER_PORT") == "" {
		os.Setenv("DD_APM_RECEIVER_PORT", "0")
	}

	// Set platform-specific APM receiver socket/pipe defaults if not explicitly set
	if runtime.GOOS == "windows" {
		// Windows: use named pipe
		if os.Getenv("DD_APM_WINDOWS_PIPE_NAME") == "" {
			os.Setenv("DD_APM_WINDOWS_PIPE_NAME", `\\.\pipe\datadog-libagent`)
		}
	} else {
		// Unix-like systems: use Unix domain socket
		if os.Getenv("DD_APM_RECEIVER_SOCKET") == "" {
			os.Setenv("DD_APM_RECEIVER_SOCKET", "/tmp/datadog_libagent.socket")
		}
	}

	// DogStatsD (Metrics) - prefer Unix socket/named pipe over UDP
	// Disable UDP port if not explicitly set
	if os.Getenv("DD_DOGSTATSD_PORT") == "" {
		os.Setenv("DD_DOGSTATSD_PORT", "0")
	}

	// Set platform-specific DogStatsD socket/pipe defaults if not explicitly set
	if runtime.GOOS == "windows" {
		// Windows: use named pipe
		if os.Getenv("DD_DOGSTATSD_PIPE_NAME") == "" {
			os.Setenv("DD_DOGSTATSD_PIPE_NAME", `\\.\pipe\datadog-dogstatsd`)
		}
	} else {
		// Unix-like systems: use Unix domain socket
		if os.Getenv("DD_DOGSTATSD_SOCKET") == "" {
			os.Setenv("DD_DOGSTATSD_SOCKET", "/tmp/datadog_dogstatsd.socket")
		}
	}

	// Set config file path
	if datadogConfigPath == "" {
		if configFile := os.Getenv("DD_CONFIG_FILE"); configFile != "" {
			datadogConfigPath = configFile
		} else {
			datadogConfigPath = "datadog.yaml"
		}
	}
}

func startMetricAgent(agentlessDaemon *daemon.Daemon, tagger tagger.Component) *metrics.ServerlessMetricAgent {
	metricAgent := &metrics.ServerlessMetricAgent{
		SketchesBucketOffset: time.Second * 10,
		Tagger:               tagger,
	}
	metricAgent.Start(daemon.FlushTimeout, &metrics.MetricConfig{}, &metrics.MetricDogStatsD{}, false)
	agentlessDaemon.SetStatsdServer(metricAgent)
	log.Debug("Metric agent started")
	return metricAgent
}

func startTraceAgent(wg *sync.WaitGroup, agentlessDaemon *daemon.Daemon, tagger tagger.Component, rcService *remoteconfig.CoreAgentService) {
	defer wg.Done()
	traceAgent := trace.StartServerlessTraceAgent(trace.StartServerlessTraceAgentArgs{
		Enabled:    configUtils.IsAPMEnabled(pkgconfigsetup.Datadog()),
		LoadConfig: &trace.LoadConfig{Path: datadogConfigPath, Tagger: tagger},
		RCService:  rcService,
	})
	agentlessDaemon.SetTraceAgent(traceAgent)
	log.Debug("Trace agent started")
}

func startLogsAgent(wg *sync.WaitGroup, agentlessDaemon *daemon.Daemon, tagger tagger.Component, compression logscompression.Component, hostname hostname.Component) {
	defer wg.Done()

	// Check if logs are enabled
	if !pkgconfigsetup.Datadog().GetBool("logs_enabled") {
		log.Debug("Logs agent disabled (logs_enabled=false)")
		return
	}

	// Simple channel-based logs setup (no Lambda-specific telemetry API)
	logsAgent, err := serverlessLogs.SetupLogAgent(nil, "Agentless", "agentless", tagger, compression, hostname)
	if err != nil {
		log.Errorf("Error setting up the logs agent: %s", err)
		return
	}
	agentlessDaemon.SetLogsAgent(logsAgent)
	log.Debug("Logs agent started")
}

func setupApiKey() bool {
	if err := apikey.HandleEnv(); err != nil {
		log.Errorf("Can't start the agentless agent as no API Key has been detected, or API Key could not be decrypted. Data will not be sent to Datadog.")
		return false
	}
	return true
}

func loadConfig() {
	ddcfg := pkgconfigsetup.GlobalConfigBuilder()
	ddcfg.SetConfigFile(datadogConfigPath)
	// Load datadog.yaml file into the config, so that metricAgent can pick these configurations
	if _, err := pkgconfigsetup.LoadWithoutSecret(ddcfg, nil); err != nil {
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
