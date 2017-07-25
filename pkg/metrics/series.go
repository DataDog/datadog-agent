package metrics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gogo/protobuf/proto"

	agentpayload "github.com/DataDog/agent-payload/gogen"
)

// Point represents a metric value at a specific time
type Point struct {
	Ts    float64
	Value float64
}

// MarshalJSON return a Point as an array of value (to be compatible with v1 API)
// FIXME(maxime): to be removed when v2 endpoints are available
func (p *Point) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("[%v, %v]", int64(p.Ts), p.Value)), nil
}

// Serie holds a timeseries (w/ json serialization to DD API format)
type Serie struct {
	Name           string        `json:"metric"`
	Points         []Point       `json:"points"`
	Tags           []string      `json:"tags"`
	Host           string        `json:"host"`
	Device         string        `json:"device,omitempty"` // FIXME(olivier): remove as soon as the v1 API can handle `device` as a regular tag
	MType          APIMetricType `json:"type"`
	Interval       int64         `json:"interval"`
	SourceTypeName string        `json:"source_type_name,omitempty"`
	ContextKey     string        `json:"-"`
	NameSuffix     string        `json:"-"`
}

// Series represents a list of Serie ready to be serialize
type Series []*Serie

func marshalPoints(points []Point) []*agentpayload.MetricsPayload_Sample_Point {
	pointsPayload := []*agentpayload.MetricsPayload_Sample_Point{}

	for _, p := range points {
		pointsPayload = append(pointsPayload, &agentpayload.MetricsPayload_Sample_Point{
			Ts:    int64(p.Ts),
			Value: p.Value,
		})
	}
	return pointsPayload
}

// Marshal serialize timeseries using agent-payload definition
func (series Series) Marshal() ([]byte, error) {
	payload := &agentpayload.MetricsPayload{
		Samples:  []*agentpayload.MetricsPayload_Sample{},
		Metadata: &agentpayload.CommonMetadata{},
	}

	for _, serie := range series {
		payload.Samples = append(payload.Samples,
			&agentpayload.MetricsPayload_Sample{
				Metric:         serie.Name,
				Type:           serie.MType.String(),
				Host:           serie.Host,
				Points:         marshalPoints(serie.Points),
				Tags:           serie.Tags,
				SourceTypeName: serie.SourceTypeName,
			})
	}

	return proto.Marshal(payload)
}

// populateDeviceField removes any `device:` tag in the series tags and uses the value to
// populate the Serie.Device field
// Mutates the `series` slice in place
//FIXME(olivier): remove this as soon as the v1 API can handle `device` as a regular tag
func populateDeviceField(series Series) {
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

// MarshalJSON serializes timeseries to JSON so it can be sent to V1 endpoints
//FIXME(maxime): to be removed when v2 endpoints are available
func (series Series) MarshalJSON() ([]byte, error) {
	// use an alias to avoid infinit recursion while serializing a Series
	type SeriesAlias Series
	populateDeviceField(series)

	data := map[string][]*Serie{
		"series": SeriesAlias(series),
	}
	reqBody := &bytes.Buffer{}
	err := json.NewEncoder(reqBody).Encode(data)
	return reqBody.Bytes(), err
}
