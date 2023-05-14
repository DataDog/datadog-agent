// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

// IterableSketches is an specialisation of iterableMetrics for Sketches.
type IterableSketches struct {
	iterableMetrics
}

// NewIterableSketches creates a new instance of *IterableSketches
//
// `callback` is called in the context of the sender's goroutine each time `Append` is called.
func NewIterableSketches(callback func(*SketchSeries), chanSize int, bufferSize int) *IterableSketches {
	return &IterableSketches{
		iterableMetrics: *newIterableMetric(func(value interface{}) {
			sketchSeries := value.(*SketchSeries)
			callback(sketchSeries)
		}, chanSize, bufferSize),
	}
}

// WaitForValue waits until a value is available for MoveNext() or until senderStopped() is called
// Returns true if a value is available, false otherwise
func (it *IterableSketches) WaitForValue() bool {
	return it.iterableMetrics.WaitForValue()
}

var _ SketchesSink = (*IterableSketches)(nil)

// Append appends a sketches
func (it *IterableSketches) Append(Sketches *SketchSeries) {
	it.iterableMetrics.Append(Sketches)
}

// Current returns the current sketches.
func (it *IterableSketches) Current() *SketchSeries {
	return it.iterableMetrics.Current().(*SketchSeries)
}

// SketchesSource is a source of sketches used by the serializer.
type SketchesSource interface {
	MoveNext() bool
	Current() *SketchSeries
	Count() uint64
	WaitForValue() bool
}
