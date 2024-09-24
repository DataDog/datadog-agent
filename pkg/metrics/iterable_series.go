// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// IterableSeries is an specialisation of iterableMetrics for serie.
type IterableSeries struct {
	iterableMetrics
}

// NewIterableSeries creates a new instance of *IterableSeries
//
// `callback` is called in the context of the sender's goroutine each time `Append` is called.
func NewIterableSeries(callback func(*Serie), chanSize int, bufferSize int, hostname string) *IterableSeries {
	return &IterableSeries{
		iterableMetrics: *newIterableMetric(func(value interface{}) {
			serie := value.(*Serie)

			tags := []string{"host:" + hostname}

			serie.Tags = tagset.CombineCompositeTagsAndSlice(serie.Tags, tags)
			callback(serie)
		}, chanSize, bufferSize),
	}
}

// Append appends a serie
func (it *IterableSeries) Append(serie *Serie) {
	it.iterableMetrics.Append(serie)
}

// Current returns the current serie.
func (it *IterableSeries) Current() *Serie {
	return it.iterableMetrics.Current().(*Serie)
}

// SerieSource is a source of series used by the serializer.
type SerieSource interface {
	MoveNext() bool
	Current() *Serie
	Count() uint64
}
