package connectivity

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/resolver"
	"github.com/DataDog/datadog-agent/pkg/forwarder/endpoints"
	"github.com/DataDog/datadog-agent/pkg/forwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

func RunDatadogConnectivityChecks() error {

	// Build endpoints
	keysPerDomain, err := config.GetMultipleEndpoints()
	if err != nil {
		log.Error("Misconfiguration of agent endpoints: ", err)
	}

	endpoint := endpoints.V1SeriesEndpoint
	// Create a domain resolver
	// Should we use NewDomainResolverWithMetricToVector ?
	domainResolvers := resolver.NewSingleDomainResolvers(keysPerDomain)

	urls := getAllUrlForAnEndpoint(domainResolvers, endpoint, true)
	fmt.Printf("'%v'\n", urls)

	return fmt.Errorf("this command is not implemented yet")
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

	return scrubber.ScrubLine(url)
}
