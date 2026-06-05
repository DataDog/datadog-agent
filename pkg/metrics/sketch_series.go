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

// A SketchSeries is a timeseries of quantile sketches or native OTel histograms.
// The concrete type behind each SketchPoint.Sketch determines the kind of data;
// use a Go type switch to discriminate (DDSketchProvider, ExplicitBoundProvider,
// ExponentialProvider).
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

// SketchData is the minimal interface every sketch point must satisfy.
// The serializer uses Go type switches on the capability interfaces below
// (DDSketchProvider, ExplicitBoundProvider, ExponentialProvider) to access
// kind-specific data without importing source-model packages.
type SketchData interface {
	// SummaryValues returns min, max, sum for the data point.
	// Used by the V3 serializer to determine the narrowest value encoding
	// across all points in a series before writing.
	SummaryValues() (min, max, sum float64)
}

// DDSketchProvider gives access to DDSketch bin data and summary statistics.
// Satisfied by *quantile.Sketch and the dogstatsd HTTP sketch iterator.
type DDSketchProvider interface {
	SketchData
	Cols() (k []int32, n []uint32)
	BasicStats() (cnt int64, min, max, sum, avg float64)
}

// ExplicitBoundProvider gives access to an explicit-boundary histogram's
// buckets and summary statistics. The method signatures use only primitive
// and slice types so that implementors need not import this package.
type ExplicitBoundProvider interface {
	SketchData
	ExplicitBounds() []float64
	BucketCounts() []uint64
	Count() uint64
	HasSum() bool
	Sum() float64
	HasMin() bool
	Min() float64
	HasMax() bool
	Max() float64
}

// ExponentialProvider gives access to an exponential histogram's scale,
// zero bucket, positive/negative bucket arrays, and summary statistics.
type ExponentialProvider interface {
	SketchData
	Scale() int32
	ZeroCount() uint64
	PositiveOffset() int32
	PositiveBucketCounts() []uint64
	NegativeOffset() int32
	NegativeBucketCounts() []uint64
	Count() uint64
	HasSum() bool
	Sum() float64
	HasMin() bool
	Min() float64
	HasMax() bool
	Max() float64
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
