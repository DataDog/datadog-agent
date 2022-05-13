package connectivity

import (
	"github.com/DataDog/datadog-agent/pkg/forwarder/endpoints"
	"github.com/DataDog/datadog-agent/pkg/forwarder/transaction"
)

type EndpointInfo struct {
	endpoint            transaction.Endpoint
	apiKeyInQueryString bool
	payload             []byte
	method              string
}

var (
	emptyPayload = []byte("")

	V1SeriesEndpointInfo   = EndpointInfo{endpoints.V1SeriesEndpoint, true, emptyPayload, "POST"}
	V1ValidateEndpointInfo = EndpointInfo{endpoints.V1ValidateEndpoint, true, emptyPayload, "GET"}

	endpointsInfo = []EndpointInfo{V1SeriesEndpointInfo, V1ValidateEndpointInfo}
)
