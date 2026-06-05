// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"bytes"
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// SketchKind discriminates the type of data in a SketchPoint.
// Defined as a type alias so that implementations in other modules
// (e.g. pkg/util/quantile) can satisfy the SketchData interface
// by returning plain uint8 without importing this package.
type SketchKind = uint8

const (
	// SketchKindDDSketch is the default DDSketch type.
	SketchKindDDSketch SketchKind = 0
	// SketchKindExplicitBound carries raw OTel explicit-bound histogram points.
	SketchKindExplicitBound SketchKind = 1
	// SketchKindExponential carries raw OTel exponential histogram points.
	SketchKindExponential SketchKind = 2
)

// A SketchSeries is a timeseries of quantile sketches or native OTel histograms.
// The Kind of data is determined per-point via SketchPoint.Sketch.Kind().
type SketchSeries struct {
	Name       string               `json:"metric"`
	Tags       tagset.CompositeTags `json:"tags"`
	Host       string               `json:"host"`
	Interval   int64                `json:"interval"`
	Points     []SketchPoint        `json:"points"`
	ContextKey ckey.ContextKey      `json:"-"`
	NoIndex    bool                 `json:"-"` // This is only used by api V2
	Source     MetricSource         `json:"-"` // This is only used by api V2
}

// NumPoints returns the number of data points.
func (sl *SketchSeries) NumPoints() int {
	return len(sl.Points)
}

// GetName returns the name of the SketchSeries
func (sl *SketchSeries) GetName() string {
	return sl.Name
}

// String returns the JSON representation of a SketchSeries as a string
// or an empty string in case of an error
func (sl SketchSeries) String() string {
	reqBody := &bytes.Buffer{}
	_ = json.NewEncoder(reqBody).Encode(sl)
	return reqBody.String()
}

// SketchData is the interface the serializer uses to read sketch content.
// It is satisfied by *quantile.Sketch (DDSketch) and the OTel histogram
// wrapper types ExplicitBoundHistogramPoint / ExponentialHistogramPoint.
//
// All method signatures use primitive types so that implementations in
// separate Go modules (e.g. pkg/util/quantile) can satisfy it without
// importing this package.
type SketchData interface {
	// Kind returns the sketch type: SketchKindDDSketch, SketchKindExplicitBound, or SketchKindExponential.
	Kind() SketchKind

	// Cols returns bin keys and per-bin counts in ascending key order.
	// Only meaningful for DDSketch; other kinds return nil, nil.
	Cols() (k []int32, n []uint32)

	// BasicStats returns the five summary fields used in the wire format.
	// Only meaningful for DDSketch; other kinds return zeros.
	BasicStats() (cnt int64, min, max, sum, avg float64)

	// SummaryValues returns min, max, sum for the data point.
	// Used by the V3 serializer to determine the narrowest value encoding
	// across all points in a series before writing.
	SummaryValues() (min, max, sum float64)
}

// A SketchPoint represents a quantile sketch at a specific time
type SketchPoint struct {
	Sketch SketchData `json:"sketch"`
	Ts     int64      `json:"ts"`
}

// SketchSeriesList is a collection of SketchSeries
type SketchSeriesList []*SketchSeries

// MarshalJSON serializes sketch series to JSON.
func (sl SketchSeriesList) MarshalJSON() ([]byte, error) {
	type SketchSeriesAlias SketchSeriesList
	data := map[string]SketchSeriesAlias{
		"sketches": SketchSeriesAlias(sl),
	}
	reqBody := &bytes.Buffer{}
	err := json.NewEncoder(reqBody).Encode(data)
	return reqBody.Bytes(), err
}

// String returns the JSON representation of a SketchSeriesList as a string
// or an empty string in case of an error
func (sl SketchSeriesList) String() string {
	json, err := sl.MarshalJSON()
	if err != nil {
		return ""
	}
	return string(json)
}

// SketchesSink is a sink for sketches.
// It provides a way to append a sketches into `SketchSeriesList`
type SketchesSink interface {
	Append(*SketchSeries)
}

var _ SketchesSink = (*SketchSeriesList)(nil)

// Append appends a sketches.
func (sl *SketchSeriesList) Append(sketches *SketchSeries) {
	*sl = append(*sl, sketches)
}
