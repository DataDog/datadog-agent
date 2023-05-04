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
	"net/url"
	"strings"

	"github.com/fatih/color"

	forwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/resolver"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// RunDatadogConnectivityDiagnose sends requests to all known endpoints for all domains
// to check if there are connectivity issues between Datadog and these endpoints
func RunDatadogConnectivityDiagnose(writer io.Writer, noTrace bool) error {

	// Create domain resolvers
	keysPerDomain, err := utils.GetMultipleEndpoints(config.Datadog)
	if err != nil {
		return log.Error("Misconfiguration of agent endpoints: ", err)
	}

	domainResolvers := resolver.NewSingleDomainResolvers(keysPerDomain)

	client := forwarder.NewHTTPClient(config.Datadog)

	// Send requests to all endpoints for all domains
	fmt.Fprintln(writer, "\n================ Starting connectivity diagnosis ================")
	for _, domainResolver := range domainResolvers {
		sendRequestToAllEndpointOfADomain(writer, client, domainResolver, noTrace)
	}

	return nil
}

// sendRequestToAllEndpointOfADomain sends HTTP request on all endpoints for a given domain
func sendRequestToAllEndpointOfADomain(writer io.Writer, client *http.Client, domainResolver resolver.DomainResolver, noTrace bool) {

	// Go through all API Keys of a domain and send an HTTP request on each endpoint
	for _, apiKey := range domainResolver.GetAPIKeys() {

		for _, endpointInfo := range endpointsInfo {

			domain, _ := domainResolver.Resolve(endpointInfo.Endpoint)

			ctx := context.Background()
			if !noTrace {
				ctx = httptrace.WithClientTrace(context.Background(), createDiagnoseTrace(writer))
			}

			statusCode, responseBody, err := sendHTTPRequestToEndpoint(ctx, writer, client, domain, endpointInfo, apiKey)

			// Check if there is a response and if it's valid
			verifyEndpointResponse(writer, statusCode, responseBody, err)
		}
	}
}

// sendHTTPRequestToEndpoint creates an URL based on the domain and the endpoint information
// then sends an HTTP Request with the method and payload inside the endpoint information
func sendHTTPRequestToEndpoint(ctx context.Context, writer io.Writer, client *http.Client, domain string, endpointInfo endpointInfo, apiKey string) (int, []byte, error) {
	url := createEndpointURL(domain, endpointInfo)
	logURL := scrubber.ScrubLine(url)

	fmt.Fprintf(writer, "\n======== '%v' ========\n", color.BlueString(logURL))

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
		return 0, nil, fmt.Errorf("fail to read the response Body: %s", scrubber.ScrubLine(err.Error()))
	}

	return resp.StatusCode, body, nil
}

// createEndpointUrl joins a domain with an endpoint
func createEndpointURL(domain string, endpointInfo endpointInfo) string {
	return domain + endpointInfo.Endpoint.Route
}

// vertifyEndpointResponse interprets the endpoint response and displays information on if the connectivity
// check was successful or not
func verifyEndpointResponse(writer io.Writer, statusCode int, responseBody []byte, err error) {

	if err != nil {
		fmt.Fprintf(writer, "could not get a response from the endpoint : %v ====> %v\n", scrubber.ScrubLine(err.Error()), color.RedString("FAIL"))
		noResponseHints(writer, err)
		return
	}

	statusString := color.GreenString("PASS")
	if statusCode >= 400 {
		statusString = color.RedString("FAIL")
		fmt.Fprintf(writer, "Received response : '%v'\n", scrubber.ScrubLine(string(responseBody)))
	}
	fmt.Fprintf(writer, "Received status code %v from the endpoint ====> %v\n", statusCode, scrubber.ScrubLine(statusString))
}

// noResponseHints aims to give hints when the endpoint did not respond.
// For instance, when sending an HTTP request to a HAProxy endpoint configured for HTTPS
// the endpoint send an empty response. As the error 'EOF' is not very informative, it can
// be interesting to 'wrap' this error to display more context.
func noResponseHints(writer io.Writer, err error) {
	endpoint := utils.GetInfraEndpoint(config.Datadog)
	parsedURL, parseErr := url.Parse(endpoint)
	if parseErr != nil {
		fmt.Fprintf(writer, "Could not parse url '%v' : %v", scrubber.ScrubLine(endpoint), scrubber.ScrubLine(parseErr.Error()))
		return
	}

	if parsedURL.Scheme == "http" {
		if strings.Contains(err.Error(), "EOF") {
			fmt.Fprintln(writer, hintColorFunc("Hint: received an empty reply from the server. You are maybe trying to contact an HTTPS endpoint using an HTTP url : '%v'", scrubber.ScrubLine(endpoint)))
		}
	}
}
