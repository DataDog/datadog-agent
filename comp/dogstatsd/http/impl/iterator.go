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
)

type seriesIterator struct {
	reader *reader.MetricDataReader
	buffer metrics.Serie
	err    error
}

// MoveNext reads one entire metric record from the dogstatsd payload into the internal buffer.
func (it *seriesIterator) MoveNext() bool {
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

	b := &it.buffer
	b.Name = it.reader.Name()
	b.Tags = tagset.NewCompositeTags(it.reader.Tags(), nil)
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

	b.Resources = b.Resources[:0]
	for _, res := range it.reader.Resources() {
		switch res.Type {
		case "host":
			b.Host = res.Name
		case "device":
			b.Device = res.Name
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
