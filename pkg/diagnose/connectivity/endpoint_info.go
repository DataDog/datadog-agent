package connectivity

import (
	"github.com/DataDog/datadog-agent/pkg/forwarder/endpoints"
	"github.com/DataDog/datadog-agent/pkg/forwarder/transaction"
)

type EndpointInfo struct {
	endpoint            transaction.Endpoint
	payload             []byte
	method              string
	apiKeyInQueryString bool
}

var (
	apiKeyInQueryString = true

	emptyPayload = []byte("")

	// TODO : add more endpoints info and a filtering function to only keep used endpoints
	V1SeriesEndpointInfo   = EndpointInfo{endpoints.V1SeriesEndpoint, emptyPayload, "POST", apiKeyInQueryString}
	V1ValidateEndpointInfo = EndpointInfo{endpoints.V1ValidateEndpoint, emptyPayload, "GET", apiKeyInQueryString}

	endpointsInfo = []EndpointInfo{V1SeriesEndpointInfo, V1ValidateEndpointInfo}
)
