// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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
		if eventKeys.HTTPMethod != "" {
			eventSource = APIGATEWAY
		}
		if eventKeys.RequestContext.RouteKey != "" {
			eventSource = HTTPAPI
		}
		if eventKeys.RequestContext.MessageDirection != "" {
			eventSource = WEBSOCKET
		}
	}
	return eventSource, eventKeys
}
