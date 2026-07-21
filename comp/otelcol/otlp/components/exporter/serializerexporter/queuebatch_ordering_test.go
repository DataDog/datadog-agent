// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build test

package serializerexporter

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

// olderTS / newerTS are the timestamps of two single-point payloads for the
// same series, offered older-first. A correct pipeline must observe them in
// that order; if the newer one is observed first, the ttlcache drops the older
// point as stale (ttlcache.go: `cnt.ts >= ts`).
const (
	olderTS = uint64(100)
	newerTS = uint64(200)
)

func metricWithTS(ts uint64) pmetric.Metrics {
	md := pmetric.NewMetrics()
	m := md.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	m.SetName("test.counter")
	dp := m.SetEmptyGauge().DataPoints().AppendEmpty()
	dp.SetTimestamp(pcommon.Timestamp(ts))
	dp.SetIntValue(int64(ts))
	return md
}

func tsOf(md pmetric.Metrics) uint64 {
	return uint64(md.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).Gauge().DataPoints().At(0).Timestamp())
}

// TestQueueBatchOrdering reproduces the DDOT cumulative-metric reordering bug at
// the exporterhelper layer.
//
// The OTel queue+batcher runs completed batches on a worker pool sized to
// NumConsumers. With batching enabled and no partitioner it clamps the *queue*
// to a single consumer, but the *batcher* flush pool keeps the original
// NumConsumers — so batches still flush concurrently and reach the pusher out of
// order. The pusher here stands in for Exporter.ConsumeMetrics -> MapMetrics ->
// ttlcache; observing the newer batch first is exactly what makes the cache drop
// the older cumulative point.
//
//   - num_consumers=1  (Agent OTLP ingest default via newDefaultConfigForAgent):
//     flushes are serialized -> order preserved.
//   - num_consumers=10 (DDOT datadogexporter default via NewDefaultQueueConfig):
//     flushes overlap -> older batch observed after the newer one.
func TestQueueBatchOrdering(t *testing.T) {
	tests := []struct {
		name         string
		numConsumers int
		wantOrder    []uint64
	}{
		{
			name:         "num_consumers=1 preserves order (Agent OTLP ingest default)",
			numConsumers: 1,
			wantOrder:    []uint64{olderTS, newerTS},
		},
		{
			name:         "num_consumers=10 reorders (DDOT datadog exporter default)",
			numConsumers: 10,
			wantOrder:    []uint64{newerTS, olderTS},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				mu            sync.Mutex
				observed      []uint64
				newerObserved = make(chan struct{})
				wg            sync.WaitGroup
			)
			wg.Add(2)

			// The older-ts flush blocks until the newer-ts flush has been
			// observed. When the two can run concurrently (num_consumers>1) this
			// forces the out-of-order observation; the bounded wait means the
			// serial case (num_consumers=1, newer can never overtake) simply
			// proceeds in order instead of deadlocking.
			pusher := func(_ context.Context, md pmetric.Metrics) error {
				ts := tsOf(md)
				if ts == olderTS {
					select {
					case <-newerObserved:
					case <-time.After(2 * time.Second):
					}
				}
				mu.Lock()
				observed = append(observed, ts)
				mu.Unlock()
				if ts == newerTS {
					close(newerObserved)
				}
				wg.Done()
				return nil
			}

			// DDOT-equivalent queue config: NumConsumers per case, batching
			// enabled, no partitioner. MinSize=1 only makes each request its own
			// flush deterministically (the DDOT default is 8192); it does not
			// affect the concurrency that causes the reorder.
			qCfg := exporterhelper.NewDefaultQueueConfig()
			qCfg.NumConsumers = tt.numConsumers
			qCfg.Batch = configoptional.Some(exporterhelper.BatchConfig{
				FlushTimeout: 200 * time.Millisecond,
				Sizer:        exporterhelper.RequestSizerTypeItems,
				MinSize:      1,
			})

			set := exportertest.NewNopSettings(component.MustNewType(TypeStr))
			exp, err := exporterhelper.NewMetrics(
				context.Background(), set, &ExporterConfig{}, pusher,
				exporterhelper.WithQueue(configoptional.Some(qCfg)),
			)
			require.NoError(t, err)
			require.NoError(t, exp.Start(context.Background(), componenttest.NewNopHost()))

			// FIFO queue: the older flush is dispatched first, so only worker-pool
			// concurrency can reorder the two.
			require.NoError(t, exp.ConsumeMetrics(context.Background(), metricWithTS(olderTS)))
			require.NoError(t, exp.ConsumeMetrics(context.Background(), metricWithTS(newerTS)))

			done := make(chan struct{})
			go func() { wg.Wait(); close(done) }()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Fatal("timed out waiting for both batches to be processed")
			}
			require.NoError(t, exp.Shutdown(context.Background()))

			assert.Equal(t, tt.wantOrder, observed,
				"observed batch processing order for num_consumers=%d", tt.numConsumers)
		})
	}
}
