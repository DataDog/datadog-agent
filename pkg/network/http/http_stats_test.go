// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
	"testing"

	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/stretchr/testify/assert"
)

func TestAddRequest(t *testing.T) {
	var stats RequestStats
	stats.AddRequest(400, 10.0)
	stats.AddRequest(404, 15.0)
	stats.AddRequest(405, 20.0)

	for i := 100; i <= 500; i += 100 {
		s := stats.Stats(i)
		if i == 400 {
			if assert.NotNil(t, s) {
				assert.Equal(t, 3, s.Count)
				assert.Equal(t, 3.0, s.Latencies.GetCount())

				verifyQuantile(t, s.Latencies, 0.0, 10.0)  // min item
				verifyQuantile(t, s.Latencies, 0.99, 15.0) // median
				verifyQuantile(t, s.Latencies, 1.0, 20.0)  // max item
			}
		} else {
			assert.Nil(t, s)
		}
	}
}

func TestCombineWith(t *testing.T) {
	var stats RequestStats
	for i := 100; i <= 500; i += 100 {
		s := stats.Stats(i)
		assert.Nil(t, s)
	}

	var stats2, stats3, stats4 RequestStats
	stats2.AddRequest(400, 10.0)
	stats3.AddRequest(404, 15.0)
	stats4.AddRequest(405, 20.0)

	stats.CombineWith(&stats2)
	stats.CombineWith(&stats3)
	stats.CombineWith(&stats4)

	for i := 100; i <= 500; i += 100 {
		s := stats.Stats(i)
		if i == 400 {
			if assert.NotNil(t, s) {
				assert.Equal(t, 3, s.Count)
				verifyQuantile(t, s.Latencies, 0.0, 10.0)
				verifyQuantile(t, s.Latencies, 0.5, 15.0)
				verifyQuantile(t, s.Latencies, 1.0, 20.0)
			}
		} else {
			assert.Nil(t, s)
		}
	}
}

func verifyQuantile(t *testing.T, sketch *ddsketch.DDSketch, q float64, expectedValue float64) {
	val, err := sketch.GetValueAtQuantile(q)
	assert.Nil(t, err)

	acceptableError := expectedValue * sketch.IndexMapping.RelativeAccuracy()
	assert.True(t, val >= expectedValue-acceptableError)
	assert.True(t, val <= expectedValue+acceptableError)
}
