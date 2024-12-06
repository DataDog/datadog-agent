// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package kafka

import (
	"github.com/cihub/seelog"

	libtelemetry "github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Telemetry is a struct to hold the telemetry for the kafka protocol
type Telemetry struct {
	metricGroup *libtelemetry.MetricGroup

	produceHits, fetchHits *apiVersionCounter
	dropped                *libtelemetry.Counter // this happens when KafkaStatKeeper reaches capacity

	invalidLatency *libtelemetry.Counter
}

// NewTelemetry creates a new Telemetry
func NewTelemetry() *Telemetry {
	metricGroup := libtelemetry.NewMetricGroup("usm.kafka")

	return &Telemetry{
		metricGroup:    metricGroup,
		produceHits:    newAPIVersionCounter(metricGroup, "total_hits", "operation:produce", libtelemetry.OptStatsd),
		fetchHits:      newAPIVersionCounter(metricGroup, "total_hits", "operation:fetch", libtelemetry.OptStatsd),
		dropped:        metricGroup.NewCounter("dropped", libtelemetry.OptStatsd),
		invalidLatency: metricGroup.NewCounter("malformed", "type:invalid-latency", libtelemetry.OptStatsd),
	}
}

// Count increments the total hits counter
func (t *Telemetry) Count(tx *KafkaTransaction) {
	switch tx.Request_api_key {
	case 0:
		t.produceHits.Add(tx)
	case 1:
		t.fetchHits.Add(tx)
	default:
		log.Errorf("unsupported request api key: %d", tx.Request_api_key)
	}
}

// Log logs the kafka stats summary
func (t *Telemetry) Log() {
	if log.ShouldLog(seelog.DebugLvl) {
		log.Debugf("kafka stats summary: %s", t.metricGroup.Summary())
	}
}
