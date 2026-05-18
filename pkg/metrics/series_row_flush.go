// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import "errors"

// SerieRowFragment is the metric-local portion of a serializer row emitted by a
// Metric flush. The aggregator adds context-derived name/tags/host/source fields
// before passing a full SerieRow to the serializer.
//
// This is intentionally narrow: it exists to let local DogStatsD direct-row
// experiments bypass *Serie allocation during metric flush while preserving the
// existing flush semantics.
type SerieRowFragment struct {
	Points         []Point
	MType          APIMetricType
	NameSuffix     string
	SourceTypeName string
	Unit           string
	Resources      []Resource
}

type metricSerieRowFlusher interface {
	flushSerieRowFragments(timestamp float64, rows []SerieRowFragment, points []Point) ([]SerieRowFragment, []Point, error)
}

func appendSerieRowFragment(
	rows []SerieRowFragment,
	points []Point,
	point Point,
	mType APIMetricType,
	nameSuffix string,
	sourceTypeName string,
	unit string,
	resources []Resource,
) ([]SerieRowFragment, []Point) {
	start := len(points)
	points = append(points, point)
	rows = append(rows, SerieRowFragment{
		Points:         points[start:len(points):len(points)],
		MType:          mType,
		NameSuffix:     nameSuffix,
		SourceTypeName: sourceTypeName,
		Unit:           unit,
		Resources:      resources,
	})
	return rows, points
}

func flushMetricToSerieRowFragments(metric Metric, timestamp float64, rows []SerieRowFragment, points []Point) ([]SerieRowFragment, []Point, error) {
	if flusher, ok := metric.(metricSerieRowFlusher); ok {
		return flusher.flushSerieRowFragments(timestamp, rows, points)
	}

	series, err := metric.flush(timestamp)
	if err != nil {
		return rows, points, err
	}
	for _, serie := range series {
		if serie == nil {
			continue
		}
		start := len(points)
		points = append(points, serie.Points...)
		rows = append(rows, SerieRowFragment{
			Points:         points[start:len(points):len(points)],
			MType:          serie.MType,
			NameSuffix:     serie.NameSuffix,
			SourceTypeName: serie.SourceTypeName,
			Unit:           serie.Unit,
			Resources:      serie.Resources,
		})
	}
	return rows, points, nil
}

func (g *Gauge) flushSerieRowFragments(timestamp float64, rows []SerieRowFragment, points []Point) ([]SerieRowFragment, []Point, error) {
	value, sampled := g.gauge, g.sampled
	g.gauge, g.sampled = 0, false

	if !sampled {
		return rows, points, NoSerieError{}
	}

	rows, points = appendSerieRowFragment(rows, points, Point{Ts: timestamp, Value: value}, APIGaugeType, "", "", "", nil)
	return rows, points, nil
}

func (c *Count) flushSerieRowFragments(timestamp float64, rows []SerieRowFragment, points []Point) ([]SerieRowFragment, []Point, error) {
	value, sampled := c.value, c.sampled
	c.value, c.sampled = 0, false

	if !sampled {
		return rows, points, NoSerieError{}
	}

	rows, points = appendSerieRowFragment(rows, points, Point{Ts: timestamp, Value: value}, APICountType, "", "", "", nil)
	return rows, points, nil
}

func (c *Counter) flushSerieRowFragments(timestamp float64, rows []SerieRowFragment, points []Point) ([]SerieRowFragment, []Point, error) {
	value, sampled := c.value, c.sampled
	c.value, c.sampled = 0, false

	if !sampled {
		return rows, points, NoSerieError{}
	}

	rows, points = appendSerieRowFragment(rows, points, Point{Ts: timestamp, Value: value / float64(c.interval)}, APIRateType, "", "", "", nil)
	return rows, points, nil
}

func (s *Set) flushSerieRowFragments(timestamp float64, rows []SerieRowFragment, points []Point) ([]SerieRowFragment, []Point, error) {
	if len(s.values) == 0 {
		return rows, points, NoSerieError{}
	}

	rows, points = appendSerieRowFragment(rows, points, Point{Ts: timestamp, Value: float64(len(s.values))}, APIGaugeType, "", "", "", nil)
	s.values = make(map[string]bool)
	return rows, points, nil
}

func (mc *MonotonicCount) flushSerieRowFragments(timestamp float64, rows []SerieRowFragment, points []Point) ([]SerieRowFragment, []Point, error) {
	if !mc.sampledSinceLastFlush || !(mc.hasPreviousSample || mc.flushFirstValue) {
		return rows, points, NoSerieError{}
	}

	value := mc.value
	mc.previousSample, mc.currentSample, mc.value = mc.currentSample, 0., 0.
	mc.hasPreviousSample = true
	mc.sampledSinceLastFlush = false
	mc.flushFirstValue = false

	rows, points = appendSerieRowFragment(rows, points, Point{Ts: timestamp, Value: value}, APICountType, "", "", "", nil)
	return rows, points, nil
}

func (r *Rate) flushSerieRowFragments(_ float64, rows []SerieRowFragment, points []Point) ([]SerieRowFragment, []Point, error) {
	if r.previousTimestamp == 0 || r.timestamp == 0 {
		return rows, points, NoSerieError{}
	}

	if r.timestamp == r.previousTimestamp {
		return rows, points, errors.New("Rate was sampled twice at the same timestamp, can't compute a rate")
	}

	value, ts := (r.sample-r.previousSample)/(r.timestamp-r.previousTimestamp), r.timestamp
	r.previousSample, r.previousTimestamp = r.sample, r.timestamp
	r.sample, r.timestamp = 0., 0.

	if value < 0 {
		return rows, points, errors.New("Rate value is negative, discarding it (the underlying counter may have been reset)")
	}

	rows, points = appendSerieRowFragment(rows, points, Point{Ts: ts, Value: value}, APIGaugeType, "", "", "", nil)
	return rows, points, nil
}

func (mt *MetricWithTimestamp) flushSerieRowFragments(_ float64, rows []SerieRowFragment, points []Point) ([]SerieRowFragment, []Point, error) {
	if len(mt.points) == 0 {
		return rows, points, NoSerieError{}
	}

	start := len(points)
	points = append(points, mt.points...)
	mt.points = nil
	rows = append(rows, SerieRowFragment{
		Points: points[start:len(points):len(points)],
		MType:  mt.apiType,
	})
	return rows, points, nil
}
