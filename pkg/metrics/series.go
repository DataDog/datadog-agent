// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package metrics

import (
	"bytes"
	"encoding/json"
	"errors"
	"expvar"
	"fmt"
	"strings"
	"unsafe"

	agentpayload "github.com/DataDog/agent-payload/gogen"
	"github.com/gogo/protobuf/proto"
	jsoniter "github.com/json-iterator/go"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

var seriesExpvar = expvar.NewMap("series")

var marshaller = jsoniter.Config{
	EscapeHTML:                    false,
	ObjectFieldMustBeSimpleString: true,
}.Froze()

var (
	jsonSeparator  = []byte(",")
	jsonArrayStart = []byte("[")
	jsonArrayEnd   = []byte("]")
)

// Point represents a metric value at a specific time
type Point struct {
	Ts    float64
	Value float64
}

// MarshalJSON return a Point as an array of value (to be compatible with v1 API)
// FIXME(maxime): to be removed when v2 endpoints are available
// Note: it is not used with jsoniter, encodePoints takes over
func (p *Point) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("[%v, %v]", int64(p.Ts), p.Value)), nil
}

// Serie holds a timeseries (w/ json serialization to DD API format)
type Serie struct {
	Name           string          `json:"metric"`
	Points         []Point         `json:"points"`
	Tags           []string        `json:"tags"`
	Host           string          `json:"host"`
	Device         string          `json:"device,omitempty"` // FIXME(olivier): remove as soon as the v1 API can handle `device` as a regular tag
	MType          APIMetricType   `json:"type"`
	Interval       int64           `json:"interval"`
	SourceTypeName string          `json:"source_type_name,omitempty"`
	ContextKey     ckey.ContextKey `json:"-"`
	NameSuffix     string          `json:"-"`
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
//FIXME(olivier): remove this as soon as the v1 API can handle `device` as a regular tag
func populateDeviceField(serie *Serie) {
	if !hasDeviceTag(serie) {
		return
	}
	// make a copy of the tags array. Otherwise the underlying array won't have
	// the device tag for the Nth iteration (N>1), and the deice field will
	// be lost
	filteredTags := make([]string, 0, len(serie.Tags))

	for _, tag := range serie.Tags {
		if strings.HasPrefix(tag, "device:") {
			serie.Device = tag[7:]
		} else {
			filteredTags = append(filteredTags, tag)
		}
	}
	serie.Tags = filteredTags
}

// hasDeviceTag checks whether a series contains a device tag
func hasDeviceTag(serie *Serie) bool {
	for _, tag := range serie.Tags {
		if strings.HasPrefix(tag, "device:") {
			return true
		}
	}
	return false
}

// MarshalJSON serializes timeseries to JSON so it can be sent to V1 endpoints
//FIXME(maxime): to be removed when v2 endpoints are available
func (series Series) MarshalJSON() ([]byte, error) {
	// use an alias to avoid infinite recursion while serializing a Series
	type SeriesAlias Series
	for _, serie := range series {
		populateDeviceField(serie)
	}

	data := map[string][]*Serie{
		"series": SeriesAlias(series),
	}
	reqBody := &bytes.Buffer{}
	err := json.NewEncoder(reqBody).Encode(data)
	return reqBody.Bytes(), err
}

// SplitPayload breaks the payload into, at least, "times" number of pieces
func (series Series) SplitPayload(times int) ([]marshaler.Marshaler, error) {
	seriesExpvar.Add("TimesSplit", 1)

	// We need to split series without splitting metrics across multiple
	// payload. So we first group series by metric name.
	metricsPerName := map[string]Series{}
	for _, s := range series {
		if _, ok := metricsPerName[s.Name]; ok {
			metricsPerName[s.Name] = append(metricsPerName[s.Name], s)
		} else {
			metricsPerName[s.Name] = Series{s}
		}
	}

	// if we only have one metric name we cannot split further
	if len(metricsPerName) == 1 {
		seriesExpvar.Add("SplitMetricsTooBig", 1)
		return nil, fmt.Errorf("Cannot split metric '%s' into %d payload (it contains %d series)", series[0].Name, times, len(series))
	}

	nbSeriesPerPayload := len(series) / times

	payloads := []marshaler.Marshaler{}
	current := Series{}
	for _, m := range metricsPerName {
		// If on metric is bigger than the targeted size we directly
		// add it as a payload.
		if len(m) >= nbSeriesPerPayload {
			payloads = append(payloads, m)
			continue
		}

		// Then either append to the current payload if "m" is small
		// enough or flush the current payload and start a new one.
		// This may result in more than twice the number of payloads
		// asked for but is "good enough" and will loop only once
		// through metricsPerName
		if len(current)+len(m) < nbSeriesPerPayload {
			current = append(current, m...)
		} else {
			payloads = append(payloads, current)
			current = m
		}
	}
	if len(current) != 0 {
		payloads = append(payloads, current)
	}
	return payloads, nil
}

// UnmarshalJSON is a custom unmarshaller for Point (used for testing)
func (p *Point) UnmarshalJSON(buf []byte) error {
	tmp := []interface{}{&p.Ts, &p.Value}
	wantLen := len(tmp)
	if err := json.Unmarshal(buf, &tmp); err != nil {
		return err
	}
	if len(tmp) != wantLen {
		return fmt.Errorf("wrong number of fields in Point: %d != %d", len(tmp), wantLen)
	}
	return nil
}

// String could be used for debug logging
func (e Serie) String() string {
	s, err := json.Marshal(e)
	if err != nil {
		return ""
	}
	return string(s)
}

//// The following methods implement the StreamJSONMarshaler interface
//// for support of the enable_stream_payload_serialization option.

// JSONHeader prints the payload header for this type
func (series Series) JSONHeader() []byte {
	return []byte(`{"series":[`)
}

// Len returns the number of items to marshal
func (series Series) Len() int {
	return len(series)
}

// JSONItem prints the json representation of an item
func (series Series) JSONItem(i int) ([]byte, error) {
	if i < 0 || i > len(series)-1 {
		return nil, errors.New("out of range")
	}
	populateDeviceField(series[i])
	return marshaller.Marshal(series[i])
}

// JSONFooter prints the payload footer for this type
func (series Series) JSONFooter() []byte {
	return []byte(`]}`)
}

// DescribeItem returns a text description for logs
func (series Series) DescribeItem(i int) string {
	if i < 0 || i > len(series)-1 {
		return "out of range"
	}
	return fmt.Sprintf("name %q, %d points", series[i].Name, len(series[i].Points))
}

// encodePoints is registered to serialize a Point array with
// limited reflections and heap allocations.
// Called when using jsoniter
func encodePoints(ptr unsafe.Pointer, stream *jsoniter.Stream) {
	if ptr == nil {
		stream.WriteEmptyArray()
	}

	points := *(*[]Point)(ptr)
	var needComa bool

	stream.WriteArrayStart()
	for _, p := range points {
		if needComa {
			stream.WriteMore()
		} else {
			needComa = true
		}
		stream.WriteArrayStart()
		stream.WriteInt64(int64(p.Ts))
		stream.WriteMore()
		stream.WriteFloat64(p.Value)
		stream.WriteArrayEnd()
	}
	stream.WriteArrayEnd()
}

func init() {
	jsoniter.RegisterTypeEncoderFunc(
		"[]metrics.Point",
		encodePoints,
		func(ptr unsafe.Pointer) bool { return ptr == nil },
	)
}
