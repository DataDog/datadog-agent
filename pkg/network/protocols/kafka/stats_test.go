// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package kafka

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/sketches-go/ddsketch"
)

func TestAddRequest(t *testing.T) {
	testErrorCode := int32(5)
	stats := NewRequestStats()
	stats.AddRequest(testErrorCode, 10, 1, 10.0)
	stats.AddRequest(testErrorCode, 15, 2, 15.0)
	stats.AddRequest(testErrorCode, 20, 3, 20.0)

	// Check we don't have stats for other error codes
	for i := int32(-1); i < 119; i++ {
		if i == testErrorCode {
			continue
		}
		assert.Nil(t, stats.ErrorCodeToStat[i])
	}
	s := stats.ErrorCodeToStat[testErrorCode]

	if assert.NotNil(t, s) {
		assert.Equal(t, 45, s.Count)
		assert.Equal(t, float64(45), s.Latencies.GetCount())

		verifyQuantile(t, s.Latencies, 0.0, 10.0) // min item
		verifyQuantile(t, s.Latencies, 0.5, 15.0) // median
		verifyQuantile(t, s.Latencies, 1.0, 20.0) // max item
	}
}

func TestCombineWith(t *testing.T) {
	testErrorCode := int32(5)

	stats := NewRequestStats()
	stats2 := NewRequestStats()
	stats3 := NewRequestStats()
	stats4 := NewRequestStats()

	stats2.AddRequest(testErrorCode, 10, 1, 10.0)
	stats3.AddRequest(testErrorCode, 15, 2, 15.0)
	stats4.AddRequest(testErrorCode, 20, 3, 20.0)

	stats.CombineWith(stats2)
	stats.CombineWith(stats3)
	stats.CombineWith(stats4)

	// Check we don't have stats for other error codes
	for i := int32(-1); i < 119; i++ {
		if i == testErrorCode {
			continue
		}
		assert.Nil(t, stats.ErrorCodeToStat[i])
	}
	s := stats.ErrorCodeToStat[testErrorCode]

	if assert.NotNil(t, s) {
		assert.Equal(t, 45, s.Count)
		assert.Equal(t, float64(45), s.Latencies.GetCount())

		verifyQuantile(t, s.Latencies, 0.0, 10.0) // min item
		verifyQuantile(t, s.Latencies, 0.5, 15.0) // median
		verifyQuantile(t, s.Latencies, 1.0, 20.0) // max item
	}
}

func verifyQuantile(t *testing.T, sketch *ddsketch.DDSketch, q float64, expectedValue float64) {
	val, err := sketch.GetValueAtQuantile(q)
	assert.Nil(t, err)

	acceptableError := expectedValue * sketch.IndexMapping.RelativeAccuracy()
	assert.GreaterOrEqual(t, val, expectedValue-acceptableError)
	assert.LessOrEqual(t, val, expectedValue+acceptableError)
}
