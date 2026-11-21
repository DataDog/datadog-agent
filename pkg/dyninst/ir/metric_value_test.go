// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package ir

import (
	"math"
	"reflect"
	"testing"
)

func TestMetricEncoding(t *testing.T) {
	for _, tc := range []struct {
		kinds []reflect.Kind
		want  MetricValueEncoding
	}{
		{[]reflect.Kind{reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64}, MetricValueSigned},
		{[]reflect.Kind{reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr, reflect.Bool}, MetricValueUnsigned},
		{[]reflect.Kind{reflect.Float32}, MetricValueFloat32},
		{[]reflect.Kind{reflect.Float64}, MetricValueFloat64},
		{[]reflect.Kind{reflect.Invalid, reflect.String, reflect.Struct, reflect.Slice, reflect.Complex64}, MetricValueInvalid},
	} {
		for _, kind := range tc.kinds {
			t.Run(kind.String(), func(t *testing.T) {
				typ := &BaseType{GoTypeAttributes: GoTypeAttributes{GoKind: kind}}
				if got := MetricEncoding(typ); got != tc.want {
					t.Fatalf("encoding = %v, want %v", got, tc.want)
				}
			})
		}
	}
	if got := MetricEncoding(&DurationType{}); got != MetricValueDuration {
		t.Fatalf("duration encoding = %v, want %v", got, MetricValueDuration)
	}
}

func TestMetricValueFloat64(t *testing.T) {
	for _, tc := range []struct {
		name     string
		encoding MetricValueEncoding
		bits     uint64
		want     float64
	}{
		{"signed", MetricValueSigned, math.MaxUint64, -1},
		{"unsigned", MetricValueUnsigned, math.MaxUint64, float64(uint64(math.MaxUint64))},
		{"bool", MetricValueUnsigned, 1, 1},
		{"float32", MetricValueFloat32, uint64(math.Float32bits(-2.5)), -2.5},
		{"float64", MetricValueFloat64, math.Float64bits(-3.25), -3.25},
		{"duration", MetricValueDuration, 1250000, 1.25},
		{"negative_duration", MetricValueDuration, math.MaxUint64 - 1249999, -1.25},
		{"infinity", MetricValueFloat64, math.Float64bits(math.Inf(1)), math.Inf(1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.encoding.Float64(tc.bits); got != tc.want {
				t.Fatalf("value = %v, want %v", got, tc.want)
			}
		})
	}
	if !math.IsNaN(MetricValueFloat64.Float64(math.Float64bits(math.NaN()))) {
		t.Fatal("NaN was not preserved")
	}
}
