// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package main

import (
	"context"
	"encoding/base64"
	_ "expvar"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/kms"
	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serverless"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const defaultLogFile = "/var/log/datadog/serverless-agent.log"

var (
	// serverlessAgentCmd is the root command
	serverlessAgentCmd = &cobra.Command{
		Use:   "agent [command]",
		Short: "Datadog Agent at your service.",
		Long: `
Datadog Serverless Agent accepts custom application metrics points over UDP, aggregates and forwards them to Datadog,
where they can be graphed on dashboards. The Datadog Serverless Agent implements the StatsD protocol, along with a few extensions for special Datadog features.`,
	}

	startCmd = &cobra.Command{
		Use:   "start",
		Short: "Start the Serverless Agent",
		Long:  `Runs the Serverless Agent`,
		RunE:  start,
	}

	versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Print the version number",
		Long:  ``,
		Run: func(cmd *cobra.Command, args []string) {
			av, _ := version.Agent()
			fmt.Println(fmt.Sprintf("Serverless Agent %s - Codename: %s - Commit: %s - Serialization version: %s - Go version: %s",
				av.GetNumber(), av.Meta, av.Commit, serializer.AgentPayloadVersion, runtime.Version()))
		},
	}

	confPath   string
	socketPath string

	statsdServer *dogstatsd.Server

	apiKeyEnvVar    = "DD_API_KEY"
	kmsApiKeyEnvVar = "DD_KMS_API_KEY"
	logLevelEnvVar  = "DD_LOG_LEVEL"
)

const (
	// loggerName is the name of the serverless agent logger
	loggerName config.LoggerName = "SAGENT"
)

func init() {
	// attach the command to the root
	serverlessAgentCmd.AddCommand(startCmd)
	serverlessAgentCmd.AddCommand(versionCmd)
}

func start(cmd *cobra.Command, args []string) error {
	// Main context passed to components
	ctx, cancel := context.WithCancel(context.Background())
	defer stopCallback(cancel)

	stopCh := make(chan struct{})

	// handle SIGTERM
	go handleSignals(stopCh)

	// run the agent
	err := runAgent(ctx, stopCh)
	if err != nil {
		return err
	}

	// block here until we receive a stop signal
	<-stopCh
	return nil
}

func main() {
	flavor.SetFlavor(flavor.ServerlessAgent)

	// go_expvar server // TODO(remy): shouldn't we remove that for the serverless agent?
	go http.ListenAndServe( //nolint:errcheck
		fmt.Sprintf("127.0.0.1:%d", config.Datadog.GetInt("dogstatsd_stats_port")),
		http.DefaultServeMux)

	// if not command has been provided, run start
	if len(os.Args) == 1 {
		os.Args = append(os.Args, "start")
	}

	if err := serverlessAgentCmd.Execute(); err != nil {
		log.Error(err)
		os.Exit(-1)
	}
}

func runAgent(ctx context.Context, stopCh chan struct{}) (err error) {
	startTime := time.Now()

	// setup logger
	// -----------

	// init the logger configuring it to not log in a file (the first empty string)
	if err = config.SetupLogger(
		loggerName,
		"info", // will be re-set later with the value from the env var
		"",     // logFile -> by setting this to an empty string, we don't write the logs to any file
		"",     // syslog URI
		false,  // syslog_rfc
		true,   // log_to_console
		false,  // log_format_json
	); err != nil {
		log.Criticalf("Unable to setup logger: %s", err)
		return
	}

	// immediately starts the communication server
	daemon := serverless.StartDaemon(stopCh)

	// serverless parts
	// ----------------

	// register
	serverlessId, err := serverless.Register()
	if err != nil {
		// at this point, we were not even able to register, thus, we don't have
		// any Id assigned, thus, we can't report an error to the init error route
		// which needs an Id.
		log.Criticalf("Can't register as a serverless agent: %s", err)
		return
	}

	// try to read apikey from KMS
	// ---------------------------

	var apiKey string
	if apiKey, err = readApiKeyFromKMS(); err != nil {
		log.Errorf("Error while trying to read an API Key from KMS: %s", err)
	} else if apiKey != "" {
		if os.Getenv(apiKeyEnvVar) != "" {
			log.Warn("An API Key has been set in both KMS and in the environment variable. Using the one set in KMS.")
		}
		log.Info("Using deciphered KMS API Key")
		os.Setenv(apiKeyEnvVar, apiKey) // it will be catched up by config.Load()
	}

	// read configuration from the environment vars
	// --------------------------------------------

	if _, confErr := config.Load(); confErr != nil {
		log.Info("Configuration will be read from environment variables")
	} else {
		log.Warn("A configuration file has been found, which should not happen in this mode.")
	}

	// validate that an apikey has been set, either by the env var or read from KMS
	// ---------------------------

	if !config.Datadog.IsSet("api_key") {
		// we're not reporting the error to AWS because we don't want the function
		// execution to be stopped. TODO(remy): discuss with AWS if there is way
		// of reporting non-critical init errors.
		// serverless.ReportInitError(serverlessId, serverless.FatalNoApiKey)
		log.Critical("No API key configured, exiting")
		return
	}

	if logLevel := os.Getenv(logLevelEnvVar); len(logLevel) > 0 {
		config.ChangeLogLevel(logLevel)
	}

	// setup the forwarder, serializer and aggregator
	// ----------------------------------------------

	keysPerDomain, err := config.GetMultipleEndpoints()
	if err != nil {
		// we're not reporting the error to AWS because we don't want the function
		// execution to be stopped. TODO(remy): discuss with AWS if there is way
		// of reporting non-critical init errors.
		// serverless.ReportInitError(serverlessId, serverless.FatalBadEndpoint)
		log.Criticalf("Misconfiguration of agent endpoints: %s", err)
		return
	}
	f := forwarder.NewDefaultForwarder(forwarder.NewOptions(keysPerDomain))
	f.Start() //nolint:errcheck
	serializer := serializer.NewSerializer(f)

	aggregatorInstance := aggregator.InitAggregator(serializer, "serverless")

	// initializes the DogStatsD server
	// --------------------------------

	statsdServer, err = dogstatsd.NewServer(aggregatorInstance)
	if err != nil {
		// we're not reporting the error to AWS because we don't want the function
		// execution to be stopped. TODO(remy): discuss with AWS if there is way
		// of reporting non-critical init errors.
		// serverless.ReportInitError(serverlessId, serverless.FatalDogstatsdInit)
		log.Criticalf("Unable to start the DogStatsD server: %s", err)
		return
	}
	statsdServer.ServerlessMode = true // we're running in a serverless environment (will removed host field from samples)

	// run the invocation loop in a routine
	// we don't want to start this mainloop before because once we're waiting on
	// the invocation route, we can't report init errors anymore.
	go func() {
		for {
			serverless.WaitForNextInvocation(stopCh, statsdServer, serverlessId)
		}
	}()

	// DogStatsD daemon ready.
	daemon.SetStatsdServer(statsdServer)
	daemon.ReadyWg.Done()

	log.Debugf("serverless agent ready in %v", time.Since(startTime))
	return
}

// handleSignals handles OS signals, if a SIGTERM is received,
// the serverless agent stops.
func handleSignals(stopCh chan struct{}) {
	// setup a channel to catch OS signals
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	// block here until we receive the interrupt signal
	// when received, shutdown the serverless agent.
	for signo := range signalCh {
		switch signo {
		default:
			log.Infof("Received signal '%s', shutting down...", signo)
			stopCh <- struct{}{}
			return
		}
	}
}

// decryptKMS deciphered the cipherText given as parameter.
// Function stolen and adapted from datadog-lambda-go/internal/metrics/kms_decrypter.go
func decryptKMS(cipherText string) (string, error) {
	kmsClient := kms.New(session.New(nil))
	decodedBytes, err := base64.StdEncoding.DecodeString(cipherText)
	if err != nil {
		return "", fmt.Errorf("Failed to encode cipher text to base64: %v", err)
	}

	params := &kms.DecryptInput{
		CiphertextBlob: decodedBytes,
	}

	response, err := kmsClient.Decrypt(params)
	if err != nil {
		return "", fmt.Errorf("Failed to decrypt ciphertext with kms: %v", err)
	}
	// Plaintext is a byte array, so convert to string
	decrypted := string(response.Plaintext[:])

	return decrypted, nil
}

// readApiKeyFromKMS reads an API Key in KMS.
// If none has been set, it is returning an empty string and a nil error.
func readApiKeyFromKMS() (string, error) {
	ciphered := os.Getenv(kmsApiKeyEnvVar)
	if ciphered == "" {
		return "", nil
	}
	log.Debug("Found DD_KMS_API_KEY value, trying to decipher it.")
	if rv, err := decryptKMS(ciphered); err != nil {
		return "", fmt.Errorf("decryptKMS error: %s", err)
	} else {
		return rv, nil
	}
}

func stopCallback(cancel context.CancelFunc) {
	// gracefully shut down any component
	cancel()

	if statsdServer != nil {
		statsdServer.Stop()
	}

	log.Info("See ya!")
	log.Flush()
	return
}
