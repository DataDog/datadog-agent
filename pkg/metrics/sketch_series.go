// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"bytes"
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/quantile"
)

// DistributionMetadata is the per-series metadata required for all distribution variants.
type DistributionMetadata struct {
	Name     string               `json:"metric"`
	Tags     tagset.CompositeTags `json:"tags"`
	Host     string               `json:"host"`
	Interval int64                `json:"interval"`
	NoIndex  bool                 `json:"-"` // This is only used by api V2
	Source   MetricSource         `json:"-"` // This is only used by api V2
}

// A SketchSeries is a timeseries of quantile sketches.
type SketchSeries struct {
	DistributionMetadata
	Points []SketchPoint `json:"points"`
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

// WriteTo emits the DDSketch flavor of this series.
//
// WriteTo may be invoked multiple times on the same value. The serializer
// calls it again on a fresh DistributionWriter after a payload split; iterating
// over Points from the start is safe and idempotent.
func (sl *SketchSeries) WriteTo(w DistributionWriter) error {
	return w.WriteDDSketch(sl.DistributionMetadata, len(sl.Points), sl)
}

// GetDDSketchPoint returns the sketch point at index i.
func (sl *SketchSeries) GetDDSketchPoint(i int) (ts, cnt int64, min, max, sum, avg float64, k []int32, n []uint32) {
	p := sl.Points[i]
	cnt, min, max, sum, avg = p.Sketch.BasicStats()
	k, n = p.Sketch.Cols()
	return p.Ts, cnt, min, max, sum, avg, k, n
}

// A SketchPoint represents a quantile sketch at a specific time
type SketchPoint struct {
	Sketch *quantile.Sketch `json:"sketch"`
	Ts     int64            `json:"ts"`
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
