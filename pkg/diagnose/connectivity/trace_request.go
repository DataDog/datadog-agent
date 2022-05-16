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
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/resolver"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
	"github.com/fatih/color"
)

func newHTTPClient() *http.Client {
	transport := httputils.CreateHTTPTransport()

	return &http.Client{
		Timeout:   config.Datadog.GetDuration("forwarder_timeout") * time.Second,
		Transport: transport,
	}
}

func RunDatadogConnectivityChecks() error {

	// Create domain resolvers
	keysPerDomain, err := config.GetMultipleEndpoints()
	if err != nil {
		log.Error("Misconfiguration of agent endpoints: ", err)
	}
	domainResolvers := resolver.NewSingleDomainResolvers(keysPerDomain)

	client := newHTTPClient()

	// Send requests to all endpoints for all domains
	for _, domainResolver := range domainResolvers {
		sendRequestToAllEndpointOfADomain(client, domainResolver)
	}

	return nil
}

func sendRequestToAllEndpointOfADomain(client *http.Client, domainResolver resolver.DomainResolver) {

	for _, apiKey := range domainResolver.GetAPIKeys() {

		for _, endpointInfo := range endpointsInfo {
			domain, _ := domainResolver.Resolve(endpointInfo.Endpoint)

			// Create the endpoint URL and send the request
			url := createEndpointURL(domain, apiKey, endpointInfo)
			sendHTTPRequestToUrl(client, url, endpointInfo)
		}
	}
}

func createEndpointURL(domain string, apiKey string, endpointInfo EndpointInfo) string {

	url := domain + endpointInfo.Endpoint.Route

	if endpointInfo.ApiKeyInQueryString {
		url = fmt.Sprintf("%s?api_key=%s", url, apiKey)
	}

	return url
}

func sendHTTPRequestToUrl(client *http.Client, url string, info EndpointInfo) {
	logURL := scrubber.ScrubLine(url)

	fmt.Printf("\n======== '%v' ========\n", logURL)

	// Create a request for the backend
	reader := bytes.NewReader(info.Payload)
	req, err := http.NewRequest(info.Method, url, reader)

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

	// Check the endpoint response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("Fail to read the response Body: %s", err)
		return
	}

	statusString := color.GreenString("PASS")
	if resp.StatusCode != http.StatusOK {
		statusString = color.RedString("FAIL")
		//fmt.Printf("Endpoint '%v' answers with status code %v\n", logURL, resp.StatusCode)
		fmt.Printf("Received response : '%v'\n", string(body))
	}
	fmt.Printf("Received status code %v from the endpoint ====> %v\n", resp.StatusCode, statusString)

}
