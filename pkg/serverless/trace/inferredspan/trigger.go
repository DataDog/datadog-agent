package inferredspan

import (
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// These keys tell us what event type we received
// If adding a new event type, place the event specific key here
type EventKeys struct {
	RequestContext RequestContextKeys `json:"requestContext"`
	HttpMethod     string             `json:"httpMethod"`
}

type RequestContextKeys struct {
	Stage            string `json:"stage"`
	RouteKey         string `json:"routeKey"`
	MessageDirection string `json:"messageDirection"`
	Domain           string `json:"domainName"`
}

// event sources
const (
	API_GATEWAY = "apigateway"
	HTTP_API    = "http-api"
	WEBSOCKET   = "websocket"
	UNKNOWN     = "unknown"
)

func ParseEventSource(event string) string {
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
	return eventSource
}
