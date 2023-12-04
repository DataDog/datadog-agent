// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/fx"

	svls "github.com/DataDog/datadog-agent/comp/serverless"
	"github.com/DataDog/datadog-agent/comp/serverless/daemon"
	"github.com/DataDog/datadog-agent/comp/serverless/daemon/daemonimpl"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/serverless"
	"github.com/DataDog/datadog-agent/pkg/serverless/apikey"
	"github.com/DataDog/datadog-agent/pkg/serverless/debug"
	"github.com/DataDog/datadog-agent/pkg/serverless/registration"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	logLevelEnvVar = "DD_LOG_LEVEL"

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
)

func main() {

	// run the agent
	err := fxutil.OneShot(runAgent,
		fx.Supply(daemonimpl.Params{Addr: httpServerAddr, SketchesBucketOffset: time.Second * 10}),
		svls.Bundle,
	)

	if err != nil {
		log.Error(err)
		os.Exit(-1)
	}
}

func runAgent(serverlessDaemon daemon.Component) {
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

	config.Datadog.SetConfigFile(datadogConfigPath)
	// Load datadog.yaml file into the config, so that metricAgent can pick these configurations
	if _, err := config.LoadWithoutSecret(); err != nil {
		log.Errorf("Error happened when loading configuration from datadog.yaml for metric agent: %s", err)
	}

	apikey.HandleEnv()

	// extension registration
	serverlessID, functionArn, err := registration.RegisterExtension(extensionRegistrationRoute, extensionRegistrationTimeout)
	if err != nil {
		// at this point, we were not even able to register, thus, we don't have
		// any ID assigned, thus, we can't report an error to the init error route
		// which needs an Id.
		log.Errorf("Can't register as a serverless agent: %s", err)
		return
	}

	// immediately starts the communication server
	serverlessDaemon.Start(startTime, datadogConfigPath, serverlessID, functionArn)

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
	return
}

// handleTerminationSignals handles OS termination signals.
// If a specified signal is received the serverless agent stops.
func handleTerminationSignals(serverlessDaemon daemon.Component, stopCh chan struct{}, notify func(c chan<- os.Signal, sig ...os.Signal)) {
	signalCh := make(chan os.Signal, 1)
	notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	signo := <-signalCh
	log.Infof("Received signal '%s', shutting down...", signo)
	serverlessDaemon.Stop()
	stopCh <- struct{}{}
}
