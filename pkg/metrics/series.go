// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package metrics

import (
	"bytes"
	"expvar"
	"fmt"
	"strings"

	json "github.com/json-iterator/go"

	"github.com/gogo/protobuf/proto"

	agentpayload "github.com/DataDog/agent-payload/gogen"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

var seriesExpvar = expvar.NewMap("series")

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
// Mutates the `series` slice in place
//FIXME(olivier): remove this as soon as the v1 API can handle `device` as a regular tag
func populateDeviceField(series Series) {
	for _, serie := range series {
		// make a copy of the tags array. Otherwise the underlying array won't have
		// the device tag for the Nth iteration (N>1), and the device field will
		// be lost
		var filteredTags []string

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
	// use an alias to avoid infinite recursion while serializing a Series
	type SeriesAlias Series
	populateDeviceField(series)

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

func (e Serie) String() string {
	s, err := json.Marshal(e)
	if err != nil {
		return ""
	}
	return string(s)
}
