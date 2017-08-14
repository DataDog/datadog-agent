// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package metrics

import (
	"bytes"
	"encoding/json"
	"expvar"
	"fmt"
	"strings"

	"github.com/gogo/protobuf/proto"

	agentpayload "github.com/DataDog/agent-payload/gogen"
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

// SplitPayload breaks the payload into times number of pieces
func (series Series) SplitPayload(times int) ([]marshaler.Marshaler, error) {
	seriesExpvar.Add("TimesSplit", 1)
	var splitPayloads []marshaler.Marshaler
	// Only split points if there isn't any other choice
	if len(series) == 1 {
		seriesExpvar.Add("SplitPoints", 1)
		s := series[0]
		points := s.Points
		if len(points) < times {
			seriesExpvar.Add("PointsShorter", 1)
			times = len(points)
		}
		splitPayloads = make([]marshaler.Marshaler, times)
		batchSize := len(points) / times
		var n = 0
		for i := 0; i < times; i++ {
			// make "times" new series
			var newS = &Serie{
				Name:           s.Name,
				Tags:           s.Tags,
				Host:           s.Host,
				Device:         s.Device,
				MType:          s.MType,
				Interval:       s.Interval,
				SourceTypeName: s.SourceTypeName,
			}
			var end int
			if i < times-1 {
				end = n + batchSize
			} else {
				end = len(points)
			}
			newPoints := []Point(points[n:end])
			newS.Points = newPoints
			// create the new series
			splitPayloads[i] = Series{newS}
			n += batchSize
		}
	} else {
		seriesExpvar.Add("SplitSeries", 1)
		// Only split the series as much as is possible
		if len(series) < times {
			seriesExpvar.Add("SeriesShorter", 1)
			times = len(series)
		}
		splitPayloads = make([]marshaler.Marshaler, times)
		batchSize := len(series) / times
		var n = 0
		// loop through the splits
		for i := 0; i < times; i++ {
			var end int
			if i < times-1 {
				end = n + batchSize
			} else {
				end = len(series)
			}
			newSeries := Series(series[n:end])
			// assign the new series to its place in the series
			splitPayloads[i] = newSeries
			n += batchSize
		}
	}
	return splitPayloads, nil
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
