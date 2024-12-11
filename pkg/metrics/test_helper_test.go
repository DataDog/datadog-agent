// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package metrics

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/quantile"
)

func TestAssertSketchSeriesEqual(t *testing.T) {
	arange := func(n int) *quantile.Sketch {
		s := &quantile.Sketch{}
		c := quantile.Default()

		for i := 0; i < n; i++ {
			s.Insert(c, float64(i))
		}

		return s
	}

	for _, tt := range []struct {
		s     [2]SketchSeries
		name  string
		valid bool
	}{
		{
			name: "Name",
			s: [2]SketchSeries{
				{Name: "a"},
				{Name: "b"},
			},
		}, {
			name: "Tags same len",
			s: [2]SketchSeries{
				{Tags: tagset.CompositeTagsFromSlice([]string{"a"})},
				{Tags: tagset.CompositeTagsFromSlice([]string{"b"})},
			},
		}, {
			name: "Tags/diff len",
			s: [2]SketchSeries{
				{Tags: tagset.CompositeTagsFromSlice([]string{"a"})},
				{Tags: tagset.CompositeTagsFromSlice([]string{"a", "b"})},
			},
		}, {
			// AssertSerieEqual and friends don't catch this case.
			// TODO: fix them
			name: "Tags/exp=nil",
			s: [2]SketchSeries{
				{Tags: tagset.CompositeTagsFromSlice(nil)},
				{Tags: tagset.CompositeTagsFromSlice([]string{"a", "b"})},
			},
		},
		{
			name: "Tags/act=nil",
			s: [2]SketchSeries{
				{Tags: tagset.CompositeTagsFromSlice([]string{"a", "b"})},
				{Tags: tagset.CompositeTagsFromSlice(nil)},
			},
		}, {
			name: "Host",
			s: [2]SketchSeries{
				{Host: "a"},
				{Host: "b"},
			},
		}, {
			name: "Points/same len/diff sketch",
			s: [2]SketchSeries{
				{
					Points: []SketchPoint{
						{Ts: 1, Sketch: arange(1)},
					},
				}, {
					Points: []SketchPoint{
						{Ts: 1, Sketch: arange(2)},
					},
				},
			},
		}, {
			name: "Points/same len/diff sketch",
			s: [2]SketchSeries{
				{
					Points: []SketchPoint{
						{Ts: 2, Sketch: arange(1)},
					},
				}, {
					Points: []SketchPoint{
						{Ts: 1, Sketch: arange(1)},
					},
				},
			},
		}, {
			name: "Points/equal",
			s: [2]SketchSeries{
				{
					Points: []SketchPoint{
						{Ts: 1, Sketch: arange(1)},
					},
				}, {
					Points: []SketchPoint{
						{Ts: 1, Sketch: arange(1)},
					},
				},
			},
			valid: true,
		}, {
			name: "Points/equal/unsorted ts",
			s: [2]SketchSeries{
				{
					Points: []SketchPoint{
						{Ts: 1, Sketch: arange(1)},
						{Ts: 2, Sketch: arange(2)},
					},
				}, {
					Points: []SketchPoint{
						{Ts: 2, Sketch: arange(2)},
						{Ts: 1, Sketch: arange(1)},
					},
				},
			},
			valid: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ft := &fakeTestingT{}

			AssertSketchSeriesEqual(ft, &tt.s[0], &tt.s[1])
			if tt.valid {
				assert.Len(t, ft.msgs, 0, "should be equal")
			} else {
				assert.True(t, len(ft.msgs) > 0, "should have an error")
			}
		})
	}
}

type fakeTestingT struct {
	msgs []string
}

func (t *fakeTestingT) Errorf(format string, args ...interface{}) {
	t.msgs = append(t.msgs, fmt.Sprintf(format, args...))
}
