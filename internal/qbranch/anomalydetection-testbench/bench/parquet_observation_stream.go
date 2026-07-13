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
	metric   *recorderdef.MetricData
	log      *recorderdef.LogData
	consumed chan struct{}
}

func (o parquetObservation) timestampSec() int64 {
	if o.metric != nil {
		return o.metric.Timestamp
	}
	return o.log.TimestampMs / 1000
}

type parquetMetricStreamResult struct {
	count int
	err   error
}

type parquetLogStreamResult struct {
	count int
	err   error
}

type parquetMetricStreamItem struct {
	value    recorderdef.MetricData
	consumed chan struct{}
}

type parquetLogStreamItem struct {
	value    recorderdef.LogData
	consumed chan struct{}
}

// streamOrderedObservations performs a bounded two-way merge of the metric and
// log parquet streams. Each producer is back-pressured to one decoded row, so
// the merge does not retain either input dataset. Rows within the same second
// may be emitted in either source order; the Observer scheduler does not
// analyze that second until a later timestamp arrives or the stream is flushed.
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

	metricCh := make(chan parquetMetricStreamItem)
	metricResultCh := make(chan parquetMetricStreamResult, 1)
	if logsOnly {
		close(metricCh)
		metricResultCh <- parquetMetricStreamResult{}
	} else {
		go func() {
			count, streamErr := streamOrderedMetricsWithContexts(dir, format, v2Contexts, func(metric recorderdef.MetricData) error {
				item := parquetMetricStreamItem{value: metric, consumed: make(chan struct{})}
				select {
				case metricCh <- item:
				case <-ctx.Done():
					return ctx.Err()
				}
				// Arrow-backed strings remain valid only while the producer's
				// callback is active. Wait until Observer ingestion has copied them.
				select {
				case <-item.consumed:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			})
			metricResultCh <- parquetMetricStreamResult{count: count, err: streamErr}
			close(metricCh)
		}()
	}

	logCh := make(chan parquetLogStreamItem)
	logResultCh := make(chan parquetLogStreamResult, 1)
	go func() {
		count, streamErr := streamOrderedLogsWithContexts(dir, format, v2Contexts, func(entry recorderdef.LogData) error {
			item := parquetLogStreamItem{value: entry, consumed: make(chan struct{})}
			select {
			case logCh <- item:
			case <-ctx.Done():
				return ctx.Err()
			}
			select {
			case <-item.consumed:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		logResultCh <- parquetLogStreamResult{count: count, err: streamErr}
		close(logCh)
	}()

	var (
		metricHead    *parquetMetricStreamItem
		logHead       *parquetLogStreamItem
		metricDone    bool
		logDone       bool
		metricResult  parquetMetricStreamResult
		logResult     parquetLogStreamResult
		streamFailure error
	)

	nextMetric := func() {
		if metricDone || metricHead != nil {
			return
		}
		metric, ok := <-metricCh
		if !ok {
			metricResult = <-metricResultCh
			metricDone = true
			if metricResult.err != nil && streamFailure == nil {
				streamFailure = fmt.Errorf("streaming parquet metrics: %w", metricResult.err)
			}
			return
		}
		metricHead = &metric
	}
	nextLog := func() {
		if logDone || logHead != nil {
			return
		}
		entry, ok := <-logCh
		if !ok {
			logResult = <-logResultCh
			logDone = true
			if logResult.err != nil && streamFailure == nil {
				streamFailure = fmt.Errorf("streaming parquet logs: %w", logResult.err)
			}
			return
		}
		logHead = &entry
	}

	for !metricDone || !logDone || metricHead != nil || logHead != nil {
		nextMetric()
		nextLog()
		if streamFailure != nil {
			cancel()
			break
		}

		var observation parquetObservation
		switch {
		case metricHead == nil && logHead == nil:
			continue
		case logHead == nil || (metricHead != nil && metricHead.value.Timestamp <= logHead.value.TimestampMs/1000):
			observation.metric = &metricHead.value
			observation.consumed = metricHead.consumed
			metricHead = nil
		case metricHead == nil || logHead.value.TimestampMs/1000 < metricHead.value.Timestamp:
			observation.log = &logHead.value
			observation.consumed = logHead.consumed
			logHead = nil
		}

		if err := fn(observation); err != nil {
			close(observation.consumed)
			streamFailure = err
			cancel()
			break
		}
		close(observation.consumed)
	}

	// Cancellation makes each producer's callback return, but it may have one
	// buffered row left. Drain both channels so no goroutine is left behind.
	if !metricDone {
		for range metricCh {
		}
		metricResult = <-metricResultCh
	}
	if !logDone {
		for range logCh {
		}
		logResult = <-logResultCh
	}

	if streamFailure != nil {
		return metricResult.count, logResult.count, streamFailure
	}
	if metricResult.err != nil {
		return metricResult.count, logResult.count, fmt.Errorf("streaming parquet metrics: %w", metricResult.err)
	}
	if logResult.err != nil {
		return metricResult.count, logResult.count, fmt.Errorf("streaming parquet logs: %w", logResult.err)
	}
	return metricResult.count, logResult.count, nil
}
