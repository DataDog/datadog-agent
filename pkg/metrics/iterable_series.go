// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"context"
	"errors"
	"sync/atomic"

	jsoniter "github.com/json-iterator/go"

	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// IterableSeries represents an iterable collection of Serie.
// Serie can be appended to IterableSeries while IterableSeries is serialized
type IterableSeries struct {
	ch                 *util.BufferedChan
	bufferedChanClosed bool
	cancel             context.CancelFunc
	callback           func(*Serie)
	current            *Serie
	count              uint64
}

// NewIterableSeries creates a new instance of *IterableSeries
// `callback` is called each time `Append` is called.
func NewIterableSeries(callback func(*Serie), chanSize int, bufferSize int) *IterableSeries {
	ctx, cancel := context.WithCancel(context.Background())
	return &IterableSeries{
		ch:       util.NewBufferedChan(ctx, chanSize, bufferSize),
		cancel:   cancel,
		callback: callback,
		current:  nil,
	}
}

// Append appends a serie
func (series *IterableSeries) Append(serie *Serie) {
	series.callback(serie)
	atomic.AddUint64(&series.count, 1)
	if !series.ch.Put(serie) && !series.bufferedChanClosed {
		series.bufferedChanClosed = true
		log.Errorf("Cannot append a serie in a closed buffered channel")
	}
}

// SeriesCount returns the number of series appended with `IterableSeries.Append`.
func (series *IterableSeries) SeriesCount() uint64 {
	return atomic.LoadUint64(&series.count)
}

// SenderStopped must be called when sender stop calling Append.
func (series *IterableSeries) SenderStopped() {
	series.ch.Close()
}

// IterationStopped must be called when the receiver stops calling `MoveNext`.
// This function prevents the case when the receiver stops iterating before the
// end of the iteration because of an error and so blocks the sender forever
// as no goroutine read the channel.
func (series *IterableSeries) IterationStopped() {
	series.cancel()
}

// WriteHeader writes the payload header for this type
func (series *IterableSeries) WriteHeader(stream *jsoniter.Stream) error {
	return writeHeader(stream)
}

// WriteFooter writes the payload footer for this type
func (series *IterableSeries) WriteFooter(stream *jsoniter.Stream) error {
	return writeFooter(stream)
}

// WriteCurrentItem writes the json representation of an item
func (series *IterableSeries) WriteCurrentItem(stream *jsoniter.Stream) error {
	current := series.Current()
	if current == nil {
		return errors.New("nil serie")
	}
	return writeItem(stream, current)
}

// DescribeCurrentItem returns a text description for logs
func (series *IterableSeries) DescribeCurrentItem() string {
	current := series.Current()
	if current == nil {
		return "nil serie"
	}
	return describeItem(current)
}

// MoveNext advances to the next element.
// Returns false for the end of the iteration.
func (series *IterableSeries) MoveNext() bool {
	v, ok := series.ch.Get()
	if v != nil {
		series.current = v.(*Serie)
	} else {
		series.current = nil
	}
	return ok
}

// Current returns the current serie.
func (series *IterableSeries) Current() *Serie {
	return series.current
}

// MarshalSplitCompress uses the stream compressor to marshal and compress series payloads.
// If a compressed payload is larger than the max, a new payload will be generated. This method returns a slice of
// compressed protobuf marshaled MetricPayload objects.
func (series *IterableSeries) MarshalSplitCompress(bufferContext *marshaler.BufferContext) ([]*[]byte, error) {
	return marshalSplitCompress(series, bufferContext)
}
