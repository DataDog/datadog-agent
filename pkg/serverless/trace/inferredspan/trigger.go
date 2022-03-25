package inferredspan

import (
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// These keys are used to tell us what event type we received

type EventKeys struct {
	RequestContext RequestContextKeys `json:"requestContext"`
	HttpMethod     string             `json:"httpMethod"`
	Path           string             `json:"path"`
}

// Request_context is nested in the payload.
// We want to pull out what we need for all event types
type RequestContextKeys struct {
	Stage            string `json:"stage"`
	RouteKey         string `json:"routeKey"`
	MessageDirection string `json:"messageDirection"`
	Domain           string `json:"domainName"`
	ApiId            string `json:"apiId"`
	RequestId        string `json:"requestID"`
}

// event sources
const (
	API_GATEWAY = "apigateway"
	HTTP_API    = "http-api"
	WEBSOCKET   = "websocket"
	UNKNOWN     = "unknown"
)

func ParseEventSource(event string) (string, EventKeys) {
	var eventKeys EventKeys
	err := json.Unmarshal([]byte(event), &eventKeys)
	if err != nil {
		log.Debug("Unable to unmarshall event payload")
	}

	eventSource := UNKNOWN
	if eventKeys.RequestContext.Stage != "" {
		if eventKeys.HttpMethod != "" {
			eventSource = API_GATEWAY
		}
		if eventKeys.RequestContext.RouteKey != "" {
			eventSource = HTTP_API
		}
		if eventKeys.RequestContext.MessageDirection != "" {
			eventSource = WEBSOCKET
		}
	}
	return eventSource, eventKeys
}
