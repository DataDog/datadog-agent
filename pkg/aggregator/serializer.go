package aggregator

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/gogo/protobuf/proto"

	agentpayload "github.com/DataDog/agent-payload/gogen"
)

func marshalPoints(points []Point) []*agentpayload.MetricsPayload_Sample_Point {
	pointsPayload := []*agentpayload.MetricsPayload_Sample_Point{}

	for _, p := range points {
		pointsPayload = append(pointsPayload, &agentpayload.MetricsPayload_Sample_Point{
			Ts:    p.Ts,
			Value: p.Value,
		})
	}
	return pointsPayload
}

// MarshalSeries serialize a timeserie payload using agent-payload definition
func MarshalSeries(series []*Serie) ([]byte, error) {
	payload := &agentpayload.MetricsPayload{
		Samples:  []*agentpayload.MetricsPayload_Sample{},
		Metadata: &agentpayload.CommonMetadata{},
	}

	for _, s := range series {
		payload.Samples = append(payload.Samples,
			&agentpayload.MetricsPayload_Sample{
				Metric:         s.Name,
				Type:           s.MType.String(),
				Host:           s.Host,
				Points:         marshalPoints(s.Points),
				Tags:           s.Tags,
				SourceTypeName: s.SourceTypeName,
			})
	}

	return proto.Marshal(payload)
}

// MarshalServiceChecks serialize check runs payload using agent-payload definition
func MarshalServiceChecks(checkRuns []*ServiceCheck) ([]byte, error) {
	payload := &agentpayload.ServiceChecksPayload{
		ServiceChecks: []*agentpayload.ServiceChecksPayload_ServiceCheck{},
		Metadata:      &agentpayload.CommonMetadata{},
	}

	for _, c := range checkRuns {
		payload.ServiceChecks = append(payload.ServiceChecks,
			&agentpayload.ServiceChecksPayload_ServiceCheck{
				Name:    c.CheckName,
				Host:    c.Host,
				Ts:      c.Ts,
				Status:  int32(c.Status),
				Message: c.Message,
				Tags:    c.Tags,
			})
	}

	return proto.Marshal(payload)
}

// MarshalEvents serialize events payload using agent-payload definition
func MarshalEvents(events []*Event) ([]byte, error) {
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

	return proto.Marshal(payload)
}

// populateDeviceField removes any `device:` tag in the series tags and uses the value to
// populate the Serie.Device field
// Mutates the `series` slice in place
//FIXME(olivier): remove this as soon as the v1 API can handle `device` as a regular tag
func populateDeviceField(series []*Serie) {
	for _, serie := range series {
		filteredTags := serie.Tags[:0] // use the same underlying array
		for _, tag := range serie.Tags {
			if strings.HasPrefix(tag, "device:") {
				serie.Device = tag[7:]
			} else {
				filteredTags = append(filteredTags, tag)
			}
		}
		serie.Tags = filteredTags
	}
}

// MarshalJSONSeries serializes timeseries to JSON so it can be sent to V1 endpoints
//FIXME(maxime): to be removed when v2 endpoints are available
func MarshalJSONSeries(series []*Serie) ([]byte, error) {
	populateDeviceField(series)

	data := map[string][]*Serie{
		"series": series,
	}
	reqBody := &bytes.Buffer{}
	err := json.NewEncoder(reqBody).Encode(data)
	return reqBody.Bytes(), err
}

// MarshalJSONServiceChecks serializes service checks to JSON so it can be sent to V1 endpoints
//FIXME(olivier): to be removed when v2 endpoints are available
func MarshalJSONServiceChecks(serviceChecks []ServiceCheck) ([]byte, error) {
	reqBody := &bytes.Buffer{}
	err := json.NewEncoder(reqBody).Encode(serviceChecks)
	return reqBody.Bytes(), err
}

// MarshalJSONEvents serializes events to JSON so it can be sent to the Agent 5 intake
// (we don't use the v1 event endpoint because it only supports 1 event per payload)
//FIXME(olivier): to be removed when v2 endpoints are available
func MarshalJSONEvents(events []Event, apiKey string, hostname string) ([]byte, error) {
	// Regroup events by their source type name
	eventsBySourceType := make(map[string][]Event)
	for _, event := range events {
		sourceTypeName := event.SourceTypeName
		if sourceTypeName == "" {
			sourceTypeName = "api"
		}

		eventsBySourceType[sourceTypeName] = append(eventsBySourceType[sourceTypeName], event)
	}

	// Build intake payload containing events and serialize
	data := map[string]interface{}{
		"apiKey":           apiKey,
		"events":           eventsBySourceType,
		"internalHostname": hostname,
	}
	reqBody := &bytes.Buffer{}
	err := json.NewEncoder(reqBody).Encode(data)
	return reqBody.Bytes(), err
}
