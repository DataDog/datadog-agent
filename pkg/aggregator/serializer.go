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
