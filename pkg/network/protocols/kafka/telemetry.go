// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package kafka

import (
	"time"

	"go.uber.org/atomic"

	libtelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type telemetry struct {
	metricGroup *libtelemetry.MetricGroup
	then        *atomic.Int64

	// Topic name -> Array[produce_count][fetch_count]
	topics map[string][2]*libtelemetry.Metric

	totalHits    *libtelemetry.Metric
	misses       *libtelemetry.Metric // this happens when we can't cope with the rate of events
	dropped      *libtelemetry.Metric // this happens when httpStatKeeper reaches capacity
	rejected     *libtelemetry.Metric // this happens when an user-defined reject-filter matches a request
	malformed    *libtelemetry.Metric // this happens when the request doesn't have the expected format
	aggregations *libtelemetry.Metric
}

func newTelemetry() (*telemetry, error) {
	metricGroup := libtelemetry.NewMetricGroup(
		"usm.kafka",
		libtelemetry.OptExpvar,
		libtelemetry.OptMonotonic,
	)

	t := &telemetry{
		metricGroup:  metricGroup,
		then:         atomic.NewInt64(time.Now().Unix()),
		topics:       map[string][2]*libtelemetry.Metric{},
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

func (t *telemetry) aggregate(transactions []kafkaTX, err error) {
	for _, transaction := range transactions {
		_ = transaction
		topicName := transaction.TopicName()
		if _, ok := t.topics[topicName]; !ok {
			t.topics[topicName] = [2]*libtelemetry.Metric{t.metricGroup.NewMetric("produce_count"), t.metricGroup.NewMetric("fetch_count")}
		}

		switch transaction.APIKey() {
		case ProduceAPIKey:
			t.topics[topicName][0].Add(1)
			break
		case FetchAPIKey:
			t.topics[topicName][1].Add(1)
			break
		default:
			log.Debugf("Unknown API key: %d", transaction.APIKey())
		}
		t.totalHits.Add(1)
	}

	if err == errLostBatch {
		t.misses.Add(int64(len(transactions)))
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
		"kafka stats summary: requests_processed=%d(%.2f/s) requests_missed=%d(%.2f/s) requests_dropped=%d(%.2f/s) requests_rejected=%d(%.2f/s) requests_malformed=%d(%.2f/s) aggregations=%d",
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
