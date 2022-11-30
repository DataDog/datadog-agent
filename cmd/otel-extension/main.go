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

	"github.com/DataDog/datadog-agent/cmd/serverless-init/metadata"
	"github.com/DataDog/datadog-agent/cmd/serverless-init/tag"
	"github.com/DataDog/datadog-agent/pkg/serverless"
	"github.com/DataDog/datadog-agent/pkg/serverless/registration"
	"github.com/DataDog/datadog-agent/pkg/serverless/trace"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	runtimeAPIEnvVar                    = "AWS_LAMBDA_RUNTIME_API"
	extensionRegistrationRoute          = "/2020-01-01/extension/register"
	datadogConfigPath                   = "/var/task/datadog.yaml"
	routeEventNext               string = "/2020-01-01/extension/event/next"
	headerExtID                  string = "Lambda-Extension-Identifier"
	extensionRegistrationTimeout        = 5 * time.Second
)

func main() {
	metadata := metadata.GetMetaData(metadata.GetDefaultConfig())
	traceAgent := &trace.ServerlessTraceAgent{}

	setupTraceAgent(traceAgent, metadata)
	defer traceAgent.Stop()

	// Register the extension
	serverlessID, err := registration.RegisterExtension(os.Getenv(runtimeAPIEnvVar), extensionRegistrationRoute, extensionRegistrationTimeout)
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

func setupTraceAgent(traceAgent *trace.ServerlessTraceAgent, metadata *metadata.Metadata) {
	os.Setenv("DD_API_KEY", "INVALID_KEY")
	traceAgent.Start(true, &trace.LoadConfig{Path: datadogConfigPath})
	traceAgent.SetTags(tag.GetBaseTagsMapWithMetadata(metadata.TagMap()))

}

func WaitForNextInvocation(id registration.ID) (bool, error) {
	request, err := http.NewRequest(http.MethodGet, buildURL(routeEventNext), nil)
	if err != nil {
		return false, fmt.Errorf("WaitForNextInvocation: can't create the GET request: %v", err)
	}
	request.Header.Set(headerExtID, id.String())

	// make a blocking HTTP call to wait for the next event from AWS
	client := &http.Client{Timeout: 0} // this one should never timeout
	response, err := client.Do(request)
	if err != nil {
		return false, fmt.Errorf("WaitForNextInvocation: while GET next route: %v", err)
	}
	defer response.Body.Close()

	body, err = ioutil.ReadAll(response.Body)
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

func buildURL(route string) string {
	prefix := os.Getenv("AWS_LAMBDA_RUNTIME_API")
	if len(prefix) == 0 {
		return fmt.Sprintf("http://localhost:9001%s", route)
	}
	return fmt.Sprintf("http://%s%s", prefix, route)
}
