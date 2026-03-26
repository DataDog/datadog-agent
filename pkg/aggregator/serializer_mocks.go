// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"github.com/DataDog/datadog-agent/pkg/metrics"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
)

// The implementation of MockSerializer.SendIterableSeries uses `s.Called(series).Error(0)`.
// It calls internaly `Printf` on each field of the real type of `IterableStreamJSONMarshaler` which is `IterableSeries`.
// It can lead to a race condition, if another goruntine call `IterableSeries.Append` which modifies `series.count`.
// MockSerializerIterableSerie overrides `SendIterableSeries` to avoid this issue.
// It also overrides `SendSeries` for simplificy.
type MockSerializerIterableSerie struct {
	series []*metrics.Serie
	serializermock.MetricSerializer
}

func (s *MockSerializerIterableSerie) SendIterableSeries(seriesSource metrics.SerieSource) error {
	for seriesSource.MoveNext() {
		s.series = append(s.series, seriesSource.Current())
	}
	return nil
}

type MockSerializerSketch struct {
	sketches []*metrics.SketchSeries
	MockSerializerIterableSerie
}

func (s *MockSerializerSketch) SendSketch(sketches metrics.SketchesSource) error {
	for sketches.MoveNext() {
		s.sketches = append(s.sketches, sketches.Current())
	}
	return nil
}
