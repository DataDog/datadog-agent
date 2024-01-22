// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package kafka

import (
	libtelemetry "github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Telemetry is a struct to hold the telemetry for the kafka protocol
type Telemetry struct {
	metricGroup *libtelemetry.MetricGroup

	totalHits *libtelemetry.Counter
	dropped   *libtelemetry.Counter // this happens when KafkaStatKeeper reaches capacity
}

// NewTelemetry creates a new Telemetry
func NewTelemetry() *Telemetry {
	metricGroup := libtelemetry.NewMetricGroup("usm.kafka", libtelemetry.OptStatsd)

	return &Telemetry{
		metricGroup: metricGroup,
		// these metrics are also exported as statsd metrics
		totalHits: metricGroup.NewCounter("total_hits"),
		dropped:   metricGroup.NewCounter("dropped"),
	}
}

// Count increments the total hits counter
func (t *Telemetry) Count(*EbpfTx) {
	t.totalHits.Add(1)
}

// Log logs the kafka stats summary
func (t *Telemetry) Log() {
	log.Debugf("kafka stats summary: %s", t.metricGroup.Summary())
}
