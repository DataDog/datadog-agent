// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package aggregator

import (
	"sort"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/quantile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDistSampler(t *testing.T) {
	const (
		defaultHost       = "default_host"
		defaultBucketSize = 10
	)

	var (
		d = newDistSampler(0, defaultHost)

		insert = func(t *testing.T, ts float64, ctx Context, values ...float64) {
			t.Helper()
			for _, v := range values {
				d.addSample(&metrics.MetricSample{
					Name:       ctx.Name,
					Tags:       ctx.Tags,
					Host:       ctx.Host,
					Value:      v,
					Mtype:      metrics.DistributionType,
					SampleRate: 1,
				}, ts)
			}
		}
	)

	assert.EqualValues(t, defaultBucketSize, d.interval,
		"interval should default to 10")

	t.Run("empty flush", func(t *testing.T) {
		flushed := d.flush(timeNowNano())
		require.Len(t, flushed, 0)
	})

	t.Run("single bucket", func(t *testing.T) {
		var (
			now float64
			ctx = Context{Name: "m.0", Tags: []string{"a"}, Host: "host"}
			exp = &quantile.Sketch{}
		)

		for i := 0; i < bucketSize; i++ {
			v := float64(i)
			insert(t, now, ctx, v)
			exp.Insert(quantile.Default(), v)

			now++
		}

		flushed := d.flush(now)
		metrics.AssertSketchSeriesEqual(t, metrics.SketchSeries{
			Name:     ctx.Name,
			Tags:     ctx.Tags,
			Host:     ctx.Host,
			Interval: 10,
			Points: []metrics.SketchPoint{
				{
					Sketch: exp,
					Ts:     0,
				},
			},
			ContextKey: ckey.Generate(ctx.Name, ctx.Host, ctx.Tags),
		}, flushed[0])

		require.Len(t, d.flush(now), 0, "these points have already been flushed")
	})

}

func TestDistSamplerBucketSampling(t *testing.T) {

	distSampler := newDistSampler(10, "")

	mSample1 := metrics.MetricSample{
		Name:       "test.metric.name",
		Value:      1,
		Mtype:      metrics.DistributionType,
		Tags:       []string{"a", "b"},
		SampleRate: 1,
	}
	mSample2 := metrics.MetricSample{
		Name:       "test.metric.name",
		Value:      2,
		Mtype:      metrics.DistributionType,
		Tags:       []string{"a", "b"},
		SampleRate: 1,
	}
	distSampler.addSample(&mSample1, 10001)
	distSampler.addSample(&mSample2, 10002)
	distSampler.addSample(&mSample1, 10011)
	distSampler.addSample(&mSample2, 10012)
	distSampler.addSample(&mSample1, 10021)

	flushed := distSampler.flush(10020.0)
	expSketch := &quantile.Sketch{}
	expSketch.Insert(quantile.Default(), 1, 2)

	assert.Equal(t, 1, len(flushed))
	metrics.AssertSketchSeriesEqual(t, metrics.SketchSeries{
		Name:     "test.metric.name",
		Tags:     []string{"a", "b"},
		Interval: 10,
		Points: []metrics.SketchPoint{
			{Ts: 10000, Sketch: expSketch},
			{Ts: 10010, Sketch: expSketch},
		},
		ContextKey: generateContextKey(&mSample1),
	}, flushed[0])

	// The samples added after the flush time remains in the dist sampler
	assert.Equal(t, 1, distSampler.m.Len())
}

func TestDistSamplerContextSampling(t *testing.T) {
	distSampler := newDistSampler(10, "")

	mSample1 := metrics.MetricSample{
		Name:       "test.metric.name1",
		Value:      1,
		Mtype:      metrics.DistributionType,
		Tags:       []string{"a", "b"},
		SampleRate: 1,
	}
	mSample2 := metrics.MetricSample{
		Name:       "test.metric.name2",
		Value:      1,
		Mtype:      metrics.DistributionType,
		Tags:       []string{"a", "c"},
		SampleRate: 1,
	}
	distSampler.addSample(&mSample1, 10011)
	distSampler.addSample(&mSample2, 10011)

	flushed := distSampler.flush(10020)
	expSketch := &quantile.Sketch{}
	expSketch.Insert(quantile.Default(), 1)

	assert.Equal(t, 2, len(flushed))
	sort.Slice(flushed, func(i, j int) bool {
		return flushed[i].Name < flushed[j].Name
	})

	metrics.AssertSketchSeriesEqual(t, metrics.SketchSeries{
		Name:     "test.metric.name1",
		Tags:     []string{"a", "b"},
		Interval: 10,
		Points: []metrics.SketchPoint{
			{Ts: 10010, Sketch: expSketch},
		},
		ContextKey: generateContextKey(&mSample1),
	}, flushed[0])

	metrics.AssertSketchSeriesEqual(t, metrics.SketchSeries{
		Name:     "test.metric.name2",
		Tags:     []string{"a", "c"},
		Interval: 10,
		Points: []metrics.SketchPoint{
			{Ts: 10010, Sketch: expSketch},
		},
		ContextKey: generateContextKey(&mSample2),
	}, flushed[1])
}
