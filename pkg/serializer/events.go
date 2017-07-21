package serializer

import (
	"bytes"
	"encoding/json"

	"github.com/gogo/protobuf/proto"

	agentpayload "github.com/DataDog/agent-payload/gogen"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// MarshalEvents serialize events payload using agent-payload definition
func MarshalEvents(events []*metrics.Event) ([]byte, string, error) {
	payload := &agentpayload.EventsPayload{
		Events:   []*agentpayload.EventsPayload_Event{},
		Metadata: &agentpayload.CommonMetadata{},
	}

	for _, e := range events {
		payload.Events = append(payload.Events,
			&agentpayload.EventsPayload_Event{
				Title:          e.Title,
				Text:           e.Text,
				Ts:             e.Ts,
				Priority:       string(e.Priority),
				Host:           e.Host,
				Tags:           e.Tags,
				AlertType:      string(e.AlertType),
				AggregationKey: e.AggregationKey,
				SourceTypeName: e.SourceTypeName,
			})
	}

	msg, err := proto.Marshal(payload)
	return msg, protobufContentType, err
}

// MarshalJSONEvents serializes events to JSON so it can be sent to the Agent 5 intake
// (we don't use the v1 event endpoint because it only supports 1 event per payload)
//FIXME(olivier): to be removed when v2 endpoints are available
func MarshalJSONEvents(events []metrics.Event, apiKey string, hostname string) ([]byte, string, error) {
	// Regroup events by their source type name
	eventsBySourceType := make(map[string][]metrics.Event)
	for _, event := range events {
		sourceTypeName := event.SourceTypeName
		if sourceTypeName == "" {
			sourceTypeName = "api"
		}

		eventsBySourceType[sourceTypeName] = append(eventsBySourceType[sourceTypeName], event)
	}

	// Build intake payload containing events and serialize
	data := map[string]interface{}{
		"apiKey":           apiKey, // legacy field, it isn't actually used by the backend
		"events":           eventsBySourceType,
		"internalHostname": hostname,
	}
	reqBody := &bytes.Buffer{}
	err := json.NewEncoder(reqBody).Encode(data)
	return reqBody.Bytes(), jsonContentType, err
}
