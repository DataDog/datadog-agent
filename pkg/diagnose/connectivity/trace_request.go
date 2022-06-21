// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package connectivity contains logic for connectivity troubleshooting between the Agent
// and Datadog endpoints. It uses HTTP request to contact different endpoints and displays
// some results depending on endpoints responses, if any.
package connectivity

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/resolver"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/fatih/color"
)

// RunDatadogConnectivityDiagnose sends requests to all known endpoints for all domains
// to check if there are connectivity issues between Datadog and these endpoints
func RunDatadogConnectivityDiagnose(noTrace bool) error {
	// Create domain resolvers
	keysPerDomain, err := config.GetMultipleEndpoints()
	if err != nil {
		return log.Error("Misconfiguration of agent endpoints: ", err)
	}

	domainResolvers := resolver.NewSingleDomainResolvers(keysPerDomain)

	client := forwarder.NewHTTPClient()

	// Send requests to all endpoints for all domains
	fmt.Println("\n================ Starting connectivity diagnosis ================")
	for _, domainResolver := range domainResolvers {
		sendRequestToAllEndpointOfADomain(client, domainResolver, noTrace)
	}

	return nil
}

// sendRequestToAllEndpointOfADomain sends HTTP request on all endpoints for a given domain
func sendRequestToAllEndpointOfADomain(client *http.Client, domainResolver resolver.DomainResolver, noTrace bool) {

	// Go through all API Keys of a domain and send an HTTP request on each endpoint
	for _, apiKey := range domainResolver.GetAPIKeys() {

		for _, endpointInfo := range endpointsInfo {

			domain, _ := domainResolver.Resolve(endpointInfo.Endpoint)

			ctx := context.Background()
			if !noTrace {
				ctx = httptrace.WithClientTrace(context.Background(), createDiagnoseTrace())
			}

			statusCode, responseBody, err := sendHTTPRequestToEndpoint(ctx, client, domain, endpointInfo, apiKey)

			// Check if there is a response and if it's valid
			verifyEndpointResponse(statusCode, responseBody, err)
		}
	}
}

// sendHTTPRequestToEndpoint creates an URL based on the domain and the endpoint information
// then sends an HTTP Request with the method and payload inside the endpoint information
func sendHTTPRequestToEndpoint(ctx context.Context, client *http.Client, domain string, endpointInfo endpointInfo, apiKey string) (int, []byte, error) {
	url := createEndpointURL(domain, endpointInfo)
	logURL := scrubber.ScrubLine(url)

	fmt.Printf("\n======== '%v' ========\n", color.BlueString(logURL))

	// Create a request for the backend
	reader := bytes.NewReader(endpointInfo.Payload)
	req, err := http.NewRequest(endpointInfo.Method, url, reader)

	if err != nil {
		return 0, nil, fmt.Errorf("cannot create request for transaction to invalid URL '%v' : %v", logURL, scrubber.ScrubLine(err.Error()))
	}

	// Add tracing and send the request
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("DD-API-KEY", apiKey)

	resp, err := client.Do(req)

	if err != nil {
		return 0, nil, fmt.Errorf("cannot send the HTTP request to '%v' : %v", logURL, scrubber.ScrubLine(err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()

	// Get the endpoint response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("fail to read the response Body: %s", err)
	}

	return resp.StatusCode, body, nil
}

// createEndpointUrl joins a domain with an endpoint
func createEndpointURL(domain string, endpointInfo endpointInfo) string {
	return domain + endpointInfo.Endpoint.Route
}

func verifyEndpointResponse(statusCode int, responseBody []byte, err error) {

	if err != nil {
		fmt.Printf("could not get a response from the endpoint : %v\n", err)
		return
	}

	statusString := color.GreenString("PASS")
	if statusCode >= 400 {
		statusString = color.RedString("FAIL")
		fmt.Printf("Received response : '%v'\n", string(responseBody))
	}
	fmt.Printf("Received status code %v from the endpoint ====> %v\n", statusCode, statusString)
}
