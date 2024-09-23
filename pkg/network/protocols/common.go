// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package protocols provides the implementation of the network tracer protocols
package protocols

import (
	"math"

	"github.com/DataDog/sketches-go/ddsketch"
)

// below is copied from pkg/trace/stats/statsraw.go

// NSTimestampToFloat converts a nanosec timestamp into a float nanosecond timestamp truncated to a fixed precision
func NSTimestampToFloat(ns uint64) float64 {
	b := math.Float64bits(float64(ns))
	// IEEE-754
	// the mask include 1 bit sign 11 bits exponent (0xfff)
	// then we filter the mantissa to 10bits (0xff8) (9 bits as it has implicit value of 1)
	// 10 bits precision (any value will be +/- 1/1024)
	// https://en.wikipedia.org/wiki/Double-precision_floating-point_format
	b &= 0xfffff80000000000
	return math.Float64frombits(b)
}

// GetSketchQuantile returns the value at the given percentile in the sketch
func GetSketchQuantile(sketch *ddsketch.DDSketch, percentile float64) float64 {
	if sketch == nil {
		return 0.0
	}

	val, _ := sketch.GetValueAtQuantile(percentile)
	return val
}
