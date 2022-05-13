package connectivity

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/resolver"
	"github.com/DataDog/datadog-agent/pkg/forwarder/transaction"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

func RunDatadogConnectivityChecks() error {

	// Build endpoints
	keysPerDomain, err := config.GetMultipleEndpoints()
	if err != nil {
		log.Error("Misconfiguration of agent endpoints: ", err)
	}
	domainResolvers := resolver.NewSingleDomainResolvers(keysPerDomain)

	client := newHTTPClient()

	for _, endpointInfo := range endpointsInfo {

		urls := getAllUrlForAnEndpoint(domainResolvers, endpointInfo.endpoint, endpointInfo.apiKeyInQueryString)

		for _, url := range urls {
			sendHTTPRequestToEndpoint(client, url, endpointInfo.method, endpointInfo.payload)
		}
	}

	return nil
}

func getAllUrlForAnEndpoint(domainResolvers map[string]resolver.DomainResolver, endpoint transaction.Endpoint, apiKeyInQueryString bool) []string {

	urls := make([]string, 0)

	for _, resolver := range domainResolvers {
		for _, apiKey := range resolver.GetAPIKeys() {
			domain, _ := resolver.Resolve(endpoint)
			url := createEndpointURL(domain, endpoint, apiKey, apiKeyInQueryString)

			urls = append(urls, url)
		}
	}

	return urls
}

func createEndpointURL(domain string, endpoint transaction.Endpoint, apiKey string, apiKeyInQueryString bool) string {

	url := domain + endpoint.Route

	if apiKeyInQueryString {
		url = fmt.Sprintf("%s?api_key=%s", url, apiKey)
	}

	return url
}

func newHTTPClient() *http.Client {
	transport := httputils.CreateHTTPTransport()

	return &http.Client{
		Timeout:   config.Datadog.GetDuration("forwarder_timeout") * time.Second,
		Transport: transport,
	}
}

func sendHTTPRequestToEndpoint(client *http.Client, url string, method string, payload []byte) {
	logURL := scrubber.ScrubLine(url)
	reader := bytes.NewReader(payload)

	// TODO: check allowed method
	req, err := http.NewRequest(method, url, reader)

	if err != nil {
		log.Errorf("Could not create request for transaction to invalid URL %q (dropping transaction): %s", url, err)
	}

	//req = req.WithContext(ctx)
	//req.Header = t.Headers
	resp, err := client.Do(req)

	if err != nil {
		fmt.Printf("Could not send the HTTP request to '%v'\n", logURL)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("Fail to read the response Body: %s", err)
	}

	fmt.Printf("Endpoint '%v' answers with status code %v\n", logURL, resp.StatusCode)
	fmt.Printf("Response : '%v'\n", string(body))
}
