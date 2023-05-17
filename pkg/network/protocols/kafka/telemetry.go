// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package kafka

import (
	"time"

	"go.uber.org/atomic"

	libtelemetry "github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type Telemetry struct {
	then *atomic.Int64

	totalHits *libtelemetry.Metric
	dropped   *libtelemetry.Metric // this happens when KafkaStatKeeper reaches capacity
}

func NewTelemetry() *Telemetry {
	metricGroup := libtelemetry.NewMetricGroup(
		"usm.kafka",
		libtelemetry.OptExpvar,
		libtelemetry.OptMonotonic,
	)

	t := &Telemetry{
		then: atomic.NewInt64(time.Now().Unix()),

		// these metrics are also exported as statsd metrics
		totalHits: metricGroup.NewMetric("total_hits", libtelemetry.OptStatsd),
		dropped:   metricGroup.NewMetric("dropped", libtelemetry.OptStatsd),
	}

	return t
}

func (t *Telemetry) Count(_ *EbpfKafkaTx) {
	t.totalHits.Add(1)
}

func (t *Telemetry) Log() {
	now := time.Now().Unix()
	then := t.then.Swap(now)

	totalRequests := t.totalHits.Delta()
	dropped := t.dropped.Delta()
	elapsed := now - then

	log.Debugf(
		"kafka stats summary: requests_processed=%d(%.2f/s) requests_dropped=%d(%.2f/s)",
		totalRequests,
		float64(totalRequests)/float64(elapsed),
		dropped,
		float64(dropped)/float64(elapsed),
	)
}
