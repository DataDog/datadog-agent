// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package metrics

import (
	"sort"

	// stdlib
	"fmt"
	"testing"

	// 3p

	"github.com/stretchr/testify/assert"
)

// AssertPointsEqual evaluate if two list of point are equal (order doesn't matters).
func AssertPointsEqual(t *testing.T, expected, actual []Point) {
	if assert.Equal(t, len(expected), len(actual)) {
		for _, point := range expected {
			assert.Contains(t, actual, point)
		}
	}
}

// AssertTagsEqual evaluate if two list of tags are equal (the order doesn't matters).
func AssertTagsEqual(t assert.TestingT, expected, actual []string) {
	if assert.Equal(t, len(expected), len(actual), fmt.Sprintf("Unexpected number of tags: expected %s, actual: %s", expected, actual)) {
		for _, tag := range expected {
			assert.Contains(t, actual, tag)
		}
	}
}

// AssertSerieEqual evaluate if two are equal.
func AssertSerieEqual(t *testing.T, expected, actual *Serie) {
	assert.Equal(t, expected.Name, actual.Name)
	if expected.Tags != nil {
		assert.NotNil(t, actual.Tags)
		AssertTagsEqual(t, expected.Tags, actual.Tags)
	}
	assert.Equal(t, expected.Host, actual.Host)
	assert.Equal(t, expected.MType, actual.MType)
	assert.Equal(t, expected.Interval, actual.Interval)
	assert.Equal(t, expected.SourceTypeName, actual.SourceTypeName)
	if !expected.ContextKey.IsZero() {
		// Only test the contextKey if it's set in the expected Serie
		assert.Equal(t, expected.ContextKey, actual.ContextKey)
	}
	assert.Equal(t, expected.NameSuffix, actual.NameSuffix)
	AssertPointsEqual(t, expected.Points, actual.Points)
}

// AssertSketchSeriesEqual checks whether two SketchSeries are equal
func AssertSketchSeriesEqual(t assert.TestingT, exp, act SketchSeries) {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}
	assert.Equal(t, exp.Name, act.Name, "Name")

	switch {
	case len(exp.Tags) == 0:
		assert.Len(t, act.Tags, 0, "(act) Tags: should be empty")
	case len(act.Tags) == 0:
		assert.Len(t, exp.Tags, 0, "(act) Tags: shouldn't be empty")
	default:
		AssertTagsEqual(t, exp.Tags, act.Tags)
	}

	assert.Equal(t, exp.Host, act.Host, "Host")
	assert.Equal(t, exp.Interval, act.Interval, "Interval")
	assert.Equal(t, exp.ContextKey, act.ContextKey, "ContextKey")

	switch {
	case len(exp.Points) != len(act.Points):
		t.Errorf("Points: %v != %v", exp.Points, act.Points)
	default:
		for _, points := range [][]SketchPoint{exp.Points, act.Points} {
			sort.SliceStable(points, func(i, j int) bool {
				return points[i].Ts < points[j].Ts
			})
		}

		assert.Equal(t, exp.Points, act.Points)

		// assert.Equal does lots of magic, lets double check with a concrete equals
		// method.
		for i := range exp.Points {
			a, e := act.Points[i], exp.Points[i]
			if a.Ts != e.Ts || !a.Sketch.Equals(e.Sketch) {
				t.Errorf("Points[%d]: %s != %s", a, e)
			}
		}
	}
}

type tHelper interface {
	Helper()
}
