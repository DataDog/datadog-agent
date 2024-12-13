// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package http

import (
	"testing"

	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/stretchr/testify/assert"
)

func TestAddRequest(t *testing.T) {
	stats := NewRequestStats()
	stats.AddRequest(405, 10.0, 1, nil)
	stats.AddRequest(405, 15.0, 2, nil)
	stats.AddRequest(405, 20.0, 3, nil)

	assert.Nil(t, stats.Data[100])
	assert.Nil(t, stats.Data[200])
	assert.Nil(t, stats.Data[300])
	assert.Nil(t, stats.Data[500])
	s := stats.Data[405]

	if assert.NotNil(t, s) {
		assert.Equal(t, 3, s.Count)
		assert.Equal(t, 3.0, s.Latencies.GetCount())

		verifyQuantile(t, s.Latencies, 0.0, 10.0)  // min item
		verifyQuantile(t, s.Latencies, 0.99, 15.0) // median
		verifyQuantile(t, s.Latencies, 1.0, 20.0)  // max item
	}
}

func TestCombineWith(t *testing.T) {
	stats := NewRequestStats()
	for i := uint16(100); i <= 500; i += 100 {
		assert.Nil(t, stats.Data[i])
	}

	stats2 := NewRequestStats()
	stats3 := NewRequestStats()
	stats4 := NewRequestStats()
	stats2.AddRequest(405, 10.0, 2, nil)
	stats3.AddRequest(405, 15.0, 3, nil)
	stats4.AddRequest(405, 20.0, 4, nil)

	stats.CombineWith(stats2)
	stats.CombineWith(stats3)
	stats.CombineWith(stats4)

	assert.Nil(t, stats.Data[100])
	assert.Nil(t, stats.Data[200])
	assert.Nil(t, stats.Data[300])
	assert.Nil(t, stats.Data[500])
	s := stats.Data[405]

	if assert.NotNil(t, s) {
		assert.Equal(t, 3.0, s.Latencies.GetCount())

		verifyQuantile(t, s.Latencies, 0.0, 10.0) // min item
		verifyQuantile(t, s.Latencies, 0.5, 15.0) // median
		verifyQuantile(t, s.Latencies, 1.0, 20.0) // max item
	}
}

func verifyQuantile(t *testing.T, sketch *ddsketch.DDSketch, q float64, expectedValue float64) {
	val, err := sketch.GetValueAtQuantile(q)
	assert.Nil(t, err)

	acceptableError := expectedValue * sketch.IndexMapping.RelativeAccuracy()
	assert.True(t, val >= expectedValue-acceptableError)
	assert.True(t, val <= expectedValue+acceptableError)
}
