// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bench

import (
	"context"
	"fmt"
	"path/filepath"

	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
)

// parquetObservation is one item in the globally ordered replay stream.
// Exactly one of metric or log is set.
type parquetObservation struct {
	metric *recorderdef.MetricData
	log    *recorderdef.LogData
}

func (o parquetObservation) timestampSec() int64 {
	if o.metric != nil {
		return o.metric.Timestamp
	}
	return o.log.TimestampMs / 1000
}

type parquetStreamResult struct {
	count int
	err   error
}

type parquetStream[T any] struct {
	values <-chan T
	result <-chan parquetStreamResult
}

func (s parquetStream[T]) next(head **T, done *bool, result *parquetStreamResult) error {
	if *done || *head != nil {
		return nil
	}
	value, ok := <-s.values
	if ok {
		*head = &value
		return nil
	}
	*result = <-s.result
	*done = true
	return result.err
}

func (s parquetStream[T]) drain(result *parquetStreamResult) {
	for range s.values {
	}
	*result = <-s.result
}

// startParquetStream adapts a callback-based reader into a pull-like stream.
// The unbuffered channel keeps at most one unread value in each producer.
func startParquetStream[T any](ctx context.Context, read func(func(T) error) (int, error)) parquetStream[T] {
	values := make(chan T)
	result := make(chan parquetStreamResult, 1)
	go func() {
		count, err := read(func(value T) error {
			select {
			case values <- value:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		result <- parquetStreamResult{count: count, err: err}
		close(values)
	}()
	return parquetStream[T]{values: values, result: result}
}

// streamOrderedObservations performs a bounded two-way merge of the metric and
// log parquet streams. Each producer is back-pressured by an unbuffered channel,
// so the merge does not retain either input dataset. Rows within the same second
// may be emitted in either source order; the Observer scheduler does not analyze
// that second until a later timestamp arrives or the stream is flushed.
func streamOrderedObservations(
	dir string,
	format ParquetFormat,
	logsOnly bool,
	fn func(parquetObservation) error,
) (metricCount, logCount int, err error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var v2Contexts map[uint64]contextEntryV2
	if format == FormatV2 {
		var contextsErr error
		v2Contexts, contextsErr = readContextsFileV2(filepath.Join(dir, "contexts.parquet"))
		if contextsErr != nil {
			return 0, 0, fmt.Errorf("reading contexts: %w", contextsErr)
		}
	}

	var metricStream parquetStream[recorderdef.MetricData]
	if !logsOnly {
		metricStream = startParquetStream(ctx, func(fn func(recorderdef.MetricData) error) (int, error) {
			return streamOrderedMetricsWithContexts(dir, format, v2Contexts, fn)
		})
	}
	logStream := startParquetStream(ctx, func(fn func(recorderdef.LogData) error) (int, error) {
		return streamOrderedLogsWithContexts(dir, format, v2Contexts, fn)
	})

	var (
		metricHead    *recorderdef.MetricData
		logHead       *recorderdef.LogData
		metricDone    = logsOnly
		logDone       bool
		metricResult  parquetStreamResult
		logResult     parquetStreamResult
		streamFailure error
	)

	for !metricDone || !logDone || metricHead != nil || logHead != nil {
		if err := metricStream.next(&metricHead, &metricDone, &metricResult); err != nil {
			streamFailure = fmt.Errorf("streaming parquet metrics: %w", err)
		}
		if err := logStream.next(&logHead, &logDone, &logResult); err != nil && streamFailure == nil {
			streamFailure = fmt.Errorf("streaming parquet logs: %w", err)
		}
		if streamFailure != nil {
			cancel()
			break
		}

		var observation parquetObservation
		switch {
		case metricHead == nil && logHead == nil:
			continue
		case logHead == nil || (metricHead != nil && metricHead.Timestamp <= logHead.TimestampMs/1000):
			observation.metric = metricHead
			metricHead = nil
		case metricHead == nil || logHead.TimestampMs/1000 < metricHead.Timestamp:
			observation.log = logHead
			logHead = nil
		}

		if err := fn(observation); err != nil {
			streamFailure = err
			cancel()
			break
		}
	}

	// A canceled producer may still be blocked sending its next value. Drain
	// both channels before returning so no goroutine is left behind.
	if !metricDone {
		metricStream.drain(&metricResult)
	}
	if !logDone {
		logStream.drain(&logResult)
	}

	if streamFailure != nil {
		return metricResult.count, logResult.count, streamFailure
	}
	return metricResult.count, logResult.count, nil
}
