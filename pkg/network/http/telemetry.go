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

	"github.com/DataDog/datadog-agent/pkg/network/http/transaction"
	libtelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type telemetry struct {
	then *atomic.Int64

	hits1XX, hits2XX, hits3XX, hits4XX, hits5XX *libtelemetry.Metric

	totalHits    *libtelemetry.Metric
	misses       *libtelemetry.Metric // this happens when we can't cope with the rate of events
	dropped      *libtelemetry.Metric // this happens when httpStatKeeper reaches capacity
	rejected     *libtelemetry.Metric // this happens when an user-defined reject-filter matches a request
	malformed    *libtelemetry.Metric // this happens when the request doesn't have the expected format
	aggregations *libtelemetry.Metric
}

func newTelemetry() (*telemetry, error) {
	metricGroup := libtelemetry.NewMetricGroup(
		"usm.http",
		libtelemetry.OptExpvar,
		libtelemetry.OptMonotonic,
	)

	t := &telemetry{
		then:         atomic.NewInt64(time.Now().Unix()),
		hits1XX:      metricGroup.NewMetric("hits1xx"),
		hits2XX:      metricGroup.NewMetric("hits2xx"),
		hits3XX:      metricGroup.NewMetric("hits3xx"),
		hits4XX:      metricGroup.NewMetric("hits4xx"),
		hits5XX:      metricGroup.NewMetric("hits5xx"),
		aggregations: metricGroup.NewMetric("aggregations"),

		// these metrics are also exported as statsd metrics
		totalHits: metricGroup.NewMetric("total_hits", libtelemetry.OptStatsd),
		misses:    metricGroup.NewMetric("misses", libtelemetry.OptStatsd),
		dropped:   metricGroup.NewMetric("dropped", libtelemetry.OptStatsd),
		rejected:  metricGroup.NewMetric("rejected", libtelemetry.OptStatsd),
		malformed: metricGroup.NewMetric("malformed", libtelemetry.OptStatsd),
	}

	return t, nil
}

func (t *telemetry) aggregate(txs []transaction.HttpTX, err error) {
	for _, tx := range txs {
		switch tx.StatusClass() {
		case 100:
			t.hits1XX.Add(1)
		case 200:
			t.hits2XX.Add(1)
		case 300:
			t.hits3XX.Add(1)
		case 400:
			t.hits4XX.Add(1)
		case 500:
			t.hits5XX.Add(1)
		}
		t.totalHits.Add(1)
	}

	if err == transaction.ErrLostBatch {
		t.misses.Add(int64(len(txs)))
	}
}

func (t *telemetry) log() {
	now := time.Now().Unix()
	then := t.then.Swap(now)

	totalRequests := t.totalHits.Delta()
	misses := t.misses.Delta()
	dropped := t.dropped.Delta()
	rejected := t.rejected.Delta()
	malformed := t.malformed.Delta()
	aggregations := t.aggregations.Delta()
	elapsed := now - then

	log.Debugf(
		"http stats summary: requests_processed=%d(%.2f/s) requests_missed=%d(%.2f/s) requests_dropped=%d(%.2f/s) requests_rejected=%d(%.2f/s) requests_malformed=%d(%.2f/s) aggregations=%d",
		totalRequests,
		float64(totalRequests)/float64(elapsed),
		misses,
		float64(misses)/float64(elapsed),
		dropped,
		float64(dropped)/float64(elapsed),
		rejected,
		float64(rejected)/float64(elapsed),
		malformed,
		float64(malformed)/float64(elapsed),
		aggregations,
	)
}
