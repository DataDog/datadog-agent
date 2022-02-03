// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// The timeSamplerWorker is running the process loop for a TimeSampler:
//  - receiving samples for the TimeSampler to process
//  - receiving flush triggers to flush the series from the TimeSampler
//    into a serializer
type timeSamplerWorker struct {
	// parent sampler the timeSamplerWorker is responsible of
	sampler *TimeSampler

	// pointer to the shared MetricSamplePool stored in the Demultiplexer.
	metricSamplePool *metrics.MetricSamplePool

	// serializer to send the series on
	serializer serializer.MetricSerializer

	// flushInterval is the automatic flush interval
	flushInterval time.Duration

	// parallel serialization configuration
	parallelSerialization flushAndSerializeInParallel

	// samplesChan is used to communicate between from the processLoop receiving the
	// samples and the TimeSampler.
	samplesChan chan []metrics.MetricSample
	// use this chan to trigger a flush of the time sampler
	flushChan chan flushTrigger
	// use this chan to stop the timeSamplerWorker
	stopChan chan struct{}
}

// flushTrigger must be used to trigger a flush of the TimeSampler.
// If `BlockChan` is not nil, a message is sent when the flush is complete.
type flushTrigger struct {
	time      time.Time
	blockChan chan struct{}
}

func newTimeSamplerWorker(sampler *TimeSampler, flushInterval time.Duration, bufferSize int,
	metricSamplePool *metrics.MetricSamplePool, serializer serializer.MetricSerializer,
	parallelSerialization flushAndSerializeInParallel) *timeSamplerWorker {
	return &timeSamplerWorker{
		sampler: sampler,

		serializer:            serializer,
		metricSamplePool:      metricSamplePool,
		parallelSerialization: parallelSerialization,

		flushInterval: flushInterval,

		samplesChan: make(chan []metrics.MetricSample, bufferSize),
		stopChan:    make(chan struct{}),
		flushChan:   make(chan flushTrigger),
	}
}

// flush flushes the TimeSampler data into the serializer.
func (w *timeSamplerWorker) flush(start time.Time, waitForSerializer bool) {
	trigger := flushTrigger{time: start}
	if waitForSerializer {
		trigger.blockChan = make(chan struct{})
	}

	w.flushChan <- trigger

	if waitForSerializer {
		<-trigger.blockChan
	}
}

// We process all receivend samples in the `select`, but we also process a flush action,
// meaning that the time sampler will not process any sample while it is flushing.
// Note that it was the same design in the BufferedAggregator (but at the aggregator level,
// not sampler level).
// If we want to move to a design where we can flush while we are processing samples,
// we could consider implementing double-buffering or locking for every sample reception.
func (w *timeSamplerWorker) run() {
	var tickerChan <-chan time.Time
	if w.flushInterval > 0 {
		tickerChan = time.NewTicker(w.flushInterval).C
	} else {
		log.Debugf("Time Sampler #%d - flushInterval set to 0: it won't automatically flush", w.sampler.id)
	}

	for {
		select {
		case <-w.stopChan:
			return
		case ms := <-w.samplesChan:
			aggregatorDogstatsdMetricSample.Add(int64(len(ms)))
			tlmProcessed.Add(float64(len(ms)), "dogstatsd_metrics")
			t := timeNowNano()
			for i := 0; i < len(ms); i++ {
				w.sampler.sample(&ms[i], t)
			}
			w.metricSamplePool.PutBatch(ms)
		case trigger := <-w.flushChan:
			w.triggerFlush(trigger.time, trigger.blockChan != nil)
			if trigger.blockChan != nil {
				trigger.blockChan <- struct{}{}
			}
		case t := <-tickerChan:
			w.triggerFlush(t, false)
		}
	}
}

func (w *timeSamplerWorker) stop() {
	w.stopChan <- struct{}{}
}

func (w *timeSamplerWorker) triggerFlush(t time.Time, waitForSerializer bool) {
	if w.parallelSerialization.enabled {
		w.triggerFlushWithParallelSerialize(t, waitForSerializer)
	} else {
		log.Debugf("Time Sampler #%d - Flushing series to the forwarder", w.sampler.id)

		var series metrics.Series
		sketches := w.sampler.flush(float64(t.Unix()), &series)
		if w.serializer != nil {
			err := w.serializer.SendSeries(series)
			updateSerieTelemetry(t, uint64(len(series)), "timeSamplerWorker", err)
			tagsetTlm.updateHugeSeriesTelemetry(&series)

			err = w.serializer.SendSketch(sketches)
			updateSketchTelemetry(t, uint64(len(sketches)), "timeSamplerWorker", err)
			tagsetTlm.updateHugeSketchesTelemetry(&sketches)
		}
	}
}

// NOTE(remy): this has been stolen from the Aggregator implementation, we will have
// to factor it at some point.
func (w *timeSamplerWorker) sendIterableSeries(
	start time.Time,
	series *metrics.IterableSeries,
	done chan<- struct{}) {
	go func() {
		log.Debugf("Time Sampler #%d - Flushing series to the forwarder in parallel", w.sampler.id)

		err := w.serializer.SendIterableSeries(series)
		// if err == nil, SenderStopped was called and it is safe to read the number of series.
		count := series.SeriesCount()
		addFlushCount("Series", int64(count))
		updateSerieTelemetry(start, count, "timeSamplerWorker", err)
		close(done)
	}()
}

// NOTE(remy): this has been stolen from the Aggregator implementation, we will have
// to factor it at some point.
func (w *timeSamplerWorker) triggerFlushWithParallelSerialize(start time.Time, waitForSerializer bool) {
	logPayloads := config.Datadog.GetBool("log_payloads")
	series := metrics.NewIterableSeries(func(se *metrics.Serie) {
		if logPayloads {
			log.Debugf("Time Sampler #%d - Flushing the following metrics: %s", w.sampler.id, se)
		}
		tagsetTlm.updateHugeSerieTelemetry(se)
	}, w.parallelSerialization.channelSize, w.parallelSerialization.bufferSize)
	done := make(chan struct{})

	// start the serialization routine
	w.sendIterableSeries(start, series, done)

	sketches := w.sampler.flush(float64(start.Unix()), series)
	series.SenderStopped()

	if waitForSerializer {
		<-done
	}

	tagsetTlm.updateHugeSketchesTelemetry(&sketches)
	if err := w.serializer.SendSketch(sketches); err != nil {
		log.Errorf("flushLoop: %+v", err)
	}
}
