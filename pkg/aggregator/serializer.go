package aggregator

import (
	"bytes"
	"encoding/json"

	log "github.com/cihub/seelog"
	"github.com/gogo/protobuf/proto"

	agentpayload "github.com/DataDog/agent-payload/go"
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
				Metric: s.Name,
				Type:   s.MType.String(),
				Host:   s.Host,
				Points: marshalPoints(s.Points),
				Tags:   s.Tags,
			})
	}

	writer, _ := log.NewConsoleWriter()
	proto.MarshalText(writer, payload)
	return proto.Marshal(payload)
}

// MarshalServiceChecks serialize check runs payload using agent-payload definition
func MarshalServiceChecks(checkRuns []*ServiceCheck) ([]byte, error) {
	payload := &agentpayload.CheckRunsPayload{
		CheckRuns: []*agentpayload.CheckRunsPayload_CheckRun{},
		Metadata:  &agentpayload.CommonMetadata{},
	}

	for _, c := range checkRuns {
		payload.CheckRuns = append(payload.CheckRuns,
			&agentpayload.CheckRunsPayload_CheckRun{
				Name:    c.CheckName,
				Host:    c.Host,
				Ts:      c.Ts,
				Status:  int32(c.Status),
				Message: c.Message,
				Tags:    c.Tags,
			})
	}

	writer, _ := log.NewConsoleWriter()
	proto.MarshalText(writer, payload)
	return proto.Marshal(payload)
}

// MarshalJSONSeries serializea timeserie to JSON so it can be sent to V1 endpoints
//FIXME(maxime): to be removed when v2 endpoints are available
func MarshalJSONSeries(series []*Serie) ([]byte, error) {
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
