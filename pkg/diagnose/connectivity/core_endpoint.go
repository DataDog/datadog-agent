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

	forwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/resolver"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

func init() {
	diagnosis.Register("connectivity-datadog-core-endpoints", diagnose)
}

func diagnose(diagCfg diagnosis.Config, senderManager sender.SenderManager) []diagnosis.Diagnosis {

	// Create domain resolvers
	keysPerDomain, err := utils.GetMultipleEndpoints(config.Datadog)
	if err != nil {
		return []diagnosis.Diagnosis{
			{
				Result:      diagnosis.DiagnosisSuccess,
				Name:        "Endpoints configuration",
				Diagnosis:   "Misconfiguration of agent endpoints",
				Remediation: "Please validate Agent configuration",
				RawError:    err.Error(),
			},
		}
	}

	var diagnoses []diagnosis.Diagnosis
	domainResolvers := resolver.NewSingleDomainResolvers(keysPerDomain)
	client := forwarder.NewHTTPClient(config.Datadog)

	// Send requests to all endpoints for all domains
	for _, domainResolver := range domainResolvers {
		// Go through all API Keys of a domain and send an HTTP request on each endpoint
		for _, apiKey := range domainResolver.GetAPIKeys() {
			for _, endpointInfo := range endpointsInfo {
				domain, _ := domainResolver.Resolve(endpointInfo.Endpoint)
				httpTraces := []string{}
				ctx := httptrace.WithClientTrace(context.Background(), createDiagnoseTraces(&httpTraces))

				statusCode, responseBody, logURL, err := sendHTTPRequestToEndpoint(ctx, client, domain, endpointInfo, apiKey)

				// Check if there is a response and if it's valid
				report, reportErr := verifyEndpointResponse(statusCode, responseBody, err)
				d := diagnosis.Diagnosis{
					Name: "Connectivity to " + logURL,
				}
				if reportErr == nil {
					d.Result = diagnosis.DiagnosisSuccess
					d.Diagnosis = fmt.Sprintf("Connectivity to `%s` is Ok\n%s", logURL, report)
				} else {
					d.Result = diagnosis.DiagnosisFail
					d.Diagnosis = fmt.Sprintf("Connection to `%s` failed\n%s", logURL, report)
					d.Remediation = "Please validate Agent configuration and firewall to access " + logURL
					d.RawError = reportErr.Error()
				}

				// Prepend http trace on error or if in verbose mode
				if len(httpTraces) > 0 && (diagCfg.Verbose || reportErr != nil) {
					d.Diagnosis = fmt.Sprintf("\n%s\n%s", strings.Join(httpTraces, "\n"), d.Diagnosis)
				}
				diagnoses = append(diagnoses, d)
			}
		}
	}
	return diagnoses
}

// sendHTTPRequestToEndpoint creates an URL based on the domain and the endpoint information
// then sends an HTTP Request with the method and payload inside the endpoint information
func sendHTTPRequestToEndpoint(ctx context.Context, client *http.Client, domain string, endpointInfo endpointInfo, apiKey string) (int, []byte, string, error) {
	url := createEndpointURL(domain, endpointInfo)
	logURL := scrubber.ScrubLine(url)

	// Create a request for the backend
	reader := bytes.NewReader(endpointInfo.Payload)
	req, err := http.NewRequest(endpointInfo.Method, url, reader)

	if err != nil {
		return 0, nil, logURL, fmt.Errorf("cannot create request for transaction to invalid URL '%v' : %v", logURL, scrubber.ScrubLine(err.Error()))
	}

	// Add tracing and send the request
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("DD-API-KEY", apiKey)

	resp, err := client.Do(req)

	if err != nil {
		return 0, nil, logURL, fmt.Errorf("cannot send the HTTP request to '%v' : %v", logURL, scrubber.ScrubLine(err.Error()))
	}
	defer func() { _ = resp.Body.Close() }()

	// Get the endpoint response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, logURL, fmt.Errorf("fail to read the response Body: %s", scrubber.ScrubLine(err.Error()))
	}

	return resp.StatusCode, body, logURL, nil
}

// createEndpointUrl joins a domain with an endpoint
func createEndpointURL(domain string, endpointInfo endpointInfo) string {
	return domain + endpointInfo.Endpoint.Route
}

// vertifyEndpointResponse interprets the endpoint response and displays information on if the connectivity
// check was successful or not
func verifyEndpointResponse(statusCode int, responseBody []byte, err error) (string, error) {

	if err != nil {
		return fmt.Sprintf("Could not get a response from the endpoint : %v\n%s\n",
			scrubber.ScrubLine(err.Error()), noResponseHints(err)), err
	}

	var verifyReport string = ""
	var newErr error = nil
	if statusCode >= 400 {
		newErr = fmt.Errorf("bad request")
		verifyReport = fmt.Sprintf("Received response : '%v'\n", scrubber.ScrubLine(string(responseBody)))
	}

	verifyReport += fmt.Sprintf("Received status code %v from the endpoint", statusCode)
	return verifyReport, newErr
}

// noResponseHints aims to give hints when the endpoint did not respond.
// For instance, when sending an HTTP request to a HAProxy endpoint configured for HTTPS
// the endpoint send an empty response. As the error 'EOF' is not very informative, it can
// be interesting to 'wrap' this error to display more context.
func noResponseHints(err error) string {
	endpoint := utils.GetInfraEndpoint(config.Datadog)
	parsedURL, parseErr := url.Parse(endpoint)
	if parseErr != nil {
		return fmt.Sprintf("Could not parse url '%v' : %v", scrubber.ScrubLine(endpoint), scrubber.ScrubLine(parseErr.Error()))
	}

	if parsedURL.Scheme == "http" {
		if strings.Contains(err.Error(), "EOF") {
			return fmt.Sprintf("Hint: received an empty reply from the server. You are maybe trying to contact an HTTPS endpoint using an HTTP url: '%v'\n",
				scrubber.ScrubLine(endpoint))
		}
	}

	return ""
}
