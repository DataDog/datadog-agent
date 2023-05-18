// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build test

package metrics

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/quantile"
)

// Makeseries creates a metrics.SketchSeries with i+5 Sketch Points
func Makeseries(i int) *metrics.SketchSeries {
	// Makeseries is deterministic so that we can test for mutation.
	ss := &metrics.SketchSeries{
		Name: fmt.Sprintf("name.%d", i),
		Tags: tagset.CompositeTagsFromSlice([]string{
			fmt.Sprintf("a:%d", i),
			fmt.Sprintf("b:%d", i),
		}),
		Host:     fmt.Sprintf("host.%d", i),
		Interval: int64(i),
	}

	// We create i+5 Sketch Points to insure all hosts have at least 5 Sketch Points for tests
	for j := 0; j < i+5; j++ {
		ss.Points = append(ss.Points, metrics.SketchPoint{
			Ts:     10 * int64(j),
			Sketch: makesketch(j),
		})
	}

	gen := ckey.NewKeyGenerator()
	ss.ContextKey = gen.Generate(ss.Name, ss.Host, tagset.NewHashingTagsAccumulatorWithTags(ss.Tags.UnsafeToReadOnlySliceString()))

	return ss
}

func makesketch(n int) *quantile.Sketch {
	s, c := &quantile.Sketch{}, quantile.Default()
	for i := 0; i < n; i++ {
		s.Insert(c, float64(i))
	}
	return s
}

type serieSourceMock struct {
	series metrics.Series
	index  int
}

func (s *serieSourceMock) MoveNext() bool {
	s.index++
	return s.index < len(s.series)
}

func (s *serieSourceMock) Current() *metrics.Serie {
	return s.series[s.index]
}

func (s *serieSourceMock) Count() uint64 {
	return uint64(len(s.series))
}

func CreateSerieSource(series metrics.Series) metrics.SerieSource {
	return &serieSourceMock{
		series: series,
		index:  -1,
	}
}
