package inferredspan

import (
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ParseEventSource parses the event payload, and based on
// specific keys in the payload, determines and sets the event source.
func ParseEventSource(event string) (string, EventKeys) {
	var eventKeys EventKeys
	log.Debug("Attempting to parse the event for inferred spans")
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
