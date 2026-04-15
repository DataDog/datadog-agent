// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package httpimpl

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/dogstatsd/http/impl/internal/reader"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/dogstatsdhttp"
)

// sketchData implements metrics.SketchData from reader-provided sketch columns and summary.
type sketchData struct {
	k                  []int32
	n                  []uint32
	cnt                int64
	min, max, sum, avg float64
}

func (s *sketchData) Cols() ([]int32, []uint32) {
	return s.k, s.n
}

func (s *sketchData) BasicStats() (int64, float64, float64, float64, float64) {
	return s.cnt, s.min, s.max, s.sum, s.avg
}

type sketchIterator struct {
	iteratorCommon
	buffer metrics.SketchSeries
}

func newSketchIterator(payload *pb.Payload, origin origin, hostname string) (*sketchIterator, error) {
	it := &sketchIterator{
		iteratorCommon: iteratorCommon{
			reader:   reader.NewMetricDataReader(payload.MetricData),
			origin:   origin,
			hostname: hostname,
		},
	}
	return it, it.reader.Initialize()
}

// MoveNext reads one entire sketch metric record from the dogstatsd payload into the internal buffer.
func (it *sketchIterator) MoveNext() bool {
	if it.err != nil {
		return false
	}

	if !it.reader.HaveMoreMetrics() {
		return false
	}

	it.err = it.reader.NextMetric()
	if it.err != nil {
		return false
	}

	if it.reader.Type() != pb.MetricType_Sketch {
		it.err = fmt.Errorf("unexpected metric type %s in a sketches payload", it.reader.Type())
		return false
	}

	b := &it.buffer
	b.Name = it.reader.Name()
	b.Tags = it.processTags()
	b.Source = metrics.MetricSourceDogstatsd
	b.Interval = int64(it.reader.Interval())

	b.Host = it.hostname
	for _, res := range it.reader.Resources() {
		if res.Type == "host" {
			b.Host = res.Name
			break
		}
	}

	b.Points = b.Points[:0]
	for it.reader.HaveMorePoints() {
		it.err = it.reader.NextPoint()
		if it.err != nil {
			return false
		}

		sum, min, max, cnt := it.reader.SketchSummary()
		k, n := it.reader.SketchCols()
		avg := float64(0)
		if cnt > 0 {
			avg = sum / float64(cnt)
		}
		b.Points = append(b.Points, metrics.SketchPoint{
			Ts: it.reader.Timestamp(),
			Sketch: &sketchData{
				k:   k,
				n:   n,
				cnt: int64(cnt),
				min: min,
				max: max,
				sum: sum,
				avg: avg,
			},
		})
	}

	return true
}

// Current returns the internal sketch series buffer, populated by MoveNext.
func (it *sketchIterator) Current() *metrics.SketchSeries {
	return &it.buffer
}

// Count does nothing and returns zero.
func (it *sketchIterator) Count() uint64 {
	return 0
}

// WaitForValue returns true because all data is already in memory.
func (it *sketchIterator) WaitForValue() bool {
	return true
}
