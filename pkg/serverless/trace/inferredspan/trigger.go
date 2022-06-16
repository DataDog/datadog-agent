// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inferredspan

import (
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// parseEvent parses the event payload.
func parseEvent(event string) EventKeys {
	var eventKeys EventKeys
	log.Debug("Attempting to parse the event for inferred spans")
	err := json.Unmarshal([]byte(event), &eventKeys)
	if err != nil {
		log.Errorf("Unable to unmarshall event payload : %s", err)
	}
	return eventKeys
}

// extractEventSource determines the event source from the
// unmarshalled event payload
func (eventKeys *EventKeys) extractEventSource() string {
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

	record := eventKeys.getFirstRecord()
	if record != nil {
		switch record.EventSource {
		case SNSType:
			eventSource = SNS
		}
	}
	return eventSource
}

// Checks if the Records array is available and returns the first entry
func (eventKeys *EventKeys) getFirstRecord() *RecordKeys {
	if eventKeys.Records != nil && len(eventKeys.Records) > 0 {
		return eventKeys.Records[0]
	}
	return nil
}
