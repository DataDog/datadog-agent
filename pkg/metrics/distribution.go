// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

// Distribution serializes itself using provided DistributionWriter.
//
// This lets the serializer support several kinds of distributions
// without tying it to any specific data layout.
type Distribution interface {
	// GetName returns the name of the metric for filtering.
	GetName() string
	// WriteTo is called by the serializer to serialize the sketch.
	//
	// May be invoked multiple times on the same value.
	WriteTo(DistributionWriter) error
}

// DistributionWriter dispatches a Distribution to one of several wire-format flavors.
// A Distribution.WriteTo implementation is expected to call exactly one
// flavor method per invocation.
//
// New additions should be made for each shape of data being
// written. Writer interfaces should have distinct method names to
// allow implementing the whole set of write interfaces by the same type.
type DistributionWriter interface {
	// Write Datadog Sketch series.
	WriteDDSketch(meta DistributionMetadata, numPoints int, points DDSketchPoints) error
}

// DDSketchPoints provides random access to a distribution's sketch points.
type DDSketchPoints interface {
	// GetDDSketchPoint returns the sketch point at index i.
	// Implementers may return K and N backed by the same storage.
	// Callers must not retain K and N across calls.
	GetDDSketchPoint(i int) DDSketchPoint
}

// DDSketchPoint is a single sketch point returned by DDSketchPoints.
type DDSketchPoint struct {
	Ts  int64
	Cnt int64
	Min float64
	Max float64
	Sum float64
	Avg float64
	K   []int32
	N   []uint32
}
