// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
)

func TestBOCPDDetector_Name(t *testing.T) {
	d := NewBOCPDDetector()
	assert.Equal(t, "bocpd_detector", d.Name())
}

func TestBOCPDDetector_NotEnoughPoints(t *testing.T) {
	d := NewBOCPDDetector()
	series := observer.Series{
		Name:   "test.metric",
		Points: []observer.Point{{Timestamp: 1, Value: 100}},
	}

	result := d.Analyze(series)
	assert.Empty(t, result.Anomalies)
}

func TestBOCPDDetector_StableData(t *testing.T) {
	d := NewBOCPDDetector()

	points := make([]observer.Point, 40)
	for i := 0; i < 40; i++ {
		points[i] = observer.Point{Timestamp: int64(i + 1), Value: 100 + float64(i%3-1)}
	}

	series := observer.Series{Name: "test.metric", Points: points}
	result := d.Analyze(series)
	assert.Empty(t, result.Anomalies, "stable data should not trigger BOCPD")
}

func TestBOCPDDetector_DetectsStepChange(t *testing.T) {
	d := NewBOCPDDetector()

	points := make([]observer.Point, 40)
	for i := 0; i < 20; i++ {
		points[i] = observer.Point{Timestamp: int64(i + 1), Value: 100}
	}
	for i := 20; i < 40; i++ {
		points[i] = observer.Point{Timestamp: int64(i + 1), Value: 140}
	}

	series := observer.Series{Name: "test.metric", Points: points}
	result := d.Analyze(series)

	if assert.Len(t, result.Anomalies, 1) {
		assert.Contains(t, result.Anomalies[0].Title, "BOCPD")
		assert.GreaterOrEqual(t, result.Anomalies[0].Timestamp, int64(21))
	}
}
