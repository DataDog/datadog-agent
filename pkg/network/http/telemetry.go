// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/stats"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type telemetry struct {
	then int64

	hits1XX, hits2XX, hits3XX, hits4XX, hits5XX int64 `stats:"atomic"`
	misses                                      int64 `stats:"atomic"` // this happens when we can't cope with the rate of events
	dropped                                     int64 `stats:"atomic"` // this happens when httpStatKeeper reaches capacity
	rejected                                    int64 `stats:"atomic"` // this happens when an user-defined reject-filter matches a request
	aggregations                                int64 `stats:"atomic"`

	reporter stats.Reporter
}

func newTelemetry() (*telemetry, error) {
	t := &telemetry{
		then: time.Now().Unix(),
	}

	var err error
	t.reporter, err = stats.NewReporter(t)
	if err != nil {
		return nil, fmt.Errorf("error creating stats reporter: %w", err)
	}

	return t, nil
}

func (t *telemetry) aggregate(txs []httpTX, err error) {
	for _, tx := range txs {
		switch tx.StatusClass() {
		case 100:
			atomic.AddInt64(&t.hits1XX, 1)
		case 200:
			atomic.AddInt64(&t.hits2XX, 1)
		case 300:
			atomic.AddInt64(&t.hits3XX, 1)
		case 400:
			atomic.AddInt64(&t.hits4XX, 1)
		case 500:
			atomic.AddInt64(&t.hits5XX, 1)
		}
	}

	if err == errLostBatch {
		atomic.AddInt64(&t.misses, int64(HTTPBatchSize))
	}
}

func (t *telemetry) reset() {
	atomic.SwapInt64(&t.aggregations, 0)
}

func (t *telemetry) report() map[string]interface{} {
	totalRequests := t.hits1XX + t.hits2XX + t.hits3XX + t.hits4XX + t.hits5XX
	elapsed := time.Now().Unix() - t.then

	stats := t.reporter.Report()
	misses := stats["misses"].(int64)
	dropped := stats["dropped"].(int64)
	rejected := stats["rejected"].(int64)
	aggregations := stats["aggregations"].(int64)

	log.Debugf(
		"http stats summary: requests_processed=%d(%.2f/s) requests_missed=%d(%.2f/s) requests_dropped=%d(%.2f/s) requests_rejected=%d(%.2f/s) aggregations=%d",
		totalRequests,
		float64(totalRequests)/float64(elapsed),
		misses,
		float64(misses)/float64(elapsed),
		dropped,
		float64(rejected)/float64(elapsed),
		rejected,
		float64(dropped)/float64(elapsed),
		aggregations,
	)

	return stats
}
