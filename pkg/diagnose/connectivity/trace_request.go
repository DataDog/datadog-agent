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
	"fmt"
	"io"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/resolver"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/fatih/color"
)

// RunDatadogConnectivityDiagnose send requests to all known endpoints for all domains
// to check if there are connectivity issues between Datadog and these endpoints
func RunDatadogConnectivityDiagnose() error {
	// Create domain resolvers
	keysPerDomain, err := config.GetMultipleEndpoints()
	if err != nil {
		log.Error("Misconfiguration of agent endpoints: ", err)
	}

	// XXX: use NewDomainResolverWithMetricToVector ?
	domainResolvers := resolver.NewSingleDomainResolvers(keysPerDomain)

	client := forwarder.NewHTTPClient()

	// Send requests to all endpoints for all domains
	fmt.Println("\n================ Starting connectivity diagnosis ================")
	for _, domainResolver := range domainResolvers {
		sendRequestToAllEndpointOfADomain(client, domainResolver)
	}

	return nil
}

// sendRequestToAllEndpointOfADomain sends HTTP request on all endpoints for a given domain
func sendRequestToAllEndpointOfADomain(client *http.Client, domainResolver resolver.DomainResolver) {

	// Go through all API Keys of a domain and send an HTTP request on each endpoint
	for _, apiKey := range domainResolver.GetAPIKeys() {

		for _, endpointInfo := range endpointsInfo {

			domain, _ := domainResolver.Resolve(endpointInfo.Endpoint)
			sendHTTPRequestToEndpoint(client, domain, endpointInfo, apiKey)
		}
	}
}

// sendHTTPRequestToEndpoint creates an URL based on the domain and the endpoint information
// then sends an HTTP Request with the method and payload inside the endpoint information
func sendHTTPRequestToEndpoint(client *http.Client, domain string, endpointInfo EndpointInfo, apiKey string) {
	url := createEndpointURL(domain, endpointInfo, apiKey)
	logURL := scrubber.ScrubLine(url)

	fmt.Printf("\n======== '%v' ========\n", color.BlueString(logURL))

	// Create a request for the backend
	reader := bytes.NewReader(endpointInfo.Payload)
	req, err := http.NewRequest(endpointInfo.Method, url, reader)

	if err != nil {
		log.Errorf("Could not create request for transaction to invalid URL '%v' : %v", logURL, scrubber.ScrubLine(err.Error()))
	}

	// Send the request
	resp, err := client.Do(req)

	if err != nil {
		fmt.Printf("Could not send the HTTP request to '%v' : %v\n", logURL, scrubber.ScrubLine(err.Error()))
		return
	}
	defer func() { _ = resp.Body.Close() }()

	// Get the endpoint response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("Fail to read the response Body: %s", err)
		return
	}

	// Check the endpoint response
	statusString := color.GreenString("PASS")
	if resp.StatusCode != http.StatusOK {
		statusString = color.RedString("FAIL")
		fmt.Printf("Received response : '%v'\n", string(body))
	}
	fmt.Printf("Received status code %v from the endpoint ====> %v\n", resp.StatusCode, statusString)

}

// createEndpointUrl joins a domain with an endpoint and adds the apiKey to the query
// string if it is necessary for the given endpoint
func createEndpointURL(domain string, endpointInfo EndpointInfo, apiKey string) string {
	url := domain + endpointInfo.Endpoint.Route

	if endpointInfo.APIKeyInQueryString {
		url = fmt.Sprintf("%s?api_key=%s", url, apiKey)
	}

	return url
}
