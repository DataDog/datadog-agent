package serializer

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/gogo/protobuf/proto"

	agentpayload "github.com/DataDog/agent-payload/gogen"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func marshalPoints(points []metrics.Point) []*agentpayload.MetricsPayload_Sample_Point {
	pointsPayload := []*agentpayload.MetricsPayload_Sample_Point{}

	for _, p := range points {
		pointsPayload = append(pointsPayload, &agentpayload.MetricsPayload_Sample_Point{
			Ts:    int64(p.Ts),
			Value: p.Value,
		})
	}
	return pointsPayload
}

// MarshalSeries serialize a timeserie payload using agent-payload definition
func MarshalSeries(series []*metrics.Serie) ([]byte, string, error) {
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

	msg, err := proto.Marshal(payload)
	return msg, protobufContentType, err
}

// populateDeviceField removes any `device:` tag in the series tags and uses the value to
// populate the Serie.Device field
// Mutates the `series` slice in place
//FIXME(olivier): remove this as soon as the v1 API can handle `device` as a regular tag
func populateDeviceField(series []*metrics.Serie) {
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
func MarshalJSONSeries(series []*metrics.Serie) ([]byte, string, error) {
	populateDeviceField(series)

	data := map[string][]*metrics.Serie{
		"series": series,
	}
	reqBody := &bytes.Buffer{}
	err := json.NewEncoder(reqBody).Encode(data)
	return reqBody.Bytes(), jsonContentType, err
}
