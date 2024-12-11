// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"context"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/util/buf"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.uber.org/atomic"
)

// iterableMetrics represents an iterable collection of metrics. A metric can be
// appended to iterableMetrics while iterableMetrics is serialized.
// iterableMetrics is designed be used with StartIteration.
//
// An iterableMetrics interfaces two goroutines, referred to below as "sender"
// and "receiver". The sender calls Append any number of times followed by
// senderStopped. The receiver calls MoveNext and Current to iterate through
// the items, and iterationStopped when it is finished.
type iterableMetrics struct {
	count              *atomic.Uint64
	ch                 *buf.BufferedChan
	bufferedChanClosed bool
	cancel             context.CancelFunc
	callback           func(interface{})
	current            interface{}
}

// newIterableMetric creates a new instance of *iterableMetrics
//
// `callback` is called in the context of the sender's goroutine each time `Append` is called.
func newIterableMetric(callback func(interface{}), chanSize int, bufferSize int) *iterableMetrics {
	ctx, cancel := context.WithCancel(context.Background())
	return &iterableMetrics{
		count:    atomic.NewUint64(0),
		ch:       buf.NewBufferedChan(ctx, chanSize, bufferSize),
		cancel:   cancel,
		callback: callback,
		current:  nil,
	}
}

// Append appends a metric
//
// This method must only be called by the sender.
func (it *iterableMetrics) Append(value interface{}) {
	it.callback(value)
	it.count.Inc()
	if !it.ch.Put(value) && !it.bufferedChanClosed {
		it.bufferedChanClosed = true
		log.Errorf("Cannot append a metric in a closed buffered channel")
	}
}

// Count returns the number of metrics appended with `iterableMetrics.Append`.
//
// Count can be called by any goroutine.
func (it *iterableMetrics) Count() uint64 {
	return it.count.Load()
}

// senderStopped must be called when sender stop calling Append.
//
// This method must only be called by the sender.
// It is automatically called by StartIteration.
func (it *iterableMetrics) senderStopped() {
	it.ch.Close()
}

// iterationStopped must be called when the receiver stops calling `MoveNext`.
// This function prevents the case when the receiver stops iterating before the
// end of the iteration because of an error and so blocks the sender forever
// as no goroutine read the channel.
//
// This method must only be called by the receiver.
// It is automatically called by StartIteration.
func (it *iterableMetrics) iterationStopped() {
	it.cancel()
}

// MoveNext advances to the next element.
// Returns false for the end of the iteration.
//
// This method must only be called by the receiver.
func (it *iterableMetrics) MoveNext() bool {
	v, ok := it.ch.Get()
	it.current = v
	return ok
}

// Current returns the current metric.
//
// This method must only be called by the receiver.
func (it *iterableMetrics) Current() interface{} {
	return it.current
}

// Wait until a value is available for MoveNext() or until senderStopped() is called
// Returns true if a value is available, false otherwise
func (it *iterableMetrics) WaitForValue() bool {
	return it.ch.WaitForValue()
}

// Serialize starts the serialization for series and sketches.
// `producer` callback is responsible for adding the data. It runs in the current goroutine.
// `serieConsumer` callback is responsible for consuming the series. It runs in its OWN goroutine.
// `sketchesConsumer` callback is responsible for consuming the sketches. It runs in its OWN goroutine.
// This function returns when all three `producer`, `serieConsumer` and `sketchesConsumer` functions are finished.
func Serialize(
	iterableSeries *IterableSeries,
	iterableSketches *IterableSketches,
	producer func(SerieSink, SketchesSink),
	serieConsumer func(SerieSource),
	sketchesConsumer func(SketchesSource),
) {
	var waitGroup sync.WaitGroup
	var serieSink SerieSink = noOpSerieSink{}
	var sketchesSink SketchesSink = noOpSketchesSink{}
	if iterableSeries != nil {
		serieSink = iterableSeries
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			serieConsumer(iterableSeries)
			iterableSeries.iterationStopped()
		}()
	}
	if iterableSketches != nil {
		sketchesSink = iterableSketches
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			sketchesConsumer(iterableSketches)
			iterableSketches.iterationStopped()
		}()
	}
	producer(serieSink, sketchesSink)
	if iterableSeries != nil {
		iterableSeries.senderStopped()
	}
	if iterableSketches != nil {
		iterableSketches.senderStopped()
	}
	waitGroup.Wait()
}

var _ SerieSink = noOpSerieSink{}

type noOpSerieSink struct{}

func (noOpSerieSink) Append(*Serie) {}

var _ SketchesSink = noOpSketchesSink{}

type noOpSketchesSink struct{}

func (noOpSketchesSink) Append(*SketchSeries) {}
