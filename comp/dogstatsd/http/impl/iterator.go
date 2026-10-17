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
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/metricname"
)

// payloadStats counts what a single payload contributed. Accumulated as the
// iterator is drained and reported to telemetry once, instead of touching the
// counters for every point.
type payloadStats struct {
	metrics         uint64
	points          uint64
	filteredMetrics uint64
	filteredPoints  uint64
}

func (s *payloadStats) report(tlm *endpointTelemetry) {
	tlm.metrics.Add(float64(s.metrics))
	tlm.points.Add(float64(s.points))
	tlm.filteredMetrics.Add(float64(s.filteredMetrics))
	tlm.filteredPoints.Add(float64(s.filteredPoints))
}

type iteratorCommon struct {
	reader     *reader.MetricDataReader
	origin     origin
	hostname   string
	filterList metricname.Matcher
	stats      payloadStats
	err        error
}

// nextUnfilteredMetric advances the reader to the next metric that is not in
// the filter list. It returns false once the payload is exhausted or the reader
// fails.
func (it *iteratorCommon) nextUnfilteredMetric() bool {
	for {
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

		// NextMetric drains any points left over from the previous metric, so
		// skipping here keeps the reader's column indexes in sync.
		if !it.filterList.Test(it.reader.Name()) {
			return true
		}

		it.stats.filteredMetrics++
		it.stats.filteredPoints += it.reader.NumPoints()
	}
}

// processTags merges the origin tags with the tags sent by the client. The
// reader has already dropped the legacy dd.internal.card tag from the tagset
// dictionary, cardinality only comes from the type column.
func (it *iteratorCommon) processTags() tagset.CompositeTags {
	originTags := it.origin.getTagsWith(it.reader.TagCardinality())
	return tagset.NewCompositeTags(originTags, it.reader.Tags())
}

type seriesIterator struct {
	iteratorCommon
	buffer metrics.Serie
}

func newSeriesIterator(payload *pb.Payload, origin origin, hostname string, filterList metricname.Matcher) (*seriesIterator, error) {
	it := &seriesIterator{
		iteratorCommon: iteratorCommon{
			reader:     reader.NewMetricDataReader(payload.MetricData),
			origin:     origin,
			hostname:   hostname,
			filterList: filterList,
		},
	}

	return it, it.reader.Initialize()
}

// MoveNext reads one entire metric record from the dogstatsd payload into the internal buffer.
func (it *seriesIterator) MoveNext() bool {
	if !it.nextUnfilteredMetric() {
		return false
	}

	b := &it.buffer
	b.Name = it.reader.Name()
	b.Tags = it.processTags()
	b.Source = metrics.MetricSourceDogstatsd

	switch it.reader.Type() {
	case pb.MetricType_Gauge:
		b.MType = metrics.APIGaugeType
	case pb.MetricType_Count:
		b.MType = metrics.APICountType
	case pb.MetricType_Rate:
		b.MType = metrics.APIRateType
	default:
		it.err = fmt.Errorf("unexpected metric type %s in a series payload", it.reader.Type())
		return false
	}

	b.Interval = int64(it.reader.Interval())
	b.SourceTypeName = it.reader.SourceTypeName()

	b.Host = it.hostname
	seenHost := false
	b.Device = ""
	seenDevice := false

	b.Resources = b.Resources[:0]
	for _, res := range it.reader.Resources() {
		switch res.Type {
		case "host":
			if !seenHost {
				b.Host = res.Name
				seenHost = true
			}
		case "device":
			if !seenDevice {
				b.Device = res.Name
				seenDevice = true
			}
		default:
			b.Resources = append(b.Resources, *res)
		}
	}

	b.Points = b.Points[:0]
	for it.reader.HaveMorePoints() {
		it.err = it.reader.NextPoint()
		if it.err != nil {
			return false
		}

		b.Points = append(b.Points, metrics.Point{
			Ts:    float64(it.reader.Timestamp()),
			Value: it.reader.Value(),
		})
	}

	it.stats.metrics++
	it.stats.points += uint64(len(b.Points))

	return true
}

// Current returns the internal series buffer, populated by MoveNext.
func (it *seriesIterator) Current() *metrics.Serie {
	return &it.buffer
}

// Count does nothing and returns zero.
func (it *seriesIterator) Count() uint64 {
	return 0
}
