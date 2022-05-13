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
			domain, _ := domainResolver.Resolve(endpointInfo.endpoint)

			// Create the endpoint URL and send the request
			url := createEndpointURL(domain, apiKey, endpointInfo)
			sendHTTPRequestToUrl(client, url, endpointInfo)
		}
	}
}

func createEndpointURL(domain string, apiKey string, endpointInfo EndpointInfo) string {

	url := domain + endpointInfo.endpoint.Route

	if endpointInfo.apiKeyInQueryString {
		url = fmt.Sprintf("%s?api_key=%s", url, apiKey)
	}

	return url
}

func sendHTTPRequestToUrl(client *http.Client, url string, info EndpointInfo) {
	logURL := scrubber.ScrubLine(url)

	// Create a request for the backend
	reader := bytes.NewReader(info.payload)
	req, err := http.NewRequest(info.method, url, reader)

	if err != nil {
		log.Errorf("Could not create request for transaction to invalid URL '%v' : %v", logURL, scrubber.ScrubLine(err.Error()))
	}

	//req = req.WithContext(ctx)
	//req.Header = t.Headers
	//req = req.WithContext(httptrace.WithClientTrace(context.Background(), Trace))

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

	fmt.Printf("Endpoint '%v' answers with status code %v\n", logURL, resp.StatusCode)
	fmt.Printf("Response : '%v'\n", string(body))
}
