// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
// +build !windows

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/serverless"
	"github.com/DataDog/datadog-agent/pkg/serverless/registration"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	runtimeAPIEnvVar             = "AWS_LAMBDA_RUNTIME_API"
	extensionRegistrationRoute   = "/2020-01-01/extension/register"
	datadogConfigPath            = "/var/task/datadog.yaml"
	routeEventNext               = "/2020-01-01/extension/event/next"
	headerExtID                  = "Lambda-Extension-Identifier"
	logLevelEnvVar               = "DD_LOG_LEVEL"
	extensionRegistrationTimeout = 5 * time.Second

	loggerName config.LoggerName = "DD_EXTENSION"
)

func init() {
	os.Setenv("DD_API_KEY", "INVALID")
}

var (
	// client is a client that should never timeout
	client = &http.Client{Timeout: 0}

	extensionNextURL = func(route string) string {
		prefix := os.Getenv("AWS_LAMBDA_RUNTIME_API")
		if len(prefix) == 0 {
			return fmt.Sprintf("http://localhost:9001%s", route)
		}
		return fmt.Sprintf("http://%s%s", prefix, route)
	}(routeEventNext)
)

func main() {
	configureLogging()
	log.Info("starting dd otel extension")

	traceAgent := &trace.ServerlessTraceAgent{}
	traceAgent.Start(true, &trace.LoadConfig{Path: datadogConfigPath}, nil)
	defer traceAgent.Stop()

	serverlessID, err := registration.RegisterExtension(
		os.Getenv(runtimeAPIEnvVar),
		extensionRegistrationRoute,
		extensionRegistrationTimeout,
	)
	if err != nil {
		log.Errorf("Can't register as a serverless agent: %s", err)
		return
	}

	for {
		shutdown, err := WaitForNextInvocation(serverlessID)
		if err != nil {
			log.Error(err)
		}
		traceAgent.Flush()
		if shutdown {
			break
		}
	}
}

func configureLogging() {
	// init the logger configuring it to not log in a file (the first empty string)
	if err := config.SetupLogger(
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
}

func WaitForNextInvocation(id registration.ID) (bool, error) {
	request, err := http.NewRequest(http.MethodGet, extensionNextURL, nil)
	if err != nil {
		return false, fmt.Errorf("WaitForNextInvocation: can't create the GET request: %v", err)
	}
	request.Header.Set(headerExtID, id.String())

	// make a blocking HTTP call to wait for the next event from AWS
	response, err := client.Do(request)
	if err != nil {
		return false, fmt.Errorf("WaitForNextInvocation: while GET next route: %v", err)
	}
	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return false, fmt.Errorf("WaitForNextInvocation: can't read the body: %v", err)
	}

	var payload serverless.Payload
	if err := json.Unmarshal(body, &payload); err != nil {
		return false, fmt.Errorf("WaitForNextInvocation: can't unmarshal the payload: %v", err)
	}

	if payload.EventType == serverless.Shutdown {
		return true, nil
	}
	return false, nil
}
