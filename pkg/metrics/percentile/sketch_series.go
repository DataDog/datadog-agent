// Unless explicitly stated otherwise all files in this repository are licensed // under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

//
// NOTE: This module contains a feature in development that is NOT supported.
//

package percentile

import (
	"bytes"
	"encoding/json"
	"expvar"

	agentpayload "github.com/DataDog/agent-payload/gogen"
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/gogo/protobuf/proto"
)

var sketchSeriesExpvar = expvar.NewMap("SketchSeries")

// Sketch represents a quantile sketch at a specific time
type Sketch struct {
	Timestamp int64   `json:"timestamp"`
	Sketch    GKArray `json:"qsketch"`
}

// SketchSeries holds an array of sketches.
type SketchSeries struct {
	Name       string          `json:"metric"`
	Tags       []string        `json:"tags"`
	Host       string          `json:"host"`
	Interval   int64           `json:"interval"`
	Sketches   []Sketch        `json:"sketches"`
	ContextKey ckey.ContextKey `json:"-"`
}

// SketchSeriesList represents a list of SketchSeries ready to be serialized
type SketchSeriesList []*SketchSeries

// NoSketchError is the error returned when not enough samples have been
//submitted to generate a sketch
type NoSketchError struct{}

func (e NoSketchError) Error() string {
	return "Not enough samples to generate sketches"
}

func marshalSketch(sketches []Sketch) []agentpayload.SketchPayload_Sketch_Distribution {
	sketchesPayload := []agentpayload.SketchPayload_Sketch_Distribution{}

	for _, s := range sketches {
		gk := s.Sketch
		v, g, delta := marshalEntries(gk.Entries)
		sketchesPayload = append(sketchesPayload,
			agentpayload.SketchPayload_Sketch_Distribution{
				Ts:    s.Timestamp,
				Cnt:   int64(gk.Count),
				Min:   gk.Min,
				Max:   gk.Max,
				Avg:   gk.Avg,
				Sum:   gk.Sum,
				V:     v,
				G:     g,
				Delta: delta,
				Buf:   gk.Incoming,
			})
	}
	return sketchesPayload
}

// Marshal serializes sketch series using protocol buffers
func (sl SketchSeriesList) Marshal() ([]byte, error) {
	payload := &agentpayload.SketchPayload{
		Sketches: []agentpayload.SketchPayload_Sketch{},
		Metadata: agentpayload.CommonMetadata{},
	}
	for _, s := range sl {
		payload.Sketches = append(payload.Sketches,
			agentpayload.SketchPayload_Sketch{
				Metric:        s.Name,
				Host:          s.Host,
				Distributions: marshalSketch(s.Sketches),
				Tags:          s.Tags,
			})
	}
	return proto.Marshal(payload)
}

// MarshalJSON serializes sketch series to JSON so it can be sent to
// v1 endpoints
func (sl SketchSeriesList) MarshalJSON() ([]byte, error) {
	data := map[string][]*SketchSeries{
		"sketch_series": sl,
	}
	reqBody := &bytes.Buffer{}
	err := json.NewEncoder(reqBody).Encode(data)
	return reqBody.Bytes(), err
}

// SplitPayload breaks the payload into times number of pieces
func (sl SketchSeriesList) SplitPayload(times int) ([]marshaler.Marshaler, error) {
	sketchSeriesExpvar.Add("TimesSplit", 1)
	// Only break it down as much as possible
	if len(sl) < times {
		sketchSeriesExpvar.Add("SketchSeriesListShorter", 1)
		times = len(sl)
	}
	splitPayloads := make([]marshaler.Marshaler, times)
	batchSize := len(sl) / times
	n := 0
	for i := 0; i < times; i++ {
		var end int
		// In many cases the batchSize is not perfect
		// so the last one will be a bit bigger or smaller than the others
		if i < times-1 {
			end = n + batchSize
		} else {
			end = len(sl)
		}
		newSL := SketchSeriesList(sl[n:end])
		splitPayloads[i] = newSL
		n += batchSize
	}
	return splitPayloads, nil
}
