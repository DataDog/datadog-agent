// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package ir

import (
	"math"
	"reflect"
	"time"
)

// MetricValueEncoding describes the interpretation of an aggregated metric value.
type MetricValueEncoding uint8

const (
	MetricValueInvalid MetricValueEncoding = iota
	MetricValueSigned
	MetricValueUnsigned
	MetricValueFloat32
	MetricValueFloat64
	MetricValueDuration
)

// MetricEncoding returns the encoding used for a value of type t.
func MetricEncoding(t Type) MetricValueEncoding {
	if _, duration := t.(*DurationType); duration {
		return MetricValueDuration
	}
	kind, _ := t.GetGoKind()
	switch kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return MetricValueSigned
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr, reflect.Bool:
		return MetricValueUnsigned
	case reflect.Float32:
		return MetricValueFloat32
	case reflect.Float64:
		return MetricValueFloat64
	default:
		return MetricValueInvalid
	}
}

// Float64 decodes a gauge value, converting durations to milliseconds.
func (e MetricValueEncoding) Float64(bits uint64) float64 {
	switch e {
	case MetricValueSigned:
		return float64(int64(bits))
	case MetricValueFloat32:
		return float64(math.Float32frombits(uint32(bits)))
	case MetricValueFloat64:
		return math.Float64frombits(bits)
	case MetricValueDuration:
		return float64(int64(bits)) / float64(time.Millisecond)
	default:
		return float64(bits)
	}
}
