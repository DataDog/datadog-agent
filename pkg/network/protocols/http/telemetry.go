// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf

package http

import (
	"fmt"

	libtelemetry "github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type Telemetry struct {
	protocol string

	// metricGroup is used here mostly for building the log message below
	metricGroup *libtelemetry.MetricGroup

	hits1XX, hits2XX, hits3XX, hits4XX, hits5XX *libtelemetry.Counter

	totalHits    *libtelemetry.Counter
	dropped      *libtelemetry.Counter // this happens when statKeeper reaches capacity
	rejected     *libtelemetry.Counter // this happens when an user-defined reject-filter matches a request
	malformed    *libtelemetry.Counter // this happens when the request doesn't have the expected format
	aggregations *libtelemetry.Counter
}

func NewTelemetry(protocol string) *Telemetry {
	metricGroup := libtelemetry.NewMetricGroup(fmt.Sprintf("usm.%s", protocol))

	return &Telemetry{
		protocol:    protocol,
		metricGroup: metricGroup,

		hits1XX:      metricGroup.NewCounter("hits", "status:1xx", libtelemetry.OptPrometheus),
		hits2XX:      metricGroup.NewCounter("hits", "status:2xx", libtelemetry.OptPrometheus),
		hits3XX:      metricGroup.NewCounter("hits", "status:3xx", libtelemetry.OptPrometheus),
		hits4XX:      metricGroup.NewCounter("hits", "status:4xx", libtelemetry.OptPrometheus),
		hits5XX:      metricGroup.NewCounter("hits", "status:5xx", libtelemetry.OptPrometheus),
		aggregations: metricGroup.NewCounter("aggregations", libtelemetry.OptPrometheus),

		// these metrics are also exported as statsd metrics
		totalHits: metricGroup.NewCounter("total_hits", libtelemetry.OptStatsd, libtelemetry.OptPayloadTelemetry),
		dropped:   metricGroup.NewCounter("dropped", libtelemetry.OptStatsd),
		rejected:  metricGroup.NewCounter("rejected", libtelemetry.OptStatsd),
		malformed: metricGroup.NewCounter("malformed", libtelemetry.OptStatsd),
	}
}

func (t *Telemetry) Count(tx Transaction) {
	statusClass := (tx.StatusCode() / 100) * 100
	switch statusClass {
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

func (t *Telemetry) Log() {
	log.Debugf("%s stats summary: %s", t.protocol, t.metricGroup.Summary())
}
