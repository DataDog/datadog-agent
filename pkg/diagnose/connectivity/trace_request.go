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
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

func init() {
	diagnosis.Register("connectivity-datadog-core-endpoints", diagnose)
}

func diagnose(diagCfg diagnosis.DiagnoseConfig) []diagnosis.Diagnosis {

	// Create domain resolvers
	keysPerDomain, err := utils.GetMultipleEndpoints(config.Datadog)
	if err != nil {
		return []diagnosis.Diagnosis{
			{
				Result:      diagnosis.DiagnosisSuccess,
				Name:        "Endpoints configuration",
				Diagnosis:   "Misconfiguration of agent endpoints",
				Remediation: "Please validate Agent configuration",
				RawError:    err,
			},
		}
	}

	diagnoses := make([]diagnosis.Diagnosis, 0)

	domainResolvers := resolver.NewSingleDomainResolvers(keysPerDomain)

	client := forwarder.NewHTTPClient(config.Datadog)

	// Send requests to all endpoints for all domains
	for _, domainResolver := range domainResolvers {
		// Go through all API Keys of a domain and send an HTTP request on each endpoint
		for _, apiKey := range domainResolver.GetAPIKeys() {

			for _, endpointInfo := range endpointsInfo {

				domain, _ := domainResolver.Resolve(endpointInfo.Endpoint)

				ctx := context.Background()
				statusCode, responseBody, logURL, err := sendHTTPRequestToEndpoint(ctx, client, domain, endpointInfo, apiKey)

				// Check if there is a response and if it's valid
				report, reportErr := verifyEndpointResponse(statusCode, responseBody, err)
				name := "Connectivity to " + logURL
				if reportErr == nil {
					diagnoses = append(diagnoses, diagnosis.Diagnosis{
						Result:    diagnosis.DiagnosisSuccess,
						Name:      name,
						Diagnosis: fmt.Sprintf("Connectivity to `%s` is Ok\n%s", logURL, report),
					})
				} else {
					diagnoses = append(diagnoses, diagnosis.Diagnosis{
						Result:      diagnosis.DiagnosisFail,
						Name:        name,
						Diagnosis:   fmt.Sprintf("Connection to `%s` is falied\n%s", logURL, report),
						Remediation: "Please validate Agent configuration and firewall to access " + logURL,
						RawError:    err,
					})
				}
			}
		}
	}

	return diagnoses
}

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
		// Go through all API Keys of a domain and send an HTTP request on each endpoint
		for _, apiKey := range domainResolver.GetAPIKeys() {

			for _, endpointInfo := range endpointsInfo {

				domain, _ := domainResolver.Resolve(endpointInfo.Endpoint)

				ctx := context.Background()
				if !noTrace {
					ctx = httptrace.WithClientTrace(context.Background(), createDiagnoseTrace(writer))
				}

				statusCode, responseBody, logURL, err := sendHTTPRequestToEndpoint(ctx, client, domain, endpointInfo, apiKey)
				fmt.Fprintf(writer, "\n======== '%v' ========\n", color.BlueString(logURL))

				// Check if there is a response and if it's valid
				report, reportErr := verifyEndpointResponse(statusCode, responseBody, err)

				var statusString string
				if reportErr == nil {
					statusString = color.GreenString("PASS")
				} else {
					statusString = color.RedString("FAIL")
				}
				fmt.Fprintf(writer, "%s====> %v\n", report, statusString)
			}
		}
	}

	return nil
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
		newErr = fmt.Errorf("Bad Request")
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
			return fmt.Sprintf("Hint: received an empty reply from the server. You are maybe trying to contact an HTTPS endpoint using an HTTP url: '%v'\n", scrubber.ScrubLine(endpoint))
		}
	}

	return ""
}
