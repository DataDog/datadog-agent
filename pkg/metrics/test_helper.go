// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package metrics

import (
	"sort"

	// stdlib
	"fmt"
	"testing"

	// 3p

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/quantile"
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

// AssertSeriesEqual evaluate if two list of series match
func AssertSeriesEqual(t *testing.T, expected Series, series Series) {
	assert.Equal(t, len(expected), len(series))
	for _, serie := range series {
		found := false
		for _, expectedSerie := range expected {
			if ckey.Equals(serie.ContextKey, expectedSerie.ContextKey) {
				AssertSerieEqual(t, expectedSerie, serie)
				found = true
			}
		}
		assert.True(t, found)
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

type sketchComparator func(exp, act *quantile.Sketch) bool

// AssertSketchSeriesEqual checks whether two SketchSeries are equal
func AssertSketchSeriesEqual(t assert.TestingT, exp, act SketchSeries) {
	assertSketchSeriesEqualWithComparator(t, exp, act, func(exp, act *quantile.Sketch) bool {
		return exp.Equals(act)
	})
}

// AssertSketchSeriesApproxEqual checks whether two SketchSeries are approximately equal. e represents the acceptable error %
func AssertSketchSeriesApproxEqual(t assert.TestingT, exp, act SketchSeries, e float64) {
	assertSketchSeriesEqualWithComparator(t, exp, act, func(exp, act *quantile.Sketch) bool {
		return quantile.SketchesApproxEqual(exp, act, e)
	})
}

func assertSketchSeriesEqualWithComparator(t assert.TestingT, exp, act SketchSeries, compareFn sketchComparator) {
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

		// assert.Equal does lots of magic, lets double check with a concrete equals
		// method.
		for i := range exp.Points {
			a, e := act.Points[i], exp.Points[i]
			if a.Ts != e.Ts {
				t.Errorf("Mismatched timestamps [%d]: %s != %s", e.Ts, a.Sketch, e.Sketch)
			}
			if !compareFn(a.Sketch, e.Sketch) {
				t.Errorf("Points[%d]: %s != %s", e.Ts, a.Sketch, e.Sketch)
			}
		}
	}
}

type tHelper interface {
	Helper()
}
