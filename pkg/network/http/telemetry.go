// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf
// +build windows,npm linux_bpf

package http

import (
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/util/atomicstats"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type telemetry struct {
	then    *atomic.Int64
	elapsed *atomic.Int64

	hits1XX, hits2XX, hits3XX, hits4XX, hits5XX *atomic.Int64 `stats:""`
	misses                                      *atomic.Int64 `stats:""` // this happens when we can't cope with the rate of events
	dropped                                     *atomic.Int64 `stats:""` // this happens when httpStatKeeper reaches capacity
	rejected                                    *atomic.Int64 `stats:""` // this happens when an user-defined reject-filter matches a request
	malformed                                   *atomic.Int64 `stats:""` // this happens when the request doesn't have the expected format
	aggregations                                *atomic.Int64 `stats:""`
}

func newTelemetry() (*telemetry, error) {
	t := &telemetry{
		then:         atomic.NewInt64(time.Now().Unix()),
		elapsed:      atomic.NewInt64(0),
		hits1XX:      atomic.NewInt64(0),
		hits2XX:      atomic.NewInt64(0),
		hits3XX:      atomic.NewInt64(0),
		hits4XX:      atomic.NewInt64(0),
		hits5XX:      atomic.NewInt64(0),
		misses:       atomic.NewInt64(0),
		dropped:      atomic.NewInt64(0),
		rejected:     atomic.NewInt64(0),
		malformed:    atomic.NewInt64(0),
		aggregations: atomic.NewInt64(0),
	}

	return t, nil
}

func (t *telemetry) aggregate(txs []httpTX, err error) {
	for _, tx := range txs {
		switch tx.StatusClass() {
		case 100:
			t.hits1XX.Inc()
		case 200:
			t.hits2XX.Inc()
		case 300:
			t.hits3XX.Inc()
		case 400:
			t.hits4XX.Inc()
		case 500:
			t.hits5XX.Inc()
		}
	}

	if err == errLostBatch {
		t.misses.Add(int64(HTTPBatchSize))
	}
}

func (t *telemetry) reset() telemetry {
	now := time.Now().Unix()
	then := t.then.Swap(now)

	delta, _ := newTelemetry()
	delta.hits1XX.Store(t.hits1XX.Swap(0))
	delta.hits2XX.Store(t.hits2XX.Swap(0))
	delta.hits3XX.Store(t.hits3XX.Swap(0))
	delta.hits4XX.Store(t.hits4XX.Swap(0))
	delta.hits5XX.Store(t.hits5XX.Swap(0))
	delta.misses.Store(t.misses.Swap(0))
	delta.dropped.Store(t.dropped.Swap(0))
	delta.rejected.Store(t.rejected.Swap(0))
	delta.malformed.Store(t.malformed.Swap(0))
	delta.aggregations.Store(t.aggregations.Swap(0))
	delta.elapsed.Store(now - then)

	totalRequests := delta.hits1XX.Load() + delta.hits2XX.Load() + delta.hits3XX.Load() + delta.hits4XX.Load() + delta.hits5XX.Load()
	log.Debugf(
		"http stats summary: requests_processed=%d(%.2f/s) requests_missed=%d(%.2f/s) requests_dropped=%d(%.2f/s) requests_rejected=%d(%.2f/s) requests_malformed=%d(%.2f/s) aggregations=%d",
		totalRequests,
		float64(totalRequests)/float64(delta.elapsed.Load()),
		delta.misses.Load(),
		float64(delta.misses.Load())/float64(delta.elapsed.Load()),
		delta.dropped.Load(),
		float64(delta.dropped.Load())/float64(delta.elapsed.Load()),
		delta.rejected.Load(),
		float64(delta.rejected.Load())/float64(delta.elapsed.Load()),
		delta.malformed.Load(),
		float64(delta.malformed.Load())/float64(delta.elapsed.Load()),
		delta.aggregations.Load(),
	)

	return *delta
}

func (t *telemetry) report() map[string]interface{} {
	return atomicstats.Report(t)
}
