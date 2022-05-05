// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.uber.org/atomic"
)

// IterableSeries represents an iterable collection of Serie.  Serie can be
// appended to IterableSeries while IterableSeries is serialized.
// IterableSeries is designed be used with StartIteration.
//
// An IterableSeries interfaces two goroutines, referred to below as "sender"
// and "receiver".  The sender calls Append any number of times followed by
// senderStopped.  The receiver calls MoveNext and Current to iterate through
// the items, and iterationStopped when it is finished.
type IterableSeries struct {
	count              *atomic.Uint64
	ch                 *util.BufferedChan
	bufferedChanClosed bool
	cancel             context.CancelFunc
	callback           func(*Serie)
	current            *Serie
}

// NewIterableSeries creates a new instance of *IterableSeries
//
// `callback` is called in the context of the sender's goroutine each time `Append` is called.
func NewIterableSeries(callback func(*Serie), chanSize int, bufferSize int) *IterableSeries {
	ctx, cancel := context.WithCancel(context.Background())
	return &IterableSeries{
		count:    atomic.NewUint64(0),
		ch:       util.NewBufferedChan(ctx, chanSize, bufferSize),
		cancel:   cancel,
		callback: callback,
		current:  nil,
	}
}

// Append appends a serie
//
// This method must only be called by the sender.
func (series *IterableSeries) Append(serie *Serie) {
	series.callback(serie)
	series.count.Inc()
	if !series.ch.Put(serie) && !series.bufferedChanClosed {
		series.bufferedChanClosed = true
		log.Errorf("Cannot append a serie in a closed buffered channel")
	}
}

// SeriesCount returns the number of series appended with `IterableSeries.Append`.
//
// SeriesCount can be called by any goroutine.
func (series *IterableSeries) SeriesCount() uint64 {
	return series.count.Load()
}

// senderStopped must be called when sender stop calling Append.
//
// This method must only be called by the sender.
// It is automatically called by StartIteration.
func (series *IterableSeries) senderStopped() {
	series.ch.Close()
}

// iterationStopped must be called when the receiver stops calling `MoveNext`.
// This function prevents the case when the receiver stops iterating before the
// end of the iteration because of an error and so blocks the sender forever
// as no goroutine read the channel.
//
// This method must only be called by the receiver.
// It is automatically called by StartIteration.
func (series *IterableSeries) iterationStopped() {
	series.cancel()
}

// MoveNext advances to the next element.
// Returns false for the end of the iteration.
//
// This method must only be called by the receiver.
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
//
// This method must only be called by the receiver.
func (series *IterableSeries) Current() *Serie {
	return series.current
}

// SerieSource is a source of series used by the serializer.
type SerieSource interface {
	MoveNext() bool
	Current() *Serie
	SeriesCount() uint64
}

// StartIteration starts the iteration over an iterableSeries.
// `sink` callback is responsible for adding the data. It runs in the current goroutine.
// `source` callback is responsible for consuming the data. It runs in its OWN goroutine.
// This function returns when both `sink`` and `source` functions are finished.
func StartIteration(iterableSeries *IterableSeries, sink func(SerieSink), source func(SerieSource)) {
	done := make(chan struct{})
	go func() {
		source(iterableSeries)
		iterableSeries.iterationStopped()
		close(done)
	}()
	sink(iterableSeries)
	iterableSeries.senderStopped()
	<-done
}
